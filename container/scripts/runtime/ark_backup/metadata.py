"""Backup metadata generation: write backup-metadata.json before archiving."""

from __future__ import annotations

import json
import os
import subprocess
from datetime import UTC, datetime
from pathlib import Path

from _logging import emit_event
from runtime_utils import SAVED_DIR

METADATA_FILENAME = "backup-metadata.json"


def read_file_safe(path: Path) -> str:
    try:
        return path.read_text().strip()
    except OSError:
        return ""


def build_metadata_dict() -> dict:
    session_name = os.environ.get("SESSION_NAME", "")
    map_name = os.environ.get("MAP_NAME", "")
    version = read_file_safe(Path("/home/ark-user/ARK_SERVER_VERSION"))
    build_id = read_file_safe(Path("/home/ark-user/ARK_SERVER_BUILDID"))
    timestamp = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
    env_snapshot = {
        "MAP_NAME": map_name,
        "SESSION_NAME": session_name,
        "ASA_PORT": os.environ.get("ASA_PORT", ""),
        "TZ": os.environ.get("TZ", ""),
    }
    return {
        "server_version": version,
        "build_id": build_id,
        "instance_name": session_name,
        "map_name": map_name,
        "backup_timestamp": timestamp,
        "environment": env_snapshot,
    }


def write_backup_metadata() -> Path:
    metadata = build_metadata_dict()
    path = SAVED_DIR / METADATA_FILENAME
    path.write_text(json.dumps(metadata, indent=2))
    emit_event("backup", f"wrote backup metadata to {path}")
    return path


def remove_backup_metadata() -> None:
    path = SAVED_DIR / METADATA_FILENAME
    try:  # noqa: SIM105
        path.unlink(missing_ok=True)
    except OSError:
        pass


def read_backup_metadata(archive_path: Path) -> dict | None:
    for member_name in (f"./{METADATA_FILENAME}", METADATA_FILENAME):
        result = subprocess.run(  # noqa: PLW1510
            ["tar", "-O", "-xf", str(archive_path), member_name],
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            try:
                return json.loads(result.stdout)
            except json.JSONDecodeError:
                return None
    return None
