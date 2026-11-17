from __future__ import annotations

import subprocess
from collections.abc import Callable

import pytest


@pytest.mark.parametrize(
    "args",
    [
        ["create", "instance"],
        ["start", "instance"],
        ["stop", "instance"],
        ["restart", "instance"],
        ["save", "instance"],
        ["logs", "instance"],
        ["wait", "instance"],
        ["check", "instance"],
        ["show", "instance"],
        ["show", "volume-paths"],
        ["show", "ini"],
        ["edit", "ini"],
        ["create", "backup"],
        ["restore", "backup"],
        ["validate", "backup"],
        ["list", "backups"],
        ["show", "backup"],
        ["prune", "backups"],
        ["reconfigure", "instance"],
        ["update", "instance"],
        ["remove", "instance"],
        ["check", "reachability"],
        ["check", "reachability", "port"],
        ["listen", "port"],
        ["show", "schedule"],
        ["enable", "schedule"],
        ["disable", "schedule"],
        ["remove", "schedule"],
        ["check", "schedule"],
        ["update", "schedule"],
        ["run", "schedule"],
        ["validate", "schedule"],
    ],
)
def test_missing_required_arg_exits_nonzero(
    run_cli: Callable[..., subprocess.CompletedProcess[str]],
    args: list[str],
) -> None:
    """Every CLI verb/object pair exits non-zero when required args are omitted.

    Scenario:
        Each top-level verb/object combination (e.g. 'create instance',
        'start instance', etc.) is invoked without the required positional
        arguments (instance name, backup name, port, etc.).

    Expected:
        - The command exits non-zero for every combination.
    """
    result = run_cli(args)
    assert result.returncode != 0
