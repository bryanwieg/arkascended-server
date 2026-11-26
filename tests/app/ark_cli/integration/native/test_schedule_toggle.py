from __future__ import annotations

from pathlib import Path

import yaml
from helpers.native_fixtures import SCHEDULE_SINGLE_DISABLED, SCHEDULE_SINGLE_ENABLED, write_schedule_file


def test_enable_schedule(run_cli, sched_path: Path) -> None:
    """enable schedule sets enabled: true in the compose file.

    Scenario:
        A compose file with a disabled 'nightly' entry exists. enable
        schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The schedules file now contains enabled true under x-ark-schedules.
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_DISABLED)
    result = run_cli(["enable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert yaml.safe_load(sched_path.read_text())["x-ark-schedules"][0]["enabled"] is True


def test_disable_schedule(run_cli, sched_path: Path) -> None:
    """disable schedule sets enabled: false in the compose file.

    Scenario:
        A compose file with an enabled 'nightly' entry exists. disable
        schedule is invoked for that entry.

    Expected:
        - The command exits zero.
        - The schedules file now contains enabled false under x-ark-schedules.
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    result = run_cli(["disable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert yaml.safe_load(sched_path.read_text())["x-ark-schedules"][0]["enabled"] is False


def test_enable_already_enabled(run_cli, sched_path: Path) -> None:
    """enable schedule on an already-enabled entry is idempotent (exits zero).

    Scenario:
        A compose file with an already-enabled 'nightly' entry exists.
        enable schedule is invoked again for that entry.

    Expected:
        - The command exits zero (no error for redundant enable).
    """
    write_schedule_file(sched_path, SCHEDULE_SINGLE_ENABLED)
    result = run_cli(["enable", "schedule", "--name", "nightly"])
    assert result.returncode == 0
