"""Save detection before backup: the save-complete log marker, then save files that stopped changing."""

from __future__ import annotations

import os
import time
from collections.abc import Collection
from pathlib import Path

from runtime_utils import SERVER_LOG, file_size_or_none, read_log_tail

SAVE_COMPLETE_MARKER = "World Save Complete"


def save_related_log_tail(log_file: Path, *, max_lines: int = 8) -> list[str]:
    """Return the last max_lines lines mentioning save activity from the tail of the log file."""
    data = read_log_tail(log_file, 262144)
    if data is None:
        return []
    interesting = []
    for line in data.splitlines():
        if "Saving world" in line or "World Save Complete" in line:
            interesting.append(line)  # noqa: PERF401
    return interesting[-max_lines:]


def read_new_log_lines(log_file: Path, cursor: int) -> tuple[str, int]:
    """Return the complete lines written after cursor, and the cursor just past them.

    A partly written last line is left for the next read, so a marker split across two polls
    is still seen whole. If the log shrank (rotated or truncated), reading restarts at 0.
    """
    size = file_size_or_none(log_file)
    if size is None:
        return "", 0
    start = 0 if size < cursor else cursor
    try:
        with log_file.open("rb") as handle:
            handle.seek(start)
            raw = handle.read(size - start)
    except OSError:
        return "", start
    end = raw.rfind(b"\n") + 1
    return raw[:end].decode("utf-8", errors="ignore"), start + end


def wait_for_save_complete(
    cursor: int,
    timeout_seconds: int,
    poll_seconds: float,
    log_file: Path = SERVER_LOG,
) -> bool:
    """Return True once a 'World Save Complete' line is written after cursor; False at timeout.

    Take cursor (the log size) just before sending saveworld: only a save that finishes
    after the request counts, so an earlier autosave can't confirm a save that failed.
    """
    deadline = time.monotonic() + timeout_seconds
    position = cursor
    while time.monotonic() < deadline:
        data, position = read_new_log_lines(log_file, position)
        if SAVE_COMPLETE_MARKER in data:
            return True
        time.sleep(poll_seconds)
    return False


def saved_files_state(root: Path, excluded_dirs: Collection[str]) -> dict[str, tuple[int, int]]:
    """Return {relative path: (size, mtime_ns)} of the files under root, skipping top-level excluded_dirs."""
    state = {}
    for dirpath, dirnames, filenames in os.walk(root):
        if Path(dirpath) == root:
            dirnames[:] = [name for name in dirnames if name not in excluded_dirs]
        for name in filenames:
            path = Path(dirpath, name)
            try:
                info = path.stat()
            except OSError:
                continue  # removed between listing and stat: the next poll sees the change
            state[str(path.relative_to(root))] = (info.st_size, info.st_mtime_ns)
    return state


def wait_for_saves_settled(
    root: Path,
    excluded_dirs: Collection[str],
    timeout_seconds: int,
    poll_seconds: float,
) -> bool:
    """Return True once no file under root changes between two polls; False at timeout.

    ARK logs 'World Save Complete' before it has finished writing the .ark file and pruning
    its rotated copies; archiving then makes tar fail with "file changed as we read it".
    """
    deadline = time.monotonic() + timeout_seconds
    previous = saved_files_state(root, excluded_dirs)
    while time.monotonic() < deadline:
        time.sleep(poll_seconds)
        current = saved_files_state(root, excluded_dirs)
        if current == previous:
            return True
        previous = current
    return False
