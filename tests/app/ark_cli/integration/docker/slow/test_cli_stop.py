from __future__ import annotations

import uuid
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import (
    compose_logs,
    container_ensure_image_tag,
    container_exit_code,
    container_exited,
    container_running,
    container_start,
)

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_cli_stop_graceful(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_instance_cli: dict[str, str],
) -> None:
    """ark-cli stop instance triggers SIGTERM, the shutdown manager runs cleanly, and the server exits 0.

    Scenario:
        A fully started instance is stopped via ark-cli stop instance. The CLI
        issues docker compose stop -t 900, which sends SIGTERM to PID 1. The
        runtime shutdown manager receives the signal, runs ark-server-stop.py
        internally, and the server process exits cleanly.

    Expected:
        - The container exit code is 0, confirming a clean shutdown.
        - Logs contain "[shutdown] received signal".
        - Logs contain "[shutdown] executing graceful stop script".
    """
    instance = started_instance_cli["instance"]

    run_cmd(
        [
            str(cli_path),
            "stop",
            "instance",
            "--instance",
            instance,
            "--compose-file",
            test_env["COMPOSE_FILE"],
        ],
        env=test_env,
        cwd=repo_root,
    )

    wait_for(
        "container to exit after stop",
        180,
        lambda: container_exited(compose_client, instance),
    )

    exit_code = container_exit_code(compose_client, instance)
    assert exit_code == 0, f"Expected clean exit after cli stop, got {exit_code}."

    logs = compose_logs(compose_client, instance, tail=300)
    assert "[shutdown] received signal" in logs
    assert "[shutdown] executing graceful stop script" in logs


def test_cli_stop_kill(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """ark-cli stop instance --kill sends SIGKILL immediately and produces a non-zero container exit code.

    Scenario:
        An instance is created via the CLI and the container is started. The test
        waits only for the container to reach "running" status — full ARK/RCON
        readiness is not required since kill bypasses all shutdown logic. The CLI
        is then used to kill the service immediately.

    Expected:
        - The container exit code is non-zero, confirming the process was killed.
    """
    instance = f"pytest-ark-kill-{uuid.uuid4().hex[:8]}"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            server_password,
            "--admin-password",
            admin_password,
        ],
        env=test_env,
        cwd=repo_root,
    )

    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    container_ensure_image_tag(compose_client, image)
    container_start(compose_client, instance)

    # Kill does not require ARK readiness — just a running container.
    running_timeout = int(test_env.get("ARK_TEST_CONTAINER_RUNNING_TIMEOUT", "60"))
    wait_for(
        "container to reach running status",
        running_timeout,
        lambda: container_running(compose_client, instance),
    )

    run_cmd(
        [
            str(cli_path),
            "stop",
            "instance",
            "--instance",
            instance,
            "--kill",
            "--compose-file",
            test_env["COMPOSE_FILE"],
        ],
        env=test_env,
        cwd=repo_root,
    )

    wait_for(
        "container to exit after kill",
        180,
        lambda: container_exited(compose_client, instance),
    )

    exit_code = container_exit_code(compose_client, instance)
    assert exit_code == 137, f"Expected exit code 137 (SIGKILL), got {exit_code}."
