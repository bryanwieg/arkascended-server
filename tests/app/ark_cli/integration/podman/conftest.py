"""Podman-specific conftest: sets runtime to podman and ensures the image exists.

Scenario:
    The Podman test suite re-exports docker CLI integration tests with the
    container runtime overridden to podman. This conftest sets
    ARK_CONTAINER_RUNTIME=podman at module load time and provides a
    session-scoped fixture that builds the server image if missing.

Expected:
    - All podman tests run with ARK_CONTAINER_RUNTIME=podman.
    - The server image is available before any test executes.
"""

from __future__ import annotations

import os

import pytest
from helpers.build import build_server_image, image_exists

from tests.app.ark_cli.integration.conftest import *  # noqa: F403

# Set podman runtime at module load time so the root-level and container
# conftest fixtures pick up the right binary.
os.environ["ARK_CONTAINER_RUNTIME"] = "podman"


@pytest.fixture(scope="session", autouse=True)
def _ensure_podman_image() -> None:
    """Build the image if missing when running Podman tests."""
    image = os.environ.get("ARK_TEST_IMAGE", "ark-server:latest")
    if not image_exists(image):
        build_server_image(image)


@pytest.fixture
def test_env(test_env):
    """Override test_env to use Podman runtime."""
    env = dict(test_env)
    env["ARK_CONTAINER_RUNTIME"] = "podman"
    return env
