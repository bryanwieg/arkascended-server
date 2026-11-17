#!/usr/bin/env python3.14
"""Gracefully stop ARK server via RCON save + shutdown with log verification."""

from __future__ import annotations

import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _logging import emit_error, emit_event, emit_warning
from ark_backup.save_markers import wait_for_save_complete
from runtime_utils import (
    SERVER_LOG,
    STOP_STATUS_FILE,
    ConfigError,
    env_int,
    file_size_or_none,
    run_rcon,
    server_log_closed,
)


def save_world() -> bool:
    """Issue a saveworld RCON command and return True if it was accepted."""
    ok = run_rcon("saveworld")
    if ok:
        emit_event("stop", "World save command issued.")
    return ok


def send_chat(message: str) -> bool:
    """Send a ServerChat RCON message to all players and return True if accepted."""
    ok = run_rcon(f"ServerChat {message}")
    if ok:
        emit_event("stop", f"Chat message sent: {message}")
    return ok


def shutdown_server() -> bool:
    """Issue a DoExit RCON command to initiate server shutdown and return True if accepted."""
    ok = run_rcon("DoExit")
    if ok:
        emit_event("stop", "Server shutdown command issued.")
    return ok


def write_stop_status(path: Path, save_ok: bool, shutdown_ok: bool, stop_ok: bool) -> None:
    """Write the save/shutdown/stop outcome flags to the stop status env file."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        f"SAVE_OK={'true' if save_ok else 'false'}\n"
        f"SHUTDOWN_OK={'true' if shutdown_ok else 'false'}\n"
        f"STOP_OK={'true' if stop_ok else 'false'}\n",
        encoding="utf-8",
    )


# -- Stop workflow helpers --


def _load_stop_config() -> dict:
    """Read all stop-workflow settings from environment variables."""
    return {
        "warning_minutes": env_int("SHUTDOWN_WARNING_MINUTES", 0, minimum=0),
        "save_max_retries": env_int("SAVE_MAX_RETRIES", 3),
        "save_retry_delay": env_int("SAVE_RETRY_DELAY_SECONDS", 3),
        "save_confirm_timeout": env_int("SAVE_CONFIRM_TIMEOUT_SECONDS", 60),
        "stop_check_timeout": env_int("STOP_CHECK_TIMEOUT_SECONDS", 120),
        "stop_check_interval": env_int("STOP_CHECK_INTERVAL_SECONDS", 5),
    }


def _send_warning_and_wait(warning_minutes: int) -> None:
    """Broadcast shutdown warning chat and sleep, or log immediate shutdown."""
    if warning_minutes > 0:
        send_chat(f"Server shutting down in {warning_minutes} minute(s). Saving world before exit.")
        time.sleep(warning_minutes * 60)
    else:
        emit_event("stop", "Immediate shutdown requested; skipping player warning delay.")


def _save_with_retries(log_file: Path, max_retries: int, retry_delay: int, confirm_timeout: int) -> bool:
    """Attempt saveworld up to max_retries times; return True on first confirmed save.

    Only a 'World Save Complete' marker written after the request counts, so an earlier
    autosave's marker can't confirm a save that failed. Waiting up to confirm_timeout
    (instead of a fixed delay) avoids re-sending saveworld while a large save is running.
    """
    for attempt in range(1, max_retries + 1):
        cursor = file_size_or_none(log_file) or 0
        if save_world() and wait_for_save_complete(cursor, confirm_timeout, 1, log_file=log_file):
            emit_event("stop", "Save operation completed.")
            return True
        emit_warning(
            "stop", f"saveworld not confirmed (attempt {attempt}/{max_retries}). Retrying in {retry_delay}s..."
        )
        time.sleep(retry_delay)
    return False


def _wait_for_stop(log_file: Path, timeout: int, interval: int) -> bool:
    """Poll log for stop confirmation; return True if confirmed within timeout."""
    elapsed = 0
    while elapsed < timeout:
        if server_log_closed(log_file):
            emit_event("stop", "Server log closed; waiting 10s for process exit.")
            time.sleep(10)
            return True
        time.sleep(interval)
        elapsed += interval
    emit_warning("stop", f"server stop not confirmed after {timeout}s.")
    emit_error("stop", "Safe stop failed: grace period expired without clean shutdown.")
    return False


def _report_stop_outcome(save_ok: bool, shutdown_ok: bool, stop_ok: bool) -> int:
    """Persist and report the final stop outcome; return 0 on full success."""
    write_stop_status(STOP_STATUS_FILE, save_ok=save_ok, shutdown_ok=shutdown_ok, stop_ok=stop_ok)
    if save_ok and shutdown_ok and stop_ok:
        return 0
    emit_warning(
        "stop",
        "stop sequence did not fully succeed "
        f"(save_ok={'true' if save_ok else 'false'} "
        f"shutdown_ok={'true' if shutdown_ok else 'false'} "
        f"stop_ok={'true' if stop_ok else 'false'}).",
    )
    return 1


def main() -> int:
    """Entry point: run the full graceful stop workflow and return an exit code."""
    try:
        cfg = _load_stop_config()
    except ConfigError as exc:
        emit_error("stop", str(exc))
        return 2

    _send_warning_and_wait(cfg["warning_minutes"])

    if not _save_with_retries(
        SERVER_LOG, cfg["save_max_retries"], cfg["save_retry_delay"], cfg["save_confirm_timeout"]
    ):
        emit_error(
            "stop",
            f"saveworld was not verified after {cfg['save_max_retries']} attempts; "
            "aborting graceful shutdown so runtime manager can force-stop.",
        )
        write_stop_status(STOP_STATUS_FILE, save_ok=False, shutdown_ok=False, stop_ok=False)
        return 1

    shutdown_ok = shutdown_server()
    if not shutdown_ok:
        emit_error("stop", "DoExit command failed; runtime manager will force-stop.")
        write_stop_status(STOP_STATUS_FILE, save_ok=True, shutdown_ok=False, stop_ok=False)
        return 1

    stop_ok = _wait_for_stop(SERVER_LOG, cfg["stop_check_timeout"], cfg["stop_check_interval"])
    return _report_stop_outcome(save_ok=True, shutdown_ok=True, stop_ok=stop_ok)


if __name__ == "__main__":
    raise SystemExit(main())
