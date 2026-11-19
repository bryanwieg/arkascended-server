#!/usr/bin/env python3.14
"""Start ARK server under Proton, stream logs, and report shutdown outcomes."""

from __future__ import annotations

import contextlib
import os
import signal
import subprocess
import sys
import time
from datetime import UTC, datetime
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import ini_history
from _logging import emit_error, emit_event, emit_warning
from runtime_utils import (
    CRASH_LOG,
    GAME_USER_SETTINGS,
    PYTHON,
    SCRIPTS_DIR,
    SERVER_BINARIES_DIR,
    SERVER_LOG,
    STOP_STATUS_FILE,
    ConfigError,
    env_int,
    server_command,
    server_log_closed,
)

# -- Log rotation helpers --


def rotated_log_candidates(path: Path) -> list[Path]:
    """Return all existing rotated log files for the given log path."""
    prefix = f"{path.name}."
    if not path.parent.is_dir():
        return []
    return [entry for entry in path.parent.iterdir() if entry.is_file() and entry.name.startswith(prefix)]


def prune_rotated_logs(path: Path, keep_files: int) -> None:
    """Delete old rotated log archives beyond the keep_files count, newest first."""
    files = sorted(rotated_log_candidates(path), key=lambda entry: entry.stat().st_mtime, reverse=True)
    for old_file in files[keep_files:]:
        try:
            old_file.unlink()
            emit_event("log-rotation", f"removed old archive {old_file}.")
        except OSError as exc:
            emit_warning("log-rotation", f"failed to remove old log archive {old_file}: {exc}.")


def rotate_log_file(path: Path, keep_files: int) -> None:
    """Rename the current log file to a timestamped archive and prune old archives beyond keep_files."""
    path.parent.mkdir(parents=True, exist_ok=True)

    if path.is_file():
        stamp = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
        rotated = path.with_name(f"{path.name}.{stamp}")
        suffix = 1
        while rotated.exists():
            rotated = path.with_name(f"{path.name}.{stamp}.{suffix}")
            suffix += 1
        try:
            path.rename(rotated)
            emit_event("log-rotation", f"archived previous log {path} -> {rotated}.")
        except OSError as exc:
            emit_warning("log-rotation", f"failed to rotate log {path}: {exc}.")

    prune_rotated_logs(path, keep_files)


def rotate_runtime_logs(server_log: Path, crash_log: Path, keep_files: int) -> None:
    """Rotate both the server log and crash log before a new server run."""
    rotate_log_file(server_log, keep_files)
    rotate_log_file(crash_log, keep_files)


# -- Status file and log parsing --


def parse_status_file(path: Path) -> tuple[bool, str, str, str]:
    """Parse the stop status env file; return (present, save_ok, shutdown_ok, stop_ok)."""
    if not path.is_file():
        return False, "unknown", "unknown", "unknown"

    values: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = raw.strip()
        if not line or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return (
        True,
        values.get("SAVE_OK", "unknown"),
        values.get("SHUTDOWN_OK", "unknown"),
        values.get("STOP_OK", "unknown"),
    )


# -- Graceful stop and process control --


def run_graceful_stop_script(
    timeout_seconds: int,
    server_proc: subprocess.Popen[bytes],
    poll_seconds: float = 0.5,
) -> bool:
    """Execute the graceful stop script with a timeout; return True if it exited 0 in time.

    The server process is watched while the script runs so its exit is logged
    when it happens, not after the script's post-stop wait.
    """
    args = [PYTHON, str(SCRIPTS_DIR / "ark-server-stop.py")]
    emit_event("shutdown", f"executing graceful stop script (timeout={timeout_seconds}s).")
    stop_proc = subprocess.Popen(args)
    deadline = time.monotonic() + timeout_seconds
    server_exit_logged = False
    while (returncode := stop_proc.poll()) is None:
        if not server_exit_logged and (server_status := server_proc.poll()) is not None:
            emit_event(
                "shutdown",
                f"server process exited with code {server_status} while the graceful stop script was running.",
            )
            server_exit_logged = True
        if time.monotonic() >= deadline:
            stop_proc.kill()
            stop_proc.wait()
            emit_warning("shutdown", "graceful stop script timed out; proceeding to forced stop path.")
            return False
        time.sleep(poll_seconds)
    if returncode != 0:
        emit_warning("shutdown", f"graceful stop script exited with status {returncode}; continuing shutdown workflow.")
        return False
    emit_event("shutdown", "graceful stop script completed successfully.")
    return True


