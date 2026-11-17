from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


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


def test_apply_schedules(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """apply schedules generates a crontab file with the expected cron expression.

    Scenario:
        An instance and a backup schedule with a known cron expression are
        created. apply schedules is run and the generated crontab file is
        inspected.

    Expected:
        - The generated crontab file exists under .ark-manage/generated/.
        - The file contains the expected cron expression "0 3 * * *".
    """
    instance = "pytest-ark-sched-4"
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
            "pytest-nightly-5",
            "--cron",
            "0 3 * * *",
            "--format",
            "tar.gz",
        ],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "apply", "schedules"],
        env=test_env,
        cwd=repo_root,
        check=True,
    )
    crontab_path = repo_root / ".ark-manage" / "generated" / "schedules.crontab"
    assert crontab_path.exists(), f"crontab file not found at {crontab_path}"
    content = crontab_path.read_text()
    assert "0 3 * * *" in content, f"cron expression not found in crontab: {content}"
