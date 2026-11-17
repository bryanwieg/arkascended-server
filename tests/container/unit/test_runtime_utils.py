from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_runtime_utils():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("runtime_utils")


@pytest.fixture(name="ru_mod")
def ru_mod_fixture():
    return _load_runtime_utils()


# ---------------------------------------------------------------------------
# env_int
# ---------------------------------------------------------------------------


def test_env_int_returns_integer_from_env(ru_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TEST_RU_INT", "42")
    assert ru_mod.env_int("TEST_RU_INT", 0) == 42


def test_env_int_returns_default_when_absent(ru_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("TEST_RU_INT", raising=False)
    assert ru_mod.env_int("TEST_RU_INT", 99) == 99


def test_env_int_accepts_value_at_minimum(ru_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TEST_RU_INT", "0")
    assert ru_mod.env_int("TEST_RU_INT", 5, minimum=0) == 0


@pytest.mark.parametrize(("value", "minimum"), [("not-a-number", 1), ("3.14", 1), ("", 1), ("0", 1), ("-1", 0)])
def test_env_int_raises_config_error(ru_mod, monkeypatch: pytest.MonkeyPatch, value: str, minimum: int) -> None:
    """Non-integers and values below minimum raise ConfigError naming the variable, the minimum and the value."""
    monkeypatch.setenv("TEST_RU_INT", value)
    with pytest.raises(ru_mod.ConfigError) as exc_info:
        ru_mod.env_int("TEST_RU_INT", 5, minimum=minimum)
    assert str(exc_info.value) == f"TEST_RU_INT must be an integer of at least {minimum} (got {value!r})."


# ---------------------------------------------------------------------------
# file_size_or_none
# ---------------------------------------------------------------------------


def test_file_size_or_none_returns_size_for_existing_file(ru_mod, tmp_path: Path) -> None:
    """file_size_or_none returns the byte count of an existing file."""
    f = tmp_path / "data.txt"
    f.write_text("hello", encoding="utf-8")
    assert ru_mod.file_size_or_none(f) == 5


def test_file_size_or_none_returns_none_for_missing_file(ru_mod, tmp_path: Path) -> None:
    """file_size_or_none returns None when the file does not exist."""
    assert ru_mod.file_size_or_none(tmp_path / "nope.txt") is None


def test_file_size_or_none_returns_none_for_directory(ru_mod, tmp_path: Path) -> None:
    """file_size_or_none returns None when the path is a directory."""
    assert ru_mod.file_size_or_none(tmp_path) is None


def test_file_size_or_none_returns_zero_for_empty_file(ru_mod, tmp_path: Path) -> None:
    """file_size_or_none returns 0 for an empty file."""
    f = tmp_path / "empty.txt"
    f.write_bytes(b"")
    assert ru_mod.file_size_or_none(f) == 0


# ---------------------------------------------------------------------------
# ark_server_running
# ---------------------------------------------------------------------------


def _fake_proc_entries(tmp_path: Path, entries: dict[str, bytes | None]) -> list[Path]:
    """Create a fake /proc tree under tmp_path and return the entry paths."""
    paths = []
    for name, cmdline in entries.items():
        d = tmp_path / name
        d.mkdir()
        if cmdline is not None:
            (d / "cmdline").write_bytes(cmdline)
        paths.append(d)
    return paths


def test_ark_server_running_returns_false_when_no_matching_process(ru_mod, tmp_path: Path) -> None:
    """ark_server_running returns False when no ArkAscendedServer.exe cmdline is found."""
    from unittest import mock

    entries = _fake_proc_entries(tmp_path, {"1234": b"some-other-process", "tty": None})
    with mock.patch("runtime_utils.Path") as mock_path_cls:
        fake_proc = mock.MagicMock()
        fake_proc.iterdir.return_value = entries
        mock_path_cls.return_value = fake_proc
        assert ru_mod.ark_server_running() is False


def test_ark_server_running_returns_true_when_matching_process_found(ru_mod, tmp_path: Path) -> None:
    """ark_server_running returns True when ArkAscendedServer.exe appears in a cmdline."""
    from unittest import mock

    entries = _fake_proc_entries(
        tmp_path,
        {
            "1": b"init",
            "999": b"wine ArkAscendedServer.exe --port 7777",
        },
    )
    with mock.patch("runtime_utils.Path") as mock_path_cls:
        fake_proc = mock.MagicMock()
        fake_proc.iterdir.return_value = entries
        mock_path_cls.return_value = fake_proc
        assert ru_mod.ark_server_running() is True


def test_ark_server_running_skips_non_numeric_entries(ru_mod, tmp_path: Path) -> None:
    """ark_server_running ignores /proc entries that are not all-digit names."""
    from unittest import mock

    # Only a non-numeric entry — no cmdline file — should return False without error.
    entries = _fake_proc_entries(tmp_path, {"tty": None, "net": None})
    with mock.patch("runtime_utils.Path") as mock_path_cls:
        fake_proc = mock.MagicMock()
        fake_proc.iterdir.return_value = entries
        mock_path_cls.return_value = fake_proc
        assert ru_mod.ark_server_running() is False


def test_ark_server_running_returns_false_for_empty_proc(ru_mod, tmp_path: Path) -> None:
    """ark_server_running returns False when /proc has no entries."""
    from unittest import mock

    with mock.patch("runtime_utils.Path") as mock_path_cls:
        fake_proc = mock.MagicMock()
        fake_proc.iterdir.return_value = []
        mock_path_cls.return_value = fake_proc
        assert ru_mod.ark_server_running() is False


# ---------------------------------------------------------------------------
# run_rcon
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("reply", "expected"),
    [
        ("Server received, But no response!! ", "[rcon] saveworld: accepted by the server (no output)\n"),
        ("", "[rcon] saveworld: accepted by the server (no output)\n"),
        ("Players: 0\n", "[rcon] saveworld: Players: 0\n"),
    ],
)
def test_run_rcon_logs_each_reply_once(
    ru_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], reply: str, expected: str
) -> None:
    monkeypatch.setattr(ru_mod, "rcon_command", lambda *_a, **_k: reply)
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)

    assert ru_mod.run_rcon("saveworld") is True

    captured = capsys.readouterr()
    assert captured.out == expected
    assert captured.err == ""


