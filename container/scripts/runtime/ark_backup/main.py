"""Backup workflow orchestrator: pre-save, archive, validate, and retry logic."""

from __future__ import annotations

import subprocess
import time
from pathlib import Path

from _logging import emit_error, emit_event, emit_warning
from runtime_utils import SAVED_DIR, ConfigError, env_int

from .archive_validation import validate_archive_contents
from .common import (
    archive_args,
    combined_process_output,
    parse_args,
)
from .metadata import remove_backup_metadata, write_backup_metadata
from .save_confirm import pre_backup_save


def finish_with_result(exit_code: int, reason: str) -> int:
    """Log the final backup result and return the exit code."""
    status = "SUCCESS" if exit_code == 0 else "FAILURE"
    message = f"result: {status} exit_code={exit_code} reason={reason}"
    if exit_code == 0:
        emit_event("backup", message)
    else:
        emit_error("backup", message)
    return exit_code


def backup_verify_retry_settings() -> tuple[int, int]:
    """Return (max_attempts, retry_delay_seconds) from the environment; raises ConfigError on a bad value."""
    return env_int("BACKUP_VERIFY_MAX_RETRIES", 1), env_int("BACKUP_VERIFY_RETRY_DELAY_SECONDS", 3)


def retry_or_finalize(
    attempt: int,
    max_attempts: int,
    retry_delay_seconds: int,
    exit_code: int,
    reason: str,
    retry_message: str | None = None,
) -> tuple[int, str] | None:
    """Return a final (exit_code, reason) if attempts are exhausted, or sleep and return None to retry."""
    if attempt >= max_attempts:
        return exit_code, reason
    if retry_message:
        emit_warning("backup", retry_message)
    time.sleep(retry_delay_seconds)
    return None


def run_archive_command(cmd: list[str]) -> subprocess.CompletedProcess[str]:
    """Execute the tar archive command, log its output, and return the CompletedProcess result."""
    emit_event("backup", f"creating archive with command: {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False, capture_output=True, text=True)
    tar_output = combined_process_output(result)
    if tar_output:
        emit_event("backup", f"archive command output: {tar_output}")
    return result


def run_backup_attempt(
    attempt: int,
    max_attempts: int,
    retry_delay_seconds: int,
    output: Path,
    cmd: list[str],
) -> tuple[int, str] | None:
    """Run a single backup attempt (pre-save, archive, validate); return (exit_code, reason) or None to retry."""
    if not pre_backup_save():
        return retry_or_finalize(
            attempt,
            max_attempts,
            retry_delay_seconds,
            1,
            "pre-backup save confirmation failed",
        )

    write_backup_metadata()
    try:
        result = run_archive_command(cmd)
    finally:
        remove_backup_metadata()
    if result.returncode != 0:
        emit_error("backup", f"archive command failed with exit code {result.returncode}.")
        return retry_or_finalize(
            attempt,
            max_attempts,
            retry_delay_seconds,
            result.returncode,
            "archive command failed",
        )

    if not output.is_file() or output.stat().st_size <= 0:
        emit_error("backup", f"backup archive was not created or is empty: {output}")
        return retry_or_finalize(
            attempt,
            max_attempts,
            retry_delay_seconds,
            1,
            "archive missing or empty",
        )

    if validate_archive_contents(output):
        emit_event("backup", f"backup archive created successfully at {output}.")
        return 0, "archive created and validated"

    return retry_or_finalize(
        attempt,
        max_attempts,
        retry_delay_seconds,
        1,
        "backup validation failed after retries",
        retry_message=(
            f"backup archive integrity validation failed (attempt {attempt}/{max_attempts}); "
            f"retrying in {retry_delay_seconds}s."
        ),
    )


def main() -> int:
    """Entry point: orchestrate the full backup workflow and return an exit code."""
    args = parse_args()
    output = Path(args.output)
    emit_event("backup", f"starting backup workflow for output {output}.")
    if not SAVED_DIR.is_dir():
        emit_error("backup", f"save directory not found: {SAVED_DIR}")
        return finish_with_result(1, "save directory not found")

    output.parent.mkdir(parents=True, exist_ok=True)
    try:
        cmd = archive_args(output)
    except ValueError as exc:
        emit_error("backup", str(exc))
        return finish_with_result(1, "invalid output path/extension")

    # Settings are read where they are used (save confirmation reads its own per attempt); report any bad one once.
    try:
        max_attempts, retry_delay_seconds = backup_verify_retry_settings()
        for attempt in range(1, max_attempts + 1):
            if attempt > 1:
                emit_event("backup", f"retrying backup validation workflow (attempt {attempt}/{max_attempts}).")
            attempt_result = run_backup_attempt(attempt, max_attempts, retry_delay_seconds, output, cmd)
            if attempt_result is None:
                continue
            exit_code, reason = attempt_result
            return finish_with_result(exit_code, reason)
    except ConfigError as exc:
        emit_error("backup", str(exc))
        return finish_with_result(2, "invalid configuration")

    return finish_with_result(1, "backup validation failed after retries")
