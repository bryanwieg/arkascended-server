from __future__ import annotations

import re
import urllib.error
import urllib.request
from collections.abc import Callable, Iterator
from pathlib import Path
from urllib.parse import urlparse

import pytest
import yaml
from helpers.compose import pick_free_port
from helpers.compose_client import get_docker_client
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]

INSTANCE = "pytest-ark-sched-svc"
SCHEDULER = "ark-scheduler"


@pytest.fixture(scope="module", autouse=True)
def _remove_scheduler_images() -> Iterator[None]:
    yield
    dc = get_docker_client()
    for image in dc.image.list(SCHEDULER):
        dc.image.remove(image.id, force=True)


@pytest.fixture
def env(test_env: dict[str, str], scheduler_socket_env: dict[str, str]) -> dict[str, str]:
    return {**test_env, **scheduler_socket_env}


@pytest.fixture
def compose_dir(env: dict[str, str]) -> Path:
    """ark-cli runs from the compose dir: enable scheduler refuses any other cwd."""
    return Path(env["COMPOSE_FILE"]).parent


@pytest.fixture
def cli(
    env: dict[str, str], compose_dir: Path, cli_path: Path, run_cmd: Callable[..., object]
) -> Callable[..., object]:
    def _cli(*args: str, check: bool = True) -> object:
        return run_cmd([str(cli_path), *args], env=env, cwd=compose_dir, check=check, capture_output=True)

    return _cli


def _create_instance(cli: Callable[..., object]) -> None:
    cli(
        "create",
        "instance",
        "--name",
        INSTANCE,
        "--server-password",
        "ServerPass123",
        "--admin-password",
        "AdminPass123",
    )


def _service_block(text: str, name: str) -> str:
    """The service's lines in the compose text, from its key to the next top-level or service key."""
    match = re.search(rf"^  {re.escape(name)}:\n(?:(?:   .*)?\n)*", text, re.MULTILINE)
    assert match, f"service {name} not found in compose file"
    return match.group(0).rstrip()


def _scheduler_service(env: dict[str, str]) -> dict | None:
    return yaml.safe_load(Path(env["COMPOSE_FILE"]).read_text())["services"].get(SCHEDULER)


def _daemon_host(env: dict[str, str]) -> str:
    """Host where the engine publishes ports: the tcp:// DOCKER_HOST's host, else this machine."""
    docker_host = env.get("DOCKER_HOST", "")
    return urlparse(docker_host).hostname if docker_host.startswith("tcp://") else "127.0.0.1"


def _health_status(env: dict[str, str], port: int) -> int | None:
    try:
        with urllib.request.urlopen(f"http://{_daemon_host(env)}:{port}/health", timeout=5) as resp:
            return resp.status
    except urllib.error.HTTPError as exc:
        return exc.code
    except OSError:
        return None


def test_enable_scheduler_adds_and_starts_service(
    env: dict[str, str], cli: Callable[..., object], compose_client: DockerClient
) -> None:
    """enable scheduler adds one labelled ark-scheduler service, builds its image and starts it.

    Scenario:
        An instance is created (never started), then enable scheduler is run from the compose dir.

    Expected:
        - Exactly one ark-scheduler service with label ark.role: scheduler, publishing 8099:8099/tcp.
        - The instance's service block is byte-identical to before.
        - The image ark-scheduler:<12 hex> exists and the service's container is running.
        - list instances does not show ark-scheduler.
    """
    _create_instance(cli)
    compose_file = Path(env["COMPOSE_FILE"])
    instance_before = _service_block(compose_file.read_text(), INSTANCE)

    cli("enable", "scheduler")

    text = compose_file.read_text()
    assert len(re.findall(rf"^  {SCHEDULER}:", text, re.MULTILINE)) == 1, text
    assert _service_block(text, INSTANCE) == instance_before
    service = _scheduler_service(env)
    assert service["labels"]["ark.role"] == "scheduler"
    assert service["ports"] == ["8099:8099/tcp"]
    assert re.fullmatch(rf"{SCHEDULER}:[0-9a-f]{{12}}", service["image"]), service["image"]
    assert compose_client.image.exists(service["image"])
    containers = compose_client.compose.ps(services=[SCHEDULER])
    assert [c.state.running for c in containers] == [True], containers

    listed = cli("list", "instances")
    assert SCHEDULER not in listed.stdout + listed.stderr


def test_enable_scheduler_again_keeps_image_and_port(
    env: dict[str, str], cli: Callable[..., object], wait_for: Callable[[str, int, Callable[[], bool]], None]
) -> None:
    """A second enable scheduler reuses the image and keeps the custom health port.

    Scenario:
        enable scheduler --health-port <free port>, then enable scheduler without the flag.

    Expected:
        - The second run reports the image is up to date and leaves one ark-scheduler block.
        - The custom port is still published, and GET /health on it answers (200 or 503).
    """
    _create_instance(cli)
    port = pick_free_port()
    cli("enable", "scheduler", "--health-port", str(port))

    again = cli("enable", "scheduler")

    out = again.stdout + again.stderr
    assert "is up to date" in out, out
    assert "Built scheduler image" not in out, out
    text = Path(env["COMPOSE_FILE"]).read_text()
    assert len(re.findall(rf"^  {SCHEDULER}:", text, re.MULTILINE)) == 1, text
    assert _scheduler_service(env)["ports"] == [f"{port}:8099/tcp"]
    status: list[int | None] = []

    def _answers() -> bool:
        status.append(_health_status(env, port))
        return status[-1] in (200, 503)

    wait_for(f"/health on {_daemon_host(env)}:{port}", 60, _answers)


def test_disable_scheduler_removes_service(
    env: dict[str, str], cli: Callable[..., object], compose_client: DockerClient
) -> None:
    """disable scheduler removes the service block and its container; a second run is a no-op.

    Scenario:
        enable scheduler, then disable scheduler twice.

    Expected:
        - The ark-scheduler block and container are gone after the first disable.
        - The second disable prints "Scheduler is not enabled".
    """
    _create_instance(cli)
    cli("enable", "scheduler")
    # By label: once the block is gone, compose no longer knows the service.
    filters = [
        ("label", f"com.docker.compose.project.working_dir={Path(env['COMPOSE_FILE']).parent}"),
        ("label", f"com.docker.compose.service={SCHEDULER}"),
    ]
    assert len(compose_client.container.list(all=True, filters=filters)) == 1

    cli("disable", "scheduler")

    assert _scheduler_service(env) is None
    assert compose_client.container.list(all=True, filters=filters) == []
    again = cli("disable", "scheduler")
    assert "Scheduler is not enabled" in again.stdout + again.stderr


def test_create_schedule_warns_without_scheduler(cli: Callable[..., object]) -> None:
    """create schedule backup warns when the scheduler service is not enabled.

    Scenario:
        An instance and a backup schedule are created without enabling the scheduler.

    Expected:
        - The output warns that the scheduler service is not enabled.
    """
    _create_instance(cli)

    result = cli(
        "create",
        "schedule",
        "backup",
        "--instance",
        INSTANCE,
        "--name",
        "pytest-sched-svc",
        "--cron",
        "0 3 * * *",
        "--format",
        "tar.gz",
    )

    assert "scheduler service is not enabled" in result.stdout + result.stderr
