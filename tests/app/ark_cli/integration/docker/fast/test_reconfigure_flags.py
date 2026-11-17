from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]

_PW = ["--server-password", "ServerPass123", "--admin-password", "AdminPass123"]


def _create(
    run_cmd: Callable[..., object],
    cli_path: Path,
    test_env: dict[str, str],
    repo_root: Path,
    name: str,
    extra: list[str] | None = None,
) -> None:
    cmd = [str(cli_path), "create", "instance", "--name", name] + _PW + (extra or [])
    run_cmd(cmd, env=test_env, cwd=repo_root)


def _env_in_compose(compose_path: Path, key: str, value: str) -> bool:
    text = compose_path.read_text(encoding="utf-8")
    return f"{key}={value}" in text or f"{key}: {value}" in text


def test_reconfigure_map_name(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create instance --map-name sets the MAP_NAME env var in the compose file.

    Scenario:
        An instance is created with --map-name TheCenter_WP. The compose file
        is inspected for the MAP_NAME environment variable.

    Expected:
        - The compose file contains MAP_NAME=TheCenter_WP (list-form) or
          MAP_NAME: TheCenter_WP (map-form).
    """
    instance = "pytest-ark-reconf-1"
    _create(run_cmd, cli_path, test_env, repo_root, instance, ["--map-name", "TheCenter_WP"])
    assert _env_in_compose(Path(test_env["COMPOSE_FILE"]), "MAP_NAME", "TheCenter_WP")


def test_reconfigure_session_name(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """reconfigure instance --session-name updates the SESSION_NAME env var in the compose file.

    Scenario:
        An instance is created with default settings. reconfigure instance
        --session-name "My Server" is run and the compose file is inspected.

    Expected:
        - The compose file contains SESSION_NAME=My Server (list-form) or
          SESSION_NAME: My Server (map-form).
    """
    instance = "pytest-ark-reconf-2"
    _create(run_cmd, cli_path, test_env, repo_root, instance)
    run_cmd(
        [str(cli_path), "reconfigure", "instance", "--instance", instance, "--session-name", "My Server"],
        env=test_env,
        cwd=repo_root,
    )
    assert _env_in_compose(Path(test_env["COMPOSE_FILE"]), "SESSION_NAME", "My Server")


def test_reconfigure_tz(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """reconfigure instance --tz updates the TZ env var in the compose file.

    Scenario:
        An instance is created with default settings. reconfigure instance
        --tz UTC is run and the compose file is inspected.

    Expected:
        - The compose file contains TZ=UTC (list-form) or TZ: UTC (map-form).
    """
    instance = "pytest-ark-reconf-3"
    _create(run_cmd, cli_path, test_env, repo_root, instance)
    run_cmd(
        [str(cli_path), "reconfigure", "instance", "--instance", instance, "--tz", "UTC"],
        env=test_env,
        cwd=repo_root,
    )
    assert _env_in_compose(Path(test_env["COMPOSE_FILE"]), "TZ", "UTC")


def test_reconfigure_no_changes_rejected(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """reconfigure instance with no flags exits non-zero.

    Scenario:
        An instance is created. reconfigure instance is run without any
        modification flags.

    Expected:
        - The command exits non-zero, rejecting a no-op reconfiguration.
    """
    instance = "pytest-ark-reconf-4"
    _create(run_cmd, cli_path, test_env, repo_root, instance)
    result = run_cmd(
        [str(cli_path), "reconfigure", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0, (
        f"expected non-zero exit with no flags, got {result.returncode}: {result.stdout}{result.stderr}"
    )
