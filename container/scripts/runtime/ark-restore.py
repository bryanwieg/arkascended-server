#!/usr/bin/env python3.14
"""Restore ShooterGame/Saved data from a tar archive inside the container."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _logging import emit_error, emit_event
from ark_backup.archive_validation import validate_archive_contents
from ark_backup.common import process_failure_detail
from ark_backup.metadata import METADATA_FILENAME, read_backup_metadata
from runtime_utils import (
    CLUSTER_DIR,
    CLUSTER_DIR_NAME,
    RESTORE_PREVIOUS_NAME,
    RESTORE_STAGING_NAME,
    SAVED_DIR,
    ark_server_running,
)


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments for the restore script."""
    parser = argparse.ArgumentParser(usage="%(prog)s --archive <path> --pre-restore <path> [--cluster]")
    parser.add_argument("--archive", required=True, help="Restore archive path")
    parser.add_argument("--pre-restore", required=True, help="Output path for pre-restore backup tar.gz")
    parser.add_argument("--cluster", action="store_true", help="Restore the shared cluster directory instead")
    return parser.parse_args()


def list_archive_entries(archive: Path) -> list[str]:
    """List all member paths in the archive; exit with status 1 on failure."""
    result = subprocess.run(["tar", "-tf", str(archive)], check=False, capture_output=True, text=True)
    if result.returncode != 0:
        detail = process_failure_detail(result, f"tar list failed with exit code {result.returncode}")
        emit_error("restore", f"failed to read restore archive: {detail}")
        raise SystemExit(1)
    return [name for line in result.stdout.splitlines() if (name := line.strip())]


def is_unsafe_path(entry: str) -> bool:
    """Return True if a tar entry path is absolute or contains directory traversal sequences."""
    return entry.startswith("/") or "../" in entry or entry == ".." or entry.endswith("/..")


def swap_blocker(save_dir: Path, keep: set[str]) -> str | None:
    """Return why the restored saves can't be swapped into save_dir, or None if they can."""
    if (save_dir / RESTORE_PREVIOUS_NAME).exists():
        return (
            f"{save_dir / RESTORE_PREVIOUS_NAME} exists: an earlier restore was interrupted while swapping. "
            "It may hold the only copy of the earlier saves; move it out of the volume, then retry."
        )
    mounted = [c.name for c in save_dir.iterdir() if c.name not in keep and os.path.ismount(c)]
    if mounted:
        return f"cannot swap mounted directories under {save_dir}: {', '.join(mounted)}"
    return None


def swap_in(save_dir: Path, staging: Path, keep: set[str]) -> None:
    """Replace save_dir's contents (except the names in keep) with staging's, by renames on the same volume."""
    previous = save_dir / RESTORE_PREVIOUS_NAME
    previous.mkdir()
    for child in list(save_dir.iterdir()):
        if child.name not in keep | {staging.name, previous.name}:
            child.rename(previous / child.name)
    for child in list(staging.iterdir()):
        child.rename(save_dir / child.name)
    staging.rmdir()
    shutil.rmtree(previous)


def log_backup_metadata(archive: Path) -> None:
    """Log the instance backup's metadata, if the archive has any."""
    metadata = read_backup_metadata(archive)
    if metadata:
        emit_event(
            "restore",
            f"Backup metadata: server_version={metadata.get('server_version', '-')} "
            f"instance={metadata.get('instance_name', '-')} "
            f"map={metadata.get('map_name', '-')} "
            f"timestamp={metadata.get('backup_timestamp', '-')}",
        )
    else:
        emit_event("restore", "No backup metadata found in archive (pre-metadata backup).")


def main() -> int:  # noqa: PLR0911
    """Entry point: validate and apply a restore archive over the save directory; return an exit code."""
    args = parse_args()
    archive = Path(args.archive)
    pre_restore = Path(args.pre_restore)
    # Instance mode leaves the shared clusters/ mount alone; cluster mode restores exactly that mount.
    target_dir = CLUSTER_DIR if args.cluster else SAVED_DIR
    keep = set() if args.cluster else {CLUSTER_DIR_NAME}
    cluster_excludes = [] if args.cluster else [f"--exclude=./{CLUSTER_DIR_NAME}"]
    maintenance_mode = os.environ.get("ARK_MAINTENANCE_MODE", "")

    if maintenance_mode != "restore":
        emit_error(
            "restore", "restore requires ARK_MAINTENANCE_MODE=restore and must run in maintenance container mode."
        )
        return 1

    if ark_server_running():
        emit_error("restore", "ARK server process detected; restore must run while server is stopped.")
        return 1

    if not target_dir.is_dir():
        emit_error("restore", f"restore directory not found: {target_dir}")
        return 1

    if not archive.is_file():
        emit_error("restore", f"restore archive not found: {archive}")
        return 1

    entries = list_archive_entries(archive)
    for name in entries:
        if is_unsafe_path(name):
            emit_error("restore", f"restore archive contains unsafe path: {name}")
            return 1

    # Cluster archives hold ARK's upload files, not SavedArks; there is nothing more to validate.
    if not args.cluster and not validate_archive_contents(archive):
        return 1

    blocker = swap_blocker(target_dir, keep)
    if blocker:
        emit_error("restore", blocker)
        return 1
    staging = target_dir / RESTORE_STAGING_NAME
    shutil.rmtree(staging, ignore_errors=True)  # leftover from an interrupted restore; never holds live data

    if not args.cluster:
        log_backup_metadata(archive)

    pre_restore.parent.mkdir(parents=True, exist_ok=True)
    emit_event("restore", f"Creating pre-restore backup at {pre_restore}")
    backup_rc = subprocess.run(
        ["tar", "-czf", str(pre_restore), *cluster_excludes, "-C", str(target_dir), "."], check=False
    ).returncode
    if backup_rc != 0:
        return backup_rc

    # Extract beside the live saves first, so a failed extraction (disk full, I/O error) leaves them untouched.
    staging.mkdir()
    emit_event("restore", f"Extracting restore archive into {staging}")
    # Older instance archives include clusters/; never extract it over the shared cluster volume.
    extract_excludes = [] if args.cluster else ["--anchored", *cluster_excludes, f"--exclude={CLUSTER_DIR_NAME}"]
    restore_rc = subprocess.run(
        ["tar", "-xf", str(archive), *extract_excludes, "-C", str(staging)],
        check=False,
    ).returncode
    if restore_rc != 0:
        shutil.rmtree(staging, ignore_errors=True)
        emit_error(
            "restore", f"extracting the archive failed (tar exit {restore_rc}); the existing saves are unchanged."
        )
        return restore_rc
    (staging / METADATA_FILENAME).unlink(missing_ok=True)

    emit_event("restore", f"Swapping the restored data into {target_dir}")
    swap_in(target_dir, staging, keep)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
