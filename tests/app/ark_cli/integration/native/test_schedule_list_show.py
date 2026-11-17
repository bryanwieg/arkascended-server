from __future__ import annotations

import json
from pathlib import Path

from helpers.native_fixtures import SCHEDULE_TWO_ENTRIES, write_schedule_file


def test_list_schedules_empty(run_cli, schedule_workdir: Path) -> None:
    """list schedules with no configured schedules shows empty message.

    Scenario:
        No schedule entries exist. list schedules is invoked.

    Expected:
        - The command exits zero.
        - The output contains 'No schedules configured'.
    """
    result = run_cli(["list", "schedules"])
    assert result.returncode == 0
    assert "No schedules configured" in result.stdout


def test_list_schedules_with_entries(run_cli, sched_path: Path) -> None:
    """list schedules displays all configured schedule names and cron expressions.

    Scenario:
        A schedule file with two entries (nightly, weekly) exists. list
        schedules is invoked.

    Expected:
        - The command exits zero.
        - Both schedule names and their cron expressions appear in the output.
    """
    write_schedule_file(sched_path, SCHEDULE_TWO_ENTRIES)
    result = run_cli(["list", "schedules"])
    assert result.returncode == 0
    assert "nightly" in result.stdout
    assert "weekly" in result.stdout
    assert "0 3 * * *" in result.stdout
    assert "0 4 * * 0" in result.stdout


def test_list_schedules_json(run_cli, sched_path: Path) -> None:
    """list schedules --json returns a list of schedule objects.

    Scenario:
        A schedule file with two entries exists. list schedules is invoked
        with --json.

    Expected:
        - The command exits zero.
        - The JSON output is a list of two entries, each containing name,
          enabled, type, instance, and cron fields.
    """
    write_schedule_file(sched_path, SCHEDULE_TWO_ENTRIES)
    result = run_cli(["list", "schedules", "--json"])
    assert result.returncode == 0
    parsed = json.loads(result.stdout)
    assert isinstance(parsed, list)
    assert len(parsed) == 2
    for entry in parsed:
        assert "name" in entry
        assert "enabled" in entry
        assert "type" in entry
        assert "instance" in entry
        assert "cron" in entry


def test_show_schedule_details(run_cli, sched_path: Path) -> None:
    """show schedule displays instance, cron, and enabled status.

    Scenario:
        A schedule file with two entries exists. show schedule is invoked
        for 'nightly'.

    Expected:
        - The command exits zero.
        - The output contains the instance name, cron expression, and an
          enabled indicator (yes/true).
    """
    write_schedule_file(sched_path, SCHEDULE_TWO_ENTRIES)
    result = run_cli(["show", "schedule", "--name", "nightly"])
    assert result.returncode == 0
    assert "testinst" in result.stdout
    assert "0 3 * * *" in result.stdout
    assert "yes" in result.stdout.lower() or "true" in result.stdout.lower()


def test_show_schedule_json(run_cli, sched_path: Path) -> None:
    """show schedule --json returns a JSON object with full schedule details.

    Scenario:
        A schedule file with two entries exists. show schedule is invoked
        for 'nightly' with --json.

    Expected:
        - The command exits zero.
        - The JSON output has name='nightly', the correct instance, cron,
          enabled=true, and format and retain_count fields.
    """
    write_schedule_file(sched_path, SCHEDULE_TWO_ENTRIES)
    result = run_cli(["show", "schedule", "--name", "nightly", "--json"])
    assert result.returncode == 0
    parsed = json.loads(result.stdout)
    assert parsed["name"] == "nightly"
    assert parsed["instance"] == "testinst"
    assert parsed["cron"] == "0 3 * * *"
    assert parsed["enabled"] is True
    assert "format" in parsed
    assert "retain_count" in parsed


def test_show_schedule_not_found(run_cli) -> None:
    """show schedule for a non-existent entry exits non-zero.

    Scenario:
        show schedule is invoked for a name that does not exist.

    Expected:
        - The command exits non-zero.
        - The output contains 'not found'.
    """
    result = run_cli(["show", "schedule", "--name", "nonexistent"])
    assert result.returncode != 0
    assert "not found" in (result.stdout + result.stderr).lower()
