from __future__ import annotations

import re

import pytest


def test_create_instance_writes_compose_file(run_cli, native_workdir):
    """create instance writes a valid docker-compose.yaml with the service definition.

    Scenario:
        An instance named 'testinst' is created with admin and server
        passwords. The resulting docker-compose.yaml is read and inspected
        for required service fields.

    Expected:
        - The command exits zero.
        - The compose file contains the service name, ASA_PORT env var, UDP
          port mapping, the port label, and the saved volume.
    """
    result = run_cli(
        [
            "create",
            "instance",
            "--name",
            "testinst",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ]
    )
    assert result.returncode == 0
    compose = (native_workdir / "docker-compose.yaml").read_text()
    assert "testinst:" in compose
    assert "ASA_PORT=" in compose
    assert "/udp" in compose
    assert "ark.port.asa:" in compose
    assert "-saved:" in compose


def test_create_instance_port_auto_assignment(run_cli, native_workdir):
    """Consecutive create instance calls auto-increment the ASA_PORT.

    Scenario:
        Two instances are created sequentially without specifying a port.
        The ASA_PORT value is extracted from each compose file and compared.

    Expected:
        - The second instance's ASA_PORT is exactly one greater than the
          first instance's ASA_PORT.
    """
    run_cli(
        [
            "create",
            "instance",
            "--name",
            "inst1",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ],
        check=True,
    )
    c1 = (native_workdir / "docker-compose.yaml").read_text()
    run_cli(
        [
            "create",
            "instance",
            "--name",
            "inst2",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ],
        check=True,
    )
    c2 = (native_workdir / "docker-compose.yaml").read_text()
    p1 = int(re.findall(r"ASA_PORT=(\d+)", c1)[-1])
    p2 = int(re.findall(r"ASA_PORT=(\d+)", c2)[-1])
    assert p2 == p1 + 1


def test_create_instance_duplicate_name_rejected(run_cli, native_workdir):
    """create instance rejects a duplicate instance name.

    Scenario:
        An instance named 'dup1' is created, then a second instance with the
        same name is attempted.

    Expected:
        - The second command exits non-zero.
        - The error output contains 'already exists'.
    """
    run_cli(
        [
            "create",
            "instance",
            "--name",
            "dup1",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ],
        check=True,
    )
    result = run_cli(
        [
            "create",
            "instance",
            "--name",
            "dup1",
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ]
    )
    assert result.returncode != 0
    assert "already exists" in (result.stderr + result.stdout).lower()


@pytest.mark.parametrize("name", ["bad name", "-dash"])
def test_create_instance_invalid_name_rejected(run_cli, name):
    """create instance rejects invalid instance names (spaces, leading dash).

    Scenario:
        create instance is invoked with names that violate naming rules: a
        name with a space and a name starting with a dash.

    Expected:
        - The command exits non-zero for each invalid name.
    """
    result = run_cli(
        [
            "create",
            "instance",
            "--name",
            name,
            "--admin-password",
            "AdminPass123",
            "--server-password",
            "ServerPass123",
        ]
    )
    assert result.returncode != 0
