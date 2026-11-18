"""Re-export of the docker/slow cluster backup/restore test under Podman runtime.

Scenario:
    The cluster backup, change and restore round trip from docker/slow is re-run
    with the Podman container runtime.

Expected:
    - The cluster backup holds the transfer files and restore brings them back.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_cluster_backup_restore import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
