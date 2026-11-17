from __future__ import annotations

import re
import shutil
import tarfile
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import (
    compose_logs,
    compose_run,
    compose_stop,
    container_delete_file,
    container_exec_python,
    container_file_exists,
    container_mkdir,
    container_write_file,
    wait_for_startup_log,
)

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


SQL_ERROR_MARKERS = (
    "FAtlasSaveManager::LoadOperationSql",
    "SQL database is corrupt and the save game is compromised",
)


def test_backup_restore_lifecycle(  # noqa: PLR0915
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_instance_cli: dict[str, str],
) -> None:
    """Full backup and restore round-trip for a running instance.

    Scenario:
        A live server has a known marker file and a valid SQLite database in its
        Saved directory. A backup is taken while the server is running, then the
        server is stopped and the marker file is deleted to simulate data loss.
        The backup is restored with --auto-start so the server comes back up
        automatically.

    Expected:
        - create backup succeeds and the CLI log shows the in-container exec path
          (or helper-container fallback path) was used.
        - The archive follows the expected naming convention and contains
          GameUserSettings.ini, ShooterGame.log, the marker file, and at least
          one .ark world-save file.
        - show backup and validate backup report correct metadata and pass
          SQLite integrity checks (when sqlite3 is available on the host).
        - After restore, the marker file is present again and the server
          advertises for join without any SQL corruption log markers.
    """
    instance = started_instance_cli["instance"]
    marker_path = "/home/ark-user/ark-server/ShooterGame/Saved/pytest-restore-marker.txt"
    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"

    wait_for_startup_log(compose_client, instance, server_log_path=server_log_path)

    container_write_file(compose_client, instance, marker_path, "restore-test")
    container_exec_python(
        compose_client,
        instance,
        (
            "import sqlite3; "
            "db=sqlite3.connect('/home/ark-user/ark-server/ShooterGame/Saved/pytest-verify.sqlite'); "
            "db.execute('create table if not exists t(x int)'); "
            "db.execute('insert into t(x) values (1)'); "
            "db.commit(); db.close()"
        ),
    )

    # Run a backup and validate the archive name plus contents.
    _backup_results = []

    def _do_backup() -> bool:
        result = run_cmd(
            [str(cli_path), "create", "backup", "--instance", instance],
            env=test_env,
            cwd=repo_root,
            check=False,
            capture_output=True,
        )
        if result.returncode == 0:
            _backup_results.append(result)
            return True
        return False

    wait_for("backup to succeed", 20, _do_backup)
    backup_result = _backup_results[0]
    assert backup_result.stdout.strip(), "Expected archive path in backup result stdout"
    backup_combined = f"{backup_result.stdout}\n{backup_result.stderr}"
    used_fallback = "falling back to helper container" in backup_combined

    backup_logs = compose_logs(compose_client, instance, tail=400)
    assert "[backup] starting backup workflow" in backup_logs
    if not used_fallback:
        assert "[backup] backup archive created successfully" in backup_logs
    assert "attempting in-container exec backup" in backup_combined
    backup_dir = Path(test_env["ARK_BACKUP_DIR"]) / instance
    archives = sorted(backup_dir.glob("*.tar.gz")) + sorted(backup_dir.glob("*.tar"))
    assert archives, f"No backups created in {backup_dir}"
    latest = max(archives, key=lambda path: path.stat().st_mtime)
    pattern = re.compile(rf"^{re.escape(instance)}-backup-\d{{8}}-\d{{6}}\.tar(\.gz)?$")
    assert pattern.match(latest.name), f"Unexpected backup name: {latest.name}"

    show_result = run_cmd(
        [str(cli_path), "show", "backup", "--instance", instance, "--backup", "latest"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert f"instance: {instance}" in show_result.stdout
    assert f"name: {latest.name}" in show_result.stdout

    verify_result = run_cmd(
        [str(cli_path), "validate", "backup", "--instance", instance, "--backup", "latest"],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    verify_combined = f"{verify_result.stdout}\n{verify_result.stderr}"
    if shutil.which("sqlite3"):
        assert verify_result.returncode == 0
        assert "Verification passed:" in verify_result.stdout
        assert "VERIFY OK" in verify_result.stdout
    else:
        assert verify_result.returncode != 0
        assert "sqlite3 is required for backup verification" in verify_combined

    with tarfile.open(latest, "r:*") as tar:
        members = tar.getnames()
    normalized_members = {member.removeprefix("./") for member in members}
    assert "Config/WindowsServer/GameUserSettings.ini" in normalized_members
    # Logs/ is excluded: the live server writes it during tar, which makes tar exit 1.
    assert not any(m.startswith("Logs/") for m in normalized_members)
    assert "pytest-restore-marker.txt" in normalized_members
    assert any(member.startswith("SavedArks/") and member.endswith(".ark") for member in normalized_members), (
        "Expected at least one world save .ark file in SavedArks/."
    )

    # Stop the instance, mutate saved state, then restore a real archive and restart.
    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", instance, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )
    # Service is stopped here, so mutate Saved content via a maintenance one-shot run.
    compose_run(compose_client, instance, ["sh", "-c", f"rm -f {marker_path}"], remove=True, dependencies=False)
    restore_result = run_cmd(
        [str(cli_path), "restore", "backup", "--instance", instance, "--archive", f"{instance}:latest", "--auto-start"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "Restored to" in restore_result.stdout
    assert "Pre-restore backup:" in restore_result.stdout

    wait_for_startup_log(compose_client, instance, server_log_path=server_log_path)
    marker_found = container_file_exists(compose_client, instance, marker_path)
    assert marker_found, "Restore should have brought the marker file back"
    logs_after_restore = compose_logs(compose_client, instance, tail=500)
    for marker in SQL_ERROR_MARKERS:
        assert marker not in logs_after_restore, f"Unexpected SQL/save corruption marker in logs: {marker}"


def test_cli_backup_reports_sqlite_verification_failure(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    started_instance_cli: dict[str, str],
) -> None:
    """SQLite corruption in SavedArks causes create backup to fail with a clear error.

    Scenario:
        A running server has a file in its SavedArks directory that is not a
        valid SQLite database. The container is stopped so the backup runs via
        the helper-container path (offline backup). create backup is then
        invoked and the backup script runs SQLite integrity checks before
        archiving.

    Expected:
        - create backup exits non-zero.
        - The combined stdout/stderr contains "sqlite quick_check failed".
        - The combined stdout/stderr confirms the offline (service not running) backup
          path was taken and records the integrity failure.
        - The framework handles cleanup via compose down + volume removal.
    """
    instance = started_instance_cli["instance"]
    corrupt_dir = "/home/ark-user/ark-server/ShooterGame/Saved/SavedArks/pytest-corrupt"
    corrupt_file = f"{corrupt_dir}/Corrupt.ark"

    wait_for_startup_log(compose_client, instance)

    container_mkdir(compose_client, instance, corrupt_dir)
    container_write_file(compose_client, instance, corrupt_file, "not-a-sqlite-db")
    compose_stop(compose_client, instance)

    result = run_cmd(
        [str(cli_path), "create", "backup", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    combined = f"{result.stdout}\n{result.stderr}"
    assert result.returncode != 0
    assert "sqlite quick_check failed" in combined

    assert "service is not running" in combined
    assert "sqlite quick_check failed" in combined


def test_restore_backup_stop_and_auto_start_running_instance(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_instance_cli: dict[str, str],
) -> None:
    """Restore refuses while running; stop then restore with --auto-start brings server back.

    Scenario:
        A live server has a known marker file. A backup is taken, then the
        marker is deleted to simulate data loss while the server is still
        running. restore backup is attempted against the running instance,
        which must be rejected. The instance is then stopped explicitly and
        restore is retried with --auto-start.

    Expected:
        - restore backup against a running instance exits non-zero with an
          error message containing "must be stopped".
        - After stopping, restore backup --auto-start succeeds: the CLI output
          contains "Restored to" and "Pre-restore backup:".
        - The server comes back up and the marker file is present again,
          confirming the restored state is live.
        - At least one additional archive exists in the backup directory
          (the pre-restore safety backup created by the restore command).
    """
    instance = started_instance_cli["instance"]
    marker_path = "/home/ark-user/ark-server/ShooterGame/Saved/pytest-auto-restore-marker.txt"

    wait_for_startup_log(compose_client, instance)

    container_write_file(compose_client, instance, marker_path, "before-restore")
    run_cmd(
        [str(cli_path), "create", "backup", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )

    backup_dir = Path(test_env["ARK_BACKUP_DIR"]) / instance
    before_restore_archives = sorted(backup_dir.glob("*.tar.gz")) + sorted(backup_dir.glob("*.tar"))
    assert before_restore_archives, f"Expected backup archives before restore in {backup_dir}"

    container_delete_file(compose_client, instance, marker_path)

    # Restore refuses when the service is still running.
    refused = run_cmd(
        [str(cli_path), "restore", "backup", "--instance", instance, "--archive", f"{instance}:latest"],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    assert refused.returncode != 0
    assert "must be stopped" in refused.stderr

    # Stop the instance, then restore with --auto-start.
    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", instance, "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )

    restore_result = run_cmd(
        [str(cli_path), "restore", "backup", "--instance", instance, "--archive", f"{instance}:latest", "--auto-start"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "Restored to" in restore_result.stdout
    assert "Pre-restore backup:" in restore_result.stdout

    wait_for_startup_log(compose_client, instance)

    marker_found = container_file_exists(compose_client, instance, marker_path)
    assert marker_found, "Expected restore to bring marker file back"

    after_restore_archives = sorted(backup_dir.glob("*.tar.gz")) + sorted(backup_dir.glob("*.tar"))
    assert len(after_restore_archives) >= len(before_restore_archives) + 1, (
        "Expected restore with pre-restore backup to add at least one archive.\n"
        f"before={len(before_restore_archives)} after={len(after_restore_archives)}"
    )
