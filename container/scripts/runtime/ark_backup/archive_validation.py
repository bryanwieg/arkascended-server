"""Archive integrity validation: tar listing, SQLite extraction, and integrity checks."""

from __future__ import annotations

import sqlite3
import subprocess
import tempfile
from pathlib import Path
from typing import NamedTuple

from _logging import emit_error, emit_event
from runtime_utils import MAINTENANCE_DIR

from .common import process_failure_detail


def normalize_member(name: str) -> str:
    """Strip a leading './' prefix from a tar member path."""
    if name.startswith("./"):
        return name[2:]
    return name


class ArchiveMember(NamedTuple):
    # Keep both names: `raw` for tar extraction, `normalized` for logical checks.
    raw: str
    normalized: str


def sqlite_candidates(members: list[ArchiveMember]) -> list[ArchiveMember]:
    """Return archive members under SavedArks/ that are sqlite-backed world save files."""
    candidates: list[ArchiveMember] = []
    for member in sorted(members, key=lambda item: item.normalized):
        lower = member.normalized.lower()
        if not lower.startswith("savedarks/"):
            continue
        # ARK world saves are sqlite-backed .ark files; include local sqlite sidecars too.
        if lower.endswith(".ark") or lower.endswith(".sqlite") or lower.endswith(".db"):  # noqa: PIE810
            candidates.append(member)
    return candidates


def sqlite_quick_check(db_path: Path) -> tuple[bool, str]:
    """Run SQLite PRAGMA quick_check on a database file; return (ok, detail)."""
    try:
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    except sqlite3.DatabaseError as exc:
        return False, str(exc)
    try:
        row = conn.execute("PRAGMA quick_check;").fetchone()
    except sqlite3.DatabaseError as exc:
        return False, str(exc)
    finally:
        conn.close()
    if not row or row[0] != "ok":
        detail = row[0] if row and row[0] else "no result"
        return False, detail
    return True, "ok"


def archive_members_from_tar_list(list_output: str) -> list[ArchiveMember]:
    """Parse tar listing output into a list of ArchiveMember tuples."""
    return [
        ArchiveMember(raw=name.strip(), normalized=normalize_member(name.strip()))
        for name in list_output.splitlines()
        if name.strip()
    ]


def archive_has_savedarks_content(members: list[ArchiveMember]) -> bool:
    """Return True if any archive member lives under the SavedArks/ directory."""
    return any(member.normalized.startswith("SavedArks/") for member in members)


def extract_sqlite_candidates(
    archive_path: Path,
    temp_root: Path,
    sqlite_files: list[ArchiveMember],
) -> bool:
    """Extract the given sqlite candidate members from the archive into temp_root; return True on success."""
    extract_cmd = ["tar", "-xf", str(archive_path), "-C", str(temp_root), *(entry.raw for entry in sqlite_files)]
    extract_result = subprocess.run(extract_cmd, check=False, capture_output=True, text=True)
    if extract_result.returncode == 0:
        return True
    detail = process_failure_detail(extract_result, f"tar extract failed with exit code {extract_result.returncode}")
    emit_error("archive", f"failed to extract sqlite candidates for validation: {detail}")
    return False


def validate_extracted_sqlite_files(
    temp_root: Path,
    sqlite_files: list[ArchiveMember],
) -> bool:
    """Run sqlite_quick_check on every previously extracted sqlite candidate; return True if all pass."""
    for entry in sqlite_files:
        target = temp_root / entry.normalized
        if not target.is_file():
            emit_error("archive", f"failed to extract sqlite file from archive: {entry.normalized}")
            return False
        ok, detail = sqlite_quick_check(target)
        if not ok:
            emit_error("archive", f"sqlite quick_check failed for {entry.normalized}: {detail}")
            return False
    return True


def validate_archive_sqlite_payload(archive_path: Path, sqlite_files: list[ArchiveMember]) -> bool:
    """Extract and validate all sqlite candidates from the archive; return True if all pass."""
    if not sqlite_files:
        emit_event("archive", "no sqlite-backed world files detected in SavedArks; skipping sqlite quick_check.")
        return True

    MAINTENANCE_DIR.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ark-backup-sqlite-", dir=str(MAINTENANCE_DIR)) as temp_dir:
        temp_root = Path(temp_dir)
        if not extract_sqlite_candidates(archive_path, temp_root, sqlite_files):
            return False
        if not validate_extracted_sqlite_files(temp_root, sqlite_files):
            return False

    emit_event("archive", f"sqlite quick_check passed for {len(sqlite_files)} file(s).")
    return True


def validate_archive_contents(archive_path: Path) -> bool:
    """List the archive, verify SavedArks content is present, and run sqlite validation."""
    list_result = subprocess.run(["tar", "-tf", str(archive_path)], check=False, capture_output=True, text=True)
    if list_result.returncode != 0:
        detail = process_failure_detail(list_result, f"tar -tf failed with exit code {list_result.returncode}")
        emit_error("archive", f"backup archive listing failed: {detail}")
        return False

    try:
        members = archive_members_from_tar_list(list_result.stdout)
        if not archive_has_savedarks_content(members):
            emit_error("archive", "backup archive is missing SavedArks content.")
            return False
        emit_event("archive", "validated SavedArks content exists in archive.")
        return validate_archive_sqlite_payload(archive_path, sqlite_candidates(members))
    except OSError as exc:
        emit_error("archive", f"backup archive validation failed: {exc}")
        return False
