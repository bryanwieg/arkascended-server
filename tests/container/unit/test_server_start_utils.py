from __future__ import annotations

import importlib.util
import os
import re
import types
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


# start_mod fixture is provided by conftest.py


# ---------------------------------------------------------------------------
# invalid settings
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(("name", "value"), [("SHUTDOWN_WARNING_MINUTES", "-1"), ("FORCE_TERM_WAIT_SECONDS", "x")])
def test_main_returns_2_for_invalid_setting(
    start_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], name: str, value: str
) -> None:
    """main() reports a bad integer setting and exits 2 before launching anything."""
    monkeypatch.setenv(name, value)
    monkeypatch.setattr(start_mod.subprocess, "Popen", lambda *_a, **_k: pytest.fail("must not launch"))
    assert start_mod.main() == 2
    assert f"Error: {name} must be an integer of at least" in capsys.readouterr().err


# ---------------------------------------------------------------------------
# prune_rotated_logs
# ---------------------------------------------------------------------------


def test_prune_rotated_logs_deletes_oldest_beyond_keep(start_mod, tmp_path: Path) -> None:
    """prune_rotated_logs removes the oldest rotated files beyond keep_files, keeping the newest."""
    log_path = tmp_path / "ShooterGame.log"
    files = [
        tmp_path / "ShooterGame.log.20240101-000000",
        tmp_path / "ShooterGame.log.20240102-000000",
        tmp_path / "ShooterGame.log.20240103-000000",
        tmp_path / "ShooterGame.log.20240104-000000",
    ]
    for idx, f in enumerate(files, start=1):
        f.write_text(f"log-{idx}\n", encoding="utf-8")
        os.utime(f, (idx, idx))  # ascending mtime

    start_mod.prune_rotated_logs(log_path, keep_files=2)

    remaining = {f.name for f in tmp_path.iterdir() if f.is_file()}
    assert remaining == {
        "ShooterGame.log.20240103-000000",
        "ShooterGame.log.20240104-000000",
    }


def test_prune_rotated_logs_keeps_all_when_under_limit(start_mod, tmp_path: Path) -> None:
    """prune_rotated_logs leaves all files untouched when count is within keep_files."""
    log_path = tmp_path / "ShooterGame.log"
    for i in range(3):
        f = tmp_path / f"ShooterGame.log.2024010{i}-000000"
        f.write_text("x\n", encoding="utf-8")

    start_mod.prune_rotated_logs(log_path, keep_files=5)

    assert len(list(tmp_path.iterdir())) == 3


def test_prune_rotated_logs_is_noop_when_no_rotated_files_exist(start_mod, tmp_path: Path) -> None:
    """prune_rotated_logs does nothing when there are no rotated candidates."""
    log_path = tmp_path / "ShooterGame.log"
    start_mod.prune_rotated_logs(log_path, keep_files=2)
    assert list(tmp_path.iterdir()) == []


# ---------------------------------------------------------------------------
# parse_status_file
# ---------------------------------------------------------------------------


def test_parse_status_file_returns_all_unknown_when_missing(start_mod, tmp_path: Path) -> None:
    """parse_status_file returns (False, unknown, unknown, unknown) when the file doesn't exist."""
    result = start_mod.parse_status_file(tmp_path / "nonexistent.env")
    assert result == (False, "unknown", "unknown", "unknown")


def test_parse_status_file_reads_all_keys(start_mod, tmp_path: Path) -> None:
    """parse_status_file parses SAVE_OK, SHUTDOWN_OK, STOP_OK correctly."""
    status_file = tmp_path / "ark-stop-status.env"
    status_file.write_text(
        "SAVE_OK=true\nSHUTDOWN_OK=true\nSTOP_OK=true\n",
        encoding="utf-8",
    )
    present, save_ok, shutdown_ok, stop_ok = start_mod.parse_status_file(status_file)
    assert present is True
    assert save_ok == "true"
    assert shutdown_ok == "true"
    assert stop_ok == "true"


def test_parse_status_file_reads_false_values(start_mod, tmp_path: Path) -> None:
    """parse_status_file returns 'false' string values correctly."""
    status_file = tmp_path / "ark-stop-status.env"
    status_file.write_text(
        "SAVE_OK=false\nSHUTDOWN_OK=false\nSTOP_OK=false\n",
        encoding="utf-8",
    )
    present, save_ok, shutdown_ok, stop_ok = start_mod.parse_status_file(status_file)
    assert present is True
    assert save_ok == "false"
    assert shutdown_ok == "false"
    assert stop_ok == "false"


