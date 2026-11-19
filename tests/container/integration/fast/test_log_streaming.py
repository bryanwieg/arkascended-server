"""Validate per-service log streaming in the integration test framework."""

from __future__ import annotations

import uuid
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose import write_single_service_compose
from helpers.compose_client import (
    compose_stop,
    wait_for_container_state,
)

if TYPE_CHECKING:
    from python_on_whales import DockerClient
from helpers.infra import wait_for as framework_wait_for

pytestmark = [pytest.mark.container, pytest.mark.lite]


def test_per_service_log_streaming(
    compose_client: DockerClient,
    test_env: dict[str, str],
    test_artifact_dir: Path,
) -> None:
    """Per-service streaming log files exist and contain container output.

    Exercises: log_streamer fixture, per-service compose.<service>.log files.
    """
    instance = f"pytest-stream-{uuid.uuid4().hex[:8]}"
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    compose_file = Path(test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_file,
        instance=instance,
        image=image,
        admin_password="AdminPass123",
        server_password="ServerPass123",
        entrypoint='["sh", "-c", "sleep 2 && echo stream-ready && sleep 10"]',
    )

    compose_client.compose.up(services=[instance], detach=True, no_build=True)

    stream_log = test_artifact_dir / f"compose.{instance}.log"
    framework_wait_for(
        f"streaming log file {stream_log.name}",
        20,
        lambda: stream_log.is_file(),  # noqa: PLW0108
    )
    framework_wait_for(
        f"stream-ready in {stream_log.name}",
        15,
        lambda: stream_log.is_file() and "stream-ready" in stream_log.read_text(),
    )

    assert stream_log.is_file(), f"Expected streaming log file {stream_log} to exist"
    content = stream_log.read_text()
    assert "stream-ready" in content, f"Expected 'stream-ready' in streaming log, got:\n{content}"

    compose_stop(compose_client, instance, timeout=10)
    wait_for_container_state(compose_client, instance, state="exited", timeout=30)
