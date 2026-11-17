from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_save_markers():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("ark_backup.save_markers")


@pytest.fixture(name="sm_mod")
def sm_mod_fixture():
    return _load_save_markers()


# ---------------------------------------------------------------------------
# file_size_or_none
# ---------------------------------------------------------------------------


def test_file_size_or_none_returns_size_for_existing_file(sm_mod, tmp_path: Path) -> None:
    """file_size_or_none returns the byte count of an existing file."""
    f = tmp_path / "log.txt"
    f.write_text("hello", encoding="utf-8")
    assert sm_mod.file_size_or_none(f) == 5


def test_file_size_or_none_returns_none_for_missing_file(sm_mod, tmp_path: Path) -> None:
    """file_size_or_none returns None when the file does not exist."""
    assert sm_mod.file_size_or_none(tmp_path / "nope.txt") is None


def test_file_size_or_none_returns_none_for_directory(sm_mod, tmp_path: Path) -> None:
    """file_size_or_none returns None when the path is a directory."""
    assert sm_mod.file_size_or_none(tmp_path) is None


# ---------------------------------------------------------------------------
# read_new_log_lines
# ---------------------------------------------------------------------------


def test_read_new_log_lines_returns_complete_lines_after_cursor(sm_mod, tmp_path: Path) -> None:
    log = tmp_path / "ShooterGame.log"
    log.write_bytes(b"old\nnew 1\nnew 2\n")
    assert sm_mod.read_new_log_lines(log, 4) == ("new 1\nnew 2\n", 16)


def test_read_new_log_lines_holds_back_a_partial_line(sm_mod, tmp_path: Path) -> None:
    """A marker split across two polls is returned once, whole, after its line ends."""
    log = tmp_path / "ShooterGame.log"
    log.write_bytes(b"done\nWorld Save")
    data, cursor = sm_mod.read_new_log_lines(log, 0)
    assert (data, cursor) == ("done\n", 5)
    with log.open("ab") as handle:
        handle.write(b" Complete\n")
    assert sm_mod.read_new_log_lines(log, cursor) == ("World Save Complete\n", 25)


def test_read_new_log_lines_counts_undecodable_bytes(sm_mod, tmp_path: Path) -> None:
    """The cursor advances by bytes read, even when some bytes are not valid UTF-8."""
    log = tmp_path / "ShooterGame.log"
    log.write_bytes(b"bad \xff byte\n")
    data, cursor = sm_mod.read_new_log_lines(log, 0)
    assert data == "bad  byte\n"
    assert cursor == 11  # bytes in the file, not the 10 decoded characters


def test_read_new_log_lines_restarts_after_truncation(sm_mod, tmp_path: Path) -> None:
    log = tmp_path / "ShooterGame.log"
    log.write_bytes(b"short\n")
    assert sm_mod.read_new_log_lines(log, 100) == ("short\n", 6)


def test_read_new_log_lines_handles_missing_file(sm_mod, tmp_path: Path) -> None:
    assert sm_mod.read_new_log_lines(tmp_path / "missing.log", 10) == ("", 0)


# ---------------------------------------------------------------------------
# wait_for_save_complete
# ---------------------------------------------------------------------------


_MARKER = "[2026.11.13-10.00.00:000][  0]2026.11.13_10.00.00: World Save Complete\n"


def test_wait_for_save_complete_ignores_marker_before_cursor(sm_mod, tmp_path: Path) -> None:
    log = tmp_path / "ShooterGame.log"
    log.write_text("startup\n" + _MARKER)
    assert sm_mod.wait_for_save_complete(log.stat().st_size, 0.3, 0.05, log_file=log) is False


def test_wait_for_save_complete_accepts_marker_after_cursor(sm_mod, tmp_path: Path) -> None:
    log = tmp_path / "ShooterGame.log"
    log.write_text("startup\n" + _MARKER)
    cursor = log.stat().st_size
    with log.open("a") as handle:
        handle.write("Saving world...\n" + _MARKER)
    assert sm_mod.wait_for_save_complete(cursor, 1, 0.05, log_file=log) is True


# ---------------------------------------------------------------------------
# save_related_log_tail
# ---------------------------------------------------------------------------


def test_save_related_log_tail_returns_only_save_lines(sm_mod, tmp_path: Path) -> None:
    """save_related_log_tail returns lines containing 'Saving world' or 'World Save Complete'."""
    f = tmp_path / "ShooterGame.log"
    f.write_text(
        "Server started\nSaving world...\nPlayer joined\n2024.01.01_00.00.01: World Save Complete\n",
        encoding="utf-8",
    )
    lines = sm_mod.save_related_log_tail(f)
    assert any("Saving world" in line for line in lines)
    assert any("World Save Complete" in line for line in lines)
    assert not any("Server started" in line for line in lines)
    assert not any("Player joined" in line for line in lines)


def test_save_related_log_tail_respects_max_lines(sm_mod, tmp_path: Path) -> None:
    """save_related_log_tail returns at most max_lines results."""
    f = tmp_path / "ShooterGame.log"
    # Write 20 save lines.
    f.write_text(
        "\n".join([f"2024.01.01_00.00.{i:02d}: World Save Complete" for i in range(20)]) + "\n", encoding="utf-8"
    )
    lines = sm_mod.save_related_log_tail(f, max_lines=5)
    assert len(lines) <= 5


def test_save_related_log_tail_returns_empty_for_missing_file(sm_mod, tmp_path: Path) -> None:
    """save_related_log_tail returns an empty list when the log file does not exist."""
    assert sm_mod.save_related_log_tail(tmp_path / "missing.log") == []