def test_run_rcon_reports_rcon_error(
    ru_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    def fail(*_a, **_k):
        raise ru_mod.RconError("RCON command timed out after 10 seconds (localhost:27020).")

    monkeypatch.setattr(ru_mod, "rcon_command", fail)
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)

    assert ru_mod.run_rcon("DoExit") is False

    captured = capsys.readouterr()
    assert captured.out == ""
    assert captured.err == "[rcon] Error: DoExit failed: RCON command timed out after 10 seconds (localhost:27020).\n"


def test_ark_server_pids_excludes_the_calling_process(ru_mod, tmp_path: Path) -> None:
    """A process whose own command line mentions ArkAscendedServer.exe is not reported as the server."""
    import os
    from unittest import mock

    entries = _fake_proc_entries(
        tmp_path,
        {str(os.getpid()): b"python3 kill ArkAscendedServer.exe", "4321": b"wine ArkAscendedServer.exe"},
    )
    with mock.patch("runtime_utils.Path") as mock_path_cls:
        fake_proc = mock.MagicMock()
        fake_proc.iterdir.return_value = entries
        mock_path_cls.return_value = fake_proc
        assert ru_mod.ark_server_pids() == [4321]


# ---------------------------------------------------------------------------
# read_log_tail / server_log_closed
# ---------------------------------------------------------------------------


def test_read_log_tail_returns_only_the_last_bytes(ru_mod, tmp_path: Path) -> None:
    log = tmp_path / "ShooterGame.log"
    log.write_bytes(b"x" * 200 + b"TAIL")
    assert ru_mod.read_log_tail(log, 10) == "xxxxxxTAIL"
    assert ru_mod.read_log_tail(log, 1000) == "x" * 200 + "TAIL"


def test_read_log_tail_returns_none_for_missing_file(ru_mod, tmp_path: Path) -> None:
    assert ru_mod.read_log_tail(tmp_path / "missing.log", 10) is None


@pytest.mark.parametrize(
    ("content", "closed"),
    [
        (None, False),
        ("startup\nLog file closed, 11/15/26 10:00:00\n", True),
        ("Server startup complete\n", False),
        ("Log file closed, old-run\n" + "noise\n" * 60, False),  # only the last 50 lines count
        (("x" * 100 + "\n") * 1000 + "Log file closed, 11/15/26\n", True),  # large log: tail only
    ],
    ids=["missing", "closed", "absent", "too-early", "large-log"],
)
def test_server_log_closed(ru_mod, tmp_path: Path, content: str | None, closed: bool) -> None:
    log = tmp_path / "ShooterGame.log"
    if content is not None:
        log.write_text(content, encoding="utf-8")
    assert ru_mod.server_log_closed(log) is closed
