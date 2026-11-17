"""Process-level compose file registry for signal-handler cleanup.

When SIGTERM or SIGINT arrives during fixture setup, pytest may exit without
draining fixture teardowns, leaving containers orphaned.  This module provides
a signal handler that runs ``docker compose down`` directly (via subprocess,
not the python-on-whales SDK) for every registered compose file.

Fixtures register the compose file path during setup via :func:`register`,
typically from ``prepare_test_env``.
"""

from __future__ import annotations

import os
import signal
import subprocess
from pathlib import Path

_compose_files: list[str] = []


def register(path: str) -> None:
    """Register a compose file for signal-handler cleanup."""
    if path not in _compose_files:
        _compose_files.append(path)


def unregister(path: str) -> None:
    """Remove a compose file from the cleanup registry."""
    try:  # noqa: SIM105
        _compose_files.remove(path)
    except ValueError:
        pass


def cleanup_all() -> None:
    """Run ``docker compose down`` for every registered compose file.

    Safe to call from a signal handler: uses ``subprocess.run`` directly
    (avoiding the python-on-whales SDK which may be in an inconsistent state
    at signal time).
    """
    runtime = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    for path in _compose_files:
        if not Path(path).is_file():
            continue
        try:  # noqa: SIM105
            subprocess.run(  # noqa: PLW1510
                [runtime, "compose", "-f", path, "down", "--volumes", "--remove-orphans"],
                capture_output=True,
                timeout=30,
            )
        except Exception:  # noqa: BLE001, S110
            pass


def cleanup_streamers() -> None:
    """Kill all active log streamer subprocesses via process group.

    Signal-safe: uses os.killpg(), no blocking wait, no SDK.
    Each streamer process was started in its own process group
    (preexec_fn=os.setsid), so os.killpg(pgid, SIGTERM) kills
    both the shell and the compose logs child.
    """
    from helpers.compose_client import _WATCHER_LOCK, _WATCHERS

    with _WATCHER_LOCK:
        items = list(_WATCHERS.items())
    for key, info in items:  # noqa: B007
        proc: subprocess.Popen | None = info.get("proc")
        if proc is None:
            continue
        try:
            pgid = os.getpgid(proc.pid)
            os.killpg(pgid, signal.SIGTERM)
        except OSError:
            pass


def _handle_signal(signum: int, frame: object) -> None:
    """Signal handler: run compose cleanup, then re-raise with default disposition."""
    signal.signal(signum, signal.SIG_IGN)
    cleanup_streamers()
    cleanup_all()
    signal.signal(signum, signal.SIG_DFL)
    os.kill(os.getpid(), signum)


def install_signal_handlers() -> None:
    """Install SIGTERM and SIGINT handlers that clean up compose projects."""
    signal.signal(signal.SIGTERM, _handle_signal)
    signal.signal(signal.SIGINT, _handle_signal)
