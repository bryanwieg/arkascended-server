from __future__ import annotations

import subprocess
import threading
import time
from pathlib import Path
from typing import TYPE_CHECKING

from helpers.compose import (
    assign_random_ports,
    disable_restart_policy,
    enable_no_battleye,
    enable_skip_ram_check,
    inject_test_suite_label,
    use_lite_image,
)
from helpers.compose_client import compose_logs
from helpers.framework_logger import log_framework
from helpers.lite_image import LITE_IMAGE_PREFIX

if TYPE_CHECKING:
    from python_on_whales import DockerClient


def tail_lines(text: str, *, max_lines: int = 120, max_chars: int = 20000) -> str:
    if not text:
        return ""
    lines = text.splitlines()
    if len(lines) > max_lines:
        lines = lines[-max_lines:]
    trimmed = "\n".join(lines)
    if len(trimmed) > max_chars:
        trimmed = trimmed[-max_chars:]
    return trimmed


def safe_read_tail(path_value: str, *, max_lines: int = 120, max_chars: int = 20000) -> str:
    if not path_value:
        return ""
    path = Path(path_value)
    if not path.is_file():
        return ""
    try:
        return tail_lines(
            path.read_text(encoding="utf-8", errors="replace"),
            max_lines=max_lines,
            max_chars=max_chars,
        )
    except OSError:
        return ""


def _cmd_str(cmd: list[str] | str) -> str:
    return cmd if isinstance(cmd, str) else " ".join(cmd)


def log_command(
    log_file: Path,
    cmd: list[str] | str,
    result: subprocess.CompletedProcess[str],
) -> None:
    with log_file.open("a", encoding="utf-8") as f:
        f.write(f"\n===== {_cmd_str(cmd)} (exit {result.returncode}) =====\n")
        if result.stdout:
            f.write(f"--- stdout ---\n{result.stdout}")
        if result.stderr:
            f.write(f"--- stderr ---\n{result.stderr}")


def log_command_start(log_file: Path, cmd: list[str] | str) -> None:
    """Write a START entry to the commands log before execution begins."""
    with log_file.open("a", encoding="utf-8") as f:
        f.write(f"\n===== {_cmd_str(cmd)} (START) =====\n")


def _heartbeat_loop(
    stop_event: threading.Event,
    log_path: str,
    cmd: list[str] | str,
    threshold: int,
    interval: int,
) -> None:
    """Periodically log heartbeat messages while a command is running."""
    cmd_display = _cmd_str(cmd)[:200]
    start = time.monotonic()
    elapsed = 0
    while not stop_event.wait(timeout=interval):
        elapsed = int(time.monotonic() - start)
        if elapsed >= threshold:
            msg = f"waiting for CLI: {cmd_display} (elapsed={elapsed}s)"
            log_framework("cli-heartbeat", msg)
            log_path_obj = Path(log_path)
            if log_path_obj.is_file():
                with log_path_obj.open("a", encoding="utf-8") as f:
                    f.write(f"\n===== heartbeat ({elapsed}s): {_cmd_str(cmd)} =====\n")


def format_command_failure(
    *,
    cmd: list[str],
    cwd: Path,
    env: dict[str, str],
    exc: subprocess.CalledProcessError,
) -> str:
    stdout_text = tail_lines(exc.stdout or exc.output or "")
    stderr_text = tail_lines(exc.stderr or "")
    cli_log_path = env.get("ARK_TEST_CLI_LOG", "")
    docker_log_path = env.get("ARK_TEST_DOCKER_LOG", "")
    cli_log_tail = safe_read_tail(cli_log_path)
    docker_log_tail = safe_read_tail(docker_log_path)

    details = [
        f"Command failed with exit code {exc.returncode}.",
        f"Command: {' '.join(cmd)}",
        f"CWD: {cwd}",
    ]
    if stdout_text:
        details.append(f"stdout (tail):\n{stdout_text}")
    if stderr_text:
        details.append(f"stderr (tail):\n{stderr_text}")
    if cli_log_tail:
        details.append(f"CLI verbose log tail ({cli_log_path}):\n{cli_log_tail}")
    elif cli_log_path:
        details.append(f"CLI verbose log path: {cli_log_path}")
    if docker_log_tail:
        details.append(f"Docker diagnostics tail ({docker_log_path}):\n{docker_log_tail}")
    elif docker_log_path:
        details.append(f"Docker diagnostics path: {docker_log_path}")
    return "\n\n".join(details)


def run_cmd(
    cmd: list[str] | str,
    *,
    env: dict[str, str],
    cwd: Path,
    check: bool = True,
    capture_output: bool = False,
    quiet: bool = False,
    shell: bool = False,
) -> subprocess.CompletedProcess[str]:
    commands_log = env.get("ARK_TEST_COMMANDS_LOG", "")
    if commands_log and not quiet:
        log_command_start(Path(commands_log), cmd)

    # Start heartbeat for CLI commands (ark-cli can hang during log-follow).
    stop_heartbeat = threading.Event()
    is_cli = isinstance(cmd, list) and cmd and Path(cmd[0]).name == "ark-cli"
    if is_cli and commands_log:
        t = threading.Thread(
            target=_heartbeat_loop,
            args=(stop_heartbeat, commands_log, cmd, 15, 15),
            daemon=True,
        )
        t.start()

    try:
        result = subprocess.run(
            cmd,
            cwd=str(cwd),
            env=env,
            check=False,
            text=True,
            capture_output=capture_output,
            shell=shell,
        )
    finally:
        if is_cli and commands_log:
            stop_heartbeat.set()

    if not quiet:  # noqa: SIM102
        if commands_log:
            log_command(Path(commands_log), cmd, result)
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(result.returncode, cmd, result.stdout, result.stderr)
    return result


