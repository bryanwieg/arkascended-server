from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container, pytest.mark.lite]


@pytest.fixture(autouse=True)
def _clean_schedules(repo_root: Path) -> None:
    manage_dir = repo_root / ".ark-manage"
    if manage_dir.exists():
        for f in manage_dir.rglob("*"):
            if f.is_file():
                f.unlink()


def test_validate_schedule_valid(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """validate schedule exits zero for a valid backup schedule.

    Scenario:
        An instance and a backup schedule are created. validate schedule is
        run against the schedule name.

    Expected:
        - validate schedule exits zero.
    """
    instance = "pytest-ark-sched-3"
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
    run_cmd(
        [
            str(cli_path),
            "create",
            "schedule",
            "backup",
            "--instance",
            instance,
            "--name",
            "pytest-nightly-4",
            "--cron",
            "0 3 * * *",
            "--format",
            "tar.gz",
        ],
        env=test_env,
        cwd=repo_root,
    )
    result = run_cmd(
        [str(cli_path), "validate", "schedule", "--name", "pytest-nightly-4"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert result.returncode == 0, f"validate schedule failed: {result.stderr}"