def kill_server_process_group(server_proc: subprocess.Popen[bytes], signum: int) -> None:
    """Send the given signal to the server's process group; a group that is already gone is fine."""
    with contextlib.suppress(ProcessLookupError):
        os.killpg(os.getpgid(server_proc.pid), signum)


def wait_for_exit(server_proc: subprocess.Popen[bytes], timeout_seconds: int, poll_seconds: int) -> bool:
    """Poll until the server process exits or timeout expires; return True if it exited."""
    elapsed = 0
    while elapsed < timeout_seconds:
        if server_proc.poll() is not None:
            return True
        time.sleep(poll_seconds)
        elapsed += poll_seconds
    return server_proc.poll() is not None


# -- Shutdown controller --


class _ShutdownController:
    """Tracks received signals and drives the ordered shutdown workflow."""

    def __init__(self, server_proc: subprocess.Popen[bytes], cfg: dict) -> None:
        self._proc = server_proc
        self._cfg = cfg
        self.received_signal: int | None = None
        self.started = False

    def handle(self, signum: int, _frame: object) -> None:
        """Signal handler: record first signal; log additional ones."""
        if self.received_signal is None:
            self.received_signal = signum
            emit_event("shutdown", f"received signal {signum}; beginning server shutdown workflow.")
        else:
            emit_event("shutdown", f"additional signal {signum} received while shutdown is in progress.")

    def setup_signals(self) -> None:
        """Register SIGTERM and SIGINT handlers to trigger the shutdown workflow."""
        signal.signal(signal.SIGTERM, self.handle)
        signal.signal(signal.SIGINT, self.handle)

    def perform(self) -> None:
        """Run graceful stop, then escalate to SIGTERM/SIGKILL if needed."""
        self.started = True
        graceful_ok = run_graceful_stop_script(self._cfg["graceful_stop_script_timeout"], self._proc)
        if graceful_ok and wait_for_exit(self._proc, self._cfg["graceful_stop_wait"], self._cfg["force_poll_interval"]):
            return
        msg = (
            "graceful stop did not end server process within configured timeout; "
            "switching to forced process-group shutdown."
            if graceful_ok
            else "graceful stop did not succeed; forcing process-group shutdown now."
        )
        emit_warning("shutdown", msg)
        kill_server_process_group(self._proc, signal.SIGTERM)
        if wait_for_exit(self._proc, self._cfg["force_term_wait"], self._cfg["force_poll_interval"]):
            return
        emit_warning("shutdown", "server process group did not exit after SIGTERM; sending SIGKILL.")
        kill_server_process_group(self._proc, signal.SIGKILL)


# -- Server process loop --


def _wait_for_server_exit(server_proc: subprocess.Popen[bytes], ctrl: _ShutdownController, poll_interval: int) -> int:
    """Poll until the server process exits, triggering shutdown on first signal."""
    while True:
        status = server_proc.poll()
        if status is not None:
            return status
        if ctrl.received_signal is not None and not ctrl.started:
            ctrl.perform()
        time.sleep(poll_interval)


def _cleanup_tail(tail_proc: subprocess.Popen[bytes] | None) -> None:
    """Terminate the tail log-streaming process if it is still running."""
    if tail_proc is None or tail_proc.poll() is not None:
        return
    tail_proc.terminate()
    try:
        tail_proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        tail_proc.kill()
        tail_proc.wait()


