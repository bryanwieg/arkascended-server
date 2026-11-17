from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_show_compose_after_create(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """show compose includes a newly created instance in its output.

    Scenario:
        An instance is created via the CLI. show compose is run and its
        stdout is inspected.

    Expected:
        - The instance name appears in the show compose output.
    """
    instance = "pytest-ark-show-1"
    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            "ServerPass123",
            "--admin-password",
            "AdminPass123",
        ],
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "show", "compose"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert instance in result.stdout, f"expected service name {instance} in output: {result.stdout}"


def test_validate_compose_valid(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """validate compose exits zero for a valid compose file.

    Scenario:
        An instance is created, producing a valid compose file. validate
        compose is run against it.

    Expected:
        - validate compose exits zero.
    """
    instance = "pytest-ark-show-3"
    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            "ServerPass123",
            "--admin-password",
            "AdminPass123",
        ],
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "validate", "compose"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert result.returncode == 0, f"validate compose failed: {result.stderr}"