def test_parse_status_file_returns_unknown_for_missing_keys(start_mod, tmp_path: Path) -> None:
    """parse_status_file returns 'unknown' for any keys absent from the file."""
    status_file = tmp_path / "ark-stop-status.env"
    status_file.write_text("SAVE_OK=true\n", encoding="utf-8")
    present, save_ok, shutdown_ok, stop_ok = start_mod.parse_status_file(status_file)
    assert present is True
    assert save_ok == "true"
    assert shutdown_ok == "unknown"
    assert stop_ok == "unknown"


def test_parse_status_file_ignores_blank_and_keyless_lines(start_mod, tmp_path: Path) -> None:
    """parse_status_file skips blank lines and lines without '='."""
    status_file = tmp_path / "ark-stop-status.env"
    status_file.write_text(
        "\n# comment\nSAVE_OK=true\n\nSTOP_OK=false\n",
        encoding="utf-8",
    )
    present, save_ok, _, stop_ok = start_mod.parse_status_file(status_file)
    assert present is True
    assert save_ok == "true"
    assert stop_ok == "false"


# ---------------------------------------------------------------------------
# _report_exit
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("server_status", "errors", "expected"),
    [
        (5, [], 0),  # confirmed stop: ARK's unreliable positive exit code is ignored
        (2, [], 0),
        (5, ["save was not confirmed"], 5),  # unconfirmed save still fails
        (-15, [], -15),  # killed by a signal still fails
    ],
)
def test_report_exit_ignores_game_code_only_after_confirmed_stop(
    start_mod, server_status: int, errors: list[str], expected: int
) -> None:
    assert start_mod._report_exit(server_status, True, errors, clean_stop=True) == expected


# ---------------------------------------------------------------------------
# run_graceful_stop_script
# ---------------------------------------------------------------------------


class _FakeProc:
    """Popen stand-in whose poll() returns None until `exits_after` polls, then `code`."""

    def __init__(self, code: int, exits_after: int) -> None:
        self.code = code
        self.remaining = exits_after
        self.killed = False

    def poll(self) -> int | None:
        if self.killed:
            return -9
        if self.remaining > 0:
            self.remaining -= 1
            return None
        return self.code

    def kill(self) -> None:
        self.killed = True

    def wait(self) -> int:
        return -9


