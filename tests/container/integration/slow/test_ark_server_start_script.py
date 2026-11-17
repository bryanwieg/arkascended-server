from __future__ import annotations

from typing import TYPE_CHECKING

import pytest

from tests.helpers.compose_client import container_running

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_ark_server_start_script(
    compose_client: DockerClient,
    slow_test_protection,
    started_container_instance: dict[str, str],
) -> None:
    """Container entrypoint starts the ARK server and the service reaches running status.

    Scenario:
        A container instance is started by the started_container_instance fixture,
        which handles the full entrypoint startup (default settings, RCON settings) and server
        launch. The test performs two quick docker checks against the live container.

    Expected:
        - ShooterGame.log exists inside the container, confirming the game server
          process has launched and written its startup log.
        - The service appears in docker compose ps --status running, confirming
          the container is healthy from Docker's perspective.
    """
    instance = started_container_instance["instance"]
    compose_client.compose.execute(
        instance,
        ["test", "-f", "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"],
    )
    assert container_running(compose_client, instance)
