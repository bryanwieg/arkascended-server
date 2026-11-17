"""Canonical logging module for ARK container runtime scripts.

All runtime scripts emit via `emit_event`, `emit_warning` or `emit_error`
rather than calling bare `print()`; tests/test_logging_discipline.py enforces
it. See docs/dev/explanation/logging-architecture.md#container-side-logging.

Channel semantics:

- ``emit_event(phase, msg)`` — operator-visible runtime event. Writes to
  the caller's stdout **and** mirrors to PID1's stdout fd so the event
  appears in ``docker logs`` even when the script is invoked via
  ``docker compose exec`` (where exec stdio is returned to the caller,
  not surfaced to ``docker logs``). The PID1 mirror is the load-bearing
  mechanism that keeps the same event visible on both streams; do not
  remove it without replacing it.
- ``emit_warning(phase, msg)`` / ``emit_error(phase, msg)`` — same, but on
  stderr and PID1's stderr fd, with ``Warning: `` / ``Error: `` added after
  the phase. Callers never type the severity themselves.

Wire format is ``"[{phase}] {msg}"``, ``"[{phase}] Warning: {msg}"`` or
``"[{phase}] Error: {msg}"``, each with a trailing newline.

When the caller's stdout/stderr already is PID1's stream (the script runs
as PID 1, or is a child sharing its output), the mirror is skipped so the
line is not written to ``docker logs`` twice.
"""

from __future__ import annotations

import os
import sys

_PID1_STDOUT = "/proc/1/fd/1"
_PID1_STDERR = "/proc/1/fd/2"


def _mirror_to_pid1(message: str, *, stderr: bool) -> None:
    """Write message to PID1's stdout or stderr file descriptor.

    Best-effort: silent on OSError so scripts never fail because the
    mirror is unavailable (e.g. in a CI sandbox without a running PID 1
    or without permission to read /proc/1). The mirror is the mechanism
    that surfaces event text into ``docker logs`` when the script runs
    under ``docker compose exec``; tests in tests/container/integration/
    exercise the end-to-end path.
    """
    stream_path = _PID1_STDERR if stderr else _PID1_STDOUT
    try:
        fd = os.open(stream_path, os.O_WRONLY)
    except OSError:
        return
    try:
        # PID 1 itself, and children sharing its output (the stop script), already wrote
        # to this stream; mirroring again would print every line twice in `docker logs`.
        try:
            if os.path.samestat(os.fstat(fd), os.fstat(2 if stderr else 1)):
                return
        except OSError:
            pass
        try:
            os.write(fd, message.encode("utf-8", errors="replace"))
        except OSError:
            return
    finally:
        try:  # noqa: SIM105
            os.close(fd)
        except OSError:
            pass


def _emit(text: str, *, stderr: bool) -> None:
    """Write one line to the caller's stdout or stderr and mirror it to PID1's matching stream."""
    stream = sys.stderr if stderr else sys.stdout
    stream.write(text)
    stream.flush()
    _mirror_to_pid1(text, stderr=stderr)


def emit_event(phase: str, msg: str) -> None:
    """Emit an operator-visible runtime event.

    Writes ``"[{phase}] {msg}\\n"`` to stdout and mirrors the same bytes
    to PID1's stdout fd. Scripts invoked via ``docker compose exec``
    rely on the mirror so the event is also visible in ``docker logs``.
    """
    _emit(f"[{phase}] {msg}\n", stderr=False)


def emit_warning(phase: str, msg: str) -> None:
    """Emit an operator-visible warning: ``"[{phase}] Warning: {msg}\\n"`` on stderr, mirrored to PID1."""
    _emit(f"[{phase}] Warning: {msg}\n", stderr=True)


def emit_error(phase: str, msg: str) -> None:
    """Emit an operator-visible error: ``"[{phase}] Error: {msg}\\n"`` on stderr, mirrored to PID1."""
    _emit(f"[{phase}] Error: {msg}\n", stderr=True)
