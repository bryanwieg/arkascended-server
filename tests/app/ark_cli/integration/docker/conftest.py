from __future__ import annotations

import os

import pytest

from tests.app.ark_cli.integration.conftest import *  # noqa: F403


@pytest.fixture(scope="session", autouse=True)
def _set_docker_env() -> None:
    os.environ["ARK_CONTAINER_RUNTIME"] = "docker"


@pytest.fixture
def test_env(test_env):
    """Override test_env to use Docker runtime."""
    env = dict(test_env)
    env["ARK_CONTAINER_RUNTIME"] = "docker"
    return env
