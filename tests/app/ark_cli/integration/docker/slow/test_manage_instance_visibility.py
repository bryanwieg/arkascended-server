from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_manage_instance_visibility_multi_instance(
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """list instances and show instance reflect accurate status as instances start.

    Scenario:
        Two instances are created but not started. list instances and
        show instance are queried to confirm both appear as stopped. One
        instance is then started and wait instance --for started is used to
        confirm it is up. The list and show commands are queried again.

    Expected:
        - Before start: list instances output contains both instance names,
          the expected column headers, and "stopped" status for both.
        - show instance for the unstarted instance reports status: stopped.
        - After start: list instances shows instance_a as running and
          instance_b as stopped simultaneously.
        - show instance for instance_a reports status: running.
    """
    instance_a = "pytest-ark-vis-1"
    instance_b = "pytest-ark-vis-2"
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

    list_before = run_cmd(
        [str(cli_path), "list", "instances"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "NAME" in list_before.stdout
    assert "STATUS" in list_before.stdout
    assert "ASA_PORT" in list_before.stdout
    assert "MAP" in list_before.stdout
    assert instance_a in list_before.stdout
    assert instance_b in list_before.stdout
    assert "stopped" in list_before.stdout

    show_before = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance_a],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert re.search(rf"name:\s+{re.escape(instance_a)}", show_before.stdout)
    assert re.search(r"status:\s+stopped", show_before.stdout)

    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance_a],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance_a, "--for", "started"],
        env=test_env,
        cwd=repo_root,
    )

    list_after = run_cmd(
        [str(cli_path), "list", "instances"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert instance_a in list_after.stdout
    assert instance_b in list_after.stdout
    assert "running" in list_after.stdout
    assert "stopped" in list_after.stdout

    show_after = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance_a],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert re.search(rf"name:\s+{re.escape(instance_a)}", show_after.stdout)
    assert re.search(r"status:\s+running", show_after.stdout)
