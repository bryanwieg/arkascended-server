"""Re-export of docker/slow single-instance restart test under Podman runtime.

Scenario:
    The same restart instance lifecycle test from docker/slow is re-run with
    the Podman container runtime to verify that restart produces a new
    container with an updated start timestamp under Podman.

Expected:
    - restart instance exits zero with "Restarted instance target" message.
    - Post-restart container has a different StartedAt timestamp.
    - wait instance --for stopped succeeds after final stop.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_restart_single_instance import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
