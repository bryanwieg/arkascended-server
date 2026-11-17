"""Framework-level debug logger for integration tests.

Module-level state configured from conftest. Every log_framework() call
writes a timestamped line to the per-test framework.log file.

Usage:
    from helpers.framework_logger import log_framework

    log_framework("poll", "waiting for container (elapsed=5s, remaining=25s)")
"""

from __future__ import annotations

import time
from typing import TextIO

_LOG_FILE: TextIO | None = None


def configure(log_path: str | None) -> None:
    """Configure the framework logger.

    Opens *log_path* for appending. Call once per test from the
    configure_test_artifacts fixture. Passing None or an empty path
    is a no-op (used for unconfigured callers outside pytest).
    """
    global _LOG_FILE  # noqa: PLW0603

    _close()  # close any previous handle

    if not log_path:
        _LOG_FILE = None
        return

    _LOG_FILE = open(log_path, "a", encoding="utf-8")  # noqa: PTH123, SIM115
    log_framework("framework", "logger configured")


def close() -> None:
    """Close the log file handle. Idempotent."""
    _close()


def _close() -> None:
    global _LOG_FILE  # noqa: PLW0603
    if _LOG_FILE is not None:
        try:  # noqa: SIM105
            _LOG_FILE.close()
        except OSError:
            pass
        _LOG_FILE = None


def log_framework(category: str, message: str, *args: object) -> None:
    """Write a timestamped entry to the framework log.

    Format: [HH:MM:SS.mmm] [category] message

    If args are provided, message is used as a format string (similar
    to logging.debug("fmt %%s", arg)). Uses %%-style formatting.
    """
    if _LOG_FILE is None:
        return

    if args:  # noqa: SIM108
        formatted = message % args
    else:
        formatted = message

    timestamp = time.strftime("%H:%M:%S", time.localtime())
    msec = int((time.time() % 1) * 1000)
    line = f"[{timestamp}.{msec:03d}] [{category}] {formatted}"

    _LOG_FILE.write(line + "\n")
    _LOG_FILE.flush()
