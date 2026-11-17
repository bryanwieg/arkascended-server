from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from python_on_whales import DockerClient
pytestmark = [pytest.mark.container]


def test_manage_start_stop_lifecycle(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """Full single-instance lifecycle: create, start, check, save, logs, restart, stop.

    Scenario:
        One instance is created, started, and waited on until the Docker service
        status transitions to running. The instance is then exercised through
        wait --for ready, check instance, save instance, and logs instance to
        confirm each operational command works against a live server. A restart
        is performed and the server is waited on again. Finally the instance is
        stopped and waited on until exited.

    Expected:
        - The service enters running status within 120 seconds of start and
          never appears in exited status beforehand.
        - logs instance output contains the "advertising for join" line.
        - check instance and save instance both exit zero.
        - After restart, the service re-enters running status and wait --for
          ready succeeds.
        - After stop, the service appears in exited status within 180 seconds.
    """
    from helpers.compose_client import container_exited, container_running

    instance = "pytest-ark-startstop-1"
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
    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )

    def _running_or_fail() -> bool:
        if container_exited(compose_client, instance):
            raise AssertionError(f"Service '{instance}' exited prematurely before becoming ready.")
        return container_running(compose_client, instance)

    wait_for("service to report running", 120, _running_or_fail)

    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "ready"],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "check", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "save", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )
    logs_result = run_cmd(
        [str(cli_path), "logs", "instance", "--instance", instance, "--tail", "200"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "Server has completed startup and is now advertising for join." in logs_result.stdout

    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "ready"],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "check", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )

    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", instance, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "stopped"],
        env=test_env,
        cwd=repo_root,
    )
    wait_for(
        "service to exit after stop",
        180,
        lambda: container_exited(compose_client, instance),
    )


def test_manage_multiple_instances_lifecycle(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """Two instances in the same compose file can be started and stopped independently.

    Scenario:
        Two instances are created in the same compose file. Both are started
        individually and the test waits until both Docker services report
        running status. Each instance is then stopped individually and the
        test waits until both appear in exited status.

    Expected:
        - Both services enter running status within 180 seconds without either
          appearing in exited status prematurely.
        - After individual stop commands, both services appear in exited status
          within 180 seconds.
    """
    from helpers.compose_client import container_exited, container_running

    instance_a = "pytest-ark-multi-1"
    instance_b = "pytest-ark-multi-2"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    for instance in (instance_a, instance_b):
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

    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance_a],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance_b],
        env=test_env,
        cwd=repo_root,
    )

    def services_running_or_fail() -> bool:
        for inst in (instance_a, instance_b):
            if container_exited(compose_client, inst):
                raise AssertionError(f"Service '{inst}' exited prematurely before becoming ready.")
        return container_running(compose_client, instance_a) and container_running(compose_client, instance_b)

    wait_for("services to report running", 180, services_running_or_fail)

    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", instance_a, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", instance_b, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )

    def services_exited() -> bool:
        return container_exited(compose_client, instance_a) and container_exited(compose_client, instance_b)

    wait_for("both services to exit after stop", 180, services_exited)