def _run_server_loop(
    server_proc: subprocess.Popen[bytes],
    server_log: Path,
    ctrl: _ShutdownController,
    poll_interval: int,
) -> int:
    """Wait for the log file to appear, start tail streaming, then wait for process exit."""
    tail_proc: subprocess.Popen[bytes] | None = None
    try:
        while not server_log.is_file():
            if server_proc.poll() is not None:
                break
            if ctrl.received_signal is not None and not ctrl.started:
                ctrl.perform()
            time.sleep(1)
        if server_log.is_file():
            tail_proc = subprocess.Popen(["tail", "-n", "+1", "-F", str(server_log)])
        return _wait_for_server_exit(server_proc, ctrl, poll_interval)
    finally:
        _cleanup_tail(tail_proc)


# -- Exit status reporting --

# Per the kernel's cgroup v2 docs, oom_kill counts kills by any OOM killer, host-wide ones included.
OOM_COUNTER_FILES = (
    Path("/sys/fs/cgroup/memory.events"),  # cgroup v2
    Path("/sys/fs/cgroup/memory/memory.oom_control"),  # cgroup v1
)


def read_oom_kill_count() -> int | None:
    """Return the container cgroup's oom_kill counter, or None if no counter is readable."""
    for path in OOM_COUNTER_FILES:
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            key, _, value = line.partition(" ")
            if key == "oom_kill" and value.strip().isdigit():
                return int(value)
    return None


def report_oom_kill(oom_before: int | None) -> None:
    """Say so when the OOM killer struck during the run; ARK logs nothing when it is killed."""
    oom_after = read_oom_kill_count()
    if oom_before is None or oom_after is None or oom_after <= oom_before:
        return
    emit_error(
        "server-oom",
        f"the kernel OOM killer killed {oom_after - oom_before} process(es) in this container "
        "while the ARK server was running: the host ran out of memory. "
        "Free host RAM before restarting the server.",
    )


def _collect_stop_errors(save_ok: str, shutdown_ok: str, stop_ok: str) -> list[str]:
    """Return a list of human-readable error strings for any stop outcome that was not 'true'."""
    errors = []
    if save_ok != "true":
        errors.append("save was not confirmed")
    if shutdown_ok != "true":
        errors.append("shutdown command was not confirmed")
    if stop_ok != "true":
        errors.append("stop was not confirmed")
    return errors


def _report_exit(server_status: int, status_file_present: bool, errors: list[str], clean_stop: bool) -> int:  # noqa: PLR0911
    """Evaluate and report the shutdown outcome; return the final exit code."""
    if server_status == 0:
        if status_file_present:
            if not errors:
                emit_event("server", "Server exited cleanly (exit code 0; save, shutdown, and stop confirmed).")
                return 0
            emit_error("server", f"Ark Server exited 0, but another error was encountered: {' '.join(errors)}.")
            return 1
        if clean_stop:
            emit_event("server", "Server exited cleanly (exit code 0, stop confirmed).")
            return 0
        emit_warning("server", "server exited with code 0 but clean stop was not confirmed.")
        return 0
    if status_file_present:
        if not errors and server_status > 0:
            # ARK intermittently exits 2 or 5 after a confirmed DoExit; Proton passes the game's
            # code through unchanged, so it says nothing about the save. Signals (<0) still fail.
            emit_event(
                "server",
                f"Server exited cleanly (save, shutdown, and stop confirmed; ignoring game exit code {server_status}).",
            )
            return 0
        if not errors:
            emit_warning("server", f"server exited with code {server_status}; save, shutdown, and stop were confirmed.")
        else:
            emit_warning("server", f"server exited with code {server_status}; additional errors: {' '.join(errors)}.")
        return server_status
    if clean_stop:
        emit_warning("server", f"server exited with code {server_status} but stop was confirmed; check logs.")
    else:
        emit_warning("server", f"server exited with code {server_status} without clean stop confirmation.")
    return server_status


def _report_exit_status(server_status: int, stop_status_file: Path, server_log: Path) -> int:
    """Parse the stop status file and print a summary of the shutdown outcome."""
    status_file_present, save_ok, shutdown_ok, stop_ok = parse_status_file(stop_status_file)
    clean_stop = server_log_closed(server_log)
    errors = _collect_stop_errors(save_ok, shutdown_ok, stop_ok)
    return _report_exit(server_status, status_file_present, errors, clean_stop)


