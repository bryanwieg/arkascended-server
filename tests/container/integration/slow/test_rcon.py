from __future__ import annotations

from typing import TYPE_CHECKING

import pytest
from python_on_whales.exceptions import DockerException

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.real_server]

ARK_RCON = ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py"]


def test_rcon_client_against_a_real_server(
    compose_client: DockerClient,
    slow_test_protection,
    started_container_instance: dict[str, str],
    subtests: pytest.Subtests,
) -> None:
    """ark-rcon.py (rcon_client) talks to a real ARK server: replies, empty replies, auth.

    Scenario:
        One RCON-ready instance (the fixture's readiness poll already ran ListPlayers through
        ark-rcon.py) receives several commands, each checked as a subtest so one server start
        covers them all. Saves through backup, the CLI and the stop script, and the stop
        sequence itself, are covered by their own tests and not repeated here.

    Expected:
        - saveworld returns a non-empty reply.
        - ServerChat, which ARK answers with "Server received, But no response!!", succeeds.
        - A wrong password fails with exit code 1 and an authentication error, and the server
          still accepts the right password afterwards (no lockout).
        - The rcon-cli binary is no longer in the image.
    """
    instance = started_container_instance["instance"]

    def rcon(*command: str, envs: dict[str, str] | None = None) -> str:
        return compose_client.compose.execute(instance, [*ARK_RCON, *command], envs=envs or {}, tty=False) or ""

    with subtests.test("saveworld returns a reply"):
        assert rcon("saveworld").strip()

    with subtests.test("command with an empty ARK reply"):
        assert "Server received" in rcon("ServerChat", "rcon client test")

    with subtests.test("wrong password is rejected"):
        with pytest.raises(DockerException) as excinfo:
            rcon("saveworld", envs={"SERVER_ADMIN_PASSWORD": "WrongPass999"})
        assert excinfo.value.return_code == 1
        assert "authentication failed" in (excinfo.value.stderr or ""), excinfo.value.stderr

    with subtests.test("right password still works after a rejected one"):
        assert rcon("saveworld").strip()

    with subtests.test("rcon-cli binary is gone"):
        assert (
            compose_client.compose.execute(instance, ["ls", "/home/ark-user"], tty=False).split().count("rcon-cli") == 0
        )
