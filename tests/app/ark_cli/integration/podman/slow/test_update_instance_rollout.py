"""Re-export of docker/slow update instance rollout test under Podman runtime.

Scenario:
    The same update instance --rollout test from docker/slow is re-run with
    the Podman container runtime to verify that rolling restart, update
    check table output, and post-rollout health work identically under
    Podman.

Expected:
    - update instance --check prints the expected table header and row.
    - update instance --rollout exits zero.
    - show/validate compose succeed after rollout.
    - check instance confirms the server is healthy.
    - show instance reports status: running.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_update_instance_rollout import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
