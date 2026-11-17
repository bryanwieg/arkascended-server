"""RCON-driven world save step used at the start of each backup run."""

from __future__ import annotations

import os
import time
from typing import NamedTuple

from _logging import emit_error, emit_event, emit_warning
from runtime_utils import env_int, run_rcon


class RconSaveSettings(NamedTuple):
    retries: int
    retry_delay: int


def load_rcon_save_settings() -> RconSaveSettings | None:
    """Read RCON save settings from the environment; return None if no admin password is set.

    Raises ConfigError for a bad retry setting.
    """
    if not os.environ.get("SERVER_ADMIN_PASSWORD"):
        # Checked up front so a missing password fails at once instead of after every retry.
        emit_error("backup", "SERVER_ADMIN_PASSWORD is required for pre-backup RCON save.")
        return None
    return RconSaveSettings(
        retries=env_int("BACKUP_RCON_SAVE_MAX_RETRIES", 3),
        retry_delay=env_int("BACKUP_RCON_SAVE_RETRY_DELAY_SECONDS", 2),
    )


def run_rcon_save() -> bool:
    """Issue a saveworld RCON command with retries; return True on success."""
    settings = load_rcon_save_settings()
    if settings is None:
        return False

    for attempt in range(1, settings.retries + 1):
        emit_event("backup", f"requesting world save via RCON (attempt {attempt}/{settings.retries}).")
        if run_rcon("saveworld", timeout=15):
            return True
        if attempt < settings.retries:
            emit_warning(
                "backup",
                f"RCON saveworld failed (attempt {attempt}/{settings.retries}); retrying in {settings.retry_delay}s.",
            )
            time.sleep(settings.retry_delay)
    return False
