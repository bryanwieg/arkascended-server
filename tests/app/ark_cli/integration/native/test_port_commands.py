from __future__ import annotations

import signal
import socket
import subprocess
from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.infra import wait_for as framework_wait_for


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _free_tcp_and_udp_port() -> int:
    for _ in range(20):
        port = _free_port()
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as u:
                u.bind(("127.0.0.1", port))
            return port
        except OSError:
            continue
    pytest.skip("could not find a port free for both TCP and UDP")


def _probe_tcp(port: int, timeout: float = 1.0) -> bool:
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=timeout):
            return True
    except OSError:
        return False


def _probe_udp_echo(port: int, payload: bytes = b"ping", timeout: float = 1.0) -> bool:
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.settimeout(timeout)
            s.sendto(payload, ("127.0.0.1", port))
            data, _ = s.recvfrom(1024)
            return data == payload
    except OSError:
        return False


def _wait_for_tcp(port: int, timeout_seconds: int = 3) -> None:
    framework_wait_for(
        f"TCP listener on port {port}",
        timeout_seconds,
        lambda: _probe_tcp(port),
    )


def _start_listen(cli_path: Path, port: int, protocol: str = "both") -> subprocess.Popen[str]:
    proc = subprocess.Popen(
        [str(cli_path), "listen", "port", "--port", str(port), "--protocol", protocol],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return proc  # noqa: RET504


def _stop_listen(proc: subprocess.Popen[str]) -> None:
    proc.send_signal(signal.SIGINT)
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=3)


# ---------------------------------------------------------------------------
# listen port
# ---------------------------------------------------------------------------


def test_listen_port_opens_tcp_listener(cli_path: Path) -> None:
    """listen port with --protocol tcp opens a TCP listener on the given port.

    Scenario:
        A free port is selected and 'listen port' is started with --protocol
        tcp. The test waits for a TCP connection to succeed on that port.

    Expected:
        - A TCP connection to the port succeeds while the listener is running.
    """
    port = _free_port()
    proc = _start_listen(cli_path, port, "tcp")
    try:
        _wait_for_tcp(port)
    finally:
        _stop_listen(proc)


def test_listen_port_opens_udp_listener(cli_path: Path) -> None:
    """listen port with --protocol udp opens a UDP echo listener.

    Scenario:
        A free port is selected and 'listen port' is started with --protocol
        udp. The test sends a UDP payload and checks for an echo reply.

    Expected:
        - The UDP echo returns the same payload that was sent.
    """
    port = _free_port()
    proc = _start_listen(cli_path, port, "udp")
    try:
        framework_wait_for(
            f"UDP echo on port {port}",
            3,
            lambda: _probe_udp_echo(port),
        )
    finally:
        _stop_listen(proc)


def test_listen_port_opens_both_protocols(cli_path: Path) -> None:
    """listen port with --protocol both opens TCP and UDP listeners simultaneously.

    Scenario:
        A port free for both TCP and UDP is selected and 'listen port' is
        started with --protocol both. Both a TCP connection and a UDP echo
        are verified.

    Expected:
        - TCP connection succeeds and UDP echo returns the sent payload.
    """
    port = _free_tcp_and_udp_port()
    proc = _start_listen(cli_path, port, "both")
    try:
        _wait_for_tcp(port)
        assert _probe_udp_echo(port), f"UDP echo not working on port {port}"
    finally:
        _stop_listen(proc)


def test_listen_port_default_protocol_is_both(cli_path: Path) -> None:
    """listen port without --protocol defaults to both TCP and UDP.

    Scenario:
        A port is selected and 'listen port' is started without specifying
        --protocol. Both TCP and UDP reachability are checked.

    Expected:
        - TCP connection succeeds and UDP echo works, confirming the default
          is 'both'.
    """
    port = _free_tcp_and_udp_port()
    proc = _start_listen(cli_path, port)
    try:
        _wait_for_tcp(port)
        assert _probe_udp_echo(port), f"UDP echo not working on port {port}"
    finally:
        _stop_listen(proc)


def test_listen_port_cleanup_releases_ports(cli_path: Path) -> None:
    """Stopping the listener releases the bound port.

    Scenario:
        A TCP listener is started on a free port, confirmed reachable, then
        stopped via SIGINT. The test waits for the port to become unavailable.

    Expected:
        - After the listener process exits, TCP probes to the port fail.
    """
    port = _free_port()
    proc = _start_listen(cli_path, port, "tcp")
    _wait_for_tcp(port)
    _stop_listen(proc)
    framework_wait_for(
        f"port {port} released",
        3,
        lambda: not _probe_tcp(port),
    )


