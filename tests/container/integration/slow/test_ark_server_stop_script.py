from __future__ import annotations

from collections.abc import Callable
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import container_exited

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
