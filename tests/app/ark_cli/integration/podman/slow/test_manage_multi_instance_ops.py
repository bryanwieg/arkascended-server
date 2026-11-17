"""Re-export of docker/slow multi-instance "all" scope tests under Podman runtime.

Scenario:
    The same multi-instance operations test from docker/slow is re-run with
    the Podman container runtime to verify that start/restart/stop/backup/prune
    with the "all" scope target work identically under Podman.

Expected:
    - start instance all, restart instance all, and stop instance all succeed.
    - create backup all produces archives for each instance.
    - prune backups all reports per-instance results.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_multi_instance_ops import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
