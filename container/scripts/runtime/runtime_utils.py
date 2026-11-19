"""Shared utilities used across multiple container runtime scripts.

Functions here are used by both standalone scripts (ark-server-start.py,
ark-server-stop.py, ark-restore.py) and by the ark_backup
packages. Keeping them in one place avoids maintaining identical copies in
each file.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

from _logging import emit_error, emit_event, emit_warning
from rcon_client import RconError, rcon_command

# Container paths. The dockerfile installs the server and scripts here and the compose
# template mounts the volumes here, so they are fixed rather than configurable.
SCRIPTS_DIR = Path(__file__).resolve().parent
PYTHON = sys.executable
SERVER_DIR = Path("/home/ark-user/ark-server")
SERVER_BINARIES_DIR = SERVER_DIR / "ShooterGame/Binaries/Win64"
SAVED_DIR = SERVER_DIR / "ShooterGame/Saved"
LOGS_DIR = SAVED_DIR / "Logs"
SERVER_LOG = LOGS_DIR / "ShooterGame.log"
CRASH_LOG = LOGS_DIR / "crashcallstack.txt"
STOP_STATUS_FILE = LOGS_DIR / "ark-stop-status.env"
# Why the next stop happens ("updating to 98.9"), named in the player warnings; ark-stop-reason.py writes it.
STOP_REASON_FILE = LOGS_DIR / "ark-stop-reason.txt"
GAME_USER_SETTINGS = SAVED_DIR / "Config/WindowsServer/GameUserSettings.ini"
# Complete default written by ARK, baked into the image at build time (installer/generate-default-ini.py).
DEFAULT_GAME_USER_SETTINGS = Path("/home/ark-user/config-defaults/GameUserSettings.ini")
# Saved/clusters is a separate volume shared by every instance in the cluster. Instance
# backups and restores leave it alone so restoring one server can't roll back another's
# cluster transfers. Cluster data has its own backup target (ark-cluster-backup.py).
CLUSTER_DIR_NAME = "clusters"
CLUSTER_DIR = SAVED_DIR / CLUSTER_DIR_NAME
# Restore stages into and swaps through these, inside the directory being restored (same volume, so renames).
RESTORE_STAGING_NAME = ".restore-staging"
RESTORE_PREVIOUS_NAME = ".restore-previous"
MAINTENANCE_DIR = Path("/home/ark-user/maintenance")


def server_command(*extra_args: str) -> list[str]:
    """Return the Proton command line that starts ArkAscendedServer.exe, with extra_args before -servergamelog.

    Raises ConfigError if MOD_IDS is set but is not a comma-separated list of mod IDs, or if
    CLUSTER_ID contains characters other than letters, digits, "_" and "-".

    Used by the server start script and by the build-time INI generator
    (installer/generate-default-ini.py).
    """
    proton_base = os.environ.get(
        "PROTON_PATH",
        f"/home/ark-user/.steam/compatibilitytools.d/GE-Proton{os.environ.get('PROTON_VERSION', '')}/",
    )
    map_name = os.environ.get("MAP_NAME") or "TheIsland_WP"
    if not map_name.endswith("_WP"):
        emit_warning(
            "server-args",
            f'MAP_NAME "{map_name}" does not end in _WP. '
            "All current ARK: Survival Ascended maps require the _WP suffix. "
            "The server may fail to start or hang indefinitely.",
        )
    session_name = os.environ.get("SESSION_NAME", "")
    session = f"?SessionName={session_name}" if session_name else ""
    url = f"{map_name}?listen{session}?Port={os.environ.get('ASA_PORT') or '27015'}"
    mod_ids = "".join(os.environ.get("MOD_IDS", "").split())
    if mod_ids and not re.fullmatch(r"\d+(,\d+)*", mod_ids):
        raise ConfigError(f'MOD_IDS must be comma-separated CurseForge mod IDs, got "{mod_ids}"')
    mods = [f"-mods={mod_ids}"] if mod_ids else []
    cluster_id = os.environ.get("CLUSTER_ID", "").strip()
    if cluster_id and not re.fullmatch(r"[A-Za-z0-9_-]+", cluster_id):
        raise ConfigError(f'CLUSTER_ID may contain only letters, digits, "_" and "-", got "{cluster_id}"')
    # No -ClusterDirOverride: the default cluster dir is Saved/clusters, where compose mounts the shared volume.
    cluster = [f"-clusterid={cluster_id}"] if cluster_id else []
    return [
        f"{proton_base.rstrip('/')}/proton",
        "run",
        "./ArkAscendedServer.exe",
        url,
        *mods,
        *cluster,
        *extra_args,
        "-servergamelog",
    ]


class ConfigError(ValueError):
    """An environment setting has an invalid value."""


def env_int(name: str, default: int, *, minimum: int = 1) -> int:
    """Return the named env var as an integer, or default if unset.

    Raises ConfigError if the value is not an integer or is below minimum; each script's
    main() reports it once and exits with its configuration-error code.
    """
    value = os.environ.get(name)
    if value is None:
        return default
    try:
        number = int(value)
    except ValueError:
        number = None
    if number is None or number < minimum:
        raise ConfigError(f"{name} must be an integer of at least {minimum} (got {value!r}).")
    return number


# ARK's reply to a command that has no output of its own (saveworld, DoExit, ServerChat).
NO_OUTPUT_REPLY = "Server received, But no response"


def run_rcon(command: str, *, timeout: float = 10.0) -> bool:
    """Send one RCON command and log its outcome once; return True if the server accepted it."""
    try:
        reply = rcon_command(command, timeout=timeout).strip()
    except RconError as exc:
        emit_error("rcon", f"{command} failed: {exc}")
        return False
    if not reply or reply.startswith(NO_OUTPUT_REPLY):
        emit_event("rcon", f"{command}: accepted by the server (no output)")
    else:
        emit_event("rcon", f"{command}: {reply}")
    return True


def file_size_or_none(path: Path) -> int | None:
    """Return the size in bytes of a regular file, or None if absent or unreadable."""
    if not path.is_file():
        return None
    try:
        return path.stat().st_size
    except OSError:
        return None


LOG_CLOSED_MARKER = "Log file closed,"


def ark_server_pids() -> list[int]:
    """Return the PIDs whose command line contains ArkAscendedServer.exe, excluding this process."""
    own_pid = os.getpid()
    pids = []
    for proc_path in Path("/proc").iterdir():
        if not proc_path.name.isdigit() or int(proc_path.name) == own_pid:
            continue
        try:
            raw = (proc_path / "cmdline").read_bytes()
        except OSError:
            continue
        if b"ArkAscendedServer.exe" in raw:
            pids.append(int(proc_path.name))
    return pids


def ark_server_running() -> bool:
    """Return True if an ArkAscendedServer.exe process is visible in /proc."""
    return bool(ark_server_pids())


def read_log_tail(path: Path, max_bytes: int) -> str | None:
    """Return the last max_bytes of a log file as text, or None if it is missing or unreadable."""
    size = file_size_or_none(path)
    if size is None:
        return None
    try:
        with path.open("rb") as handle:
            handle.seek(max(size - max_bytes, 0))
            return handle.read().decode("utf-8", errors="ignore")
    except OSError:
        return None


def server_log_closed(log_file: Path = SERVER_LOG) -> bool:
    """Return True if ARK's 'Log file closed' line is among the last 50 lines of the server log."""
    tail = read_log_tail(log_file, 64 * 1024)
    return tail is not None and any(LOG_CLOSED_MARKER in line for line in tail.splitlines()[-50:])


def shutdown_warning_minutes() -> int:
    """Minutes the stop script warns online players before saving; the start script budgets for it too."""
    return env_int("SHUTDOWN_WARNING_MINUTES", 5, minimum=0)
