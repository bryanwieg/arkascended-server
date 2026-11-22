from __future__ import annotations

import json
import os
import re
import shutil
import threading
import uuid
from collections.abc import Callable, Iterator
from pathlib import Path

import pytest
from helpers.compose import pick_free_port
from helpers.web_client import scheduler_health
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.usefixtures("remove_new_scheduler_images")]

SCHEDULER = "ark-scheduler"
SCHEDULE = "pytest-sched-every-minute"


@pytest.fixture
def instance(env: dict[str, str]) -> Iterator[str]:
    """The instance name; its backup dir (outside the compose dir) is removed afterwards."""
    name = f"pytest-ark-{uuid.uuid4().hex[:8]}"
    yield name
    shutil.rmtree(Path(env["ARK_BACKUP_DIR"]) / name, ignore_errors=True)


def _check_scheduler(cli: Callable[..., object]) -> tuple[int, dict | None]:
    """check scheduler --json: (exit code, report), report None when it printed none."""
    result = cli("check", "scheduler", "--json", check=False)
    try:
        return result.returncode, json.loads(result.stdout)
    except ValueError:
        return result.returncode, None


def _wait_until[T](
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    description: str,
    timeout: int,
    fetch: Callable[[], T],
    accept: Callable[[T], bool],
) -> T:
    """Poll fetch until accept holds; return the accepted value, or fail showing the last one seen."""
    seen: list[T] = []

    def _accepted() -> bool:
        seen.append(fetch())
        return accept(seen[-1])

    try:
        wait_for(description, timeout, _accepted)
    except AssertionError as exc:
        raise AssertionError(f"{exc} Last seen: {seen[-1] if seen else None}") from exc
    return seen[-1]


def _archives(backup_dir: Path) -> set[Path]:
    return set(backup_dir.glob("*.tar.gz")) | set(backup_dir.glob("*.tar"))


def _instance_checks(report: dict | None, instance: str) -> dict:
    return ((report or {}).get("instances") or {}).get(instance, {}).get("checks", {})


def _instance_state(report: dict | None, instance: str) -> str | None:
    return ((report or {}).get("instances") or {}).get(instance, {}).get("state")


def test_scheduler_backup_and_health_end_to_end(
    instance: str,
    env: dict[str, str],
    slow_test_protection,
    compose_dir: Path,
    cli: Callable[..., object],
    compose_client: DockerClient,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """The scheduler container runs a scheduled backup and reports the instance's health.

    Scenario:
        A real instance is started; an every-minute backup schedule is created and the
        scheduler enabled on a free health port. Once the scheduled archive exists the
        schedule is removed. The instance is then stopped, started, and restarted while
        /health is polled every 5 s.

    Expected:
        - A new archive lands in $ARK_BACKUP_DIR/<instance>/ and check schedule reports success;
          the archive and the schedule state files are owned by the test user.
        - check scheduler exits 0 with rcon and udp ok; GET /health is 200 with rcon ok; the
          scheduler container's Docker health becomes healthy.
        - After stop instance, check scheduler still exits 0 with state stopped and /health
          shows every check skipped.
        - During restart instance the polled states include busy and no check ever fails;
          afterwards restarts is empty and the instance is running with every check ok.
    """
    port = pick_free_port()
    backup_dir = Path(env["ARK_BACKUP_DIR"]) / instance
    cli(
        "create",
        "instance",
        "--name",
        instance,
        "--server-password",
        "ServerPass123",
        "--admin-password",
        "AdminPass123",
    )
    cli("start", "instance", "--instance", instance)  # waits for ready
    archives_before = _archives(backup_dir)

    # Scheduled backup by the scheduler container.
    cli("create", "schedule", "backup", "--instance", instance, "--name", SCHEDULE, "--cron", "* * * * *")
    cli("enable", "scheduler", "--health-port", str(port))
    _wait_until(
        wait_for,
        "a scheduled archive and a successful run",
        240,
        lambda: (_archives(backup_dir) - archives_before, cli("check", "schedule", "--name", SCHEDULE).stdout),
        lambda seen: bool(seen[0]) and re.search(r"last_status:\s+success", seen[1]) is not None,
    )
    state_files = list((compose_dir / ".ark-manage" / "state" / "schedules").glob("*.json"))
    assert state_files, "expected schedule state files"
    owners = {p: p.stat().st_uid for p in [*(_archives(backup_dir) - archives_before), *state_files]}
    assert set(owners.values()) == {os.getuid()}, owners
    # An every-minute backup would hold the instance lock at every round: instance busy, checks skipped.
    cli("remove", "schedule", "--name", SCHEDULE)

    # Healthy report: check scheduler, /health, Docker health.
    _wait_until(
        wait_for,
        "check scheduler healthy with rcon and udp ok",
        300,
        lambda: _check_scheduler(cli),
        lambda seen: (
            seen[0] == 0 and {"rcon": "ok", "udp": "ok"}.items() <= _instance_checks(seen[1], instance).items()
        ),
    )
    status, body = scheduler_health(env, port)
    assert status == 200, (status, body)
    assert body["instances"][instance]["checks"]["rcon"] == "ok", body
    _wait_until(
        wait_for,
        "the scheduler container's Docker health to become healthy",
        240,
        lambda: [c.state.health.status for c in compose_client.compose.ps(services=[SCHEDULER])],
        lambda seen: seen == ["healthy"],
    )

    # A stopped instance stays healthy, its checks skipped.
    cli("stop", "instance", "--instance", instance)
    _wait_until(
        wait_for,
        "check scheduler healthy with the instance stopped",
        180,
        lambda: _check_scheduler(cli),
        lambda seen: seen[0] == 0 and _instance_state(seen[1], instance) == "stopped",
    )
    _, body = scheduler_health(env, port)
    assert set(_instance_checks(body, instance).values()) == {"skipped"}, body

    # Lock edge case (design 13a): rounds during restart instance see busy, never a failing check.
    cli("start", "instance", "--instance", instance)
    seen_during_restart: list[dict] = []
    stop_polling = threading.Event()

    def _poll_health() -> None:
        while not stop_polling.wait(5):
            _, polled = scheduler_health(env, port)
            if polled and instance in polled["instances"]:
                seen_during_restart.append(polled["instances"][instance])

    poller = threading.Thread(target=_poll_health, daemon=True)
    poller.start()
    try:
        cli("restart", "instance", "--instance", instance)
    finally:
        stop_polling.set()
        poller.join(timeout=10)
    assert "busy" in {s["state"] for s in seen_during_restart}, seen_during_restart
    assert all("fail" not in s["checks"].values() for s in seen_during_restart), seen_during_restart
    _, body = _wait_until(
        wait_for,
        "the restarted instance running with every check ok",
        180,
        lambda: scheduler_health(env, port),
        lambda seen: (
            _instance_state(seen[1], instance) == "running"
            and set(_instance_checks(seen[1], instance).values()) == {"ok"}
        ),
    )
    assert body["restarts"] == [], body

    cli("disable", "scheduler")
