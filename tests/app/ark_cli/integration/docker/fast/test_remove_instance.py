from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_remove_stopped_instance(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """remove instance --skip-backup removes a stopped instance from the compose file.

    Scenario:
        An instance is created and a Docker volume is manually created for it.
        remove instance --skip-backup is then run against the stopped instance.

    Expected:
        - The instance service name no longer appears in the compose file,
          confirming the instance was removed.
    """
    instance = "pytest-ark-remove-1"
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
    compose_path = Path(test_env["COMPOSE_FILE"])
    project = compose_path.parent.name
    volume_name = f"{project}_{instance}-saved"
    run_cmd(
        ["docker", "volume", "create", volume_name],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "remove", "instance", "--instance", instance, "--skip-backup"],
        env=test_env,
        cwd=repo_root,
    )
    text = compose_path.read_text(encoding="utf-8")
    assert instance not in text, f"expected service {instance} removed from compose"


def test_remove_instance_not_found(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """remove instance exits non-zero for a nonexistent instance.

    Scenario:
        remove instance --skip-backup is run against a name that does not
        exist in the compose file.

    Expected:
        - The command exits non-zero.
    """
    result = run_cmd(
        [str(cli_path), "remove", "instance", "--instance", "nonexistent", "--skip-backup"],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0, (
        f"expected non-zero exit for nonexistent instance, got {result.returncode}: {result.stdout}{result.stderr}"
    )
