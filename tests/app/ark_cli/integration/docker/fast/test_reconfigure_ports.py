from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose import asa_port_from_compose

pytestmark = [pytest.mark.container, pytest.mark.lite]


def _assert_new_scheme_consistent(compose_path: Path, expected_port: str) -> None:
    """Assert the compose file has a consistent new-scheme port config.

    After a LoadProject -> SaveProject round-trip, compose-go expands
    short-form syntax into long-form.  Ports change from
    ``59331:59331/udp`` to ``target/published/protocol/mode`` fields,
    and environment variables change from list-form ``- ASA_PORT=59331``
    to map-form ``ASA_PORT: "59331"``.  Both formats are semantically
    equivalent, so the assertions accept either.
    """
    text = compose_path.read_text(encoding="utf-8")
    short_form = f"{expected_port}:{expected_port}/udp" in text
    long_form = (
        f"target: {expected_port}" in text and f'published: "{expected_port}"' in text and "protocol: udp" in text
    )
    assert short_form or long_form, f"expected 1:1 ASA port binding for {expected_port} in compose"
    assert re.search(rf"ark\.port\.asa:\s*\"?{expected_port}\"?", text), (
        f"expected ark.port.asa={expected_port} in compose"
    )
    assert re.search(r"ark\.port\.rcon:\s*\"?\d+\"?", text), "expected ark.port.rcon label in compose"
    # List-form: ASA_PORT=59331  or  map-form: ASA_PORT: "59331"
    env_list = f"ASA_PORT={expected_port}" in text
    env_map = f'ASA_PORT: "{expected_port}"' in text
    assert env_list or env_map, f"expected ASA_PORT={expected_port} in compose"


def test_reconfigure_asa_port_new_scheme(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """reconfigure instance --asa-port keeps the 1:1 binding, label, and env in sync.

    Scenario:
        An instance is created using the default template which produces a
        new-scheme compose file (1:1 ASA port binding, ark.port.asa/rcon
        labels, and ASA_PORT env var). The conftest randomises ports after
        create, so the initial port is discovered from the compose file.
        reconfigure instance --asa-port is then run with a new port value.
        The compose file, show instance, and list instances are inspected.

    Expected:
        - After create: compose file has a consistent new-scheme config
          (1:1 binding, matching label, matching ASA_PORT env var).
        - After reconfigure: binding, label, and env are all updated to the
          new port value atomically.
        - show instance reports the new port.
        - list instances output contains the new port.
    """
    instance = "pytest-ark-reconf-port-1"
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
    original_port = asa_port_from_compose(compose_path, instance)

    # Verify new-scheme consistency after create (ports were randomised by conftest).
    _assert_new_scheme_consistent(compose_path, original_port)

    # Pick a target port that differs from the randomised one.
    new_port = str(int(original_port) + 1000)

    run_cmd(
        [
            str(cli_path),
            "reconfigure",
            "instance",
            "--instance",
            instance,
            "--asa-port",
            new_port,
        ],
        env=test_env,
        cwd=repo_root,
    )

    # Binding, label, and env must all reflect the new port.
    _assert_new_scheme_consistent(compose_path, new_port)

    # Old port must be gone from UDP bindings (both short-form and
    # long-form).
    text = compose_path.read_text(encoding="utf-8")
    assert f"{original_port}:{original_port}/udp" not in text and f"target: {original_port}" not in text, (  # noqa: PT018
        f"old port {original_port} still in bindings"
    )

    # show instance must reflect the new port.
    show_result = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert f"port:     {new_port}" in show_result.stdout, (
        f"expected port: {new_port} in show output: {show_result.stdout}"
    )

    # list instances must reflect the new port.
    list_result = run_cmd(
        [str(cli_path), "list", "instances"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert new_port in list_result.stdout, f"expected {new_port} in list output: {list_result.stdout}"


def test_reconfigure_asa_port_conflict_detected(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """reconfigure instance rejects a port that conflicts with another instance.

    Scenario:
        Two instances are created. The second instance is reconfigured to
        use the same ASA port as the first instance.

    Expected:
        - The reconfigure command exits non-zero and the output mentions the
          conflicting instance name.
    """
    instance_a = "pytest-ark-reconf-conflict-1"
    instance_b = "pytest-ark-reconf-conflict-2"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    for inst in (instance_a, instance_b):
        run_cmd(
            [
                str(cli_path),
                "create",
                "instance",
                "--name",
                inst,
                "--server-password",
                server_password,
                "--admin-password",
                admin_password,
            ],
            env=test_env,
            cwd=repo_root,
        )

    compose_path = Path(test_env["COMPOSE_FILE"])
    port_a = asa_port_from_compose(compose_path, instance_a)

    result = run_cmd(
        [
            str(cli_path),
            "reconfigure",
            "instance",
            "--instance",
            instance_b,
            "--asa-port",
            port_a,
        ],
        env=test_env,
        cwd=repo_root,
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0, "expected reconfigure to fail on port conflict"
    combined = result.stdout + result.stderr
    assert instance_a in combined, f"expected conflicting instance name in error output: {combined}"