# -- Config loading and validation --


def _load_server_config() -> dict:
    """Read all runtime settings from environment variables."""
    return {
        # The stop script sleeps through the player warning before saving, so the warning must not eat its budget.
        "graceful_stop_script_timeout": env_int("GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS", 600)
        + env_int("SHUTDOWN_WARNING_MINUTES", 5, minimum=0) * 60,
        "graceful_stop_wait": env_int("GRACEFUL_STOP_WAIT_SECONDS", 180),
        "force_term_wait": env_int("FORCE_TERM_WAIT_SECONDS", 20),
        "force_poll_interval": env_int("FORCE_STOP_POLL_SECONDS", 1),
        "log_rotation_keep_files": env_int("LOG_ROTATION_KEEP_FILES", 10, minimum=0),
    }


def _clear_stale_stop_file(stop_status_file: Path) -> None:
    """Remove any stop status file left over from a prior run."""
    if not stop_status_file.is_file():
        return
    try:
        stop_status_file.unlink()
    except OSError:
        emit_warning("server", f"failed to remove stale stop status file at {stop_status_file}.")


def _print_crash_log(crash_log: Path, server_status: int) -> None:
    """Print the crash log contents to stdout if the file exists."""
    if not crash_log.is_file():
        return
    emit_event("server", f"Crash log detected at {crash_log} (exit code: {server_status}):")
    crash_text = crash_log.read_text(encoding="utf-8", errors="ignore")
    sys.stdout.write(crash_text)
    if not crash_text.endswith("\n"):
        sys.stdout.write("\n")


def record_server_ini_write(server_status: int) -> None:
    """Commit the INI files as ARK's if they changed during the run (ini_history).

    The entrypoint committed them just before the server started, and ark-cli refuses to write
    them while it runs, so whatever changed since is the server's own write (normally at shutdown).
    """
    try:
        changed = ini_history.commit(
            GAME_USER_SETTINGS.parent,
            ini_history.ARK,
            f"written by the server during its run (exit code {server_status})",
        )
    except (ini_history.HistoryError, OSError) as exc:
        emit_warning("server", f"could not record the server's INI changes: {exc}")
        return
    if changed:
        emit_event("server", "Recorded the server's changes to its INI files in their history.")


def main() -> int:
    """Entry point: configure, launch, and monitor the ARK server process; return an exit code."""
    try:
        cfg = _load_server_config()
        no_battleye = os.environ.get("NoBattlEye", "false").strip().lower() == "true"  # noqa: SIM112
        cmd = server_command(*(["-NoBattlEye"] if no_battleye else []))
    except ConfigError as exc:
        emit_error("server", str(exc))
        return 2

    emit_event("server", f"Starting server: {' '.join(cmd)}")

    rotate_runtime_logs(SERVER_LOG, CRASH_LOG, cfg["log_rotation_keep_files"])
    _clear_stale_stop_file(STOP_STATUS_FILE)

    oom_before = read_oom_kill_count()
    if oom_before is None:
        emit_warning("server-oom", "cannot read the cgroup oom_kill counter; out-of-memory kills won't be reported.")
    server_proc = subprocess.Popen(cmd, cwd=SERVER_BINARIES_DIR, start_new_session=True)
    ctrl = _ShutdownController(server_proc, cfg)
    ctrl.setup_signals()

    server_status = _run_server_loop(server_proc, SERVER_LOG, ctrl, cfg["force_poll_interval"])
    report_oom_kill(oom_before)
    record_server_ini_write(server_status)
    _print_crash_log(CRASH_LOG, server_status)
    exit_code = _report_exit_status(server_status, STOP_STATUS_FILE, SERVER_LOG)
    # Popen reports signal deaths as -N; use the shell convention 128+N instead of letting it wrap to 256-N.
    return 128 - exit_code if exit_code < 0 else exit_code


if __name__ == "__main__":
    raise SystemExit(main())
