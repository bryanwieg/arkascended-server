from __future__ import annotations

from pathlib import Path

from helpers.native_fixtures import SCHEDULE_SINGLE_DISABLED, SCHEDULE_SINGLE_ENABLED, write_schedule_file


def test_enable_schedule(run_cli, sched_path: Path) -> None:
    """enable schedule sets enabled: true in the schedules file.

    Scenario:
        A schedule file with a disabled 'nightly' entry exists. enable
        schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The schedules file now contains 'enabled: true'.
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_DISABLED)
    result = run_cli(["enable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert "enabled: true" in sched_path.read_text()


def test_disable_schedule(run_cli, sched_path: Path) -> None:
    """disable schedule sets enabled: false in the schedules file.

    Scenario:
        A schedule file with an enabled 'nightly' entry exists. disable
        schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The schedules file now contains 'enabled: false'.
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    result = run_cli(["disable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert "enabled: false" in sched_path.read_text()


def test_enable_already_enabled(run_cli, sched_path: Path) -> None:
    """enable schedule on an already-enabled entry is idempotent (exits zero).

    Scenario:
        A schedule file with an already-enabled 'nightly' entry exists.
        enable schedule is invoked again for that entry.

    Expected:
        - The command exits zero (no error for redundant enable).
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    result = run_cli(["enable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
