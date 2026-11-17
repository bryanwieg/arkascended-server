"""Unit tests for container/scripts/runtime/_logging.py.

Verifies the channel semantics promised by the module docstring:
emit_event writes to stdout, emit_warning and emit_error write to stderr
with their severity prefix, and the PID1 fd mirror is attempted but
silent on failure. End-to-end verification that the PID1 mirror
surfaces events into ``docker logs`` lives in
tests/container/integration/slow/test_ark_backup_script.py and
tests/app/ark-cli/integration/slow/test_manage_backup_restore.py.
"""

from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_logging():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("_logging")


@pytest.fixture(name="logging_mod")
def logging_mod_fixture():
    return _load_logging()


def test_emit_event_writes_to_stdout(logging_mod, capsys, monkeypatch) -> None:
    """emit_event prints "[phase] msg\\n" to stdout."""
    # Stub the PID1 mirror so tests don't actually touch /proc/1.
    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", lambda *a, **kw: None)
    logging_mod.emit_event("backup", "starting backup workflow")
    captured = capsys.readouterr()
    assert captured.out == "[backup] starting backup workflow\n"
    assert captured.err == ""


def test_emit_error_writes_to_stderr(logging_mod, capsys, monkeypatch) -> None:
    """emit_error prints "[phase] Error: msg\\n" to stderr, not stdout."""
    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", lambda *a, **kw: None)
    logging_mod.emit_error("restore", "archive contains unsafe path")
    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == "[restore] Error: archive contains unsafe path\n"


def test_emit_warning_writes_to_stderr(logging_mod, capsys, monkeypatch) -> None:
    """emit_warning prints "[phase] Warning: msg\\n" to stderr, not stdout."""
    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", lambda *a, **kw: None)
    logging_mod.emit_warning("server-args", "MAP_NAME does not end in _WP")
    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == "[server-args] Warning: MAP_NAME does not end in _WP\n"


def test_emit_event_mirrors_to_pid1_stdout(logging_mod, capsys, monkeypatch) -> None:
    """emit_event calls _mirror_to_pid1 with stderr=False and the full text."""
    calls: list[tuple[str, bool]] = []

    def fake_mirror(message: str, *, stderr: bool) -> None:
        calls.append((message, stderr))

    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", fake_mirror)
    logging_mod.emit_event("backup", "starting")
    capsys.readouterr()
    assert calls == [("[backup] starting\n", False)]


def test_emit_error_mirrors_to_pid1_stderr(logging_mod, capsys, monkeypatch) -> None:
    """emit_error and emit_warning call _mirror_to_pid1 with stderr=True."""
    calls: list[tuple[str, bool]] = []

    def fake_mirror(message: str, *, stderr: bool) -> None:
        calls.append((message, stderr))

    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", fake_mirror)
    logging_mod.emit_error("backup", "failed to write archive")
    logging_mod.emit_warning("backup", "retrying")
    capsys.readouterr()
    assert calls == [("[backup] Error: failed to write archive\n", True), ("[backup] Warning: retrying\n", True)]


def test_mirror_silent_on_oserror_opening_fd(logging_mod, monkeypatch) -> None:
    """The PID1 mirror swallows OSError on open (e.g. /proc unavailable)."""

    def failing_open(path, flags):
        raise OSError("unavailable")

    monkeypatch.setattr("os.open", failing_open)
    # Must not raise.
    logging_mod._mirror_to_pid1("[backup] x\n", stderr=False)


def test_mirror_silent_on_oserror_writing_fd(logging_mod, monkeypatch) -> None:
    """The PID1 mirror swallows OSError during write."""
    opened: list[int] = []
    closed: list[int] = []

    def fake_open(path, flags):
        opened.append(42)
        return 42

    def failing_write(fd, data):
        raise OSError("write failed")

    def fake_close(fd):
        closed.append(fd)

    monkeypatch.setattr("os.open", fake_open)
    monkeypatch.setattr("os.write", failing_write)
    monkeypatch.setattr("os.close", fake_close)
    # Must not raise; must still close the fd.
    logging_mod._mirror_to_pid1("[backup] x\n", stderr=False)
    assert opened == [42]
    assert closed == [42]


def test_mirror_writes_utf8_bytes(logging_mod, monkeypatch) -> None:
    """The PID1 mirror encodes as UTF-8 with replacement on error."""
    written: list[bytes] = []

    monkeypatch.setattr("os.open", lambda path, flags: 42)
    monkeypatch.setattr("os.write", lambda fd, data: written.append(data) or len(data))
    monkeypatch.setattr("os.close", lambda fd: None)

    logging_mod._mirror_to_pid1("[backup] café\n", stderr=False)
    assert written == [b"[backup] caf\xc3\xa9\n"]


def test_phase_prefix_consistency(logging_mod, capsys, monkeypatch) -> None:
    """All three emit functions honor the [phase] prefix convention
    where applicable (prompt does not use phase)."""
    monkeypatch.setattr(logging_mod, "_mirror_to_pid1", lambda *a, **kw: None)
    logging_mod.emit_event("config-init", "waiting for config file")
    logging_mod.emit_error("config-init", "config file missing")
    captured = capsys.readouterr()
    assert "[config-init] waiting for config file\n" in captured.out
    assert "[config-init] Error: config file missing\n" in captured.err


def test_mirror_skipped_when_own_stream_is_pid1_stream(logging_mod, monkeypatch, tmp_path) -> None:
    """No mirror write when the caller's fd already is PID1's stream (would duplicate docker logs lines)."""
    shared = tmp_path / "stream"
    shared.write_text("")
    written: list[bytes] = []
    real_open = __import__("os").open
    monkeypatch.setattr("os.open", lambda path, flags: real_open(shared, flags))
    monkeypatch.setattr("os.fstat", lambda fd: __import__("os").stat(shared))
    monkeypatch.setattr("os.write", lambda fd, data: written.append(data))
    logging_mod._mirror_to_pid1("[x] y\n", stderr=False)
    assert written == []
