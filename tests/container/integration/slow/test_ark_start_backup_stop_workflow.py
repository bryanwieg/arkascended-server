from __future__ import annotations

from collections.abc import Callable
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import compose_exec, compose_stop, container_exited

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_container_start_backup_stop_workflow(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """Full container workflow: startup readiness → backup archive → graceful stop.

    Scenario:
        A running instance (provided by the started_container_instance fixture)
        is targeted in strict sequence. ark-backup.py is invoked to produce a
        .tar archive, retrying up to 3 times with a short sleep between attempts.
        After backup succeeds, docker compose stop sends SIGTERM to PID 1, which
        triggers the runtime shutdown manager to run ark-server-stop.py internally.

    Expected:
        - ark-backup.py exits zero within 3 attempts.
        - Backup stdout contains "[backup] starting backup workflow" and
          "[backup] backup archive created successfully".
        - The archive file is non-empty inside the container.
        - The service enters exited status within 300 seconds after compose stop.
    """
    instance = started_container_instance["instance"]
    output_path = "/home/ark-user/maintenance/pytest-workflow-backup.tar"

    # Step 2: backup once startup fixture has confirmed readiness.
    _backup_stdout: list[str] = []

    def _do_backup() -> bool:
        try:
            _backup_stdout.append(
                compose_exec(
                    compose_client,
                    instance,
                    ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-backup.py", "--output", output_path],
                )
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    wait_for("backup to succeed", 15, _do_backup)
    backup_stdout = _backup_stdout[0]
    assert "[backup] starting backup workflow" in backup_stdout
    assert "[backup] backup archive created successfully" in backup_stdout

    compose_client.compose.execute(
        instance,
        ["test", "-s", output_path],
    )

    # Step 3: stop via docker compose stop (SIGTERM to PID 1) and confirm container exits.
    compose_stop(compose_client, instance, timeout=240)

    wait_for(
        "service to exit after workflow stop",
        300,
        lambda: container_exited(compose_client, instance),
    )
