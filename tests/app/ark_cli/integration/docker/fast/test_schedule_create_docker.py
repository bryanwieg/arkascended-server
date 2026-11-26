from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest
import yaml

pytestmark = [pytest.mark.container, pytest.mark.lite]


def _schedule_entry(test_env: dict[str, str], name: str) -> dict:
    data = yaml.safe_load(Path(test_env["COMPOSE_FILE"]).read_text())
    entry = next((s for s in data.get("x-ark-schedules", []) if s["name"] == name), None)
    assert entry is not None, f"schedule {name} not found in: {data}"
    return entry


def test_create_schedule_backup(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule backup writes the schedule to the compose file with correct fields.

    Scenario:
        An instance and a backup schedule with --cron and --format are
        created. The compose file is inspected.

    Expected:
        - The schedule entry exists under x-ark-schedules with matching name,
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
    entry = _schedule_entry(test_env, "pytest-nightly-1")
    assert entry["instance"] == instance
    assert entry["cron"] == "0 3 * * *"
    assert entry["format"] == "tar.gz"


def test_create_schedule_disabled(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule --disabled sets enabled to false in the compose file.

    Scenario:
        An instance and a backup schedule are created with the --disabled
        flag. The compose file is inspected.

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
    entry = _schedule_entry(test_env, "pytest-nightly-2")
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


def test_create_schedule_writes_compose_extension(
    test_env: dict[str, str],
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """create schedule appends entries under x-ark-schedules and leaves the rest of the file alone.

    Scenario:
        A comment line is added to a compose file holding one instance. A backup and an
        rcon schedule whose command contains `$5` are created, then listed, shown and
        the rcon schedule is run (the instance is not started).

    Expected:
        - Every line of the file before the schedules is unchanged, the comment included,
          and the rest is the x-ark-schedules block holding both entries.
        - The rcon command is stored as `$$5`; show schedule prints it with `$5`.
        - No .ark-manage/schedules.yaml is created.
        - list schedules prints both; run schedule finds the rcon entry and skips the stopped instance.
    """
    compose_file = Path(test_env["COMPOSE_FILE"])
    instance = "pytest-ark-sched-ext"

    def cli(*args: str) -> object:
        return run_cmd([str(cli_path), *args], env=test_env, cwd=compose_file.parent, capture_output=True)

    cli(
        "create",
        "instance",
        "--name",
        instance,
        "--server-password",
        "ServerPass123",
        "--admin-password",
        "AdminPass123",
    )
    before = "# pytest: keep this comment\n" + compose_file.read_text()
    compose_file.write_text(before)

    cli("create", "schedule", "backup", "--instance", instance, "--name", "ext-backup", "--cron", "0 3 * * *")
    cli(
        "create",
        "schedule",
        "rcon",
        "--instance",
        instance,
        "--name",
        "ext-rcon",
        "--cron",
        "0 * * * *",
        "--command",
        "ServerChat $5",
    )

    after = compose_file.read_text().splitlines()
    before_lines = before.splitlines()
    assert after[: len(before_lines)] == before_lines
    assert after[len(before_lines)] == "x-ark-schedules:"
    assert "ServerChat $$5" in "\n".join(after)
    entries = yaml.safe_load(compose_file.read_text())["x-ark-schedules"]
    assert {e["name"] for e in entries} == {"ext-backup", "ext-rcon"}
    assert not (compose_file.parent / ".ark-manage" / "schedules.yaml").exists()

    listed = cli("list", "schedules").stdout
    assert "ext-backup" in listed
    assert "ext-rcon" in listed
    assert "command: ServerChat $5\n" in cli("show", "schedule", "--name", "ext-rcon").stdout
    ran = cli("run", "schedule", "--name", "ext-rcon")
    assert f"Warning: {instance} is stopped; rcon command unsuccessful" in ran.stdout + ran.stderr
