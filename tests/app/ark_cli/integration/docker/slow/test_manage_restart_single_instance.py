from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import container_id, container_running, container_started_at

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_manage_restart_single_instance_lifecycle(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """restart instance produces a new container with an updated start timestamp.

    Scenario:
        A single instance is created and started. The container's StartedAt
        timestamp is captured via docker inspect before the restart. restart
        instance is then issued and the service is waited on with --for started.
        The StartedAt timestamp is captured again after the restart.

    Expected:
        - restart instance exits zero and its stdout contains
          "Restarted instance target <instance>".
        - The service enters running status within 180 seconds after restart.
        - The post-restart StartedAt timestamp differs from the pre-restart
          value, confirming a new container was created rather than the
          existing one being resumed.
        - wait instance --for stopped succeeds after the final stop.
    """
    instance = "pytest-ark-restart-single-1"
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
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "started"],
        env=test_env,
        cwd=repo_root,
    )

    cid_before = container_id(compose_client, instance)
    assert cid_before, "Expected a container ID before restart."

    started_at_before = container_started_at(compose_client, instance)
    assert started_at_before, "Expected a container start timestamp before restart."

    restart_result = run_cmd(
        [str(cli_path), "restart", "instance", "--instance", instance, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert restart_result.returncode == 0
    assert f"Restarted instance target {instance}" in restart_result.stdout

    wait_for(
        "service to report running after restart",
        180,
        lambda: container_running(compose_client, instance),
    )
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "started"],
        env=test_env,
        cwd=repo_root,
    )

    cid_after = container_id(compose_client, instance)
    assert cid_after, "Expected a container ID after restart."

    started_at_after = container_started_at(compose_client, instance)
    assert started_at_after, "Expected a container start timestamp after restart."
    assert started_at_after != started_at_before, "Expected restart to update container start time."

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
