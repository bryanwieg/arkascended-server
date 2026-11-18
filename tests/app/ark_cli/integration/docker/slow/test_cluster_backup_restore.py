from __future__ import annotations

import shutil
import tarfile
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose_client import compose_run

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]

CLUSTER = "/home/ark-user/ark-server/ShooterGame/Saved/clusters"


def _sh(compose_client: DockerClient, instance: str, script: str) -> str:
    """Run a shell script in a one-off container of a stopped instance; return its stdout."""
    return str(compose_run(compose_client, instance, ["sh", "-c", script], remove=True, dependencies=False))


def _members(archive: Path) -> set[str]:
    with tarfile.open(archive, "r:*") as tar:
        return {name.removeprefix("./") for name in tar.getnames()}


def test_cluster_backup_restore_round_trip(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """Cluster data survives a backup, a change and a restore, all through the CLI.

    Scenario:
        Two stopped instances share the cluster volume. One writes two transfer files,
        `create backup --instance cluster` archives them, the files are changed (one
        edited, one deleted, one added), then `restore backup --instance cluster
        --archive latest` restores the backup.

    Expected:
        - The backup lands in <backups>/cluster/ and holds both files.
        - After restore, the other instance sees the backed-up files exactly; the added
          file is gone and is kept in the pre-restore archive.
        - `create backup --instance all` reports a row for each instance and the cluster.
    """
    # A private backup root: the shared one may hold cluster backups from other runs.
    backup_root = repo_root / "backups" / f"pytest-cluster-{uuid.uuid4().hex[:8]}"
    env = {**test_env, "ARK_BACKUP_DIR": str(backup_root)}
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    if image != "ark-server:latest":
        compose_client.image.tag(image, "ark-server:latest")

    def cli(*args: str, check: bool = True) -> object:
        return run_cmd([str(cli_path), *args], env=env, cwd=repo_root, capture_output=True, check=check)

    first, second = (f"pytest-ark-{uuid.uuid4().hex[:8]}" for _ in range(2))
    try:
        for instance in (first, second):
            cli("create", "instance", "--name", instance, "--admin-password", "AdminPass123", "--no-server-password")

        _sh(
            compose_client,
            first,
            f"mkdir -p {CLUSTER}/pytest && printf v1 > {CLUSTER}/pytest/player1 && printf v1 > {CLUSTER}/pytest/player2",
        )
        cli("create", "backup", "--instance", "cluster")
        archives = sorted((backup_root / "cluster").glob("cluster-backup-*.tar.gz"))
        assert len(archives) == 1, archives
        assert {"pytest/player1", "pytest/player2"} <= _members(archives[0])

        _sh(
            compose_client,
            first,
            f"printf v2 > {CLUSTER}/pytest/player1 && rm {CLUSTER}/pytest/player2 && printf x > {CLUSTER}/pytest/extra",
        )
        restore = cli("restore", "backup", "--instance", "cluster", "--archive", "latest")
        assert "Restored cluster data" in restore.stdout

        listing = _sh(compose_client, second, f"cd {CLUSTER}/pytest && ls && cat player1")
        assert "player2" in listing
        assert "extra" not in listing
        assert listing.rstrip().endswith("v1"), listing
        pre_restore = sorted((backup_root / "cluster").glob("cluster-pre-restore-*.tar.gz"))
        assert len(pre_restore) == 1, pre_restore
        assert "pytest/extra" in _members(pre_restore[0])

        all_result = cli("create", "backup", "--instance", "all", check=False)
        combined = f"{all_result.stdout}\n{all_result.stderr}"
        for target in (first, second, "cluster"):
            assert f"Backup result {target}:" in combined, combined
        assert "Backup result cluster: OK" in combined, combined
    finally:
        shutil.rmtree(backup_root, ignore_errors=True)
