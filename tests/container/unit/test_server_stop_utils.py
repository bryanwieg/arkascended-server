from __future__ import annotations

import importlib.util
import os
import time
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_stop_mod():
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/ark-server-stop.py"
    spec = importlib.util.spec_from_file_location("ark_server_stop", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load module from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(name="stop_mod")
def stop_mod_fixture():
    return _load_stop_mod()


# ---------------------------------------------------------------------------
# invalid settings
# ---------------------------------------------------------------------------


def test_main_returns_2_for_invalid_setting(
    stop_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    """main() reports a bad integer setting and exits 2 before sending any RCON command."""
    monkeypatch.setenv("SAVE_MAX_RETRIES", "0")
    monkeypatch.setattr(stop_mod, "run_rcon", lambda *_a, **_k: pytest.fail("must not send RCON"))
    assert stop_mod.main() == 2
    assert "Error: SAVE_MAX_RETRIES must be an integer of at least 1 (got '0')." in capsys.readouterr().err


# ---------------------------------------------------------------------------
# write_stop_status
# ---------------------------------------------------------------------------


def test_write_stop_status_writes_all_true(stop_mod, tmp_path: Path) -> None:
    """write_stop_status writes SAVE_OK=true, SHUTDOWN_OK=true, STOP_OK=true."""
    status_file = tmp_path / "logs" / "ark-stop-status.env"
    stop_mod.write_stop_status(status_file, save_ok=True, shutdown_ok=True, stop_ok=True)

    content = status_file.read_text(encoding="utf-8")
    assert "SAVE_OK=true" in content
    assert "SHUTDOWN_OK=true" in content
    assert "STOP_OK=true" in content


def test_write_stop_status_writes_all_false(stop_mod, tmp_path: Path) -> None:
    """write_stop_status writes SAVE_OK=false, SHUTDOWN_OK=false, STOP_OK=false."""
    status_file = tmp_path / "logs" / "ark-stop-status.env"
    stop_mod.write_stop_status(status_file, save_ok=False, shutdown_ok=False, stop_ok=False)

    content = status_file.read_text(encoding="utf-8")
    assert "SAVE_OK=false" in content
    assert "SHUTDOWN_OK=false" in content
    assert "STOP_OK=false" in content


def test_write_stop_status_creates_parent_directories(stop_mod, tmp_path: Path) -> None:
    """write_stop_status creates any missing parent directories before writing."""
    deep_path = tmp_path / "a" / "b" / "c" / "status.env"
    stop_mod.write_stop_status(deep_path, save_ok=True, shutdown_ok=True, stop_ok=True)
    assert deep_path.is_file()


def test_write_stop_status_mixed_values(stop_mod, tmp_path: Path) -> None:
    """write_stop_status encodes each flag independently."""
    status_file = tmp_path / "ark-stop-status.env"
    stop_mod.write_stop_status(status_file, save_ok=True, shutdown_ok=False, stop_ok=False)

    content = status_file.read_text(encoding="utf-8")
    assert "SAVE_OK=true" in content
    assert "SHUTDOWN_OK=false" in content
    assert "STOP_OK=false" in content


# ---------------------------------------------------------------------------
# _save_with_retries — save confirmation
# ---------------------------------------------------------------------------

_MARKER = "[2026.11.13-10.00.00:000][  0]2026.11.13_10.00.00: World Save Complete\n"


def test_save_not_confirmed_by_marker_written_before_request(stop_mod, tmp_path: Path, monkeypatch) -> None:
    """An earlier autosave's marker at the end of the log must not confirm a save that never completed."""
    log = tmp_path / "ShooterGame.log"
    log.write_text("startup\n" + _MARKER + "idle\n")
    monkeypatch.setattr(stop_mod, "save_world", lambda: True)
    monkeypatch.setattr(stop_mod.time, "sleep", lambda _s: None)
    assert stop_mod._save_with_retries(log, max_retries=2, retry_delay=0, confirm_timeout=1) is False


def test_save_confirmed_by_marker_written_after_request(stop_mod, tmp_path: Path, monkeypatch) -> None:
    """A marker appended after saveworld is sent confirms the save on the first attempt."""
    log = tmp_path / "ShooterGame.log"
    log.write_text("startup\n")
    calls: list[int] = []

    def fake_save_world() -> bool:
        calls.append(1)
        with log.open("a") as handle:
            handle.write("Saving world...\n" + _MARKER)
        return True

    monkeypatch.setattr(stop_mod, "save_world", fake_save_world)
    assert stop_mod._save_with_retries(log, max_retries=3, retry_delay=0, confirm_timeout=5) is True
    assert calls == [1], "saveworld must not be re-sent once the save is confirmed"


# ---------------------------------------------------------------------------
# player warning countdown
# ---------------------------------------------------------------------------


def _record_countdown(stop_mod, monkeypatch: pytest.MonkeyPatch, list_players) -> tuple[list[str], list[int]]:
    """Patch RCON and sleep; return the RCON commands sent and the sleeps taken."""
    sent: list[str] = []
    slept: list[int] = []
    monkeypatch.setattr(stop_mod, "rcon_command", list_players)
    monkeypatch.setattr(stop_mod, "run_rcon", lambda cmd, **_k: sent.append(cmd) or True)
    monkeypatch.setattr(stop_mod.time, "sleep", slept.append)
    return sent, slept


@pytest.mark.parametrize(
    ("minutes", "want"),
    [(10, [600, 300, 60, 30]), (15, [900, 600, 300, 60, 30]), (3, [180, 60, 30]), (1, [60, 30])],
)
def test_countdown_marks(stop_mod, minutes: int, want: list[int]) -> None:
    assert stop_mod.countdown_marks(minutes * 60) == want


def test_countdown_warns_at_each_mark_naming_the_reason(stop_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    sent, slept = _record_countdown(stop_mod, monkeypatch, lambda _cmd, **_k: "0. Alice, 0002abc")
    stop_mod._send_warning_and_wait(10, "updating to 98.9")
    chats = [c.removeprefix("ServerChat ") for c in sent if c.startswith("ServerChat ")]
    assert chats == [
        f"Server shutting down in {when} (updating to 98.9). The world is saved before exit."
        for when in ("10 minutes", "5 minutes", "1 minute", "30 seconds")
    ]
    assert [c.removeprefix("Broadcast ") for c in sent if c.startswith("Broadcast ")] == chats
    assert slept == [300, 240, 30, 30]  # the full 10 minutes


def test_countdown_skipped_when_no_players(stop_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    sent, slept = _record_countdown(stop_mod, monkeypatch, lambda _cmd, **_k: "No Players Connected")
    stop_mod._send_warning_and_wait(10, "")
    assert sent == []
    assert slept == []


def test_countdown_runs_when_players_cannot_be_listed(stop_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    def rcon_down(_cmd, **_k):
        raise stop_mod.RconError("connection refused")

    sent, slept = _record_countdown(stop_mod, monkeypatch, rcon_down)
    stop_mod._send_warning_and_wait(1, "")
    assert "ServerChat Server shutting down in 1 minute. The world is saved before exit." in sent
    assert slept == [30, 30]


def test_no_countdown_without_warning_minutes(stop_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    sent, slept = _record_countdown(stop_mod, monkeypatch, lambda *_a, **_k: pytest.fail("must not list players"))
    stop_mod._send_warning_and_wait(0, "updating to 98.9")
    assert sent == []
    assert slept == []


def test_read_stop_reason_consumes_the_file(stop_mod, tmp_path: Path) -> None:
    reason_file = tmp_path / "ark-stop-reason.txt"
    reason_file.write_text("updating to 98.9\n", encoding="utf-8")
    assert stop_mod.read_stop_reason(reason_file) == "updating to 98.9"
    assert not reason_file.exists()
    assert stop_mod.read_stop_reason(reason_file) == ""


def test_read_stop_reason_ignores_a_stale_reason(stop_mod, tmp_path: Path) -> None:
    reason_file = tmp_path / "ark-stop-reason.txt"
    reason_file.write_text("updating to 98.6\n", encoding="utf-8")
    old = time.time() - stop_mod.REASON_MAX_AGE_SECONDS - 60
    os.utime(reason_file, (old, old))
    assert stop_mod.read_stop_reason(reason_file) == ""
    assert not reason_file.exists()


def test_stop_reason_script_writes_the_reason(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/ark-stop-reason.py"
    spec = importlib.util.spec_from_file_location("ark_stop_reason", module_path)
    assert spec is not None
    assert spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    monkeypatch.setattr(mod, "STOP_REASON_FILE", tmp_path / "Logs/ark-stop-reason.txt")
    assert mod.main(["updating", "to", "98.9"]) == 0
    assert (tmp_path / "Logs/ark-stop-reason.txt").read_text(encoding="utf-8") == "updating to 98.9\n"
    assert mod.main([]) == 2
