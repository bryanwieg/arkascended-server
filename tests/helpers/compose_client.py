"""Typed Docker/Podman client built on python-on-whales.

Provides a thread-safe ``DockerClient`` factory and high-level test helpers
for compose operations. Replaces hand-rolled subprocess wrappers in
``container_runtime.py``, ``docker.py``, and ``diagnostics.py`` while
fixing the compose project-name mismatch between the Go SDK backend and
the test framework.

Usage::

    from helpers.compose_client import get_docker_client, container_running

    dc = get_docker_client(compose_file=compose_path)
    assert container_running(dc, "my-instance")
"""

from __future__ import annotations

import os
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path

import yaml
from helpers.framework_logger import log_framework
from python_on_whales import DockerClient

# ---------------------------------------------------------------------------
# Per-service log watchers (shell subprocess, not SDK)
# ---------------------------------------------------------------------------
#
# Each watcher runs ``compose logs --follow --tail=all <service>`` in a
# subprocess and streams output to ``compose.<service>.log``. When the
# subprocess exits (container stopped/removed), the watcher thread checks
# ``_EXPECTED_STOP`` to determine whether the exit was planned (via
# ``compose_stop``/``compose_down``) or unexpected (crash). Dead watchers
# are never restarted — this replaces the old polling approach that produced
# a flood of "started log streamer" messages.

_WATCHERS: dict[tuple[str, str], dict] = {}
_EXPECTED_STOP: set[tuple[str, str]] = set()
_WATCHER_LOCK = threading.Lock()


def _watcher_thread(
    compose_file: str,
    service: str,
    proc: subprocess.Popen,
    log_path: Path,
) -> None:
    """Read from a ``compose logs --follow`` subprocess until it exits."""
    stdout_pipe = proc.stdout
    if stdout_pipe is None:
        return
    try:
        with open(log_path, "ab") as output_handle:  # noqa: PTH123
            for line in iter(stdout_pipe.readline, b""):
                output_handle.write(line)
                output_handle.flush()
                sys.stderr.buffer.write(line)
                sys.stderr.buffer.flush()
    except ValueError, OSError:
        pass

    proc.wait()
    key = (compose_file, service)

    with _WATCHER_LOCK:
        _WATCHERS.pop(key, None)
        if key in _EXPECTED_STOP:
            _EXPECTED_STOP.discard(key)
            log_framework(
                "container",
                "container %s stopped (exit code %d)",
                service,
                proc.returncode,
            )
        else:
            log_framework(
                "container",
                "container %s exited unexpectedly (exit code %d)",
                service,
                proc.returncode,
            )


def start_service_watcher(
    compose_file: Path,
    service: str,
    output_dir: Path,
    runtime: str = "docker",
) -> None:
    """Start a watcher thread for a single compose service.

    The watcher runs ``compose logs --follow --tail=all <service>`` and
    streams output to ``compose.<service>.log``. When the container exits,
    the watcher logs the event and stops permanently.

    If no container exists for the service, the subprocess exits immediately
    and no watcher thread is started — the function returns silently.
    """
    key = (str(compose_file), service)
    with _WATCHER_LOCK:
        if key in _WATCHERS:
            return

    log_path = output_dir / f"compose.{service}.log"
    proc = subprocess.Popen(
        [runtime, "compose", "-f", str(compose_file), "logs", "--follow", "--timestamps", "--tail=all", service],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        preexec_fn=os.setsid,  # noqa: PLW1509
    )

    # If the subprocess exits within 2 seconds, no container exists yet.
    # Clean up silently — the watcher will be re-attempted on the next start.
    try:
        proc.wait(timeout=2)
    except subprocess.TimeoutExpired:
        pass
    else:
        return

    with _WATCHER_LOCK:
        _WATCHERS[key] = {
            "proc": proc,
            "log_path": log_path,
            "started_at": time.monotonic(),
        }

    thread = threading.Thread(
        target=_watcher_thread,
        args=(str(compose_file), service, proc, log_path),
        daemon=True,
    )
    thread.start()

    log_framework("compose", "started watcher for %s (pid=%d)", service, proc.pid)


def _kill_proc(proc: subprocess.Popen) -> None:
    try:
        pgid = os.getpgid(proc.pid)
        os.killpg(pgid, signal.SIGTERM)
        proc.wait(timeout=5)
    except OSError, subprocess.TimeoutExpired:
        try:
            pgid = os.getpgid(proc.pid)
            os.killpg(pgid, signal.SIGKILL)
        except OSError:
            pass


