from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import (
    compose_logs,
    compose_stop,
    container_exit_code,
    container_exited,
    container_id,
    copy_from_service,
)

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_docker_compose_stop_uses_runtime_shutdown_manager(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """docker compose stop triggers the runtime shutdown manager and exits cleanly.

    Scenario:
        A running instance is stopped via docker compose stop (SIGTERM to PID 1),
        which exercises the signal-based shutdown path in container-entrypoint.py.
        The test waits for the service to reach exited status and inspects logs
        and exit code.

    Expected:
        - The service exits with code 0, confirming a clean shutdown.
        - Logs contain "[shutdown] received signal".
        - Logs contain "[shutdown] executing graceful stop script",
          confirming the runtime shutdown manager forwarded the signal correctly.
    """
    instance = started_container_instance["instance"]

    compose_stop(compose_client, instance, timeout=240)

    wait_for(
        "service to exit after docker compose stop",
        300,
        lambda: container_exited(compose_client, instance),
    )

    exit_code = container_exit_code(compose_client, instance)
    assert exit_code == 0, f"Expected clean exit code after compose stop, got {exit_code}."

    logs = compose_logs(compose_client, instance, tail=300)
    assert "[shutdown] received signal" in logs
    assert "[shutdown] executing graceful stop script" in logs


def test_docker_compose_stop_writes_stop_status_file(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
    tmp_path: Path,
) -> None:
    """Graceful shutdown via docker compose stop writes a stop status file with all OK flags.

    Scenario:
        A running instance is stopped via docker compose stop. The test waits
        for the service to exit, then copies the ark-stop-status.env file from
        the stopped container to the host for inspection.

    Expected:
        - The ark-stop-status.env file exists in the container after shutdown.
        - SAVE_OK=true — world save was confirmed.
        - SHUTDOWN_OK=true — do-exit command was confirmed.
        - STOP_OK=true — the overall stop workflow completed successfully.
    """
    instance = started_container_instance["instance"]

    compose_stop(compose_client, instance, timeout=240)
    wait_for(
        "service to exit after docker compose stop",
        300,
        lambda: container_exited(compose_client, instance),
    )

    cid = container_id(compose_client, instance)
    assert cid, f"Expected container id for service {instance}."
    status_copy = tmp_path / "ark-stop-status.env"
    copy_from_service(
        compose_client,
        instance,
        "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ark-stop-status.env",
        str(status_copy),
    )

    assert status_copy.is_file(), "Expected stop status file to exist after stop workflow."
    entries: dict[str, str] = {}
    for raw in status_copy.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = raw.strip()
        if not line or "=" not in line:
            continue
        key, value = line.split("=", 1)
        entries[key.strip()] = value.strip()

    assert entries.get("SAVE_OK") == "true"
    assert entries.get("SHUTDOWN_OK") == "true"
    assert entries.get("STOP_OK") == "true"
