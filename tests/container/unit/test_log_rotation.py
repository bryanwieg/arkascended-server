from __future__ import annotations

import os
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def test_rotate_log_file_prunes_to_retention(start_mod, tmp_path: Path) -> None:
    """rotate_log_file renames the active log and prunes archived copies to keep_files.

    Scenario:
        Three archived log files are created with distinct modification times,
        simulating previous rotations. A current active log is written. The
        rotation function is called with keep_files=2.

    Expected:
        - The active log file no longer exists at its original path.
        - Exactly 2 archived candidates remain after pruning.
        - One of the remaining candidates contains the content of the active log,
          confirming it was rotated (not deleted).
    """
    log_path = tmp_path / "ShooterGame.log"

    old_files = [
        tmp_path / "ShooterGame.log.20240101-000000",
        tmp_path / "ShooterGame.log.20240102-000000",
        tmp_path / "ShooterGame.log.20240103-000000",
    ]
    for idx, old_file in enumerate(old_files, start=1):
        old_file.write_text(f"old-{idx}\n", encoding="utf-8")
        os.utime(old_file, (idx, idx))

    log_path.write_text("current-run\n", encoding="utf-8")

    start_mod.rotate_log_file(log_path, keep_files=2)

    assert not log_path.exists()
    rotated = start_mod.rotated_log_candidates(log_path)
    assert len(rotated) == 2
    assert any(candidate.read_text(encoding="utf-8") == "current-run\n" for candidate in rotated)


def test_rotate_log_file_prunes_even_without_active_log(start_mod, tmp_path: Path) -> None:
    """rotate_log_file prunes archived files even when no active log is present.

    Scenario:
        Three archived crash log files exist with distinct modification times.
        No active log file is present. The rotation function is called with
        keep_files=1.

    Expected:
        - Exactly 1 archived candidate remains after pruning.
        - The retained file is the most recent one (20240103 timestamp),
          confirming oldest-first pruning order.
    """
    log_path = tmp_path / "crashcallstack.txt"

    archived = [
        tmp_path / "crashcallstack.txt.20240101-000000",
        tmp_path / "crashcallstack.txt.20240102-000000",
        tmp_path / "crashcallstack.txt.20240103-000000",
    ]
    for idx, archived_file in enumerate(archived, start=1):
        archived_file.write_text(f"crash-{idx}\n", encoding="utf-8")
        os.utime(archived_file, (idx, idx))

    start_mod.rotate_log_file(log_path, keep_files=1)

    rotated = start_mod.rotated_log_candidates(log_path)
    assert len(rotated) == 1
    assert rotated[0].name.endswith("20240103-000000")
