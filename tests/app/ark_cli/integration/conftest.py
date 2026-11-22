from __future__ import annotations

import os
from collections.abc import Callable, Iterator
from pathlib import Path

import pytest
from helpers.compose_client import get_docker_client

# CLI integration tests need the same Docker/container fixtures as container
# integration tests (test_env, run_cmd, started_instance_cli, etc.).
# Re-export them via wildcard import so pytest discovers them as local fixtures.
from tests.container.integration.conftest import *  # noqa: F403

SCHEDULER = "ark-scheduler"


@pytest.fixture
def scheduler_socket_env() -> dict[str, str]:
    """Extra env for ark-cli calls that start the scheduler: the engine socket it mounts.

    ark-cli derives the socket from a unix DOCKER_HOST; over tcp:// it cannot, so the
    daemon-side socket comes from ARK_SCHEDULER_SOCKET, else SANDBOX_DOCKER_SOCKET
    (rootless daemon: no gid needed).
    """
    if not os.environ.get("DOCKER_HOST", "").startswith("tcp://"):
        return {}
    socket = os.environ.get("ARK_SCHEDULER_SOCKET") or os.environ.get("SANDBOX_DOCKER_SOCKET")
    if not socket:
        pytest.skip("the engine's unix socket is not known; set ARK_SCHEDULER_SOCKET")
    return {"ARK_SCHEDULER_SOCKET": socket}


# Scheduler tests (docker/fast, docker/slow and their podman copies) share the fixtures below.


@pytest.fixture(scope="module")
def remove_new_scheduler_images() -> Iterator[None]:
    """Remove the ark-scheduler images a module built; images that existed before it stay."""
    dc = get_docker_client()
    before = {image.id for image in dc.image.list(SCHEDULER)}
    yield
    for image in dc.image.list(SCHEDULER):
        if image.id not in before:
            dc.image.remove([t for t in image.repo_tags if t.startswith(f"{SCHEDULER}:")] or image.id)


@pytest.fixture
def env(test_env: dict[str, str], scheduler_socket_env: dict[str, str]) -> dict[str, str]:
    """test_env plus the scheduler's engine socket (skips when the socket is not known)."""
    return {**test_env, **scheduler_socket_env}


@pytest.fixture
def compose_dir(env: dict[str, str]) -> Path:
    """ark-cli runs from the compose dir: enable scheduler refuses any other cwd."""
    return Path(env["COMPOSE_FILE"]).parent


@pytest.fixture
def cli(
    env: dict[str, str], compose_dir: Path, cli_path: Path, run_cmd: Callable[..., object]
) -> Callable[..., object]:
    def _cli(*args: str, check: bool = True) -> object:
        return run_cmd([str(cli_path), *args], env=env, cwd=compose_dir, check=check, capture_output=True)

    return _cli
