"""Re-export of docker/slow backup/restore tests under Podman runtime.

Scenario:
    The same backup and restore lifecycle tests from docker/slow are re-run
    with the Podman container runtime to verify that live backup, archive
    validation, restore with --auto-start, SQLite corruption detection, and
    running-instance rejection all work identically under Podman.

Expected:
    - Backup round-trip preserves marker files and passes SQLite integrity.
    - Restore recovers deleted data and auto-starts the server.
    - SQLite corruption causes backup failure with clear error message.
    - Restore against a running instance is rejected.
"""

import pytest

from tests.app.ark_cli.integration.docker.slow.test_manage_backup_restore import *  # noqa: F403

pytestmark = [pytest.mark.podman, pytest.mark.container]
