from __future__ import annotations

import io
import shutil
import tarfile
from pathlib import Path

from helpers.native_fixtures import create_backup_archive

INSTANCE = "asa1"
NAME_VALID = "ark-backup-asa1-20260514-030000.tar.gz"
NAME_CORRUPT = "ark-backup-asa1-20260514-040000.tar.gz"


def _create_corrupt_archive(backup_workdir: Path, instance: str, name: str) -> Path:
    instance_dir = backup_workdir / instance
    instance_dir.mkdir(parents=True, exist_ok=True)
    archive_path = instance_dir / name
    corrupt_data = b"not a valid sqlite database"
    with tarfile.open(str(archive_path), "w:gz") as tar:
        info = tarfile.TarInfo(name="ShooterGame/Saved/test.sqlite")
        info.size = len(corrupt_data)
        tar.addfile(info, io.BytesIO(corrupt_data))
    return archive_path


def test_validate_backup_valid_archive(run_cli, backup_workdir: Path) -> None:
    """validate backup reports VERIFY OK for a valid archive.

    Scenario:
        A valid backup archive (containing a real SQLite database) exists.
        validate backup is invoked for it.

    Expected:
        - If sqlite3 is available, the command exits zero.
        - The output contains 'VERIFY OK'.
    """
    if not shutil.which("sqlite3"):
        return
    create_backup_archive(backup_workdir, INSTANCE, NAME_VALID)
    result = run_cli(["validate", "backup", "--instance", INSTANCE, "--backup", NAME_VALID])
    assert result.returncode == 0
    assert "VERIFY OK" in result.stdout


def test_validate_backup_corrupt_sqlite(run_cli, backup_workdir: Path) -> None:
    """validate backup reports VERIFY FAIL for an archive with corrupt SQLite data.

    Scenario:
        A backup archive is created with invalid (non-SQLite) data in a
        .sqlite file. validate backup is invoked for it.

    Expected:
        - The output contains 'VERIFY FAIL' in stdout or stderr.
    """
    if not shutil.which("sqlite3"):
        return
    _create_corrupt_archive(backup_workdir, INSTANCE, NAME_CORRUPT)
    result = run_cli(["validate", "backup", "--instance", INSTANCE, "--backup", NAME_CORRUPT])
    assert "VERIFY FAIL" in result.stdout or "VERIFY FAIL" in result.stderr


def test_validate_backup_not_found(run_cli, backup_workdir: Path) -> None:
    """validate backup for a non-existent archive exits non-zero.

    Scenario:
        An instance backup directory exists but the specified archive does
        not. validate backup is invoked with a nonexistent filename.

    Expected:
        - The command exits non-zero.
    """
    (backup_workdir / INSTANCE).mkdir(parents=True, exist_ok=True)
    result = run_cli(["validate", "backup", "--instance", INSTANCE, "--backup", "nonexistent.tar.gz"])
    assert result.returncode != 0
