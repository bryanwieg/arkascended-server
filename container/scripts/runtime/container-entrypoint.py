#!/usr/bin/env python3.14
"""Runtime startup decision logic for ARK container entrypoint."""

from __future__ import annotations

import os
import shutil
import sys
from pathlib import Path

from _logging import emit_error, emit_event, emit_warning
from ini_settings import update_config
from runtime_utils import (
    CLUSTER_DIR,
    DEFAULT_GAME_USER_SETTINGS,
    GAME_USER_SETTINGS,
    PYTHON,
    SAVED_DIR,
    SCRIPTS_DIR,
)

CGROUP_V2_MEM_LOW = Path("/sys/fs/cgroup/memory.low")
CGROUP_V1_MEM_SOFT_LIMIT = Path("/sys/fs/cgroup/memory/memory.soft_limit_in_bytes")
MEMINFO = Path("/proc/meminfo")
PSI_MEMORY = Path("/proc/pressure/memory")
# Host kernel parameters checked at startup (read-only from inside the container).
HOST_KERNEL_PARAMS = [
    ("/proc/sys/vm/max_map_count", "vm.max_map_count", 16_777_216),
    ("/proc/sys/net/core/rmem_max", "net.core.rmem_max", 16_777_216),
    ("/proc/sys/net/core/wmem_max", "net.core.wmem_max", 16_777_216),
    ("/proc/sys/fs/file-max", "fs.file-max", 1_048_576),
]

SERVER_START_SCRIPT = str(SCRIPTS_DIR / "ark-server-start.py")
RESTORE_SCRIPT = str(SCRIPTS_DIR / "ark-restore.py")
VOLUME_PATHS = [SAVED_DIR, CLUSTER_DIR]
MOUNTINFO = Path("/proc/self/mountinfo")


def _read_cgroup_mem_reservation_bytes() -> int | None:
    """Return the container memory reservation in bytes from cgroup, or None if unset/unlimited."""
    if CGROUP_V2_MEM_LOW.exists():
        text = CGROUP_V2_MEM_LOW.read_text(encoding="utf-8").strip()
        if text == "max":
            return None
        try:
            return int(text)
        except ValueError:
            pass
    if CGROUP_V1_MEM_SOFT_LIMIT.exists():
        text = CGROUP_V1_MEM_SOFT_LIMIT.read_text(encoding="utf-8").strip()
        try:
            value = int(text)
            if value >= (1 << 62):  # very large → no reservation set
                return None
            return value
        except ValueError:
            pass
    return None


def _read_meminfo_bytes() -> dict[str, int]:
    """Return /proc/meminfo values in bytes keyed by field name, or {} if unreadable."""
    try:
        lines = MEMINFO.read_text(encoding="ascii").splitlines()
    except OSError:
        return {}
    meminfo = {}
    for line in lines:
        parts = line.split()
        if len(parts) >= 2 and parts[1].isdigit():
            meminfo[parts[0].rstrip(":")] = int(parts[1]) * 1024
    return meminfo


def _memory_pressure(meminfo: dict[str, int]) -> str | None:
    """Describe current host memory pressure (PSI stalls or swap over 90% full), or None if there is none."""
    try:
        for line in PSI_MEMORY.read_text(encoding="ascii").splitlines():
            if line.startswith("full "):
                avg10 = float(line.split()[1].removeprefix("avg10="))
                if avg10 > 0:
                    return f"memory stalls in the last 10 s, PSI full avg10={avg10:.2f}"
    except OSError, ValueError, IndexError:
        pass  # PSI unavailable: fall through to the swap check
    swap_total = meminfo.get("SwapTotal", 0)
    if swap_total:
        used_pct = (swap_total - meminfo.get("SwapFree", 0)) / swap_total * 100
        if used_pct > 90:
            return f"swap {used_pct:.0f}% full"
    return None


