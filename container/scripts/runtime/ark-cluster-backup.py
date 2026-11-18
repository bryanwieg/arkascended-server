#!/usr/bin/env python3.14
"""Create a backup archive of the shared cluster directory (Saved/clusters) inside a one-off container."""

from __future__ import annotations

import argparse
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _logging import emit_error, emit_event, emit_warning
from ark_backup.common import combined_process_output, tar_create_args
from runtime_utils import CLUSTER_DIR, RESTORE_PREVIOUS_NAME, RESTORE_STAGING_NAME

PHASE = "cluster-backup"
MAX_ATTEMPTS = 3
RETRY_DELAY_SECONDS = 2
# tar's exit code when a file changed or vanished while it was read (servers may write transfers meanwhile).
TAR_FILES_CHANGED = 1


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments for the cluster backup script."""
    parser = argparse.ArgumentParser(usage="%(prog)s --output <path>")
    parser.add_argument("--output", required=True, help="Archive output path (.tar, .tar.gz, or .tar.zst)")
    return parser.parse_args()


def write_archive(cmd: list[str], partial: Path) -> bool:
    """Run tar until it reads every file unchanged; return True when partial holds a complete archive."""
    for attempt in range(1, MAX_ATTEMPTS + 1):
        result = subprocess.run(cmd, check=False, capture_output=True, text=True)
        if result.returncode == 0:
            return True
        partial.unlink(missing_ok=True)
        output = combined_process_output(result)
        if result.returncode != TAR_FILES_CHANGED:
            emit_error(PHASE, f"tar failed with exit code {result.returncode}: {output}")
            return False
        emit_warning(PHASE, f"cluster files changed while archiving (attempt {attempt}/{MAX_ATTEMPTS}): {output}")
        if attempt < MAX_ATTEMPTS:
            time.sleep(RETRY_DELAY_SECONDS)
    emit_error(PHASE, f"cluster files kept changing during {MAX_ATTEMPTS} attempts; try again later.")
    return False


def main() -> int:
    """Archive CLUSTER_DIR to --output; return 0 on success, 1 on failure (leaving no file at --output)."""
    output = Path(parse_args().output)
    if not CLUSTER_DIR.is_dir():
        emit_error(PHASE, f"cluster directory not found: {CLUSTER_DIR}")
        return 1
    partial = output.with_name(output.name + ".partial")
    try:
        cmd = tar_create_args(
            output, CLUSTER_DIR, [f"./{RESTORE_STAGING_NAME}", f"./{RESTORE_PREVIOUS_NAME}"], dest=partial
        )
    except ValueError as exc:
        emit_error(PHASE, str(exc))
        return 1
    output.parent.mkdir(parents=True, exist_ok=True)
    emit_event(PHASE, f"archiving {CLUSTER_DIR} to {output}")
    if not write_archive(cmd, partial):
        return 1
    listing = subprocess.run(["tar", "-tf", str(partial)], check=False, capture_output=True, text=True)
    if listing.returncode != 0:
        partial.unlink(missing_ok=True)
        emit_error(PHASE, f"the new archive could not be listed: {combined_process_output(listing)}")
        return 1
    partial.replace(output)
    emit_event(PHASE, f"cluster backup created at {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
