from __future__ import annotations

import json
import subprocess
from collections.abc import Callable

import pytest

_KERNEL_PARAMS = ["vm.max_map_count", "fs.file-max"]


def test_check_host_kernel_config_produces_output(
    run_cli: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check host-kernel-config outputs kernel parameter names (vm.max_map_count, fs.file-max).

    Scenario:
        check host-kernel-config is invoked and the output is inspected for
        expected kernel parameter names.

    Expected:
        - The output contains at least one of the expected kernel parameter names.
    """
    result = run_cli(["check", "host-kernel-config"])
    assert any(p in (result.stdout + result.stderr) for p in _KERNEL_PARAMS)


def test_check_host_kernel_config_json_flag(
    run_cli: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check host-kernel-config --json returns a parsable dict.

    Scenario:
        check host-kernel-config is invoked with --json. The output is
        parsed as JSON and checked for dict structure.

    Expected:
        - If --json is supported, the output is a dict with at least one
          kernel parameter key.
        - If --json is not supported, the test is skipped.
    """
    result = run_cli(["check", "host-kernel-config", "--json"])
    if result.returncode != 0:  # noqa: SIM102
        if "flag provided but not defined" in result.stderr:
            pytest.skip("--json not supported by check host-kernel-config")
    data = json.loads(result.stdout)
    assert isinstance(data, dict)
    assert any(p in data for p in _KERNEL_PARAMS)
