from __future__ import annotations

import os
import time
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.compose import write_single_service_compose
from helpers.diagnostics import capture_diagnostics
from helpers.framework_logger import log_framework

if TYPE_CHECKING:
    from python_on_whales import DockerClient


def wait_for(
    description: str,
    timeout_seconds: int,
    action: Callable[[], bool],
) -> None:
    deadline = time.monotonic() + timeout_seconds
    start = time.monotonic()
    log_framework("poll", "waiting for %s (timeout=%ds)", description, timeout_seconds)
    while True:
        if action():
            log_framework("poll", "done waiting for %s (elapsed=%ds)", description, int(time.monotonic() - start))
            return
        elapsed = int(time.monotonic() - start)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        log_framework("poll", "waiting for %s (elapsed=%ds, remaining=%ds)", description, elapsed, int(remaining))
        time.sleep(5)
    log_framework("poll", "TIMEOUT waiting for %s", description)
    raise AssertionError(f"Timed out waiting for {description}.")


def _ensure_image_exists(dc: DockerClient, image: str | None = None) -> str:
    """Check docker binary, daemon, compose, and image exist.

    Skips with pytest.skip if any check fails. Returns the image name.
    """
    import shutil

    binary = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    if not shutil.which(binary):
        pytest.skip(f"{binary} is required for integration tests.")

    try:
        dc.compose.version()
    except Exception:  # noqa: BLE001
        pytest.skip(f"{binary} compose is not available for integration tests.")

    image = image or os.environ.get("ARK_TEST_IMAGE", "ark-server:latest")
    try:
        dc.images(image)
    except Exception:  # noqa: BLE001
        pytest.skip(f"Container image not found: {image}. Build it before running.")
    return image


def prune_orphaned_test_resources(dc: DockerClient) -> None:
    """Remove all test-created containers, volumes, and networks.

    Uses the ``io.ark.test-suite=true`` label to identify resources
    created by any prior test run, including resources from processes
    that were killed before teardown could run.

    Order: containers first (removes volume/network dependencies),
    then volumes, then networks. Each step is silently resilient
    to the previous step's failures.

    Logs via helpers.framework_logger for artifact traceability.
    """
    log_framework("cleanup", "pruning orphaned test resources...")

    # Phase 1: Containers — kill + force-remove
    try:
        labeled = dc.container.list(all=True, filters=[("label", "io.ark.test-suite=true")])
    except Exception:  # noqa: BLE001
        labeled = []
    if labeled:
        try:  # noqa: SIM105
            dc.container.kill(labeled)
        except Exception:  # noqa: BLE001, S110
            pass
        try:  # noqa: SIM105
            dc.container.remove(labeled, force=True)
        except Exception:  # noqa: BLE001, S110
            pass

    # Phase 2: Volumes — prune by label
    # Docker requires --all to remove named volumes (default: dangling only).
    # Podman removes all unused volumes by default and rejects --all as unknown.
    _runtime = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    try:  # noqa: SIM105
        dc.volume.prune(filters=[("label", "io.ark.test-suite=true")], all=(_runtime != "podman"))
    except Exception:  # noqa: BLE001, S110
        pass

    # Phase 3: Networks — prune by label
    try:  # noqa: SIM105
        dc.network.prune(filters=[("label", "io.ark.test-suite=true")])
    except Exception:  # noqa: BLE001, S110
        pass

    # Phase 4: Clean stale lock files from dead processes
    try:
        repo_root = Path(__file__).resolve().parents[2]
        lock_path = repo_root / ".tests" / ".lock"
        if lock_path.is_file():
            try:
                content = lock_path.read_text()
                pid_str, *_ = content.split("|")
                pid = int(pid_str)
                if pid <= 0 or not os.path.exists(f"/proc/{pid}"):  # noqa: PTH110
                    lock_path.unlink(missing_ok=True)
                    log_framework("cleanup", "removed stale lock from pid=%d", pid)
            except ValueError, OSError:
                lock_path.unlink(missing_ok=True)
    except Exception:  # noqa: BLE001, S110
        pass

    log_framework("cleanup", "orphaned test resource pruning complete")


def prepare_test_env(
    dc: DockerClient,
    *,
    compose_dir: Path,
    backup_dir: Path,
    compose_prefix: str,
    image: str | None = None,
) -> tuple[dict[str, str], Path]:
    """Build a test_env dict with universal config paths and runtime prereqs.

    image overrides ARK_TEST_IMAGE (the lite image for `lite` tests, see helpers.lite_image).
    """
    image = _ensure_image_exists(dc, image)
    compose_dir.mkdir(parents=True, exist_ok=True)

    compose_filename = f"{compose_prefix}{uuid.uuid4().hex[:8]}.yaml"
    compose_path = compose_dir / compose_filename
    compose_file = compose_path.resolve()

    from helpers.compose_cleanup import register as _register_compose

    _register_compose(str(compose_file))

    backup_dir.mkdir(parents=True, exist_ok=True)

    env = os.environ.copy()
    env["COMPOSE_FILE"] = str(compose_file)
    env["ARK_BACKUP_DIR"] = str(backup_dir)
    env["ARK_TEST_IMAGE"] = image
    env["SHUTDOWN_WARNING_MINUTES"] = "0"
    env["ARK_PULL_POLICY"] = "never"

    # Remove stale test resources before creating fresh ones
    prune_orphaned_test_resources(dc)
    return env, compose_file


