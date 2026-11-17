from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest
import yaml

pytestmark = [pytest.mark.container]


@pytest.fixture(autouse=True)
def _clean_schedules(repo_root: Path) -> None:
    manage_dir = repo_root / ".ark-manage"
    if manage_dir.exists():
        for f in manage_dir.rglob("*"):
            if f.is_file():
                f.unlink()


def test_create_schedule_backup(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule backup writes the schedule to schedules.yaml with correct fields.

    Scenario:
        An instance and a backup schedule with --cron and --format are
        created. The schedules.yaml file is inspected.

    Expected:
        - The schedule entry exists in schedules.yaml with matching name,
          instance, cron, and format fields.
    """
    instance = "pytest-ark-sched-1"
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
            "pytest-nightly-1",
            "--cron",
            "0 3 * * *",
            "--format",
            "tar.gz",
        ],
        env=test_env,
        cwd=repo_root,
    )
    sched_path = repo_root / ".ark-manage" / "schedules.yaml"
    data = yaml.safe_load(sched_path.read_text())
    entry = next((s for s in data["schedules"] if s["name"] == "pytest-nightly-1"), None)
    assert entry is not None, f"schedule not found in: {data}"
    assert entry["instance"] == instance
    assert entry["cron"] == "0 3 * * *"
    assert entry["format"] == "tar.gz"


def test_create_schedule_disabled(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule --disabled sets enabled to false in schedules.yaml.

    Scenario:
        An instance and a backup schedule are created with the --disabled
        flag. The schedules.yaml file is inspected.

    Expected:
        - The schedule entry has enabled: false.
    """
    instance = "pytest-ark-sched-2"
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
            "pytest-nightly-2",
            "--cron",
            "0 4 * * *",
            "--format",
            "tar.gz",
            "--disabled",
        ],
        env=test_env,
        cwd=repo_root,
    )
    sched_path = repo_root / ".ark-manage" / "schedules.yaml"
    data = yaml.safe_load(sched_path.read_text())
    entry = next((s for s in data["schedules"] if s["name"] == "pytest-nightly-2"), None)
    assert entry is not None, f"schedule not found in: {data}"
    assert entry["enabled"] is False


def test_create_schedule_invalid_instance(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule rejects a nonexistent instance with a non-zero exit.

    Scenario:
        create schedule backup is run with a nonexistent instance name.

    Expected:
        - The command exits non-zero.
    """
    result = run_cmd(
        [
            str(cli_path),
            "create",
            "schedule",
            "backup",
            "--instance",
            "nonexistent",
            "--name",
            "pytest-nightly-3",
            "--cron",
            "0 3 * * *",
            "--format",
            "tar.gz",
        ],
        env=test_env,
        cwd=repo_root,
        check=False,
    )
    assert result.returncode != 0, f"expected non-zero exit for invalid instance, got: {result.stdout}"
