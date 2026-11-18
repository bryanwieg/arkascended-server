from __future__ import annotations

import os

import pytest


@pytest.fixture(autouse=True)
def _no_pid1_mirror(monkeypatch):
    """Point _logging's PID 1 mirror at /dev/null for in-process container script tests.

    Outside a container, /proc/1/fd/1 is the host's PID 1 terminal; in the dev sandbox that
    is the terminal the Claude TUI draws on, so mirrored lines overwrite its display.
    Tests that exercise the mirror patch os.open themselves, which overrides this.
    """
    real_open = os.open

    def guarded_open(path, flags, *args, **kwargs):
        if isinstance(path, str) and path.startswith("/proc/1/fd/"):
            path = os.devnull
        return real_open(path, flags, *args, **kwargs)

    monkeypatch.setattr(os, "open", guarded_open)