def check_host_kernel_params() -> None:
    """Warn for any host kernel parameters below the Proton/Wine minimums."""
    for proc_path, sysctl_name, minimum in HOST_KERNEL_PARAMS:
        try:
            value = int(Path(proc_path).read_text(encoding="utf-8").strip())
        except OSError, ValueError:
            emit_warning("entrypoint", f"could not read {proc_path}; skipping check.")
            continue
        if value < minimum:
            emit_warning(
                "entrypoint",
                f"{sysctl_name}={value} is below the recommended minimum of "
                f"{minimum} for Proton/Wine. The server may crash or fail to start. "
                "Run 'ark-cli fix host-kernel-config' on the Docker host "
                "to apply the required setting.",
            )


def check_memory_headroom() -> None:
    """Warn when host RAM is below this container's memory reservation; never block the start.

    Blocking looped forever under `restart: unless-stopped`, and low MemAvailable is normal on
    balloon-memory VMs. A server that does run out of memory is reported by ark-server-start.py.
    """
    if os.environ.get("ARK_SKIP_RAM_CHECK") == "1":
        return
    reservation = _read_cgroup_mem_reservation_bytes()
    if not reservation:
        return  # unset (cgroup v2 reads 0), or the runtime didn't apply mem_reservation
    meminfo = _read_meminfo_bytes()
    available = meminfo.get("MemAvailable")
    if available is None or available >= reservation:
        return
    low = (
        f"low host RAM: need {reservation / 1024**3:.1f} GB (container mem_reservation), "
        f"{available / 1024**3:.1f} GB available"
    )
    pressure = _memory_pressure(meminfo)
    if pressure:
        emit_warning(
            "entrypoint",
            f"{low}, and the host is already under memory pressure ({pressure}). "
            "The server will likely be killed by the OOM killer; free memory on the host.",
        )
    else:
        emit_warning(
            "entrypoint",
            f"{low}. The server may run out of memory. On a balloon-memory VM "
            "(e.g. Hyper-V Dynamic Memory) this is expected: memory is added on demand.",
        )


def _mount_points() -> set[str] | None:
    """Return the container's mount points from /proc/self/mountinfo, or None if unreadable."""
    try:
        lines = MOUNTINFO.read_text(encoding="utf-8").splitlines()
    except OSError:
        return None
    # Field 5 is the mount point; the kernel escapes space, tab, newline and backslash as octal.
    return {
        fields[4].replace("\\040", " ").replace("\\011", "\t").replace("\\012", "\n").replace("\\134", "\\")
        for line in lines
        if len(fields := line.split()) > 4
    }


def check_volume_mounts() -> bool:
    """Verify all required data volume paths exist and are writable; return False on failure."""
    failed = False
    user_label = os.environ.get("USER") or str(os.getuid())
    emit_event("entrypoint", f"validating writable data mounts for {user_label}.")

    for path in VOLUME_PATHS:
        if not path.is_dir():
            emit_error("entrypoint", f"data directory missing or not a directory: {path}")
            failed = True
            continue
        if not os.access(path, os.W_OK):
            emit_error("entrypoint", f"data directory not writable for {user_label}: {path}")
            failed = True

    if failed:
        emit_error("entrypoint", "data directory write check failed; refusing to start server.")
        return False

    # The image ships these directories, so without a volume they still exist and are writable:
    # the world would be saved into the container itself and lost when it is recreated.
    mounts = _mount_points()
    if mounts is None:
        emit_warning("entrypoint", f"could not read {MOUNTINFO}; skipping the volume mount check.")
        return True
    if str(SAVED_DIR) not in mounts:
        emit_error(
            "entrypoint",
            f"no volume is mounted at {SAVED_DIR}; world saves would be lost when the "
            "container is recreated. Mount a volume there (the ark-cli compose template does); "
            "refusing to start server.",
        )
        return False
    if str(CLUSTER_DIR) not in mounts:
        emit_warning(
            "entrypoint",
            f"no volume is mounted at {CLUSTER_DIR}; cluster transfers will be lost when the container is recreated.",
        )
    return True