def test_graceful_stop_logs_server_exit_while_script_runs(
    start_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setattr(start_mod.subprocess, "Popen", lambda _args: _FakeProc(0, exits_after=3))
    server = _FakeProc(5, exits_after=1)

    assert start_mod.run_graceful_stop_script(60, server, poll_seconds=0) is True
    out = capsys.readouterr().out
    assert out.count("[shutdown] server process exited with code 5 while the graceful stop script was running") == 1


def test_graceful_stop_kills_script_on_timeout(start_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    script = _FakeProc(0, exits_after=10**6)
    monkeypatch.setattr(start_mod.subprocess, "Popen", lambda _args: script)

    assert start_mod.run_graceful_stop_script(0, _FakeProc(0, exits_after=10**6), poll_seconds=0) is False
    assert script.killed


def test_stop_script_timeout_includes_shutdown_warning(start_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    """The player warning sleep runs inside the stop script, so it extends the stop-script timeout."""
    monkeypatch.setenv("GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS", "600")
    monkeypatch.setenv("SHUTDOWN_WARNING_MINUTES", "10")
    assert start_mod._load_server_config()["graceful_stop_script_timeout"] == 1200
    monkeypatch.delenv("SHUTDOWN_WARNING_MINUTES")
    assert start_mod._load_server_config()["graceful_stop_script_timeout"] == 900  # default 5 minutes
    monkeypatch.delenv("GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS")
    assert start_mod._load_server_config()["graceful_stop_script_timeout"] == 420  # 120 + 5 * 60


_STOP_ENV = (
    "SHUTDOWN_WARNING_MINUTES",
    "SAVE_MAX_RETRIES",
    "SAVE_CONFIRM_TIMEOUT_SECONDS",
    "STOP_CHECK_TIMEOUT_SECONDS",
    "GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS",
    "GRACEFUL_STOP_WAIT_SECONDS",
    "FORCE_TERM_WAIT_SECONDS",
)


def test_default_stop_budget_fits_compose_grace_period(start_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    """The default worst-case stop fits the default compose stop_grace_period, or Docker kills mid-save."""
    for name in _STOP_ENV:
        monkeypatch.delenv(name, raising=False)
    repo = Path(__file__).resolve().parents[3]
    defaults_yaml = (repo / "app/ark-cli/internal/compose/assets/docker-compose.defaults.yaml").read_text()
    match = re.search(r"stop_grace_period:\s*(\d+)m", defaults_yaml)
    assert match, "stop_grace_period not found in docker-compose.defaults.yaml"
    grace_seconds = int(match.group(1)) * 60

    server = start_mod._load_server_config()
    worst_case = server["graceful_stop_script_timeout"] + server["graceful_stop_wait"] + server["force_term_wait"]
    assert worst_case <= grace_seconds

    stop_path = repo / "container/scripts/runtime/ark-server-stop.py"
    spec = importlib.util.spec_from_file_location("ark_server_stop", stop_path)
    assert spec is not None and spec.loader is not None
    stop_mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(stop_mod)
    stop = stop_mod._load_stop_config()
    script_timeout = server["graceful_stop_script_timeout"] - stop["warning_minutes"] * 60
    assert stop["save_max_retries"] * stop["save_confirm_timeout"] + stop["stop_check_timeout"] <= script_timeout


# ---------------------------------------------------------------------------
# OOM kill reporting
# ---------------------------------------------------------------------------


def test_read_oom_kill_count_parses_v2_then_falls_back_to_v1(
    start_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """read_oom_kill_count reads cgroup v2 memory.events, falls back to v1, and returns None when neither exists."""
    v2 = tmp_path / "memory.events"
    v1 = tmp_path / "memory.oom_control"
    monkeypatch.setattr(start_mod, "OOM_COUNTER_FILES", (v2, v1))
    assert start_mod.read_oom_kill_count() is None
    v1.write_text("oom_kill_disable 0\nunder_oom 0\noom_kill 2\n")
    assert start_mod.read_oom_kill_count() == 2
    v2.write_text("low 0\nhigh 0\nmax 3\noom 1\noom_kill 5\noom_group_kill 0\n")
    assert start_mod.read_oom_kill_count() == 5


@pytest.mark.parametrize(
    ("before", "after", "reported"), [(0, 1, True), (1, 1, False), (None, 1, False), (0, None, False)]
)
def test_report_oom_kill_only_when_counter_rose(
    start_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], before, after, reported
) -> None:
    """report_oom_kill emits an error only when both reads succeed and the counter went up."""
    monkeypatch.setattr(start_mod, "read_oom_kill_count", lambda: after)
    start_mod.report_oom_kill(before)
    assert ("OOM killer" in capsys.readouterr().err) is reported


# ---------------------------------------------------------------------------
# main(): recording GameUserSettings.ini as ARK wrote it
# ---------------------------------------------------------------------------


def test_main_commits_the_servers_ini_changes(start_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """After the server exits, main() commits the INI changes made during the run as ARK's."""
    import ini_history

    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nA=1\n")
    ini_history.commit(tmp_path, ini_history.ENV, "RCON and password settings from the environment")

    def launch(*_a, **_k):
        gus.write_text("[ServerSettings]\nA=2\n")  # the server's own write
        return object()

    class _Ctrl:
        def __init__(self, *_a) -> None:
            pass

        def setup_signals(self) -> None:
            pass

    monkeypatch.setattr(start_mod, "GAME_USER_SETTINGS", gus)
    monkeypatch.setattr(start_mod, "server_command", lambda *_a: ["server"])
    monkeypatch.setattr(start_mod, "rotate_runtime_logs", lambda *_a: None)
    monkeypatch.setattr(start_mod, "_clear_stale_stop_file", lambda *_a: None)
    monkeypatch.setattr(start_mod, "read_oom_kill_count", lambda: 0)
    monkeypatch.setattr(start_mod, "report_oom_kill", lambda *_a: None)
    monkeypatch.setattr(start_mod, "_ShutdownController", _Ctrl)
    monkeypatch.setattr(start_mod, "_run_server_loop", lambda *_a: 0)
    monkeypatch.setattr(start_mod, "_print_crash_log", lambda *_a: None)
    monkeypatch.setattr(start_mod, "_report_exit_status", lambda *_a: 0)
    # Only the start script's subprocess: git (ini_history) still needs the real one.
    monkeypatch.setattr(start_mod, "subprocess", types.SimpleNamespace(Popen=launch))

    assert start_mod.main() == 0
    assert ini_history._git(tmp_path, "log", "--format=%an: %s").splitlines() == [
        "ARK server: written by the server during its run (exit code 0)",
        "instance env: RCON and password settings from the environment",
    ]
    assert ini_history._git(tmp_path, "show", "HEAD:GameUserSettings.ini") == "[ServerSettings]\nA=2\n"


def test_record_server_ini_write_failure_only_warns(
    start_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys
) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nA=1\n")
    monkeypatch.setattr(start_mod, "GAME_USER_SETTINGS", gus)
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)
    monkeypatch.setenv("PATH", str(tmp_path / "no-bin"))  # no git
    start_mod.record_server_ini_write(0)
    assert "[server] Warning: could not record the server's INI changes: git" in capsys.readouterr().err
