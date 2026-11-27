from __future__ import annotations

import re
import uuid
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose_client import container_running
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.lite]


def _ark_server_tags(dc: DockerClient) -> set[str]:
    return {tag for image in dc.image.list() for tag in image.repo_tags if tag.startswith("ark-server:")}


def test_update_schedule_skips_stopped_instance(
    cli: Callable[..., object], compose_client: DockerClient, env: dict[str, str]
) -> None:
    """An update schedule skips a stopped instance and builds nothing.

    Scenario:
        An instance is created but not started. An update schedule targets it and is run.

    Expected:
        - run schedule exits zero and warns that the instance is stopped and the command unsuccessful.
        - check schedule shows last_status skipped.
        - No new ark-server tag exists, the compose file (and so the instance's image) is
          unchanged, and the instance is still not running.
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
    cli("create", "schedule", "update", "--instance", instance, "--name", "stopped-update", "--cron", "0 4 * * *")
    compose_file = Path(env["COMPOSE_FILE"])
    compose_before = compose_file.read_text()
    tags_before = _ark_server_tags(compose_client)

    result = cli("run", "schedule", "--name", "stopped-update")

    assert f"Warning: {instance} is stopped; update command unsuccessful" in result.stdout + result.stderr
    assert re.search(r"last_status:\s+skipped", cli("check", "schedule", "--name", "stopped-update").stdout)
    assert _ark_server_tags(compose_client) == tags_before
    assert compose_file.read_text() == compose_before
    assert not container_running(compose_client, instance)
