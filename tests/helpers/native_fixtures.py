from __future__ import annotations

import json
import sqlite3
import tarfile
from pathlib import Path


def write_schedule_file(path: Path, schedules_yaml: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(schedules_yaml, encoding="utf-8")


def write_run_state(
    state_dir: Path,
    name: str,
    status: str,
    timestamp: str = "2026-05-14T03:00:00Z",
) -> None:
    state_dir.mkdir(parents=True, exist_ok=True)
    state = {
        "name": name,
        "last_started_at": timestamp,
        "last_finished_at": "2026-05-14T03:05:00Z",
        "last_status": status,
        "last_exit_code": 0 if status == "success" else 1,
        "last_error": "" if status == "success" else "simulated failure",
    }
    (state_dir / f"{name}.last.json").write_text(
        json.dumps(state, indent=2) + "\n",
        encoding="utf-8",
    )


def create_backup_archive(
    backup_dir: Path,
    instance: str,
    name: str,
    with_sqlite: bool = True,
) -> Path:
    instance_dir = backup_dir / instance
    instance_dir.mkdir(parents=True, exist_ok=True)
    archive_path = instance_dir / name
    with tarfile.open(str(archive_path), "w:gz") as tar:
        if with_sqlite:
            db_path = instance_dir / "temp.sqlite"
            conn = sqlite3.connect(str(db_path))
            conn.execute("CREATE TABLE test (id INTEGER)")
            conn.execute("INSERT INTO test VALUES (1)")
            conn.commit()
            conn.close()
            tar.add(str(db_path), arcname="ShooterGame/Saved/test.sqlite")
            db_path.unlink()
    return archive_path


SCHEDULE_TWO_ENTRIES = """\
version: 1
schedules:
  - name: nightly
    type: backup
    instance: testinst
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 5
    retain_days: 30
    retain_size_gb: 10
  - name: weekly
    type: backup
    instance: testinst2
    enabled: false
    cron: "0 4 * * 0"
    format: tar.gz
    retain_count: 3
    retain_days: 14
    retain_size_gb: 5
"""

SCHEDULE_SINGLE_ENABLED = """\
version: 1
schedules:
  - name: nightly
    type: backup
    instance: testinst
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 5
    retain_days: 30
    retain_size_gb: 10
"""

SCHEDULE_SINGLE_DISABLED = """\
version: 1
schedules:
  - name: nightly
    type: backup
    instance: testinst
    enabled: false
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 5
    retain_days: 30
    retain_size_gb: 10
"""


def write_minimal_compose(path: Path, instance_name: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        f"services:\n"
        f"  {instance_name}:\n"
        f"    image: ark-server:latest\n"
        f"    environment:\n"
        f"      - ASA_PORT=27015\n"
        f"    labels:\n"
        f"      ark.port.asa: '27015'\n",
        encoding="utf-8",
    )
