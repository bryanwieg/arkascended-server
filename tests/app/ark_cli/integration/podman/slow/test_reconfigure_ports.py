"""Re-export of docker/slow port reconfiguration test under Podman runtime.

Scenario:
    The same container-listens-on-configured-asa-port test from docker/slow
    is re-run with the Podman container runtime to verify that the ARK
    server inside the container binds on the port specified by ASA_PORT and
    that port bindings, env vars, and labels remain consistent under Podman.

Expected:
    - ASA_PORT inside container matches the compose label.
    - Port binding is 1:1 (published == container target).
    - Server startup log contains ?Port=<assigned port>.
    - show instance reports the correct port.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_reconfigure_ports import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