def cleanup_test_env(dc: DockerClient, env: dict[str, str], compose_file: Path) -> None:
    """Capture diagnostics and tear down containers."""
    captured = env.get("_ARK_DIAGNOSTICS_CAPTURED")
    # Skip diagnostics if already captured by slow_test_protection
    if not captured:
        docker_log_path = env.get("ARK_TEST_DOCKER_LOG")
        if docker_log_path:
            capture_diagnostics(dc, compose_file, Path(docker_log_path))
    # Compose down: slow_test_protection already handled it (via
    # compose_client with the correct compose file).  For non-slow
    # tests, use a properly-scoped client that targets this project.
    if not captured:
        from helpers.compose_client import discover_services, get_docker_client, mark_expected_stop

        try:
            if compose_file.is_file():
                for svc in discover_services(compose_file):
                    mark_expected_stop(compose_file, svc)
                project_dc = get_docker_client(compose_file=compose_file)
                project_dc.compose.down(volumes=True, remove_orphans=True)
            else:
                dc.compose.down(volumes=True, remove_orphans=True)
        except Exception:  # noqa: BLE001, S110
            pass
    # Safety net — catch any resources the compose-down missed
    prune_orphaned_test_resources(dc)


def build_container_instance_config(
    test_env: dict[str, str],
    extra_env: dict[str, str] | None = None,
) -> dict[str, str]:
    instance = f"pytest-ark-{uuid.uuid4().hex[:8]}"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"
    image = test_env.get("ARK_TEST_IMAGE", "ark-server:latest")
    compose_path = Path(test_env["COMPOSE_FILE"])
    write_single_service_compose(
        compose_path,
        instance=instance,
        image=image,
        admin_password=admin_password,
        server_password=server_password,
        extra_env=extra_env,
    )
    return {
        "instance": instance,
        "admin_password": admin_password,
        "server_password": server_password,
    }


def acquire_lock(lock_path: Path, test_name: str) -> bool:
    """Try to acquire the lock file atomically. Returns True if acquired."""
    lock_path = lock_path.resolve()
    lock_dir = lock_path.parent  # noqa: F841

    log_framework("lock", "acquiring lock %s for %s", lock_path, test_name)

    # Use O_CREAT|O_EXCL for atomic lock acquisition
    try:
        fd = os.open(
            str(lock_path),
            os.O_CREAT | os.O_EXCL | os.O_WRONLY,
            0o644,
        )
        with os.fdopen(fd, "w") as f:
            f.write(f"{os.getpid()}|{time.time()}|{test_name}")
        log_framework("lock", "lock acquired (pid=%d)", os.getpid())
        return True
    except FileExistsError:
        # Lock exists — check if it's stale
        try:
            content = lock_path.read_text()
            pid_str, ts_str, name = content.split("|")  # noqa: RUF059
            pid = int(pid_str)
            if pid > 0 and os.path.exists(f"/proc/{pid}"):  # noqa: PTH110
                log_framework("lock", "lock held by pid=%d (%s), skipping", pid, name)
                return False
            log_framework("lock", "stale lock from pid=%d (%s), removing", pid, name)
            lock_path.unlink(missing_ok=True)
            return acquire_lock(lock_path, test_name)
        except ValueError, OSError:
            lock_path.unlink(missing_ok=True)
            return acquire_lock(lock_path, test_name)


def release_lock(lock_path: Path) -> None:
    """Release the lock file if we hold it."""
    lock_path = lock_path.resolve()
    if lock_path.exists():
        try:
            content = lock_path.read_text()
            pid_str, *_ = content.split("|")
            owner_pid = int(pid_str)
            # Only release if we own it or it's stale
            if owner_pid != os.getpid():
                if os.path.exists(f"/proc/{owner_pid}"):  # noqa: PTH110
                    log_framework("lock", "not releasing lock owned by pid=%d", owner_pid)
                    return
                log_framework("lock", "releasing stale lock from pid=%d", owner_pid)
        except ValueError, OSError:
            pass
        lock_path.unlink(missing_ok=True)


def cleanup_compose_files(dc: DockerClient, compose_file: Path) -> None:
    if compose_file and compose_file.exists():
        from helpers.compose_client import discover_services, mark_expected_stop

        for svc in discover_services(compose_file):
            mark_expected_stop(compose_file, svc)
        try:  # noqa: SIM105
            dc.compose.down(volumes=True, remove_orphans=True)
        except Exception:  # noqa: BLE001, S110
            pass


def release_lock_and_cleanup(
    dc: DockerClient,
    lock_path: Path,
    compose_file: Path,
) -> None:
    log_framework("lock", "cleanup: compose down + lock release for %s", compose_file)
    cleanup_compose_files(dc, compose_file)
    release_lock(lock_path)


def relabel_image(dc: DockerClient, image: str, labels: dict[str, str], workdir: Path) -> None:
    """Rebuild image in place with extra labels (one metadata-only layer, no file changes)."""
    workdir.mkdir(parents=True, exist_ok=True)
    lines = [f"FROM {image}", *(f'LABEL {key}="{value}"' for key, value in labels.items())]
    (workdir / "Dockerfile").write_text("\n".join(lines) + "\n")
    dc.buildx.build(workdir, tags=[image], load=True)


def image_label(
    dc: DockerClient,
    image: str,
    label: str,
) -> str:
    """Read a single label from a container image."""
    try:
        labels = dc.images(image)[0].config.labels
        value = labels.get(label, "")
        return "" if value == "<no value>" else value
    except Exception:  # noqa: BLE001
        return ""
