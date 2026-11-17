"""Re-export of docker/fast local update check tests under Podman runtime.

Scenario:
    The same update instance --check tests from docker/fast are re-run with
    the Podman container runtime to verify that local image build-ID
    comparison against Steam API (skip when current, update when stale)
    works identically under Podman.

Expected:
    --check reports skip when the build ID matches and update when it
    differs; compose file is not modified in check-only mode — same as
    under Docker.
"""

import pytest

from tests.app.ark_cli.integration.docker.fast.test_update_local_check import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
