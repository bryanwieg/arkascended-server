"""Re-export of docker/slow start/stop lifecycle tests under Podman runtime.

Scenario:
    The same single-instance and multi-instance start/stop lifecycle tests
    from docker/slow are re-run with the Podman container runtime to
    verify that create, start, wait, check, save, logs, restart, and stop
    all work identically under Podman.

Expected:
    - Single instance: service enters running, check/save/logs succeed,
      restart works, stop transitions to exited.
    - Multi instance: both instances start and stop independently.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_start_stop import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
