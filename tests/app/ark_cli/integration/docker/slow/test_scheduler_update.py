from __future__ import annotations

import re
import uuid
from collections.abc import Callable

import pytest
from helpers.compose import pick_free_port
from helpers.compose_client import compose_exec
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.real_server, pytest.mark.usefixtures("remove_new_scheduler_images")]

SCHEDULER = "ark-scheduler"


def _skip_row(output: str, instance: str) -> bool:
    return any(line.startswith(instance) and re.search(r"\|\s*skip\s*\|", line) for line in output.splitlines())


def test_update_schedule_runs_in_scheduler(
    cli: Callable[..., object], compose_client: DockerClient, slow_test_protection
) -> None:
    """An update schedule run inside the scheduler container reaches Steam over HTTPS.

    Scenario:
        A real instance is created; host update instance --check must show action skip
        (the test skips otherwise). The instance is started, an update schedule `upd`
        created and the scheduler enabled. `/ark-cli run schedule --name upd` is executed
        in the ark-scheduler container.

    Expected:
        - The exec exits zero and prints `SUCCESS upd`.
        - The output has the instance's row with action skip and `local image is up to date`.
        - The instance runs the same image as before and check instance exits zero.
    """
    instance = f"pytest-ark-{uuid.uuid4().hex[:8]}"
    cli(
        "create",
        "instance",
        "--name",
        instance,
        "--server-password",
        "ServerPass123",
        "--admin-password",
        "AdminPass123",
    )
    check = cli("update", "instance", "--instance", instance, "--check").stdout
    if not _skip_row(check, instance):
        pytest.skip(f"ark-server:latest is not Steam's current build; run a host update instance first:\n{check}")
    cli("start", "instance", "--instance", instance)  # waits for ready
    image_before = compose_client.compose.ps(services=[instance])[0].image
    cli("create", "schedule", "update", "--instance", instance, "--name", "upd", "--cron", "0 4 * * *")
    cli("enable", "scheduler", "--health-port", str(pick_free_port()))

    output = compose_exec(compose_client, SCHEDULER, ["/ark-cli", "run", "schedule", "--name", "upd"]) or ""

    assert "SUCCESS upd" in output
    assert _skip_row(output, instance), output
    assert "local image is up to date" in output
    assert compose_client.compose.ps(services=[instance])[0].image == image_before
    cli("check", "instance", "--instance", instance)
