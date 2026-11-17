"""Baseline framework test — validates framework primitives and demonstrates conventions.

This test exists to:
1. Verify the integration test framework helpers work correctly
2. Serve as the reference implementation for new integration tests
3. Catch framework regressions early

When writing new integration tests:

- Prefer the SDK-based helpers (``compose_exec``, ``compose_run``, ``compose_stop``)
  from ``helpers.compose_client`` over raw ``docker`` / ``podman`` subprocess calls.
- Only request the fixtures your test actually uses. Common pre-SDK boilerplate
  (``test_env``, ``repo_root``) is unnecessary when all operations go through
  the compose SDK client.
- Use ``test_env`` only when you need host-side environment values (port mappings,
  image names, paths). Use ``repo_root`` only when you access host filesystem paths
  (fixture archives, CLI binary).
- ``slow_test_protection`` is a side-effect fixture — include it for any test in a
  ``slow/`` directory to acquire the inter-test lock.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from python_on_whales import DockerClient

import uuid
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose import write_single_service_compose
from helpers.compose_client import (
    compose_exec,
    compose_stop,
    container_exit_code,
    container_running,
    wait_for_container_state,
)
from helpers.infra import prune_orphaned_test_resources

pytestmark = [pytest.mark.container]


def test_baseline_compose_lifecycle(
    compose_client: DockerClient,
    test_env: dict[str, str],
) -> None:
    """Framework correctly manages a container through its full lifecycle using the SDK.

    Exercises: test_env, write compose, compose up via SDK, SDK poll, compose stop.
    New tests should prefer ``compose_exec`` / ``compose_run`` / ``compose_stop``
    helpers over raw ``docker compose exec`` subprocess calls.
    """
    instance = f"pytest-baseline-{uuid.uuid4().hex[:8]}"
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    compose_file = Path(test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_file,
        instance=instance,
        image=image,
        admin_password="AdminPass123",
        server_password="ServerPass123",
        entrypoint='["sh", "-c", "echo baseline-ready && sleep 30"]',
    )

    compose_client.compose.up(services=[instance], detach=True, no_build=True)

    wait_for_container_state(compose_client, instance, state="running", timeout=30)
    assert container_running(compose_client, instance)

    stdout = compose_exec(
        compose_client,
        instance,
        ["echo", "sdk-exec-works"],
    )
    assert "sdk-exec-works" in stdout

    compose_stop(compose_client, instance, timeout=10)

    wait_for_container_state(compose_client, instance, state="exited", timeout=30)

    exit_code = container_exit_code(compose_client, instance)
    assert exit_code is not None


def test_baseline_command_logging(
    test_env: dict[str, str],
    test_artifact_dir: Path,
    run_cmd: Callable[..., object],
) -> None:
    """run_cmd writes every command to commands.log in the test artifact directory.

    Exercises: command logging via ARK_TEST_COMMANDS_LOG.
    """
    commands_log = test_artifact_dir / "commands.log"

    run_cmd(
        ["echo", "baseline-log-check"],
        env=test_env,
        cwd=test_artifact_dir,
        capture_output=True,
    )

    assert commands_log.is_file(), "commands.log should exist after run_cmd"
    content = commands_log.read_text()
    assert "echo baseline-log-check" in content
    assert "exit 0" in content


def test_baseline_artifact_structure(
    test_artifact_dir: Path,
) -> None:
    """Test artifact directory is created and accessible.

    Exercises: test_artifact_dir fixture, artifact directory layout,
    framework.log creation.
    """
    assert test_artifact_dir.is_dir()
    assert test_artifact_dir.name  # non-empty name

    # framework.log should be created by configure_test_artifacts fixture
    framework_log = test_artifact_dir / "framework.log"
    assert framework_log.is_file(), "framework.log should exist"
    content = framework_log.read_text()
    assert "[framework]" in content
    assert content.strip().splitlines()[0].startswith("[")


def test_orphan_cleanup_prunes_labeled_resources(
    compose_client: DockerClient,
    test_env: dict[str, str],
) -> None:
    """Verify prune_orphaned_test_resources removes labeled containers and volumes.

    Creates a minimal container and volume with io.ark.test-suite=true,
    runs the prune function, then asserts they're gone.
    Does NOT need a full ARK container — only a lightweight test image.
    """
    import time as _time

    dc = compose_client

    # Create a labeled volume
    volume_name = f"ark-test-validate-{uuid.uuid4().hex[:8]}"
    dc.volume.create(volume_name, labels={"io.ark.test-suite": "true"})

    # Create a labeled container
    container_name = f"ark-test-validate-{uuid.uuid4().hex[:8]}"
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    dc.container.run(
        image,
        ["echo", "labeled-container-created"],
        name=container_name,
        labels={"io.ark.test-suite": "true"},
        detach=True,
        remove=False,
    )
    _time.sleep(2)

    # Verify they exist before prune
    matching_volumes = dc.volume.list(filters=[("label", "io.ark.test-suite=true")])
    assert volume_name in [v.name for v in matching_volumes], "volume should exist before prune"

    matching_containers = dc.container.list(all=True, filters=[("label", "io.ark.test-suite=true")])
    assert container_name in [c.name for c in matching_containers], "container should exist before prune"

    # Run prune
    prune_orphaned_test_resources(dc)

    # Verify they're gone
    matching_volumes = dc.volume.list(filters=[("label", "io.ark.test-suite=true")])
    assert volume_name not in [v.name for v in matching_volumes], "volume should have been pruned"

    matching_containers = dc.container.list(all=True, filters=[("label", "io.ark.test-suite=true")])
    assert container_name not in [c.name for c in matching_containers], "container should have been removed"
