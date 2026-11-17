from __future__ import annotations

from pathlib import Path

import pytest
from python_on_whales import DockerClient
from python_on_whales.exceptions import DockerException

pytestmark = [pytest.mark.container]


def test_container_restore_script(
    compose_client: DockerClient,
    slow_test_protection,
    repo_root: Path,
    started_container_instance: dict[str, str],
    test_artifact_dir: Path,
) -> None:
    """ark-restore.py extracts a zstd archive and creates a pre-restore backup.

    Scenario:
        A sentinel marker file is written into the running container's save
        volume. The service is stopped for offline restore. A .tar.zst restore
        archive is built inside the container from fixture .ark files plus the
        marker. ark-restore.py is invoked with --archive and --pre-restore paths.
        A final shell command verifies the restore output.

    Expected:
        - The pre-restore archive is a non-empty file after the restore completes.
        - The sentinel marker file is present at its original path inside the
          volume, confirming the archive was extracted correctly.
        - The test is skipped if zstd is not available in the container image.
        - No host /tmp or host zstd is needed: bind mounts come from the repo and .tests/.
    """
    instance = started_container_instance["instance"]
    save_dir = "/home/ark-user/ark-server/ShooterGame/Saved"
    marker_path = f"{save_dir}/pytest-restore-marker.txt"
    fixture_archive = repo_root / "tests/fixtures/restore/theisland_wp_fixture.tar.zst"
    assert fixture_archive.is_file()
    # Check zstd availability before stopping — exec requires a running container.
    try:
        compose_client.compose.execute(
            instance,
            ["sh", "-c", "command -v zstd >/dev/null 2>&1"],
        )
    except Exception:  # noqa: BLE001
        pytest.skip("zstd is not available in the container image; rebuild image with zstd support first.")

    compose_client.compose.execute(  # Create marker while service is running so it lands in the save volume.
        instance,
        ["sh", "-c", f"echo restore-test > {marker_path}"],
    )
    compose_client.compose.stop(services=[instance])  # Restore must run offline in maintenance mode.

    # Bind mounts use the repo and .tests/, which the Docker daemon can see; /tmp may not be.
    restore_io = test_artifact_dir / "restore-io"
    restore_io.mkdir()
    restore_io.chmod(0o777)
    archive_path = "/home/ark-user/restore-io/pytest-restore-archive.tar.zst"
    pre_restore_path = "/home/ark-user/restore-io/pytest-pre-restore.tar.gz"
    # Build fixture archive payload in one run, then invoke restore directly
    # so entrypoint can stamp ARK_MAINTENANCE_MODE=restore.
    compose_client.compose.run(
        instance,
        command=[
            "sh",
            "-c",
            (
                "rm -rf /home/ark-user/maintenance/pytest-restore-src && "
                "mkdir -p /home/ark-user/maintenance/pytest-restore-src/SavedArks/TheIsland_WP && "
                f"cp {marker_path} /home/ark-user/maintenance/pytest-restore-src/pytest-restore-marker.txt && "
                # Unpacked in the container, which has zstd, so the host needs none.
                f"tar --zstd -xf /home/ark-user/fixtures/{fixture_archive.name} "
                "-C /home/ark-user/maintenance/pytest-restore-src && "
                "test -f /home/ark-user/maintenance/pytest-restore-src/SavedArks/TheIsland_WP/TheIsland_WP.ark && "
                "test -f /home/ark-user/maintenance/pytest-restore-src/SavedArks/TheIsland_WP/"
                "TheIsland_WP_AntiCorruptionBackup.bak && "
                f"tar --zstd -cf {archive_path} -C /home/ark-user/maintenance/pytest-restore-src . && "
                f"rm -f {marker_path}"
            ),
        ],
        remove=True,
        volumes=[
            (str(fixture_archive.parent), "/home/ark-user/fixtures", "ro"),
            (str(restore_io), "/home/ark-user/restore-io"),
        ],
    )
    compose_client.compose.run(
        instance,
        command=[
            "/usr/bin/python3.14",
            "/home/ark-user/scripts/runtime/ark-restore.py",
            "--archive",
            archive_path,
            "--pre-restore",
            pre_restore_path,
        ],
        remove=True,
        volumes=[
            (str(restore_io), "/home/ark-user/restore-io"),
        ],
    )
    compose_client.compose.run(
        instance,
        command=[
            "sh",
            "-c",
            f"test -s {pre_restore_path} && test -f {marker_path}",
        ],
        remove=True,
        volumes=[
            (str(restore_io), "/home/ark-user/restore-io"),
        ],
    )


def test_container_restore_script_rejects_unsafe_paths(
    compose_client: DockerClient,
    slow_test_protection,
    started_container_instance: dict[str, str],
    test_artifact_dir: Path,
) -> None:
    """ark-restore.py exits non-zero and logs an error when the archive contains path traversal entries.

    Scenario:
        The service is stopped for offline restore. A malformed .tar.zst archive
        is built inside the container using a --transform that adds a leading
        "../" to all paths, creating a path traversal entry. ark-restore.py is
        invoked against this unsafe archive.

    Expected:
        - ark-restore.py exits non-zero.
        - stderr contains "unsafe path", confirming the path validation check
          fired before any extraction was attempted.
        - The test is skipped if zstd is not available in the container image.
    """
    instance = started_container_instance["instance"]

    # Check zstd availability before stopping — exec requires a running container.
    try:
        compose_client.compose.execute(
            instance,
            ["sh", "-c", "command -v zstd >/dev/null 2>&1"],
        )
    except Exception:  # noqa: BLE001
        pytest.skip("zstd is not available in the container image; rebuild image with zstd support first.")

    compose_client.compose.stop(services=[instance])

    restore_io = test_artifact_dir / "restore-io"
    restore_io.mkdir()
    restore_io.chmod(0o777)
    archive_path = "/home/ark-user/restore-io/pytest-restore-unsafe.tar.zst"
    pre_restore_path = "/home/ark-user/restore-io/pytest-pre-restore-unsafe.tar.gz"
    compose_client.compose.run(
        instance,
        command=[
            "sh",
            "-c",
            (
                "mkdir -p /home/ark-user/maintenance/pytest-unsafe-archive && "
                "echo unsafe > /home/ark-user/maintenance/pytest-unsafe-archive/unsafe.txt && "
                "tar --zstd -cf /home/ark-user/restore-io/pytest-restore-unsafe.tar.zst "
                "-C /home/ark-user/maintenance/pytest-unsafe-archive --transform 's,^,../,' unsafe.txt && "
                f"rm -f {pre_restore_path}"
            ),
        ],
        remove=True,
        volumes=[
            (str(restore_io), "/home/ark-user/restore-io"),
        ],
    )
    try:
        compose_client.compose.run(
            instance,
            command=[
                "/usr/bin/python3.14",
                "/home/ark-user/scripts/runtime/ark-restore.py",
                "--archive",
                archive_path,
                "--pre-restore",
                pre_restore_path,
            ],
            remove=True,
            tty=False,
            volumes=[
                (str(restore_io), "/home/ark-user/restore-io"),
            ],
        )
        assert False, "Expected non-zero exit"  # noqa: B011, PT015
    except DockerException as exc:
        assert exc.stderr is not None  # noqa: PT017
        assert "unsafe path" in exc.stderr  # noqa: PT017
