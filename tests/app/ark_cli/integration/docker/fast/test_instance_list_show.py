from __future__ import annotations

import json
from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container, pytest.mark.lite]

_PW = ["--server-password", "ServerPass123", "--admin-password", "AdminPass123"]


def _create_and_read(
    run_cmd: Callable[..., object],
    cli_path: Path,
    test_env: dict[str, str],
    repo_root: Path,
    instance: str,
    extra_args: list[str] | None = None,
) -> object:
    cmd = [str(cli_path), "create", "instance", "--name", instance] + _PW + (extra_args or [])
    run_cmd(cmd, env=test_env, cwd=repo_root)
    return run_cmd(
        [str(cli_path), "list", "instances"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )


def test_list_instances_after_create(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """list instances includes a newly created instance by name.

    Scenario:
        An instance is created via the CLI. list instances is then run and
        its stdout is inspected.

    Expected:
        - The instance name appears in the list instances output.
    """
    instance = "pytest-ark-show-5"
    result = _create_and_read(run_cmd, cli_path, test_env, repo_root, instance)
    assert instance in result.stdout


def test_list_instances_json(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """list instances --json returns valid JSON with name and status fields.

    Scenario:
        An instance is created. list instances --json is run and the output
        is parsed as JSON.

    Expected:
        - The output is a valid JSON list.
        - The list contains an entry whose name matches the created instance.
        - The entry has both name and status keys.
    """
    instance = "pytest-ark-show-6"
    run_cmd(
        [str(cli_path), "create", "instance", "--name", instance] + _PW,  # noqa: RUF005
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "list", "instances", "--json"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    parsed = json.loads(result.stdout)
    assert isinstance(parsed, list)
    entry = next((e for e in parsed if e.get("name") == instance), None)
    assert entry is not None
    assert "name" in entry and "status" in entry  # noqa: PT018


def test_show_instance_after_create(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """show instance reports port info for a newly created instance.

    Scenario:
        An instance is created. show instance is run and its stdout is
        inspected for port information.

    Expected:
        - The output contains "port:" confirming the instance's port is
          displayed.
    """
    instance = "pytest-ark-show-7"
    run_cmd(
        [str(cli_path), "create", "instance", "--name", instance] + _PW,  # noqa: RUF005
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "port:" in result.stdout


def test_show_instance_json(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """show instance --json returns valid JSON with name and status.

    Scenario:
        An instance is created. show instance --json is run and the output
        is parsed as JSON.

    Expected:
        - The parsed JSON name field matches the created instance name.
        - The parsed JSON contains a status key.
    """
    instance = "pytest-ark-show-8"
    run_cmd(
        [str(cli_path), "create", "instance", "--name", instance] + _PW,  # noqa: RUF005
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance, "--json"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    parsed = json.loads(result.stdout)
    assert parsed["name"] == instance
    assert "status" in parsed
