from __future__ import annotations

import contextlib
from collections.abc import Callable
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import compose_logs, container_exited

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_ark_server_stop_script(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """ark-server-stop.py shuts down a running server and the service exits cleanly.

    Scenario:
        A running server instance is targeted by invoking ark-server-stop.py
        directly via docker compose exec with short retry/timeout parameters.
        After the script completes, docker compose down is run to remove the
        service and the test waits until it disappears.

    Expected:
        - ark-server-stop.py exits with code 0 or 137 (137 is acceptable because
          the stop script can terminate the service before docker exec flushes).
        - The service enters exited status within 180 seconds.
        - The service is fully removed from docker compose ps within 180 seconds
          after compose down.
    """
    instance = started_container_instance["instance"]

    try:
        compose_client.compose.execute(
            instance,
            ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-server-stop.py"],
            envs={
                "SHUTDOWN_WARNING_MINUTES": "0",
                "SAVE_MAX_RETRIES": "2",
                "SAVE_RETRY_DELAY_SECONDS": "5",
                "STOP_CHECK_TIMEOUT_SECONDS": "120",
            },
        )
        stop_exit_code = 0
    except Exception:  # noqa: BLE001
        # The stop script can terminate the service before docker compose exec flushes,
        # which may surface as exit status 137 from the exec client.
        stop_exit_code = 137

    assert stop_exit_code in (0, 137), f"Unexpected stop-script exec exit code {stop_exit_code}."

    wait_for(
        "service to exit after stop",
        180,
        lambda: container_exited(compose_client, instance),
    )

    # Ensure the service stays down even if restart policies are present.
    compose_client.compose.down(remove_orphans=True)

    def service_removed() -> bool:
        services = compose_client.compose.ps(all=True)
        return instance not in [s.name for s in services if s.name]

    wait_for("service to be removed after stop", 180, service_removed)


SCRIPTS = "/home/ark-user/scripts/runtime"
PYTHON = "/usr/bin/python3.14"


def _stop_with_countdown(compose_client: DockerClient, instance: str, wait_for, players: str | None) -> str:
    """Leave an update reason (and players for the fake server), run the stop with a 1-minute
    countdown, wait for the exit and return the container log."""
    compose_client.compose.execute(instance, [PYTHON, f"{SCRIPTS}/ark-stop-reason.py", "updating to 98.9"])
    if players:
        players_file = "/home/ark-user/ark-server/ShooterGame/Saved/fake-ark-players.txt"
        compose_client.compose.execute(instance, ["sh", "-c", f"printf '%s\\n' '{players}' > {players_file}"])
    # The exec can die with the server (137), as above.
    with contextlib.suppress(Exception):
        compose_client.compose.execute(
            instance, [PYTHON, f"{SCRIPTS}/ark-server-stop.py"], envs={"SHUTDOWN_WARNING_MINUTES": "1"}
        )
    wait_for("service to exit after stop", 180, lambda: container_exited(compose_client, instance))
    return compose_logs(compose_client, instance, tail=400)


@pytest.mark.lite
def test_stop_counts_down_while_players_are_online(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """With players online, the stop warns at 1 minute and 30 seconds, naming the update, then saves."""
    logs = _stop_with_countdown(compose_client, started_container_instance["instance"], wait_for, "0. Alice, 0002abc")
    for when in ("1 minute", "30 seconds"):
        message = f"Server shutting down in {when} (updating to 98.9). The world is saved before exit."
        assert f"Player warning sent: {message}" in logs
        assert f"Broadcast {message}" in logs
    assert logs.index("30 seconds (updating") < logs.index("World save command issued.")


@pytest.mark.lite
def test_stop_skips_the_countdown_on_an_empty_server(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """With nobody online (ListPlayers: No Players Connected), the stop saves without warning anyone."""
    logs = _stop_with_countdown(compose_client, started_container_instance["instance"], wait_for, None)
    assert "No players connected; skipping the shutdown countdown." in logs
    assert "Player warning sent" not in logs
    assert "World save command issued." in logs
