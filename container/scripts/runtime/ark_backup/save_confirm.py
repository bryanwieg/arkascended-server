"""Pre-backup save confirmation: trigger saveworld and wait for a save completed after the request."""

from __future__ import annotations

import time
from typing import NamedTuple

from _logging import emit_event, emit_warning
from runtime_utils import SERVER_LOG, ark_server_running, env_int, file_size_or_none

from .rcon_save import run_rcon_save
from .save_markers import save_related_log_tail, wait_for_save_complete


class SaveConfirmSettings(NamedTuple):
    save_timeout: int
    poll_seconds: int
    confirm_retries: int
    confirm_retry_delay: int


def load_save_confirm_settings() -> SaveConfirmSettings:
    """Read save-confirmation settings from environment variables; raises ConfigError on a bad value."""
    return SaveConfirmSettings(
        save_timeout=env_int("BACKUP_SAVE_CONFIRM_TIMEOUT_SECONDS", 60),
        poll_seconds=env_int("BACKUP_SAVE_CONFIRM_POLL_SECONDS", 2),
        confirm_retries=env_int("BACKUP_SAVE_CONFIRM_MAX_RETRIES", 3),
        confirm_retry_delay=env_int("BACKUP_SAVE_CONFIRM_RETRY_DELAY_SECONDS", 3),
    )


def pre_backup_save() -> bool:
    """Trigger an RCON world save and wait for confirmation before archiving; return True on success."""
    if not ark_server_running():
        emit_event("backup", "ARK server process not detected; skipping RCON save trigger.")
        return True

    settings = load_save_confirm_settings()

    for confirm_attempt in range(1, settings.confirm_retries + 1):
        attempt_label = f"attempt {confirm_attempt}/{settings.confirm_retries}"
        cursor = file_size_or_none(SERVER_LOG) or 0
        if run_rcon_save():
            emit_event(
                "backup",
                "waiting for 'World Save Complete' marker before archiving "
                f"(timeout={settings.save_timeout}s; {attempt_label}).",
            )
            if wait_for_save_complete(cursor, settings.save_timeout, settings.poll_seconds):
                emit_event("backup", "save completion marker detected in server log.")
                return True
            tail_lines = save_related_log_tail(SERVER_LOG)
            emit_warning(
                "backup",
                "did not observe 'World Save Complete' in server log before timeout "
                f"({attempt_label}); recent_save_log_tail={' | '.join(tail_lines) or 'none'}",
            )
        else:
            emit_warning("backup", f"RCON save request did not succeed ({attempt_label}).")
        if confirm_attempt < settings.confirm_retries:
            emit_event("backup", f"retrying backup save confirmation in {settings.confirm_retry_delay}s.")
            time.sleep(settings.confirm_retry_delay)
    return False
