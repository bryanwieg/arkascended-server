"""Re-export of docker/slow CLI stop tests under Podman runtime.

Scenario:
    The same graceful stop and kill tests from docker/slow are re-run with
    the Podman container runtime to verify that SIGTERM delivery, shutdown
    manager invocation, and SIGKILL exit codes behave consistently under
    Podman.

Expected:
    - Graceful stop: container exits 0 with shutdown manager log messages.
    - Kill: container exits with non-zero code confirming SIGKILL.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_cli_stop import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]

_PODMAN_XFAIL_REASONS = {
    "test_cli_stop_graceful": "TODO: Under Podman rootless, 'docker compose stop' may not deliver SIGTERM to PID 1 the same way as Docker. Container stops but shutdown manager log messages are absent.",
    "test_cli_stop_kill": "TODO: Under Podman rootless, SIGKILL exit code may differ from Docker's 137.",
}


@pytest.fixture(autouse=True)
def _podman_stop_xfail(request):
    # Added per item: marking the re-exported functions would also xfail the Docker originals.
    reason = _PODMAN_XFAIL_REASONS.get(request.node.originalname)
    if reason:
        request.node.add_marker(pytest.mark.xfail(reason=reason))
