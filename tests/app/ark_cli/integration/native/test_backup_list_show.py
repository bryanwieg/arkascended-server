from __future__ import annotations

import json
from pathlib import Path

from helpers.native_fixtures import create_backup_archive

INSTANCE = "asa1"
NAME_A = "ark-backup-asa1-20260514-030000.tar.gz"
NAME_B = "ark-backup-asa1-20260514-040000.tar.gz"


def test_list_backups_empty_dir(run_cli, backup_workdir: Path) -> None:
    """list backups for an instance with no archives reports an error.

    Scenario:
        The backup directory for an instance exists but is empty. list
        backups is invoked for that instance.

    Expected:
        - The command exits non-zero.
        - The output contains 'no backup'.
    """
    (backup_workdir / INSTANCE).mkdir(parents=True, exist_ok=True)
    result = run_cli(["list", "backups", "--instance", INSTANCE])
    assert result.returncode != 0
    assert "no backup" in (result.stderr + result.stdout).lower()


def test_list_backups_with_archives(run_cli, backup_workdir: Path) -> None:
    """list backups shows archive filenames for an instance with backups.

    Scenario:
        Two backup archives exist for an instance. list backups is invoked.

    Expected:
        - The command exits zero.
        - Both archive filenames appear in the output.
    """
    create_backup_archive(backup_workdir, INSTANCE, NAME_A)
    create_backup_archive(backup_workdir, INSTANCE, NAME_B)
    result = run_cli(["list", "backups", "--instance", INSTANCE])
    assert result.returncode == 0
    assert NAME_A in result.stdout
    assert NAME_B in result.stdout


def test_list_backups_json(run_cli, backup_workdir: Path) -> None:
    """list backups --json returns a list of backup objects.

    Scenario:
        A backup archive exists for an instance. list backups is invoked
        with --json.

    Expected:
        - The command exits zero.
        - The JSON output is a list where each entry has name, size_bytes,
          and date fields.
    """
    create_backup_archive(backup_workdir, INSTANCE, NAME_A)
    result = run_cli(["list", "backups", "--instance", INSTANCE, "--json"])
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert isinstance(data, list)
    assert len(data) >= 1
    for entry in data:
        assert "name" in entry
        assert "size_bytes" in entry
        assert "date" in entry


def test_show_backup_metadata(run_cli, backup_workdir: Path) -> None:
    """show backup displays metadata fields (size, modified date).

    Scenario:
        A backup archive exists for an instance. show backup is invoked with
        the archive name.

    Expected:
        - The command exits zero.
        - The output contains size and modified date information.
    """
    create_backup_archive(backup_workdir, INSTANCE, NAME_A)
    result = run_cli(["show", "backup", "--instance", INSTANCE, "--backup", NAME_A])
    assert result.returncode == 0
    assert "size_bytes" in result.stdout.lower() or "size" in result.stdout.lower()
    assert "modified_utc" in result.stdout.lower() or "modified" in result.stdout.lower()


def test_show_backup_json(run_cli, backup_workdir: Path) -> None:
    """show backup --json returns a JSON object with full metadata.

    Scenario:
        A backup archive exists for an instance. show backup is invoked with
        --json.

    Expected:
        - The command exits zero.
        - The JSON output contains instance, name, size_bytes, and format fields.
    """
    create_backup_archive(backup_workdir, INSTANCE, NAME_A)
    result = run_cli(["show", "backup", "--instance", INSTANCE, "--backup", NAME_A, "--json"])
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert "instance" in data
    assert "name" in data
    assert "size_bytes" in data
    assert "format" in data


def test_show_backup_not_found(run_cli, backup_workdir: Path) -> None:
    """show backup for a non-existent archive exits non-zero.

    Scenario:
        An instance backup directory exists but the specified archive does
        not. show backup is invoked with a nonexistent filename.

    Expected:
        - The command exits non-zero.
    """
    (backup_workdir / INSTANCE).mkdir(parents=True, exist_ok=True)
    result = run_cli(["show", "backup", "--instance", INSTANCE, "--backup", "nonexistent.tar.gz"])
    assert result.returncode != 0
