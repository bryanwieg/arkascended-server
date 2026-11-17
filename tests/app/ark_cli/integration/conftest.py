from __future__ import annotations

# CLI integration tests need the same Docker/container fixtures as container
# integration tests (test_env, run_cmd, started_instance_cli, etc.).
# Re-export them via wildcard import so pytest discovers them as local fixtures.
from tests.container.integration.conftest import *  # noqa: F403
