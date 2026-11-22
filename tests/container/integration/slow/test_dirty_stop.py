from __future__ import annotations

import uuid
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose import write_single_service_compose
from helpers.compose_client import (
    compose_kill,
    compose_logs,
    compose_stop,
    container_exit_code,
    container_exited,
    container_file_exists,
    container_id,
    container_running,
    copy_from_service,
)

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]

_SERVER_LOG = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"


def test_container_start_dirty_stop_workflow(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    container_instance_config: dict[str, str],
) -> None:
    """SIGKILL forces an unclean container exit with a non-zero exit code.

    Scenario:
        A container is started and waited on until RCON is ready, confirming the
        server is fully operational. docker compose kill -s SIGKILL is then issued
        to deliver SIGKILL directly to PID 1, bypassing all shutdown managers and
        graceful stop scripts.

    Expected:
        - The service enters exited status within 60 seconds after SIGKILL.
        - The container's exit code is non-zero, confirming that killed containers
          are distinguishable from clean shutdowns.
    """
    instance = container_instance_config["instance"]

    compose_client.compose.up(services=[instance], detach=True, no_build=True)

    def rcon_ready() -> bool:
        if not container_running(compose_client, instance):
            raise AssertionError("Service exited before RCON became ready.")
        try:
            compose_client.compose.execute(
                instance,
                ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py", "saveworld"],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    rcon_timeout = int(test_env.get("ARK_TEST_RCON_TIMEOUT", "300"))
    wait_for("RCON readiness", rcon_timeout, rcon_ready)

    compose_kill(compose_client, instance, signal="SIGKILL")

    wait_for(
        "service to exit after SIGKILL",
        60,
        lambda: container_exited(compose_client, instance),
    )

    cid = container_id(compose_client, instance)
    assert cid, f"Expected container id for service {instance}."
    exit_code = container_exit_code(compose_client, instance)
    assert exit_code == 137, f"Expected exit code 137 (SIGKILL), got {exit_code}."


def test_docker_compose_stop_unresponsive_rcon_forces_kill_exit_code(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    tmp_path: Path,
) -> None:
    """docker compose stop with RCON unreachable exercises the shutdown manager's force-kill path.

    Scenario:
        Shutdown signal is sent to a hung / unresponsive server process.

        Simulated hung scenario:
        A compose service is generated with RCON_HOST set to 192.0.2.1
        (TEST-NET-1, never assigned), so every RCON call the stop script makes
        times out (10 s), deterministically simulating a server whose RCON
        listener is unavailable (crashed, hung, or not yet started). RCON_PORT
        cannot do this: the entrypoint writes it into GameUserSettings.ini on
        every start, so the server would listen on that port. RCON_HOST is read
        only by the RCON client. GameUserSettings.ini is
        also pre-seeded, so the entrypoint keeps it (does not install the
        image's default) and the runtime manager starts immediately.

        docker compose stop -t 30 is then issued, sending SIGTERM to the
        runtime manager (PID 1). The manager runs the graceful stop script,
        which fails in ~10 s (one save attempt timing out, SAVE_MAX_RETRIES=1,
        SAVE_RETRY_DELAY_SECONDS=1). The manager then
        sends SIGTERM to the game process group and waits for it to exit,
        logging the unconfirmed save/shutdown/stop outcome before exiting
        cleanly — well within the 30-second stop window.

    NOTE — RCON failure is the deliberate condition under test, not a setup
    problem to work around.  Do NOT add a wait for RCON readiness before the
    stop call.  Do NOT increase the -t timeout or raise the retry counts.
    The injected env vars (RCON_HOST, SAVE_MAX_RETRIES, SAVE_RETRY_DELAY)
    are what keep the graceful stop script fast enough for the -t 30 budget.

    Expected:
        - The service exits within 180 seconds.
        - Logs do not contain "installed the image's default" (the pre-seeded INI is kept).
        - Logs contain a warning that the server exited with code -15 (game
          process killed by SIGTERM after graceful stop failed) and that save,
          shutdown, and stop were not confirmed.
        - If the stop status file was written, it contains SAVE_OK=false,
          SHUTDOWN_OK=false, and STOP_OK=false.
    """
    instance = f"pytest-ark-pre-rcon-stop-{uuid.uuid4().hex[:8]}"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    compose_file = Path(test_env["COMPOSE_FILE"])

    write_single_service_compose(
        compose_file,
        instance=instance,
        image=image,
        admin_password=admin_password,
        server_password=server_password,
        extra_env={
            "RCON_HOST": "192.0.2.1",
            "SAVE_MAX_RETRIES": "1",
            "SAVE_RETRY_DELAY_SECONDS": "1",
        },
    )

    # Pre-seed GameUserSettings.ini so the entrypoint keeps it and goes straight to the
    # runtime server manager path under test.
    compose_client.compose.run(
        instance,
        command=[
            "sh",
            "-c",
            (
                "mkdir -p /home/ark-user/ark-server/ShooterGame/Saved/Config/WindowsServer && "
                "mkdir -p /home/ark-user/ark-server/ShooterGame/Saved && "
                "cat > /home/ark-user/ark-server/ShooterGame/Saved/Config/WindowsServer/GameUserSettings.ini <<'EOF'\n"
                "[ServerSettings]\n"
                "RCONEnabled=True\n"
                "RCONPort=27020\n"
                f"ServerAdminPassword={admin_password}\n"
                f"ServerPassword={server_password}\n"
                "EOF"
            ),
        ],
        remove=True,
        entrypoint="",
    )

    compose_client.compose.up(services=[instance], detach=True, no_build=True)

    # Wait for the server's log before issuing the stop, not just a "running" container:
    # ark-server-start.py installs its SIGTERM handler (ctrl.setup_signals()) right after
    # launching the server, and the server writes this log at once. A SIGTERM sent while the
    # container is running but before the handler exists kills the manager outright, so the
    # force-kill path under test never runs (a fast-starting image, like the lite one, hits it).
    wait_for(
        "server process to start before stop",
        120,
        lambda: container_file_exists(compose_client, instance, _SERVER_LOG),
    )

    # Send SIGTERM to the runtime manager.  Because RCON_HOST is unreachable, the
    # graceful stop script's single save attempt times out in ~10 s and the full
    # shutdown sequence completes well within this 30-second window.
    compose_stop(compose_client, instance, timeout=30)

    wait_for(
        "service to exit after early docker compose stop",
        180,
        lambda: container_exited(compose_client, instance),
    )

    cid = container_id(compose_client, instance)
    assert cid, f"Expected container id for service {instance}."

    logs = compose_logs(compose_client, instance, tail=400)

    assert "installed the image's default" not in logs
    assert "Warning: server exited with code -15" in logs
    assert "save was not confirmed" in logs
    assert "shutdown command was not confirmed" in logs

    status_copy = tmp_path / "ark-stop-status-early.env"
    try:  # noqa: SIM105
        copy_from_service(
            compose_client,
            instance,
            "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ark-stop-status.env",
            str(status_copy),
        )
    except Exception:  # noqa: BLE001, S110
        pass
    assert status_copy.is_file(), "Expected ark-stop-status.env to be written by the stop script when RCON fails."
    status_text = status_copy.read_text(encoding="utf-8", errors="ignore")
    assert "SAVE_OK=false" in status_text
    assert "SHUTDOWN_OK=false" in status_text
    assert "STOP_OK=false" in status_text