def stop_service_watcher(compose_file: Path, service: str) -> None:
    """Kill a running watcher subprocess for a single service."""
    key = (str(compose_file), service)
    with _WATCHER_LOCK:
        info = _WATCHERS.pop(key, None)
        _EXPECTED_STOP.discard(key)
    if info is not None:
        _kill_proc(info["proc"])


def stop_all_watchers() -> None:
    """Kill all watcher subprocesses. Called during fixture teardown."""
    with _WATCHER_LOCK:
        keys = list(_WATCHERS.keys())
        _WATCHERS.clear()
        _EXPECTED_STOP.clear()
    for key in keys:
        info = _WATCHERS.get(key)
        if info is not None:
            _kill_proc(info["proc"])
    log_framework("compose", "stopped all watchers")


def mark_expected_stop(compose_file: Path, service: str) -> None:
    """Mark a service stop as expected so the watcher logs it cleanly."""
    with _WATCHER_LOCK:
        _EXPECTED_STOP.add((str(compose_file), service))


def mark_unexpected_stop(compose_file: Path, service: str) -> None:
    """Clear expected-stop for a service (e.g., before a restart)."""
    with _WATCHER_LOCK:
        _EXPECTED_STOP.discard((str(compose_file), service))


def reset_watchers() -> None:
    """Clear all watcher state for a fresh test."""
    with _WATCHER_LOCK:
        _WATCHERS.clear()
        _EXPECTED_STOP.clear()


def start_all_watchers(
    compose_file: Path,
    output_dir: Path,
    runtime: str = "docker",
) -> dict[str, dict]:
    """Start watchers for all services defined in the compose file.

    This is a **discovery-only** function — it never restarts watchers for
    services that already have one or that have permanently exited.
    """
    services = discover_services(compose_file)
    started: dict[str, dict] = {}
    for service in services:
        key = (str(compose_file), service)
        if key not in _WATCHERS:
            start_service_watcher(compose_file, service, output_dir, runtime)
            with _WATCHER_LOCK:
                info = _WATCHERS.get(key)
                if info is not None:
                    started[service] = info
    return started


# --- backward-compatible wrappers ---


def start_all_streamers(*args, **kwargs):
    return start_all_watchers(*args, **kwargs)


def stop_all_streamers():
    return stop_all_watchers()


def kill_service_streamers(*args, **kwargs):
    return stop_all_watchers()


# ---------------------------------------------------------------------------
# Service discovery
# ---------------------------------------------------------------------------


def discover_services(compose_file: Path) -> list[str]:
    try:
        content = compose_file.read_text(encoding="utf-8")
        if not content.strip():
            return []
        data = yaml.safe_load(content)
        if not isinstance(data, dict):
            return []
        services = data.get("services", {})
        if not isinstance(services, dict):
            return []
        return list(services.keys())
    except Exception:  # noqa: BLE001
        return []


# ---------------------------------------------------------------------------
# Thread-local client cache
# ---------------------------------------------------------------------------

_thread_local = threading.local()


def _cache_key(compose_file: Path | None, runtime: str) -> str:
    return f"{compose_file}|{runtime}"


def get_docker_client(
    *,
    compose_file: Path | None = None,
) -> DockerClient:
    """Get or create a DockerClient for the current test thread.

    Clients are cached per thread, compose file, and runtime, so repeated
    calls with the same parameters return the same instance.

    The compose project name is derived from the compose file path's parent
    directory by both ``python-on-whales`` and ``docker compose -f <file>``,
    so no explicit ``compose_project_name`` is needed — they stay in sync.

    Parameters
    ----------
    compose_file
        Path to the compose YAML file. Pass ``None`` for global ops (``ps``,
        ``pull``, etc.) that don't target a specific project.
    """
    runtime = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    key = _cache_key(compose_file, runtime)

    cache = getattr(_thread_local, "_dc_cache", {})
    if key in cache:
        return cache[key]

    kwargs: dict = {}
    if runtime == "podman":
        kwargs["client_binary"] = "podman"
    if compose_file is not None:
        kwargs["compose_files"] = [str(compose_file)]

    client = DockerClient(**kwargs)
    cache = getattr(_thread_local, "_dc_cache", {})
    cache[key] = client
    _thread_local._dc_cache = cache
    return client


def _containers(dc: DockerClient, service: str) -> list:
    """Return containers for a service, or empty list on error."""
    try:
        return dc.compose.ps(services=[service], all=True)
    except Exception:  # noqa: BLE001
        return []


# ---------------------------------------------------------------------------
# High-level helpers
# ---------------------------------------------------------------------------


def container_running(dc: DockerClient, service: str) -> bool:
    """Return ``True`` if the compose service has a running container."""
    for c in _containers(dc, service):  # noqa: SIM110
        if c.state.status == "running":
            return True
    return False


