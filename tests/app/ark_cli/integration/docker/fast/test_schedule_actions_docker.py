from __future__ import annotations

import re
import uuid
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose_client import compose_exec, container_running, container_started_at
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.lite]

SERVER_LOG = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"


def _schedule_log(compose_dir: Path, name: str) -> str:
    return (compose_dir / ".ark-manage" / "logs" / "schedules" / f"{name}.log").read_text()


def _saves(dc: DockerClient, instance: str) -> int:
    return (compose_exec(dc, instance, ["cat", SERVER_LOG]) or "").count("World Save Complete")


def test_rcon_schedule_runs_command(
    cli: Callable[..., object],
    compose_client: DockerClient,
    compose_dir: Path,
    env: dict[str, str],
    started_instance_cli: dict[str, str],
) -> None:
    """An rcon schedule sends its command to a running instance and logs the reply.

    Scenario:
        An rcon schedule `ServerChat costs $5` is created for a started lite instance and run.

    Expected:
        - The compose file stores the command as `$$5`.
        - run schedule exits zero and prints `SUCCESS chat`.
        - The server log holds `RCON command: ServerChat costs $5`: the command sent has one `$`.
        - The schedule log holds the server's RCON reply for the instance.
        - check schedule shows last_status success.
    """
    instance = started_instance_cli["instance"]
    cli(
        "create",
        "schedule",
        "rcon",
        "--instance",
        instance,
        "--name",
        "chat",
        "--cron",
        "0 * * * *",
        "--command",
        "ServerChat costs $5",
    )
    assert "ServerChat costs $$5" in Path(env["COMPOSE_FILE"]).read_text()

    result = cli("run", "schedule", "--name", "chat")

    assert "SUCCESS chat" in result.stdout
    server_log = (compose_exec(compose_client, instance, ["cat", SERVER_LOG]) or "").splitlines()
    assert any(line.endswith("RCON command: ServerChat costs $5") for line in server_log)
    assert f"{instance}: Server received" in _schedule_log(compose_dir, "chat")
    assert re.search(r"last_status:\s+success", cli("check", "schedule", "--name", "chat").stdout)


def test_save_schedule_saves_world(
    cli: Callable[..., object], compose_client: DockerClient, started_instance_cli: dict[str, str]
) -> None:
    """A save schedule saves the world of a running instance.

    Scenario:
        A save schedule is created for a started lite instance and run.

    Expected:
        - run schedule exits zero and prints `SUCCESS save1`.
        - The server log has one more `World Save Complete` than before the run.
    """
    instance = started_instance_cli["instance"]
    cli("create", "schedule", "save", "--instance", instance, "--name", "save1", "--cron", "*/30 * * * *")
    before = _saves(compose_client, instance)

    result = cli("run", "schedule", "--name", "save1")

    assert "SUCCESS save1" in result.stdout
    assert _saves(compose_client, instance) == before + 1


def test_restart_schedule_restarts_instance(
    cli: Callable[..., object], compose_client: DockerClient, started_instance_cli: dict[str, str]
) -> None:
    """A restart schedule restarts a running instance, which is ready again afterwards.

    Scenario:
        A restart schedule is created for a started lite instance and run.

    Expected:
        - run schedule exits zero and prints `SUCCESS nightly-restart`.
        - The container's start time is later than before the run.
        - check instance exits zero.
    """
    instance = started_instance_cli["instance"]
    cli("create", "schedule", "restart", "--instance", instance, "--name", "nightly-restart", "--cron", "0 5 * * *")
    started_before = container_started_at(compose_client, instance)

    result = cli("run", "schedule", "--name", "nightly-restart")

    assert "SUCCESS nightly-restart" in result.stdout
    assert container_started_at(compose_client, instance) > started_before
    cli("check", "instance", "--instance", instance)


def test_action_schedule_skips_stopped_instance(cli: Callable[..., object], compose_client: DockerClient) -> None:
    """rcon, save and restart schedules skip a stopped instance with a warning.

    Scenario:
        An instance is created but not started. An rcon, a save and a restart schedule
        target it, and each is run.

    Expected:
        - Each run exits zero and warns that the instance is stopped and the command unsuccessful.
        - Each schedule records last_status skipped and check schedule exits zero.
        - The instance is still not running afterwards (the restart does not start it).
    """
    instance = f"pytest-ark-{uuid.uuid4().hex[:8]}"
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
    cli(
        "create",
        "schedule",
        "rcon",
        "--instance",
        instance,
        "--name",
        "stopped-rcon",
        "--cron",
        "0 * * * *",
        "--command",
        "ServerChat hello",
    )
    cli("create", "schedule", "save", "--instance", instance, "--name", "stopped-save", "--cron", "0 * * * *")
    cli("create", "schedule", "restart", "--instance", instance, "--name", "stopped-restart", "--cron", "0 * * * *")

    for kind in ("rcon", "save", "restart"):
        name = f"stopped-{kind}"
        result = cli("run", "schedule", "--name", name)
        assert f"Warning: {instance} is stopped; {kind} command unsuccessful" in result.stdout + result.stderr
        assert re.search(r"last_status:\s+skipped", cli("check", "schedule", "--name", name).stdout)

    assert not container_running(compose_client, instance)
