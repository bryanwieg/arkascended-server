from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_manage_schedule_backup_lifecycle(
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    started_instance_cli: dict[str, str],
    tmp_path: Path,
) -> None:
    """Full schedule management lifecycle: create, validate, run, list, show, remove.

    Scenario:
        A backup schedule is created for a running instance using a temporary
        working directory so the schedule config files do not pollute the repo.
        The schedule is run immediately to produce an actual backup, then
        inspected via list and show. Finally the schedule is removed and list
        schedules confirms it is gone.

    Expected:
        - create schedule exits zero and output contains "Created schedule <name>".
        - schedules.yaml exists on disk after creation.
        - validate schedule exits zero with "Validated schedule <name>".
        - run schedule exits zero with "Ran schedule <name>" and produces a
          backup archive in the instance backup directory.
        - list schedules output contains the schedule name, "LAST_STATUS" header,
          and "success".
        - show schedule output contains name, instance, type, last_status, and
          command_preview fields with correct values.
        - remove schedule exits zero with "Removed schedule <name>".
        - list schedules after removal exits zero with "No schedules configured."
    """
    instance = started_instance_cli["instance"]
    schedule_name = "nightly-asa1"
    workdir = tmp_path / "schedule-workdir"
    workdir.mkdir(parents=True, exist_ok=True)

    create_result = run_cmd(
        [
            str(cli_path),
            "create",
            "schedule",
            "backup",
            "--instance",
            instance,
            "--name",
            schedule_name,
            "--cron",
            "0 3 * * *",
        ],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    assert f"Created schedule {schedule_name}" in create_result.stdout

    schedules_yaml = workdir / ".ark-manage" / "schedules.yaml"
    assert schedules_yaml.is_file()

    validate_result = run_cmd(
        [str(cli_path), "validate", "schedule", "--name", schedule_name],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    assert f"Validated schedule {schedule_name}" in validate_result.stdout

    run_result = run_cmd(
        [str(cli_path), "run", "schedule", "--name", schedule_name],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    assert f"Ran schedule {schedule_name}" in run_result.stdout

    list_result = run_cmd(
        [str(cli_path), "list", "schedules"],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    assert schedule_name in list_result.stdout
    assert "LAST_STATUS" in list_result.stdout
    assert "success" in list_result.stdout

    show_result = run_cmd(
        [str(cli_path), "show", "schedule", "--name", schedule_name],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    for want in (
        f"name: {schedule_name}",
        f"instance: {instance}",
        "type: backup",
        "last_status: success",
        "command_preview: ark-cli create backup",
    ):
        assert want in show_result.stdout

    backup_dir = Path(test_env["ARK_BACKUP_DIR"]) / instance
    archives = sorted(backup_dir.glob("*.tar.gz")) + sorted(backup_dir.glob("*.tar"))
    assert archives, f"Expected scheduled backup archive in {backup_dir}"

    remove_result = run_cmd(
        [str(cli_path), "remove", "schedule", "--name", schedule_name],
        env=test_env,
        cwd=workdir,
        capture_output=True,
    )
    assert f"Removed schedule {schedule_name}" in remove_result.stdout

    list_after_remove = run_cmd(
        [str(cli_path), "list", "schedules"],
        env=test_env,
        cwd=workdir,
        capture_output=True,
        check=False,
    )
    # Empty list is a successful command with a friendly message.
    assert list_after_remove.returncode == 0
    assert "No schedules configured." in list_after_remove.stdout
