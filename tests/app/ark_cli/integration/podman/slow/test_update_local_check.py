"""Re-export of docker/slow local update check and rebuild tests under Podman runtime.

Scenario:
    The same update instance --check (real Steam API) and local image
    rebuild tests from docker/slow are re-run with the Podman container
    runtime to verify that Steam API queries, build-ID comparison, and
    image rebuild with label verification work identically under Podman.

Expected:
    - --check against real Steam API exits zero with a valid row.
    - Local image rebuild produces correct version and build_id labels.
    - show instance still reports the instance name after rebuild.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_update_local_check import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
