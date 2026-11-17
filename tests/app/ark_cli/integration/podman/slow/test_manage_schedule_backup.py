"""Re-export of docker/slow schedule backup lifecycle test under Podman runtime.

Scenario:
    The same full schedule management lifecycle test from docker/slow is
    re-run with the Podman container runtime to verify that create,
    validate, apply, run, list, show, and remove schedule all work
    identically under Podman.

Expected:
    - Schedule CRUD operations succeed and produce correct output.
    - run schedule produces a backup archive.
    - apply schedules invokes the crontab shim.
    - remove schedule cleans up and list confirms removal.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_schedule_backup import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
