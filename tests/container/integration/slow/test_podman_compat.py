"""Podman compatibility tests for the container image.

These tests verify that the ARK server image works correctly under Podman,
independent of the CLI's runtime detection. They exercise Podman-specific
command paths (podman run, podman compose) to catch compatibility issues
that Docker-only testing would miss.

Requires: podman 5.0+, a compose provider, and the test image available in
Docker storage (it will be copied into Podman automatically and cleaned up
after the test session).
"""

from __future__ import annotations

import os
import shutil
import subprocess
import uuid
from pathlib import Path

import pytest
from helpers.build import image_exists
from helpers.compose import write_single_service_compose
from helpers.diagnostics import log_command
from helpers.diagnostics import run_cmd as run_cmd_impl
from helpers.infra import wait_for

pytestmark = [pytest.mark.container, pytest.mark.podman]

_SERVER_LOG_PATH = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"


@pytest.fixture(scope="module")
def podman_image() -> str:
    """Ensure the test image is available in Podman storage.

    Copies the image from Docker via `docker save | podman load` if it isn't
    already present in Podman. Removes it from Podman storage after the test
    module completes to avoid doubling disk usage.
    """
    if not shutil.which("podman"):
        pytest.skip("podman is not available")

    podman_env = os.environ.copy()

    result = run_cmd_impl(
        ["podman", "--version"],
        env=podman_env,
        cwd=Path(),
        check=False,
        capture_output=True,
        quiet=True,
    )
    if result.returncode != 0:
        pytest.skip("podman is not functional")

    version_text = result.stdout.strip()
    parts = version_text.split()
    version_str = parts[-1] if parts else ""
    try:
        major = int(version_str.split(".")[0])
    except ValueError, IndexError:
        pytest.skip(f"could not parse podman version: {version_text}")
    if major < 5:
        pytest.skip(f"podman 5.0+ required, found {version_text}")

    compose_check = run_cmd_impl(
        ["podman", "compose", "version"],
        env=podman_env,
        cwd=Path(),
        check=False,
        capture_output=True,
        quiet=True,
    )
    if compose_check.returncode != 0:
        pytest.skip("podman compose provider is not available")

    image = os.environ.get("ARK_TEST_IMAGE", "ark-server:latest")

    already_present = (
        run_cmd_impl(
            ["podman", "image", "inspect", image],
            env=podman_env,
            cwd=Path(),
            check=False,
            capture_output=True,
            quiet=True,
        ).returncode
        == 0
    )

    if not already_present:
        if not shutil.which("docker"):
            pytest.skip("docker is required to copy the image into podman storage")
        docker_has_image = image_exists(image)
        if not docker_has_image:
            pytest.skip(f"image not found in docker storage: {image}")

        pipe = run_cmd_impl(  # noqa: S604
            f"docker save {image} | podman load",
            env=podman_env,
            cwd=Path(),
            check=False,
            capture_output=True,
            shell=True,
        )
        if pipe.returncode != 0:
            pytest.skip(f"failed to copy image to podman: {pipe.stderr.strip()}")

    yield image

    run_cmd_impl(
        ["podman", "rmi", image],
        env=podman_env,
        cwd=Path(),
        check=False,
        capture_output=True,
        quiet=True,
    )


@pytest.fixture
def podman_log(test_artifact_dir: Path) -> Path:
    log_path = test_artifact_dir / "podman.log"
    log_path.write_text("", encoding="utf-8")
    return log_path


def test_podman_server_startup(
    podman_image: str,
    slow_test_protection,
    tmp_path: Path,
    podman_log: Path,
) -> None:
    """Verify the ARK server starts and advertises for join under Podman.

    Creates a full compose service with all required env vars, starts it via
    `podman compose up -d`, and polls the server log inside the container
    until it reports "Server has completed startup and is now advertising
    for join." This is the same success criterion used by the Docker
    integration tests.
    """
    instance = f"pytest-podman-{uuid.uuid4().hex[:8]}"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    compose_file = tmp_path / "compose.yaml"
    write_single_service_compose(
        compose_file,
        instance=instance,
        image=podman_image,
        admin_password=admin_password,
        server_password=server_password,
    )

    podman_env = os.environ.copy()
    podman_env["ARK_TEST_COMMANDS_LOG"] = str(podman_log)

    log_command(
        podman_log,
        ["cat", str(compose_file)],
        subprocess.CompletedProcess(
            args=[],
            returncode=0,
            stdout=compose_file.read_text(),
            stderr="",
        ),
    )

    compose_cmd = ["podman", "compose", "-f", str(compose_file)]
    startup_timeout = int(os.environ.get("ARK_TEST_STARTUP_LOG_TIMEOUT", "600"))

    def fail_with_logs(reason: str) -> None:
        result = run_cmd_impl(
            [*compose_cmd, "logs", "--no-color", "--tail", "200", instance],
            env=podman_env,
            cwd=tmp_path,
            check=False,
            capture_output=True,
        )
        parts = [reason]
        if result.stdout.strip():
            parts.append(f"Recent logs:\n{result.stdout.strip()}")
        raise AssertionError("\n".join(parts))

    # Intentionally local — uses podman compose, not docker compose.
    # Cannot reuse helpers.infra.service_has_status which hardcodes
    # the docker binary.
    def service_has_status(status: str) -> bool:
        result = run_cmd_impl(
            [*compose_cmd, "ps", "--status", status, "--services"],
            env=podman_env,
            cwd=tmp_path,
            check=False,
            capture_output=True,
            quiet=True,
        )
        services = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        return instance in services

    def startup_log_ready() -> bool:
        if not service_has_status("running"):
            if service_has_status("exited"):
                fail_with_logs("Service exited before server advertised for join.")
            return False
        result = run_cmd_impl(
            [
                *compose_cmd,
                "exec",
                "-T",
                instance,
                "sh",
                "-c",
                (
                    f"test -f {_SERVER_LOG_PATH} && "
                    'grep -q "Server has completed startup and is now advertising for join." '
                    f"{_SERVER_LOG_PATH}"
                ),
            ],
            env=podman_env,
            cwd=tmp_path,
            check=False,
            capture_output=True,
            quiet=True,
        )
        return result.returncode == 0

    try:
        up_result = run_cmd_impl(
            [*compose_cmd, "up", "-d", "--no-build", instance],
            env=podman_env,
            cwd=tmp_path,
        )
        assert up_result.returncode == 0, (
            f"podman compose up failed:\nstdout: {up_result.stdout}\nstderr: {up_result.stderr}"
        )

        wait_for("server advertising for join", startup_timeout, startup_log_ready)

        # Log success marker
        log_command(
            podman_log,
            ["# server startup succeeded"],
            subprocess.CompletedProcess(
                args=[],
                returncode=0,
                stdout="Server has completed startup and is now advertising for join.\n",
                stderr="",
            ),
        )
    finally:
        # Capture container logs for diagnostics
        run_cmd_impl(
            [*compose_cmd, "logs", "--no-color", "--timestamps"],
            env=podman_env,
            cwd=tmp_path,
            check=False,
            capture_output=True,
        )
        run_cmd_impl(
            [*compose_cmd, "down", "-v", "--remove-orphans"],
            env=podman_env,
            cwd=tmp_path,
            check=False,
            capture_output=True,
        )
