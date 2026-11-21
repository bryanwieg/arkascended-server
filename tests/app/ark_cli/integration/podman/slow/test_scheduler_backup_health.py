"""Re-export of docker/slow scheduler backup and health test under Podman runtime.

Scenario:
    The same scheduler end-to-end test from docker/slow is re-run with the
    Podman container runtime: a scheduled backup made by the scheduler
    container, its health report, and an instance stopped and restarted.

Expected:
    - The scheduled archive is owned by the test user.
    - check scheduler, /health and the container's health report healthy,
      with a stopped instance skipped and a restarting one busy.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_scheduler_backup_health import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
