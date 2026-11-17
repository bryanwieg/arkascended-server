from __future__ import annotations

import tarfile
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose_client import compose_exec, compose_run, copy_from_service
from python_on_whales import DockerClient
from python_on_whales.exceptions import DockerException

pytestmark = [pytest.mark.container]


def test_container_backup_script(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
    test_artifact_dir: Path,
) -> None:
    """ark-backup.py creates a valid .tar archive containing save data and logs.

    Scenario:
        The test waits for the server advertising log line to confirm the server
        is ready. A sentinel marker file is written inside the container's save
        directory. ark-backup.py is invoked to produce a .tar archive. The
        archive is copied to the host and inspected with Python's tarfile module.

    Expected:
        - ark-backup.py stdout contains "starting backup workflow",
          "requesting world save via RCON", and "backup archive created successfully".
        - Container logs also contain the backup workflow messages.
        - The archive is a non-empty file inside the container.
        - The extracted member list contains Config/WindowsServer/GameUserSettings.ini,
          the sentinel marker file (and no Logs/), and at least one .ark
          world save file under SavedArks/.
    """
    instance = started_container_instance["instance"]
    output_path = "/home/ark-user/maintenance/pytest-backup.tar"
    marker_path = "/home/ark-user/ark-server/ShooterGame/Saved/pytest-backup-marker.txt"
    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"

    def server_ready_for_backup() -> bool:
        try:
            compose_client.compose.execute(
                instance,
                [
                    "sh",
                    "-c",
                    (
                        f"test -f {server_log_path} && "
                        'grep -q "Server has completed startup and is now advertising for join." '
                        f"{server_log_path}"
                    ),
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    wait_for("server advertising log before backup", 300, server_ready_for_backup)

    compose_client.compose.execute(
        instance,
        ["sh", "-c", f"echo backup-test > {marker_path}"],
    )

    backup_output = compose_exec(
        compose_client,
        instance,
        ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-backup.py", "--output", output_path],
    )
    assert "[backup] starting backup workflow" in backup_output
    assert "[backup] requesting world save via RCON" in backup_output
    assert "[backup] backup archive created successfully" in backup_output

    logs = compose_client.compose.logs(services=[instance], tail="300")
    assert "[backup] starting backup workflow" in logs
    assert "[backup] backup archive created successfully" in logs

    compose_client.compose.execute(instance, ["test", "-s", output_path])

    archive_copy = test_artifact_dir / "pytest-backup.tar"
    copy_from_service(compose_client, instance, output_path, str(archive_copy))
    with tarfile.open(archive_copy, "r:*") as tar:
        members = tar.getnames()
    normalized_members = {member.removeprefix("./") for member in members}

    assert "Config/WindowsServer/GameUserSettings.ini" in normalized_members
    # Logs/ is excluded: the live server writes it during tar, which makes tar exit 1.
    assert not any(m.startswith("Logs/") for m in normalized_members)
    # clusters/ is the shared cluster volume; instance backups must not include it.
    assert not any(m.startswith("clusters/") for m in normalized_members)
    assert "pytest-backup-marker.txt" in normalized_members
    assert any(member.startswith("SavedArks/") and member.endswith(".ark") for member in normalized_members), (
        "Expected at least one world save .ark file in SavedArks/."
    )


def test_container_backup_script_rejects_bad_extension(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """ark-backup.py exits non-zero when the output path has an unsupported extension.

    Scenario:
        The test waits for the server to reach advertising state. ark-backup.py
        is invoked with --output pointing to a .zip file, which is not a
        supported archive format.

    Expected:
        - ark-backup.py exits non-zero.
        - stderr contains "output must end with", reporting the bad extension
          before any backup work begins.
    """
    instance = started_container_instance["instance"]
    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"

    def server_ready_for_backup() -> bool:
        try:
            compose_client.compose.execute(
                instance,
                [
                    "sh",
                    "-c",
                    (
                        f"test -f {server_log_path} && "
                        'grep -q "Server has completed startup and is now advertising for join." '
                        f"{server_log_path}"
                    ),
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    wait_for(
        "server advertising log before backup extension validation",
        300,
        server_ready_for_backup,
    )

    try:
        compose_exec(
            compose_client,
            instance,
            [
                "/usr/bin/python3.14",
                "/home/ark-user/scripts/runtime/ark-backup.py",
                "--output",
                "/home/ark-user/maintenance/pytest-backup.zip",
            ],
        )
        assert False, "Expected non-zero exit"  # noqa: B011, PT015
    except DockerException as exc:
        assert exc.stderr is not None  # noqa: PT017
        assert "output must end with" in exc.stderr  # noqa: PT017


def test_container_backup_script_fails_sqlite_verification(
    compose_client: DockerClient,
    slow_test_protection,
    started_container_instance: dict[str, str],
) -> None:
    """ark-backup.py exits non-zero when SavedArks contains a corrupt .ark SQLite file.

    Scenario:
        A corrupt .ark file containing "not-a-sqlite-db" is written into a
        pytest-corrupt subdirectory of SavedArks. The service is stopped for
        offline backup. ark-backup.py is invoked via docker compose run. A
        finally block cleans up the corrupt file.

    Expected:
        - ark-backup.py exits non-zero.
        - The combined stdout+stderr contains "sqlite quick_check failed",
          confirming the integrity check caught the corrupt file.
    """
    instance = started_container_instance["instance"]
    output_path = "/home/ark-user/maintenance/pytest-backup-corrupt.tar"
    corrupt_dir = "/home/ark-user/ark-server/ShooterGame/Saved/SavedArks/pytest-corrupt"
    corrupt_file = f"{corrupt_dir}/Corrupt.ark"
    try:
        compose_client.compose.execute(
            instance,
            ["sh", "-c", f"mkdir -p {corrupt_dir} && printf 'not-a-sqlite-db' > {corrupt_file}"],
        )
        compose_client.compose.stop(services=[instance])
        try:
            compose_run(
                compose_client,
                instance,
                cmd=["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-backup.py", "--output", output_path],
                remove=True,
            )
            assert False, "Expected non-zero exit"  # noqa: B011, PT015
        except DockerException as exc:
            combined = f"{exc.stdout or ''}\n{exc.stderr or ''}"
            assert "sqlite quick_check failed" in combined

    finally:
        try:  # noqa: SIM105
            compose_client.compose.run(
                instance,
                command=["sh", "-c", f"rm -f {corrupt_file} {output_path}"],
                remove=True,
            )
        except Exception:  # noqa: BLE001, S110
            pass


def test_container_backup_script_tar_zst(
    compose_client: DockerClient,
    slow_test_protection,
    wait_for: Callable[[str, int, Callable[[], bool]], None],
    started_container_instance: dict[str, str],
) -> None:
    """ark-backup.py creates a valid zstd-compressed .tar.zst archive.

    Scenario:
        The test waits for the server advertising log line, then checks that
        zstd is available in the container image (skip if not). A sentinel
        marker file is written. ark-backup.py is invoked with a .tar.zst output
        path. The archive's member list is retrieved via tar -tf inside the container.

    Expected:
        - ark-backup.py stdout contains "backup archive created successfully".
        - The archive is a non-empty file inside the container.
        - The archive's member list (after stripping leading "./") contains the
          sentinel marker file, confirming the saved data was packaged correctly.
        - The test is skipped if zstd is not available in the container image.
    """
    instance = started_container_instance["instance"]
    output_path = "/home/ark-user/maintenance/pytest-backup.tar.zst"
    marker_path = "/home/ark-user/ark-server/ShooterGame/Saved/pytest-backup-zst-marker.txt"
    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"

    def server_ready_for_backup() -> bool:
        try:
            compose_client.compose.execute(
                instance,
                [
                    "sh",
                    "-c",
                    (
                        f"test -f {server_log_path} && "
                        'grep -q "Server has completed startup and is now advertising for join." '
                        f"{server_log_path}"
                    ),
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    wait_for("server advertising log before zst backup", 300, server_ready_for_backup)
    try:
        compose_client.compose.execute(
            instance,
            ["sh", "-c", "command -v zstd >/dev/null 2>&1"],
        )
    except Exception:  # noqa: BLE001
        pytest.skip("zstd is not available in the container image; rebuild image with zstd support first.")
    compose_client.compose.execute(
        instance,
        ["sh", "-c", f"mkdir -p /home/ark-user/maintenance && echo backup-zst-test > {marker_path}"],
    )
    stdout = compose_exec(
        compose_client,
        instance,
        ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-backup.py", "--output", output_path],
    )
    assert "[backup] backup archive created successfully" in stdout
    compose_client.compose.execute(instance, ["test", "-s", output_path])
    list_output = compose_exec(
        compose_client,
        instance,
        ["tar", "-tf", output_path],
    )
    normalized_members = {member.removeprefix("./") for member in list_output.splitlines() if member.strip()}
    assert "pytest-backup-zst-marker.txt" in normalized_members