def container_exited(dc: DockerClient, service: str) -> bool:
    """Return ``True`` if the compose service has an exited container."""
    for c in _containers(dc, service):  # noqa: SIM110
        if c.state.status == "exited":
            return True
    return False


def container_id(dc: DockerClient, service: str) -> str:
    """Return the container ID for a compose service, or empty string."""
    for c in _containers(dc, service):
        return c.id
    return ""


def container_exit_code(dc: DockerClient, service: str) -> int | None:
    """Return the exit code for a compose service container, or ``None``."""
    for c in _containers(dc, service):
        return c.state.exit_code
    return None


# ---------------------------------------------------------------------------
# SDK-based replacements for ContainerRuntime shell command builders
# ---------------------------------------------------------------------------


def compose_exec(
    dc: DockerClient,
    service: str,
    cmd: list[str],
    *,
    envs: dict[str, str] | None = None,
    capture: bool = True,
) -> str | None:
    """Execute a command inside a running compose service container.

    When *capture* is True (default), sets ``tty=False`` so python-on-whales
    returns stdout as a string. Pass ``capture=False`` for fire-and-forget
    commands where you only care about the exit code.
    """
    log_framework("sdk", "compose execute %s: %s", service, " ".join(cmd))
    return dc.compose.execute(
        service,
        cmd,
        envs=envs or {},
        tty=not capture,
    )


def compose_run(
    dc: DockerClient,
    service: str,
    cmd: list[str] | None = None,
    *,
    detach: bool = False,
    remove: bool = False,
    envs: dict[str, str] | None = None,
    entrypoint: str | None = None,
    dependencies: bool = True,
    volumes: list | None = None,
    name: str | None = None,
    capture: bool = True,
) -> object:
    """Run a one-off command in a new container for the compose service.

    When *capture* is True (default), sets ``tty=False`` so stderr is
    captured separately and DockerException.exc.stderr is populated
    on non-zero exits. Pass ``capture=False`` for interactive sessions.
    """
    log_framework("sdk", "compose run %s: %s", service, " ".join(cmd or []))
    return dc.compose.run(
        service,
        command=cmd,
        detach=detach,
        remove=remove,
        envs=envs or {},
        entrypoint=entrypoint,
        dependencies=dependencies,
        volumes=volumes or [],
        name=name,
        tty=not capture,
    )


def _compose_file_from_dc(dc: DockerClient) -> Path | None:
    """Derive the compose file path from a DockerClient, if available."""
    cmd = getattr(dc.compose, "docker_compose_cmd", None)
    if cmd:
        for i, part in enumerate(cmd):
            if part == "--file" and i + 1 < len(cmd):
                return Path(cmd[i + 1])
    return None


def compose_stop(
    dc: DockerClient,
    service: str,
    *,
    timeout: int = 10,
) -> None:
    """Stop a compose service with a timeout (seconds)."""
    compose_file = _compose_file_from_dc(dc)
    if compose_file is not None:
        mark_expected_stop(compose_file, service)
    log_framework("sdk", "compose stop %s (timeout=%ds)", service, timeout)
    dc.compose.stop(services=[service], timeout=timeout)


def compose_kill(
    dc: DockerClient,
    service: str,
    signal: str = "SIGKILL",
) -> None:
    """Send a signal to a compose service container."""
    compose_file = _compose_file_from_dc(dc)
    if compose_file is not None:
        mark_expected_stop(compose_file, service)
    log_framework("sdk", "compose kill %s (signal=%s)", service, signal)
    dc.compose.kill(services=[service], signal=signal)


def copy_from_service(
    dc: DockerClient,
    service: str,
    src_path: str,
    dst_path: str,
) -> None:
    """Copy a file or directory from a compose service container to the host.

    Resolves the service name to its container, then uses
    ``dc.copy()`` (container-level API).
    """
    containers = dc.compose.ps(services=[service], all=True)
    if not containers:
        raise RuntimeError(f"No container found for service {service!r}")
    container = containers[0]
    dc.copy((container, src_path), dst_path)


def compose_logs(
    dc: DockerClient,
    service: str,
    *,
    tail: int = 200,
) -> str:
    """Return recent logs for a compose service."""
    return dc.compose.logs(services=[service], tail=str(tail))


def compose_ps_all(
    dc: DockerClient,
    service: str,
) -> list:
    """Return all containers for a compose service, including stopped ones."""
    return dc.compose.ps(services=[service], all=True)


