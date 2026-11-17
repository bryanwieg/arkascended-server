from __future__ import annotations

import os
import subprocess
import threading
import uuid
from collections.abc import Callable, Generator
from pathlib import Path

import pytest
from helpers.artifacts import tests_root
from helpers.build import build_server_image, image_exists
from helpers.compose import assign_random_ports, disable_restart_policy
from helpers.compose_client import (
    start_container_instance,
)
from helpers.diagnostics import run_with_compose_patch
from helpers.infra import (
    acquire_lock,
    build_container_instance_config,
    cleanup_test_env,
    prepare_test_env,
    release_lock_and_cleanup,
)
from helpers.infra import (
    wait_for as _wait_for,
)


@pytest.fixture(scope="session", autouse=True)
def _ensure_container_image() -> None:
    """Build the container image if missing for this runtime."""
    image = os.environ.get("ARK_TEST_IMAGE", "ark-server:latest")
    if not image_exists(image):
        build_server_image(image)


@pytest.fixture(autouse=True)
def log_streamer(
    test_env: dict[str, str],
    test_artifact_dir: Path,
) -> Generator[None]:
    from helpers.compose_client import (
        discover_services,
        reset_watchers,
        start_all_watchers,
        start_service_watcher,
        stop_all_watchers,
    )

    reset_watchers()

    compose_file = Path(test_env["COMPOSE_FILE"])
    runtime = test_env.get("ARK_CONTAINER_RUNTIME", "docker")

    # Start watchers for all currently-defined services.
    for service in discover_services(compose_file):
        start_service_watcher(compose_file, service, test_artifact_dir, runtime)

    # Slow discovery poll (15s) to catch services created mid-test.
    # This does NOT monitor container health or restart dead watchers.
    _stop_polling = threading.Event()

    def _discovery_loop() -> None:
        while not _stop_polling.wait(timeout=15.0):
            if not compose_file.is_file():
                continue
            start_all_watchers(
                compose_file,
                test_artifact_dir,
                runtime=runtime,
            )

    poller = threading.Thread(target=_discovery_loop, daemon=True)
    poller.start()

    yield

    _stop_polling.set()
    poller.join(timeout=3)
    stop_all_watchers()


@pytest.fixture
def run_cmd() -> Callable[..., subprocess.CompletedProcess[str]]:
    return run_with_compose_patch


@pytest.fixture
def wait_for() -> Callable[[str, int, Callable[[], bool]], None]:
    return _wait_for


@pytest.fixture
def assign_random_ports_fixture() -> Callable[[Path], None]:
    return assign_random_ports


@pytest.fixture
def disable_restart_policy_fixture() -> Callable[[Path], None]:
    return disable_restart_policy


@pytest.fixture
def test_env(
    repo_root: Path,
    test_artifact_dir: Path,
) -> Generator[dict[str, str]]:
    from helpers.compose_client import get_docker_client

    dc = get_docker_client()
    env, compose_file = prepare_test_env(
        dc=dc,
        compose_dir=test_artifact_dir,
        backup_dir=repo_root / "backups",
        compose_prefix="pytest-compose-",
    )
    yield env
    cleanup_test_env(dc, env, compose_file)


@pytest.fixture
def compose_client(test_env: dict[str, str]) -> DockerClient:
    """python-on-whales DockerClient wired to the test's compose file."""
    from helpers.compose_client import get_docker_client

    compose_file = Path(test_env["COMPOSE_FILE"])
    return get_docker_client(compose_file=compose_file)


@pytest.fixture
def container_instance_config(test_env: dict[str, str]) -> dict[str, str]:
    return build_container_instance_config(test_env)


@pytest.fixture
def started_container_instance(
    compose_client: DockerClient,
    test_env: dict[str, str],
    repo_root: Path,
    container_instance_config: dict[str, str],
) -> dict[str, str]:
    """Start a container instance and wait for RCON readiness.

    Uses python-on-whales (compose_client) instead of hand-rolled run_cmd calls.
    """
    return start_container_instance(
        dc=compose_client,
        test_env=test_env,
        repo_root=repo_root,
        instance=container_instance_config["instance"],
        admin_password=container_instance_config["admin_password"],
        server_password=container_instance_config["server_password"],
    )


@pytest.fixture
def started_instance_cli(
    compose_client: DockerClient,
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., subprocess.CompletedProcess[str]],
) -> dict[str, str]:
    instance = f"pytest-ark-{uuid.uuid4().hex[:8]}"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            server_password,
            "--admin-password",
            admin_password,
        ],
        env=test_env,
        cwd=repo_root,
    )

    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    if image != "ark-server:latest":
        compose_client.image.tag(image, "ark-server:latest")

    return start_container_instance(
        dc=compose_client,
        test_env=test_env,
        repo_root=repo_root,
        instance=instance,
        admin_password=admin_password,
        server_password=server_password,
    )


@pytest.fixture
def slow_test_protection(
    request: pytest.FixtureRequest,
    compose_client: DockerClient,
    test_env: dict[str, str],
) -> Generator[None]:
    """Ensures only one slow test runs at a time, with guaranteed cleanup.

    This fixture:
    1. Checks if test is in a 'slow' directory
    2. Acquires lock before test runs
    3. Guarantees cleanup in finally: block even if test crashes
    """
    # Only apply to slow tests
    test_path = Path(request.node.fspath)
    if "slow" not in test_path.parts:
        yield
        return

    # Get lock path

    repo_root = Path(request.config.rootdir) if hasattr(request.config, "rootdir") else Path.cwd()
    lock_path = tests_root(repo_root) / ".lock"

    # Try to acquire lock
    test_name = request.node.name
    if not acquire_lock(lock_path, test_name):
        pytest.skip("Another slow test is currently running. Wait for it to complete.")

    try:
        yield
    finally:
        # Capture diagnostics before teardown — container is still alive here
        from helpers.diagnostics import capture_diagnostics

        docker_log_path = test_env.get("ARK_TEST_DOCKER_LOG", "")
        if docker_log_path:
            capture_diagnostics(compose_client, Path(test_env.get("COMPOSE_FILE", "")), Path(docker_log_path))
            test_env["_ARK_DIAGNOSTICS_CAPTURED"] = "1"
        # Always cleanup - even if test crashes, is killed, or raises exception
        compose_file_raw = test_env.get("COMPOSE_FILE", "")
        compose_file = Path(compose_file_raw) if compose_file_raw else Path()
        release_lock_and_cleanup(compose_client, lock_path, compose_file)


from typing import TYPE_CHECKING  # noqa: E402

if TYPE_CHECKING:
    from python_on_whales import DockerClient
