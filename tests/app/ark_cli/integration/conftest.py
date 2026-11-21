from __future__ import annotations

import os

import pytest

# CLI integration tests need the same Docker/container fixtures as container
# integration tests (test_env, run_cmd, started_instance_cli, etc.).
# Re-export them via wildcard import so pytest discovers them as local fixtures.
from tests.container.integration.conftest import *  # noqa: F403


@pytest.fixture
def scheduler_socket_env() -> dict[str, str]:
    """Extra env for ark-cli calls that start the scheduler: the engine socket it mounts.

    ark-cli derives the socket from a unix DOCKER_HOST; over tcp:// it cannot, so the
    daemon-side socket comes from SANDBOX_DOCKER_SOCKET (rootless daemon: no gid needed).
    """
    docker_host = os.environ.get("DOCKER_HOST", "")
    if not docker_host.startswith("tcp://"):
        return {}
    socket = os.environ.get("SANDBOX_DOCKER_SOCKET")
    if not socket:
        pytest.skip("the engine's unix socket is not known; set ARK_SCHEDULER_SOCKET")
    return {"ARK_SCHEDULER_SOCKET": socket}
