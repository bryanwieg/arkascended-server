"""Canary: verifies test infrastructure after directory reorganization."""

from __future__ import annotations

from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_canary_imports_resolve(repo_root: Path) -> None:
    """repo_root fixture works from new location."""
    assert (repo_root / "run-tests.py").is_file()
    assert (repo_root / "container" / "scripts" / "runtime").is_dir()


def test_canary_fixture_archive_exists(repo_root: Path) -> None:
    """Fixture archive is accessible at new path."""
    assert (repo_root / "tests/fixtures/restore/theisland_wp_fixture.tar.zst").is_file()


def test_canary_root_fixtures(repo_root: Path, test_run_dir: Path, test_artifact_dir: Path) -> None:
    """Session fixtures from tests/conftest.py are inherited."""
    assert repo_root.is_dir()
    assert test_run_dir.is_dir()
    assert test_artifact_dir.is_dir()


def test_canary_helper_modules() -> None:
    """All helper modules import and expose expected names."""
    from helpers.artifacts import ensure_cli_binary, repo_root, tests_root
    from helpers.compose import assign_random_ports, disable_restart_policy, pick_free_port
    from helpers.compose_client import (
        compose_exec,
        compose_kill,
        compose_logs,
        compose_ps_all,
        compose_run,
        compose_stop,
        container_exit_code,
        container_exited,
        container_id,
        container_running,
        copy_from_service,
        get_container_port,
        get_docker_client,
        start_container_instance,
        wait_for_container_state,
    )
    from helpers.diagnostics import format_command_failure, log_command, run_cmd, run_with_compose_patch
    from helpers.infra import (
        build_container_instance_config,
        cleanup_test_env,
        prepare_test_env,
        wait_for,
    )

    assert callable(repo_root)
    assert callable(ensure_cli_binary)
    assert callable(tests_root)
    assert callable(assign_random_ports)
    assert callable(disable_restart_policy)
    assert callable(pick_free_port)
    assert callable(run_cmd)
    assert callable(run_with_compose_patch)
    assert callable(format_command_failure)
    assert callable(log_command)
    assert callable(container_exit_code)
    assert callable(container_exited)
    assert callable(container_id)
    assert callable(container_running)
    assert callable(compose_exec)
    assert callable(compose_kill)
    assert callable(compose_logs)
    assert callable(compose_ps_all)
    assert callable(compose_run)
    assert callable(compose_stop)
    assert callable(copy_from_service)
    assert callable(get_container_port)
    assert callable(get_docker_client)
    assert callable(start_container_instance)
    assert callable(wait_for_container_state)
    assert callable(wait_for)
    assert callable(prepare_test_env)
    assert callable(cleanup_test_env)
    assert callable(build_container_instance_config)


def test_canary_helpers_package() -> None:
    """helpers __init__.py re-exports all public names."""
    import helpers

    expected = [
        "assign_random_ports",
        "build_container_instance_config",
        "capture_diagnostics",
        "cleanup_test_env",
        "cli_path",
        "cli_command_args_without_global_options",
        "compose_exec",
        "compose_kill",
        "compose_logs",
        "compose_ps_all",
        "compose_run",
        "compose_stop",
        "container_exit_code",
        "container_exited",
        "container_id",
        "container_running",
        "copy_from_service",
        "disable_restart_policy",
        "enable_no_battleye",
        "enable_skip_ram_check",
        "ensure_cli_binary",
        "fail_with_container_logs",
        "format_command_failure",
        "get_container_port",
        "get_docker_client",
        "log_command",
        "log_framework",
        "log_phase_snapshot",
        "pick_free_port",
        "prepare_test_env",
        "repo_root",
        "run_cmd",
        "run_with_compose_patch",
        "safe_read_tail",
        "sanitize_path_component",
        "start_container_instance",
        "steam_api_server",
        "tail_lines",
        "test_node_dir_name",
        "tests_root",
        "wait_for",
        "wait_for_container_state",
        "write_pytest_report_artifact",
        "write_single_service_compose",
    ]
    for name in expected:
        assert hasattr(helpers, name), f"helpers missing: {name}"
    for name in expected:
        assert name in helpers.__all__, f"helpers.__all__ missing: {name}"


def test_canary_command_logging(test_artifact_dir: Path) -> None:
    """run_cmd logs commands to commands.log when ARK_TEST_COMMANDS_LOG is set."""
    import os
    import subprocess

    from helpers.diagnostics import log_command, run_cmd

    commands_log = test_artifact_dir / "commands.log"

    # Direct log_command call
    fake_result = subprocess.CompletedProcess(["echo", "hello"], 0, stdout="hello\n", stderr="")
    log_command(commands_log, ["echo", "hello"], fake_result)
    assert commands_log.is_file()
    content = commands_log.read_text()
    assert "echo hello (exit 0)" in content
    assert "hello\n" in content

    # run_cmd with ARK_TEST_COMMANDS_LOG set
    env = os.environ.copy()
    env["ARK_TEST_COMMANDS_LOG"] = str(commands_log)
    run_cmd(
        ["echo", "canary-logging-test"],
        env=env,
        cwd=test_artifact_dir,
        capture_output=True,
    )
    content = commands_log.read_text()
    assert "echo canary-logging-test" in content
    assert "canary-logging-test" in content

    # quiet=True suppresses logging
    pre_len = len(content)
    run_cmd(
        ["echo", "quiet-test"],
        env=env,
        cwd=test_artifact_dir,
        capture_output=True,
        quiet=True,
    )
    post_content = commands_log.read_text()
    assert "quiet-test" not in post_content[pre_len:]