def get_container_port(
    dc: DockerClient,
    service: str,
    container_port: int,
    protocol: str = "udp",
) -> str | None:
    """Return the host-side mapped port for a container port.

    Replaces ``container_runtime.compose_port(...)``.

    Parameters
    ----------
    dc
        The ``DockerClient`` for the project.
    service
        Compose service name.
    container_port
        Port inside the container.
    protocol
        Protocol (``"udp"`` or ``"tcp"``).

    Returns
    -------
    str or None
        The host-side mapped port, or ``None`` if not found.
    """
    result = dc.compose.port(service, container_port, protocol=protocol)
    if result is None:
        return None
    if isinstance(result, tuple):
        return str(result[1])
    return str(result)


def wait_for_container_state(
    dc: DockerClient,
    service: str,
    state: str = "running",
    timeout: float = 120.0,
    interval: float = 5.0,
    description: str = "",
) -> None:
    """Wait until a compose service reaches the given container state.

    Parameters
    ----------
    dc
        The ``DockerClient`` for the project.
    service
        Compose service name.
    state
        Target container state (``"running"``, ``"exited"``).
    timeout
        Maximum seconds to wait.
    interval
        Polling interval in seconds.
    description
        Human-readable label for log messages.
    """
    label = description or f"service {service} to reach state {state}"
    log_framework("poll", "waiting for %s (timeout=%ds)", label, int(timeout))
    deadline = time.monotonic() + timeout
    state_fn = container_running if state == "running" else container_exited

    while True:
        if state_fn(dc, service):
            elapsed = int(time.monotonic() - (deadline - timeout))
            log_framework("poll", "done waiting for %s (elapsed=%ds)", label, elapsed)
            return
        elapsed = int(time.monotonic() - (deadline - timeout))
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            log_framework("poll", "TIMEOUT waiting for %s", label)
            raise TimeoutError(f"Timed out waiting for {label}.")
        log_framework("poll", "waiting for %s (elapsed=%ds, remaining=%ds)", label, elapsed, int(remaining))
        time.sleep(interval)


def wait_for_service_premature_exit(
    dc: DockerClient,
    service: str,
    timeout: float = 120.0,
    interval: float = 5.0,
) -> None:
    """Poll and raise if the service exits before becoming ready.

    Meant to be called concurrently (e.g., in a separate thread) alongside
    readiness polling to detect early exits.
    """
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if container_exited(dc, service):
            raise RuntimeError(f"Service exited prematurely: {service}")
        time.sleep(interval)


# ---------------------------------------------------------------------------
# High-level container operation helpers
# These encapsulate common compose operations so tests never call
# dc.compose.* directly — only framework helper functions.
# ---------------------------------------------------------------------------


