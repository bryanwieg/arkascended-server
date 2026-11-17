#!/usr/bin/env python3.14
"""Generate the image's default GameUserSettings.ini by running the ARK server once (image build only).

ARK writes a partial GameUserSettings.ini shortly after launch and the complete file, with every
[ServerSettings] default, only after SIGTERM. So this starts the server, waits for the file, sends
SIGTERM, waits for the complete file and keeps it. The random SessionName ARK makes up is removed,
so no server inherits it. Any failure fails the image build.
"""

from __future__ import annotations

import contextlib
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "runtime"))
from _logging import emit_error, emit_event
from runtime_utils import DEFAULT_GAME_USER_SETTINGS, GAME_USER_SETTINGS, SERVER_BINARIES_DIR, server_command

APPEAR_TIMEOUT_SECONDS = 300
SHUTDOWN_TIMEOUT_SECONDS = 120
PHASE = "ini-defaults"


def _is_section(line: str) -> bool:
    stripped = line.strip()
    return stripped.startswith("[") and stripped.endswith("]")


def strip_session_name(text: str) -> str:
    """Remove SessionName= lines inside [SessionSettings], keeping every other line and its line ending."""
    kept = []
    in_session_settings = False
    for line in text.splitlines(keepends=True):
        if _is_section(line):
            in_session_settings = line.strip() == "[SessionSettings]"
        elif in_session_settings and line.strip().startswith("SessionName="):
            continue
        kept.append(line)
    return "".join(kept)


def is_complete(text: str) -> bool:
    """Return True for the file ARK writes after SIGTERM: only that one has a [ServerSettings] section."""
    return any(line.strip() == "[ServerSettings]" for line in text.splitlines())


def _read_settings() -> str:
    with GAME_USER_SETTINGS.open(encoding="utf-8", newline="") as handle:
        return handle.read()


def _wait_for_settings_file(proc: subprocess.Popen[bytes], started: float) -> bool:
    while time.monotonic() - started < APPEAR_TIMEOUT_SECONDS:
        if GAME_USER_SETTINGS.is_file():
            return True
        if proc.poll() is not None:
            emit_error(PHASE, f"the ARK server exited with code {proc.returncode} before writing {GAME_USER_SETTINGS}.")
            return False
        time.sleep(1)
    emit_error(PHASE, f"{GAME_USER_SETTINGS} did not appear within {APPEAR_TIMEOUT_SECONDS}s.")
    return False


def _wait_for_complete_file() -> bool:
    """Return True once the INI is complete and its size has held for 2 s (ARK may still be writing it)."""
    deadline = time.monotonic() + SHUTDOWN_TIMEOUT_SECONDS
    last_size, stable_polls = -1, 0
    while time.monotonic() < deadline:
        size = GAME_USER_SETTINGS.stat().st_size
        stable_polls = stable_polls + 1 if size == last_size else 0
        if stable_polls >= 2 and is_complete(_read_settings()):
            return True
        last_size = size
        time.sleep(1)
    return False


def _kill(proc: subprocess.Popen[bytes], signum: int) -> None:
    with contextlib.suppress(ProcessLookupError):
        os.killpg(proc.pid, signum)


def main() -> int:
    """Run the server once and write the default GameUserSettings.ini; return 0 on success, 1 on failure."""
    emit_event(PHASE, "starting the ARK server to generate GameUserSettings.ini.")
    started = time.monotonic()
    proc = subprocess.Popen(server_command(), cwd=SERVER_BINARIES_DIR, start_new_session=True)
    try:
        if not _wait_for_settings_file(proc, started):
            return 1
        emit_event(
            PHASE, f"{GAME_USER_SETTINGS.name} appeared after {time.monotonic() - started:.0f}s; sending SIGTERM."
        )
        _kill(proc, signal.SIGTERM)
        # ARK writes the complete file on SIGTERM but does not reliably exit, so the file is the signal.
        if not _wait_for_complete_file():
            emit_error(
                PHASE,
                f"ARK did not write its complete GameUserSettings.ini within {SHUTDOWN_TIMEOUT_SECONDS}s of SIGTERM.",
            )
            return 1
    finally:
        _kill(proc, signal.SIGKILL)
        proc.wait()

    default = strip_session_name(_read_settings())
    DEFAULT_GAME_USER_SETTINGS.parent.mkdir(parents=True, exist_ok=True)
    with DEFAULT_GAME_USER_SETTINGS.open("w", encoding="utf-8", newline="") as handle:
        handle.write(default)
    emit_event(PHASE, f"wrote {DEFAULT_GAME_USER_SETTINGS} ({len(default.splitlines())} lines).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
