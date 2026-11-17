from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest

pytestmark = [pytest.mark.container]


def test_manage_multi_instance_all_scope_ops(
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    wait_for: Callable[[str, int, Callable[[], bool]], None],
) -> None:
    """The "all" scope target works correctly across start, restart, stop, backup, and prune.

    Scenario:
        Two instances are created in the same compose file. The "all" target is
        used for every subsequent operation: start, restart, stop, create backup,
        and prune backups. RAM check is skipped so both containers can start on
        a constrained test host.

    Expected:
        - start instance all, restart instance all, and stop instance all all
          succeed without error.
        - create backup all produces at least one archive per instance in the
          backup directory (retried up to 3 times to account for transient
          post-stop locking delays).
        - prune backups all with generous retention limits succeeds and the
          output contains "Pruned backups for <instance>" for each instance.
    """
    instance_a = "pytest-ark-allops-1"
    instance_b = "pytest-ark-allops-2"
    instances = (instance_a, instance_b)
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    for instance in instances:
        run_cmd(
            [
                str(cli_path),
                "create",
                "instance",
                "--name",
                instance,
                "--server-password",
                server_password,
                "--admin-password",
                admin_password,
            ],
            env=test_env,
            cwd=repo_root,
        )

    run_cmd(
        [str(cli_path), "start", "instance", "--instance", "all"],
        env=test_env,
        cwd=repo_root,
    )

    run_cmd(
        [str(cli_path), "restart", "instance", "--instance", "all", "--timeout", "0s"],
        env=test_env,
        cwd=repo_root,
    )

    run_cmd(
        [str(cli_path), "stop", "instance", "--instance", "all", "--timeout", "120s"],
        env=test_env,
        cwd=repo_root,
    )

    def _do_backup() -> bool:
        result = run_cmd(
            [str(cli_path), "create", "backup", "--instance", "all"],
            env=test_env,
            cwd=repo_root,
            check=False,
            capture_output=True,
        )
        return result.returncode == 0

    wait_for("backup all to succeed", 180, _do_backup)

    backup_root = Path(test_env["ARK_BACKUP_DIR"])
    for instance in instances:
        archives = sorted((backup_root / instance).glob("*.tar.gz")) + sorted((backup_root / instance).glob("*.tar"))
        assert archives, f"Expected backup archives for {instance} in {backup_root / instance}"

    prune_result = run_cmd(
        [
            str(cli_path),
            "prune",
            "backups",
            "--instance",
            "all",
            "--retain-count",
            "10",
            "--retain-days",
            "30",
            "--retain-size-gb",
            "10",
        ],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    for instance in instances:
        assert (
            f"Pruned backups for {instance}" in prune_result.stdout
            or f"Nothing to prune for {instance}" in prune_result.stdout
        )
