from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import container_exit_code, container_exited, container_id, container_running
from helpers.diagnostics import fail_with_container_logs, log_phase_snapshot

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_ark_server_start_integrated_init(  # noqa: PLR0915
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    container_instance_config: dict[str, str],
) -> None:
    """First boot installs the image's default GameUserSettings.ini, then reaches RCON-ready state.

    Scenario:
        A fresh container is started via docker compose up with an empty save volume.
        The entrypoint copies the baked default GameUserSettings.ini into the volume,
        applies the RCON settings, and starts the server (no temporary server, no
        marker files). The test polls in three phases: settings file present, startup
        log readiness, and RCON readiness. A final docker exec checks the INI.

    Expected:
        - Phase 1: GameUserSettings.ini appears inside the container within
          settings_timeout seconds.
        - Phase 2: ShooterGame.log contains the "advertising for join" line within
          startup_log_timeout seconds.
        - Phase 3: ark-rcon.py saveworld succeeds within rcon_timeout seconds.
        - The INI has RCONEnabled=true, RCONPort=27020, the correct
          ServerAdminPassword, and the baked server defaults (AutoSavePeriodMinutes).
    """
    instance = container_instance_config["instance"]
    admin_password = container_instance_config["admin_password"]
    startup_log_timeout = int(test_env.get("ARK_TEST_STARTUP_LOG_TIMEOUT", "300"))
    rcon_timeout = int(test_env.get("ARK_TEST_RCON_TIMEOUT", "300"))
    settings_timeout = int(test_env.get("ARK_TEST_SETTINGS_TIMEOUT", "300"))

    snapshot_kwargs = dict(  # noqa: C408
        dc=compose_client,
        instance=instance,
        compose_file=test_env["COMPOSE_FILE"],
        env=test_env,
        cwd=repo_root,
    )

    def _fail(reason: str) -> None:
        fail_with_container_logs(reason, **snapshot_kwargs)

    # Start the container; the entrypoint installs the default settings on first boot.
    compose_client.compose.up(services=[instance], detach=True, no_build=True)

    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"
    config_path = "/home/ark-user/ark-server/ShooterGame/Saved/Config/WindowsServer/GameUserSettings.ini"

    def settings_installed() -> bool:
        is_running = container_running(compose_client, instance)
        if not is_running and container_exited(compose_client, instance):
            cid = container_id(compose_client, instance)
            exit_c = container_exit_code(compose_client, instance) if cid else None
            if exit_c is not None:
                _fail(f"Service exited before the settings file was installed (exit code {exit_c}).")
        try:
            compose_client.compose.execute(instance, ["sh", "-c", f"test -f {config_path}"])
            return True
        except Exception:  # noqa: BLE001
            return False

    def startup_log_ready() -> bool:
        if not container_running(compose_client, instance):
            if container_exited(compose_client, instance):
                cid = container_id(compose_client, instance)
                exit_c = container_exit_code(compose_client, instance) if cid else None
                if exit_c is not None:
                    _fail(f"Service exited before startup log became ready (exit code {exit_c}).")
                _fail("Service exited before startup log became ready.")
            return False
        try:
            compose_client.compose.execute(
                instance,
                [
                    "sh",
                    "-c",
                    f"test -f {server_log_path} && "  # noqa: ISC004
                    'grep -q "Server has completed startup and is now advertising for join." '
                    f"{server_log_path}",
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    def rcon_ready() -> bool:
        if not container_running(compose_client, instance):
            if container_exited(compose_client, instance):
                cid = container_id(compose_client, instance)
                exit_c = container_exit_code(compose_client, instance) if cid else None
                if exit_c is not None:
                    _fail(f"Service exited before RCON became ready (exit code {exit_c}).")
                _fail("Service exited before RCON became ready.")
            return False
        try:
            compose_client.compose.execute(
                instance, ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py", "saveworld"]
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    # Phase 1: the default settings file should be installed.
    wait_for("default settings installed", settings_timeout, settings_installed)
    log_phase_snapshot("after settings installed", **snapshot_kwargs)

    # Phase 2: server startup log should appear.
    wait_for("startup log readiness", startup_log_timeout, startup_log_ready)
    log_phase_snapshot("after startup log", **snapshot_kwargs)

    # Phase 3: RCON should accept queries.
    wait_for("RCON readiness", rcon_timeout, rcon_ready)
    log_phase_snapshot("after RCON ready", **snapshot_kwargs)

    # Confirm the installed INI has the RCON settings and the baked server defaults.
    try:
        compose_client.compose.execute(
            instance,
            [
                "sh",
                "-c",
                f"test -f {config_path} && "  # noqa: ISC004
                "python3 - <<'PY'\n"
                "import configparser\n"
                "import os\n"
                "import sys\n"
                f"path = {config_path!r}\n"
                f"expected_admin = {admin_password!r}\n"
                "parser = configparser.RawConfigParser(interpolation=None, strict=False)\n"
                "with open(path, encoding='utf-8', errors='ignore') as handle:\n"
                "    parser.read_file(handle)\n"
                "if not parser.has_section('ServerSettings'):\n"
                "    sys.exit(1)\n"
                "section = parser['ServerSettings']\n"
                "enabled = section.get('RCONEnabled', '')\n"
                "port = section.get('RCONPort', '')\n"
                "admin = section.get('ServerAdminPassword', '')\n"
                "if enabled.lower() != 'true' or port != '27020' or admin != expected_admin:\n"
                "    sys.exit(1)\n"
                "if 'autosaveperiodminutes' not in section:\n"
                "    sys.exit(1)\n"
                "PY",
            ],
        )
    except Exception:  # noqa: BLE001
        _fail("GameUserSettings.ini lacks the expected RCON settings or the baked server defaults.")