def apply_rcon_settings() -> bool:
    """Re-apply RCON and password settings from the environment; return False if that is impossible.

    Re-applying on every start keeps the INI in step with the compose env, so a changed SERVER_ADMIN_PASSWORD or RCON_PORT doesn't leave the
    stop and backup scripts unable to authenticate (every stop would become a forced kill).
    """
    admin_password = os.environ.get("SERVER_ADMIN_PASSWORD", "")
    if not admin_password:
        # Not fatal: the server runs without RCON; only saves before stop and backups need it.
        emit_warning(
            "entrypoint",
            "SERVER_ADMIN_PASSWORD is not set; RCON settings were not applied. "
            "Saves before stop and backups will fail until it is set "
            "(ark-cli reconfigure instance --admin-password ...).",
        )
        return True
    try:
        update_config(
            GAME_USER_SETTINGS,
            admin_password,
            os.environ.get("RCON_PORT", "27020"),
            os.environ.get("SERVER_PASSWORD", ""),
        )
    except OSError as exc:
        emit_error("entrypoint", f"failed to apply RCON settings to {GAME_USER_SETTINGS}: {exc}")
        return False
    emit_event("entrypoint", f"RCON and password settings applied to {GAME_USER_SETTINGS.name} from the environment.")
    return True


def install_default_settings() -> bool:
    """Install the image's default GameUserSettings.ini if the volume has none; return False if that fails.

    Runs on every start, so a new volume of any kind (bind mount, Kubernetes volume, restored data)
    gets ARK's complete defaults. An existing file is never touched.
    """
    if GAME_USER_SETTINGS.is_file():
        return True
    partial = GAME_USER_SETTINGS.with_name(f"{GAME_USER_SETTINGS.name}.installing")
    try:
        GAME_USER_SETTINGS.parent.mkdir(parents=True, exist_ok=True)
        # Copy then rename: an interrupted copy must not leave a partial INI that later starts would keep.
        shutil.copyfile(DEFAULT_GAME_USER_SETTINGS, partial)
        partial.replace(GAME_USER_SETTINGS)
    except OSError as exc:
        emit_error("entrypoint", f"failed to install the default GameUserSettings.ini: {exc}")
        return False
    emit_event("entrypoint", "no GameUserSettings.ini found; installed the image's default.")
    return True


def run_default_startup() -> int:
    """Run host checks, install the default settings if missing, apply RCON settings, then exec the server manager."""
    emit_event("entrypoint", "default server startup workflow selected.")

    check_memory_headroom()
    check_host_kernel_params()

    if not check_volume_mounts():
        return 1

    if not install_default_settings() or not apply_rcon_settings():
        return 1

    emit_event("entrypoint", "launching runtime server manager via exec.")
    os.execv(PYTHON, [PYTHON, SERVER_START_SCRIPT])  # noqa: S606
    return 0  # never reached (execv replaces the process); ruff RET503 wants an explicit return


def main() -> int:
    """Exec an explicit command override if provided, otherwise run the default startup path."""
    if len(sys.argv) > 1:
        # Nothing may reach stdout before the command runs: callers read its output
        # (e.g. `docker run <image> cat /home/ark-user/ARK_SERVER_VERSION` in build-image.sh).
        # Restore must run in maintenance mode (no game server process active).
        # Stamp the mode here so the restore script can enforce safe invocation.
        if RESTORE_SCRIPT in sys.argv[1:]:
            os.environ["ARK_MAINTENANCE_MODE"] = "restore"
        try:
            os.execvp(sys.argv[1], sys.argv[1:])  # noqa: S606
        except FileNotFoundError:
            emit_error("entrypoint", f"command not found: {sys.argv[1]}")
            return 127
        except OSError as exc:
            emit_error("entrypoint", f"failed to execute command {sys.argv[1]!r}: {exc}")
            return 126

    return run_default_startup()


if __name__ == "__main__":
    raise SystemExit(main())
