"""Fixtures for web integration tests."""

from __future__ import annotations

import os
import subprocess
from collections.abc import Generator
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from helpers.web_client import WebClient


@pytest.fixture(scope="session")
def web_binary(repo_root: Path) -> Path:
    """Build (if needed) and return the path to the ark-web binary."""
    binary = repo_root / "app" / "ark-web" / "ark-web"
    if not binary.exists():
        subprocess.run(
            ["go", "build", "-o", str(binary), "./app/ark-web/"],
            cwd=str(repo_root),
            check=True,
            capture_output=True,
        )
    return binary


@pytest.fixture
def web_test_env(
    test_env: dict[str, str],
) -> dict[str, str]:
    """Extend test_env with a compose file the web app can discover.

    The web app scans for docker-compose*.yaml, but test_env creates
    pytest-compose-*.yaml. This fixture creates a symlink with the
    expected prefix so the web app discovers the test compose file.
    """
    compose_file = Path(test_env["COMPOSE_FILE"])
    web_compose = compose_file.parent / f"docker-compose.{compose_file.stem}.yaml"
    if not web_compose.exists():
        web_compose.symlink_to(compose_file.name)

    env = dict(test_env)
    env["COMPOSE_FILE"] = str(compose_file)
    return env


@pytest.fixture
def start_web_server(
    web_binary: Path,
    web_test_env: dict[str, str],
    test_artifact_dir: Path,
) -> Generator[tuple[WebClient, int]]:
    """Start ark-web pointed at the test compose directory, yield WebClient.

    The server binds to a random free port on 127.0.0.1. Yields
    (web_client, port). Stops the server on teardown.
    """
    import socket

    from helpers.web_client import WebClient, wait_for_web_ready

    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]

    compose_dir = Path(web_test_env["COMPOSE_FILE"]).parent
    env = os.environ.copy()
    env["ARK_WEB_ADDR"] = f"127.0.0.1:{port}"

    commands_log = test_artifact_dir / "commands.log"

    proc = subprocess.Popen(
        [str(web_binary)],
        cwd=str(compose_dir),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    client = WebClient(f"http://127.0.0.1:{port}", commands_log=str(commands_log))

    try:
        wait_for_web_ready(client, timeout=15)
    except RuntimeError:
        proc.kill()
        out, err = proc.communicate(timeout=5)
        pytest.fail(
            f"Web server failed to start.\n"
            f"stdout:\n{out.decode(errors='replace')}\n"
            f"stderr:\n{err.decode(errors='replace')}"
        )

    yield client, port

    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()

    client.close()
