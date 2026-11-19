"""ark-server-start.py reports an OOM kill of the server, using a real cgroup limit and kernel OOM killer."""

from __future__ import annotations

from pathlib import Path

import pytest
from helpers.compose_client import get_docker_client
from helpers.infra import wait_for

pytestmark = [pytest.mark.container, pytest.mark.lite]

_LIMIT_BYTES = 256 * 1024 * 1024
# Stand-in for proton: its child fills memory past the limit, like ArkAscendedServer.exe under proton.
_FAKE_PROTON = """#!/bin/sh
python3.14 -c "x = b'x' * (1 << 30)"
"""
_RUN = (
    "cat /sys/fs/cgroup/memory.max 2>/dev/null | sed 's/^/memory.max=/'; "
    "exec python3.14 /home/ark-user/scripts/runtime/ark-server-start.py"
)


def test_server_start_reports_oom_kill(
    test_env: dict[str, str],
    test_artifact_dir: Path,
) -> None:
    """The server manager says the OOM killer killed the server when its child runs out of memory.

    Scenario:
        The real ark-server-start.py runs on the lite image with PROTON_PATH pointing at a fake
        proton whose child allocates 1 GB in a container limited to 256 MB with no swap.

    Expected:
        - The container's cgroup memory limit was applied (else skip, or fail in CI).
        - The logs contain the [server-oom] error.
    """
    image = test_env["ARK_TEST_IMAGE"]  # the lite image (lite marker)
    proton_dir = test_artifact_dir / "fake-proton"
    proton_dir.mkdir()
    (proton_dir / "proton").write_text(_FAKE_PROTON)
    (proton_dir / "proton").chmod(0o755)

    dc = get_docker_client()
    container = dc.container.run(
        image,
        entrypoint="sh",
        command=["-c", _RUN],
        envs={"PROTON_PATH": "/fake-proton"},
        memory=_LIMIT_BYTES,
        memory_swap=_LIMIT_BYTES,
        volumes=[(str(proton_dir), "/fake-proton", "ro")],
        labels={"io.ark.test-suite": "true"},
        detach=True,
    )
    try:
        wait_for("server manager to exit", 60, lambda: not container.state.running)
        logs = container.logs()
    finally:
        container.remove(force=True)

    if f"memory.max={_LIMIT_BYTES}" not in logs:
        message = f"the container runtime did not apply the {_LIMIT_BYTES}-byte memory limit"
        if test_env.get("CI"):
            pytest.fail(f"{message}; the test cannot trigger an OOM kill.\nOutput:\n{logs}")
        pytest.skip(message)
    assert "[server-oom] Error: the kernel OOM killer killed" in logs, f"Expected the OOM report.\nOutput:\n{logs}"
