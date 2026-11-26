from __future__ import annotations

from pathlib import Path

from helpers.native_fixtures import SCHEDULE_SINGLE_ENABLED, SCHEDULE_TWO_ENTRIES, write_run_state, write_schedule_file


def test_remove_schedule(run_cli, sched_path: Path) -> None:
    """remove schedule deletes a named entry; other entries remain.

    Scenario:
        A compose file with two entries (nightly, weekly) exists. remove
        schedule is invoked for 'nightly'. The remaining entries are
        verified via list schedules.

    Expected:
        - The command exits zero.
        - 'nightly' no longer appears in list schedules output.
        - 'weekly' still appears in list schedules output.
    """
    write_schedule_file(sched_path, SCHEDULE_TWO_ENTRIES)
    result = run_cli(["remove", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    list_result = run_cli(["list", "schedules"])
    assert list_result.returncode == 0
    assert "nightly" not in list_result.stdout
    assert "weekly" in list_result.stdout


def test_remove_schedule_not_found(run_cli) -> None:
    """remove schedule for a non-existent entry exits non-zero.

    Scenario:
        remove schedule is invoked for a name that does not exist in the
        compose file.

    Expected:
        - The command exits non-zero.
    """
    result = run_cli(["remove", "schedule", "--name", "nonexistent"])
    assert result.returncode != 0


def test_check_schedule_with_state(run_cli, schedule_workdir: Path, sched_path: Path) -> None:
    """check schedule shows last run status when state exists.

    Scenario:
        A schedule entry exists and a run state file records a 'success'
        status. check schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The output contains 'success'.
    """
    state_dir = schedule_workdir / ".ark-manage" / "state" / "schedules"
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    write_run_state(state_dir, "nightly", "success")
    result = run_cli(["check", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert "success" in result.stdout


def test_check_schedule_no_state(run_cli, sched_path: Path) -> None:
    """check schedule shows placeholder when no run state exists.

    Scenario:
        A schedule entry exists but no run state file has been written.
        check schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The output contains 'last_status' and a dash placeholder.
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    result = run_cli(["check", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert "last_status" in result.stdout
    assert "-" in result.stdout


def test_check_schedule_not_found(run_cli) -> None:
    """check schedule for a non-existent entry exits non-zero.

    Scenario:
        check schedule is invoked for a name that does not exist in the
        compose file.

    Expected:
        - The command exits non-zero.
    """
    result = run_cli(["check", "schedule", "--name", "nonexistent"])
    assert result.returncode != 0