def test_listen_port_output_table_columns(cli_path: Path) -> None:
    """listen port prints a table with LABEL, PORT, and PROTO columns.

    Scenario:
        A listener is started with --protocol both and then stopped. The
        stdout output is checked for expected table columns and values.

    Expected:
        - The output contains 'LABEL', 'PORT', 'PROTO', 'tcp', 'udp', and
          the port number.
    """
    port = _free_tcp_and_udp_port()
    proc = _start_listen(cli_path, port, "both")
    try:
        framework_wait_for(
            f"TCP listener on port {port}",
            3,
            lambda: _probe_tcp(port),
        )
    finally:
        _stop_listen(proc)
    out = proc.stdout.read()
    assert "LABEL" in out
    assert "PORT" in out
    assert "PROTO" in out
    assert "tcp" in out
    assert "udp" in out
    assert str(port) in out


def test_listen_port_invalid_port_returns_error(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """listen port rejects a non-numeric port argument.

    Scenario:
        'listen port' is invoked with 'abc' as the port value.

    Expected:
        - The command exits non-zero.
        - The error output mentions 'invalid port'.
    """
    result = run_cmd_native(
        [str(cli_path), "listen", "port", "--port", "abc"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0
    combined = (result.stderr + result.stdout).lower()
    assert "parse error" in combined


def test_listen_port_invalid_protocol_returns_error(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """listen port rejects an unsupported --protocol value.

    Scenario:
        'listen port' is invoked with --protocol icmp, which is not a
        supported protocol.

    Expected:
        - The command exits non-zero.
        - The error output mentions 'protocol'.
    """
    result = run_cmd_native(
        [str(cli_path), "listen", "port", "--port", "8000", "--protocol", "icmp"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0
    combined = (result.stderr + result.stdout).lower()
    assert "protocol" in combined


# ---------------------------------------------------------------------------
# check reachability — port mode, local checks
# ---------------------------------------------------------------------------


def test_check_reachability_port_from_local_open_tcp(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --from local reports OPEN for a bound TCP socket.

    Scenario:
        A TCP socket is bound and listening on a free port. 'check reachability
        port' is invoked with --from local --protocol tcp.

    Expected:
        - The output contains 'OPEN'.
    """
    port = _free_port()
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", port))
        s.listen(1)
        result = run_cmd_native(
            [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local", "--protocol", "tcp"],
            capture_output=True,
            timeout=15,
        )
    assert "OPEN" in result.stdout


def test_check_reachability_port_from_local_closed_tcp(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --from local reports CLOSED for an unbound port.

    Scenario:
        A free port with no listener is probed with 'check reachability port'
        using --from local --protocol tcp.

    Expected:
        - The output contains 'CLOSED'.
    """
    port = _free_port()
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local", "--protocol", "tcp"],
        capture_output=True,
    )
    assert "CLOSED" in result.stdout


def test_check_reachability_port_from_local_open_udp(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --from local reports LISTENING/OPEN for a bound UDP socket.

    Scenario:
        A UDP socket is bound on a free port. 'check reachability port' is
        invoked with --from local --protocol udp.

    Expected:
        - The output contains 'LISTENING' or 'OPEN'.
    """
    port = _free_port()
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
        s.bind(("127.0.0.1", port))
        result = run_cmd_native(
            [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local", "--protocol", "udp"],
            capture_output=True,
            timeout=15,
        )
    assert "LISTENING" in result.stdout or "OPEN" in result.stdout


def test_check_reachability_port_from_local_protocol_both(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --from local with --protocol both reports TCP and UDP.

    Scenario:
        A free port with no listener is probed with --from local --protocol
        both. The output is checked for both protocol labels.

    Expected:
        - The output contains 'tcp' and 'udp' (case-insensitive).
    """
    port = _free_tcp_and_udp_port()
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local", "--protocol", "both"],
        capture_output=True,
    )
    assert "tcp" in result.stdout.lower()
    assert "udp" in result.stdout.lower()


def test_check_reachability_port_default_protocol_is_both(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port without --protocol defaults to both.

    Scenario:
        A free port is probed with --from local but no --protocol flag. The
        output is checked for both protocol labels.

    Expected:
        - The output contains 'tcp' and 'udp', confirming the default is
          'both'.
    """
    port = _free_tcp_and_udp_port()
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local"],
        capture_output=True,
    )
    assert "tcp" in result.stdout.lower()
    assert "udp" in result.stdout.lower()


# ---------------------------------------------------------------------------
# check reachability --listen
# ---------------------------------------------------------------------------


def test_check_reachability_port_with_listen_tcp(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --listen starts a listener and reports OPEN.

    Scenario:
        'check reachability port' is invoked with --listen and --protocol
        tcp on a free port. The command should start a TCP listener and
        report the port as open.

    Expected:
        - The output contains 'OPEN' and mentions 'listener'.
    """
    port = _free_port()
    result = run_cmd_native(
        [
            str(cli_path),
            "check",
            "reachability",
            "--port",
            str(port),
            "--from",
            "local",
            "--protocol",
            "tcp",
            "--listen",
        ],
        capture_output=True,
    )
    assert "OPEN" in result.stdout
    assert "listener" in result.stdout.lower()


def test_check_reachability_port_with_listen_udp(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port --listen with UDP starts a listener and reports open.

    Scenario:
        'check reachability port' is invoked with --listen and --protocol
        udp on a free port. The command should start a UDP listener and
        report the port as open/listening.

    Expected:
        - The output contains 'open' or 'listening' and mentions 'listener'.
    """
    port = _free_port()
    result = run_cmd_native(
        [
            str(cli_path),
            "check",
            "reachability",
            "--port",
            str(port),
            "--from",
            "local",
            "--protocol",
            "udp",
            "--listen",
        ],
        capture_output=True,
    )
    output_lower = result.stdout.lower()
    assert "open" in output_lower or "listening" in output_lower
    assert "listener" in output_lower


def test_check_reachability_port_listen_incompatible_with_from_remote(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port rejects --listen combined with --from remote.

    Scenario:
        'check reachability port' is invoked with both --listen and --from
        remote, which is an incompatible flag combination.

    Expected:
        - The command exits non-zero.
        - The error output mentions both 'listen' and 'remote'.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", "8000", "--from", "remote", "--listen"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0
    combined = (result.stderr + result.stdout).lower()
    assert "listen" in combined and "remote" in combined  # noqa: PT018


# ---------------------------------------------------------------------------
# check reachability --from column selection
# ---------------------------------------------------------------------------


def test_check_reachability_from_local_omits_internet_column(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability --from local shows LOCALHOST column but not INTERNET.

    Scenario:
        A free port is probed with --from local. The output columns are
        checked to ensure the local-only view is presented.

    Expected:
        - The output contains 'LOCALHOST' and does not contain 'INTERNET'.
    """
    port = _free_port()
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", str(port), "--from", "local", "--protocol", "tcp"],
        capture_output=True,
    )
    assert "LOCALHOST" in result.stdout
    assert "INTERNET" not in result.stdout


def test_check_reachability_from_addr_shows_host_column(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability --from <addr> shows HOST column instead of LOCALHOST.

    Scenario:
        A TCP socket is bound on a free port and 'check reachability port' is
        invoked with --from 127.0.0.1. The output is checked for the HOST
        column and the absence of LOCALHOST/INTERNET columns.

    Expected:
        - The output contains 'HOST' and '127.0.0.1'.
        - The output does not contain 'INTERNET' or 'LOCALHOST'.
    """
    port = _free_port()
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", port))
        s.listen(1)
        result = run_cmd_native(
            [str(cli_path), "check", "reachability", "--port", str(port), "--protocol", "tcp", "--from", "127.0.0.1"],
            capture_output=True,
            timeout=15,
        )
    assert "HOST" in result.stdout
    assert "127.0.0.1" in result.stdout
    assert "INTERNET" not in result.stdout
    assert "LOCALHOST" not in result.stdout


# ---------------------------------------------------------------------------
# check reachability — error cases
# ---------------------------------------------------------------------------


def test_check_reachability_missing_subcommand(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability without a subcommand exits non-zero.

    Scenario:
        'check reachability' is invoked with no subcommand (e.g. 'port').

    Expected:
        - The command exits non-zero.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "reachability"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0


def test_check_reachability_port_missing_port_arg(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port without a port argument exits non-zero.

    Scenario:
        'check reachability port' is invoked without providing a port number.

    Expected:
        - The command exits non-zero.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "port"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0


def test_check_reachability_port_ports_flag_rejected(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port rejects the --ports flag (instance-only flag).

    Scenario:
        'check reachability port' is invoked with --ports, which is only
        valid for the instance-based reachability check, not the port mode.

    Expected:
        - The command exits non-zero.
        - The error mentions both 'ports' and 'instance'.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", "8000", "--ports", "asa"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0
    combined = (result.stderr + result.stdout).lower()
    assert "ports" in combined and "instance" in combined  # noqa: PT018


def test_check_reachability_invalid_protocol(
    cli_path: Path,
    run_cmd_native: Callable[..., subprocess.CompletedProcess[str]],
) -> None:
    """check reachability port rejects an unsupported --protocol value.

    Scenario:
        'check reachability port' is invoked with --protocol icmp, which
        is not a supported protocol.

    Expected:
        - The command exits non-zero.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "reachability", "--port", "8000", "--protocol", "icmp"],
        check=False,
        capture_output=True,
    )
    assert result.returncode != 0