def cli_command_args_without_global_options(cmd: list[str]) -> list[str]:
    if not cmd or Path(cmd[0]).name != "ark-cli":
        return []
    args = cmd[1:]
    idx = 0
    while idx < len(args):
        arg = args[idx]
        if arg in ("-v", "--verbose"):
            idx += 1
            if idx < len(args):
                next_arg = args[idx]
                if next_arg and not next_arg.startswith("-"):
                    idx += 1
            continue
        if arg.startswith("-v=") or arg.startswith("--verbose="):  # noqa: PIE810
            idx += 1
            continue
        if arg in ("-h", "--help"):
            idx += 1
            continue
        if arg.startswith("-"):
            return []
        return args[idx:]
    return []


def run_with_compose_patch(
    cmd: list[str] | str,
    *,
    env: dict[str, str],
    cwd: Path,
    check: bool = True,
    capture_output: bool = False,
    quiet: bool = False,
    shell: bool = False,
) -> subprocess.CompletedProcess[str]:
    is_cli = isinstance(cmd, list) and cmd and Path(cmd[0]).name == "ark-cli"
    if is_cli:
        cli_log_file = env.get("ARK_TEST_CLI_LOG", "")
        has_logfile_flag = False
        for idx, arg in enumerate(cmd[1:], start=1):
            if arg.startswith("-v=") or arg.startswith("--verbose="):  # noqa: PIE810
                has_logfile_flag = True
                break
            if arg in ("-v", "--verbose"):
                next_idx = idx + 1
                if next_idx < len(cmd):
                    next_arg = cmd[next_idx]
                    if next_arg and not next_arg.startswith("-"):
                        has_logfile_flag = True
                break
        if cli_log_file and not has_logfile_flag:
            cmd = [cmd[0], "-v", cli_log_file, *cmd[1:]]
    try:
        result = run_cmd(
            cmd,
            env=env,
            cwd=cwd,
            check=check,
            capture_output=capture_output,
            quiet=quiet,
            shell=shell,
        )
    except subprocess.CalledProcessError as exc:
        raise AssertionError(
            format_command_failure(
                cmd=cmd if isinstance(cmd, list) else cmd.split(),
                cwd=cwd,
                env=env,
                exc=exc,
            )
        ) from exc
    cli_args = cli_command_args_without_global_options(cmd) if is_cli else []
    is_create_instance = (len(cli_args) >= 2 and cli_args[0] == "create" and cli_args[1] == "instance") or (
        len(cli_args) >= 2 and cli_args[0] == "create" and cli_args[1] not in {"backup", "schedule", "instance"}
    )
    if is_create_instance:
        compose_path = Path(env.get("COMPOSE_FILE", "docker-compose.yaml"))
        inject_test_suite_label(compose_path)
        assign_random_ports(compose_path)
        disable_restart_policy(compose_path)
        enable_no_battleye(compose_path)
        enable_skip_ram_check(compose_path)
        if env.get("ARK_TEST_IMAGE", "").startswith(LITE_IMAGE_PREFIX):
            use_lite_image(compose_path, env["ARK_TEST_IMAGE"])
    return result


def fail_with_container_logs(
    reason: str,
    *,
    dc: DockerClient,
    instance: str,
    compose_file: str,
    env: dict[str, str],
    cwd: Path,
    tail: int = 200,
) -> None:
    logs = compose_logs(dc, instance, tail=tail)
    details = logs.strip()
    parts = [reason]
    if details:
        parts.append(f"Recent logs:\n{details}")
    cli_log_path = env.get("ARK_TEST_CLI_LOG", "")
    if cli_log_path:
        parts.append(f"CLI verbose log: {cli_log_path}")
    raise AssertionError("\n".join(parts))


def log_phase_snapshot(
    label: str,
    *,
    dc: DockerClient,
    instance: str,
    compose_file: str,
    env: dict[str, str],
    cwd: Path,
    tail: int = 60,
) -> None:
    details = compose_logs(dc, instance, tail=tail).strip()
    if not details:
        return
    commands_log = env.get("ARK_TEST_COMMANDS_LOG", "")
    log_framework("phase", "snapshot: %s", label)
    if commands_log:
        with Path(commands_log).open("a", encoding="utf-8") as f:
            f.write(f"\n===== phase snapshot: {label} =====\n{details}\n")


def capture_diagnostics(dc: DockerClient, compose_file: Path, output_path: Path) -> None:
    """Append a container state summary (name, id, image, status, PID, exit code) to ``output_path``.

    Per-service log content is streamed continuously by the ``log_streamer``
    fixture to ``compose.<service>.log`` files.
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)
    from helpers.compose_client import get_docker_client

    dc_log = get_docker_client(compose_file=compose_file) if compose_file and compose_file.is_file() else dc

    containers_list: list = []
    try:
        containers_list = dc_log.compose.ps(all=True)
    except Exception:  # noqa: BLE001
        containers_list = []

    with output_path.open("a", encoding="utf-8") as log_file:
        log_file.write(f"\n===== {time.strftime('%y%m%d-%H%M%S')} container states =====\n")
        if containers_list:
            for c in containers_list:
                s = c.state
                created_t = getattr(c, "created", "")
                log_file.write(
                    f"  name: {c.name}\n"
                    f"    id: {c.id}\n"
                    f"    image: {c.config.image}\n"
                    f"    created: {created_t}\n"
                    f"    status: {s.status}\n"
                    f"    running: {s.running}\n"
                    f"    exit_code: {s.exit_code}\n"
                    f"    pid: {s.pid}\n"
                )
        else:
            log_file.write("  (no containers found for this compose project)\n")
