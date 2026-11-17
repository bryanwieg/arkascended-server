from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose import asa_port_from_compose
from helpers.compose_client import compose_logs, container_env, get_container_port

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def _assert_new_scheme_consistent(compose_path: Path, expected_port: str) -> None:
    """Assert the compose file has a consistent new-scheme port config."""
    text = compose_path.read_text(encoding="utf-8")
    assert f"{expected_port}:{expected_port}/udp" in text, (
        f"expected 1:1 binding {expected_port}:{expected_port}/udp in compose"
    )
    assert re.search(rf"ark\.port\.asa:\s*\"?{expected_port}\"?", text), (
        f"expected ark.port.asa={expected_port} in compose"
    )
    assert re.search(r"ark\.port\.rcon:\s*\"?\d+\"?", text), "expected ark.port.rcon label in compose"
    assert f"ASA_PORT={expected_port}" in text, f"expected ASA_PORT={expected_port} in compose"


def test_container_listens_on_configured_asa_port(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """The ARK server inside the container binds on the port specified by ASA_PORT.

    Scenario:
        An instance is created (conftest randomises ports). The assigned
        ASA port is read from the compose file's ark.port.asa label. The
        instance is started and waited on until the startup log confirms
        the server is advertising for join. The test then verifies:
        (a) the ASA_PORT env var inside the container matches the label,
        (b) the docker port binding is 1:1 (published == container target),
        (c) the server process was started with ?Port=<assigned port> in
            its command-line arguments.

    Expected:
        - ASA_PORT inside the container equals the compose label value.
        - docker compose port reports the assigned port on both sides.
        - The server startup log contains ?Port=<assigned port>.
    """
    instance = "pytest-ark-port-listen-1"
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

    compose_path = Path(test_env["COMPOSE_FILE"])
    expected_port = asa_port_from_compose(compose_path, instance)

    # Verify compose consistency before starting.
    _assert_new_scheme_consistent(compose_path, expected_port)

    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "started"],
        env=test_env,
        cwd=repo_root,
    )

    # (a) Verify ASA_PORT env var inside the running container.
    container_asa_port = container_env(compose_client, instance, "ASA_PORT")
    assert container_asa_port == expected_port, (
        f"expected ASA_PORT={expected_port} inside container, got: {container_asa_port}"
    )

    # (b) Verify docker port binding is 1:1.
    port_output = get_container_port(compose_client, instance, int(expected_port), protocol="udp")
    assert port_output is not None, f"expected port {expected_port} binding, got None"
    assert port_output == expected_port, f"expected host port {expected_port} in binding, got: {port_output}"

    # (c) Verify the server process was started with ?Port=<expected_port>.
    logs_result = compose_logs(compose_client, instance, tail=200)
    assert f"?Port={expected_port}" in logs_result, (
        f"expected ?Port={expected_port} in server startup logs, got:\n{logs_result[:2000]}"
    )

    # show instance must report the correct port.
    show_result = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert f"port:     {expected_port}" in show_result.stdout, (
        f"expected port: {expected_port} in show output: {show_result.stdout}"
    )
