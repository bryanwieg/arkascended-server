"""Re-export of docker/slow instance visibility tests under Podman runtime.

Scenario:
    The same list/show instance visibility tests from docker/slow are re-run
    with the Podman container runtime to verify that status reporting
    (stopped/running) across multiple instances works identically under
    Podman.

Expected:
    - list instances shows both instances with correct status.
    - show instance reports stopped before start and running after start.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_instance_visibility import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
