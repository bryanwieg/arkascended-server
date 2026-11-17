"""Re-export of docker/fast reconfigure ports tests under Podman runtime.

Scenario:
    The same ASA port reconfiguration tests from docker/fast are re-run
    with the Podman container runtime to verify port binding consistency,
    label/env sync, and conflict detection work identically under Podman.

Expected:
    All re-exported tests pass with the same outcomes as under Docker.
"""

import pytest

from tests.app.ark_cli.integration.docker.fast.test_reconfigure_ports import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