def wait_for_startup_log(
    dc: DockerClient,
    service: str,
    server_log_path: str = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log",
    timeout: int = 300,
) -> None:
    """Wait until the service's server log contains the 'advertising for join' line."""
    from helpers.infra import wait_for as _wait_for

    def _ready() -> bool:
        if not container_running(dc, service):
            return False
        try:
            dc.compose.execute(
                service,
                [
                    "sh",
                    "-c",
                    f"test -f {server_log_path} && "  # noqa: ISC004
                    f'grep -q "Server has completed startup and is now advertising for join." '
                    f"{server_log_path}",
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    _wait_for("startup log (advertising for join)", timeout, _ready)


def container_mkdir(dc: DockerClient, service: str, path: str) -> None:
    """Create a directory (and parents) inside a running container."""
    compose_exec(dc, service, ["sh", "-c", f"mkdir -p {path}"])


def container_write_file(dc: DockerClient, service: str, path: str, content: str) -> None:
    """Write content to a file inside a running container."""
    from helpers.framework_logger import log_framework as _log

    _log("sdk", "write file %s", path)
    compose_exec(dc, service, ["sh", "-c", f"cat > {path} <<'ARKTESTEOF'\n{content}\nARKTESTEOF"])


def container_delete_file(dc: DockerClient, service: str, path: str) -> None:
    """Delete a file inside a running container."""
    compose_exec(dc, service, ["sh", "-c", f"rm -f {path}"])


def container_file_exists(dc: DockerClient, service: str, path: str) -> bool:
    """Check if a file exists inside a running container."""
    try:
        compose_exec(dc, service, ["test", "-f", path])
        return True
    except Exception:  # noqa: BLE001
        return False


def container_exec_python(dc: DockerClient, service: str, python_code: str) -> str:
    """Execute Python code inside a running container and return stdout."""
    result = compose_exec(dc, service, ["python3", "-c", python_code])
    return result or ""


def container_started_at(dc: DockerClient, service: str) -> str | None:
    # __戻り値__: str | None
    """Return the started_at timestamp for a service's primary container, or None."""
    containers = dc.compose.ps(services=[service], all=True)
    if not containers:
        return None
    return containers[0].state.started_at


def container_start(
    dc: DockerClient, service: str, *, output_dir: Path | None = None, runtime: str | None = None
) -> None:
    """Start a compose service (compose up -d) and attach a watcher."""
    compose_file = _compose_file_from_dc(dc)
    if compose_file is not None:
        mark_unexpected_stop(compose_file, service)
    log_framework("sdk", "compose start %s", service)
    dc.compose.up(services=[service], detach=True, no_build=True)
    if compose_file is not None and output_dir is not None:
        rt = runtime or os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
        start_service_watcher(compose_file, service, output_dir, rt)


def container_ensure_image_tag(dc: DockerClient, name: str, tag: str = "ark-server:latest") -> None:
    """Tag an image so the compose file resolves it, if not already tagged."""
    if name != tag:
        dc.image.tag(name, tag)


def container_env(dc: DockerClient, service: str, var_name: str) -> str:
    """Read an environment variable from a running container."""
    result = compose_exec(dc, service, ["sh", "-c", f"echo ${var_name}"])
    return result.strip() if result else ""


# ---------------------------------------------------------------------------
# Container instance startup (V2 — uses DockerClient instead of run_cmd)
# ---------------------------------------------------------------------------


def start_container_instance(
    *,
    dc: DockerClient,
    test_env: dict[str, str],
    repo_root: Path,
    instance: str,
    admin_password: str,
    server_password: str,
) -> dict[str, str]:
    """Start a container instance and wait for RCON readiness using the SDK client.

    This is the V2 replacement for :func:`~helpers.infra.start_container_instance_common`.
    It uses ``python-on-whales`` instead of hand-rolled ``run_cmd`` calls, and
    benefits from explicit project-name coordination.
    """
    # Clear expected-stop and attach watcher before starting.
    compose_file = Path(test_env["COMPOSE_FILE"])
    artifact_dir = test_env.get("ARK_TEST_ARTIFACT_DIR", "")
    runtime = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    mark_unexpected_stop(compose_file, instance)

    # Start the compose service.
    dc.compose.up(services=[instance], detach=True, no_build=True)
    log_framework("lifecycle", "started container %s (compose up via SDK)", instance)

    # Attach a log watcher now that the container exists.
    if artifact_dir:
        start_service_watcher(compose_file, instance, Path(artifact_dir), runtime)

    rcon_timeout = int(os.environ.get("ARK_TEST_RCON_TIMEOUT", "300"))
    startup_log_timeout = int(os.environ.get("ARK_TEST_STARTUP_LOG_TIMEOUT", "300"))
    server_log_path = "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"

    def _fail(reason: str) -> None:
        from helpers.diagnostics import fail_with_container_logs

        fail_with_container_logs(
            reason,
            dc=dc,
            instance=instance,
            compose_file=test_env["COMPOSE_FILE"],
            env=test_env,
            cwd=repo_root,
        )

    def startup_log_ready() -> bool:
        if not container_running(dc, instance):
            if container_exited(dc, instance):
                log_framework("lifecycle", "service %s exited before startup log ready", instance)
                _fail("Service exited before startup log became ready.")
            return False
        try:
            dc.compose.execute(
                instance,
                [
                    "sh",
                    "-c",
                    f"test -f {server_log_path} && "  # noqa: ISC004
                    f'grep -q "Server has completed startup and is now advertising for join." '
                    f"{server_log_path}",
                ],
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    def rcon_ready() -> bool:
        if not container_running(dc, instance):
            if container_exited(dc, instance):
                log_framework("lifecycle", "service %s exited before RCON ready", instance)
                _fail("Service exited before RCON became ready.")
            return False
        try:
            dc.compose.execute(
                instance, ["/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py", "listplayers"]
            )
            return True
        except Exception:  # noqa: BLE001
            return False

    log_framework("lifecycle", "waiting for startup log (instance=%s, timeout=%ds)", instance, startup_log_timeout)
    from helpers.infra import wait_for as _wait_for

    _wait_for("startup log readiness", startup_log_timeout, startup_log_ready)
    log_framework("lifecycle", "startup log ready, waiting for RCON (instance=%s, timeout=%ds)", instance, rcon_timeout)
    _wait_for("RCON readiness", rcon_timeout, rcon_ready)
    log_framework("lifecycle", "container %s fully ready (RCON connected)", instance)

    return {
        "instance": instance,
        "admin_password": admin_password,
        "server_password": server_password,
    }
