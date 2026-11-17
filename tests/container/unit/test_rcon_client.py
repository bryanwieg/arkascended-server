"""rcon_client and its callers against a scripted fake RCON server (no ARK server needed).

The real-server checks live in tests/container/integration/slow/test_rcon.py; these cover the
protocol edge cases a real server can't be made to produce on demand.
"""

from __future__ import annotations

import contextlib
import importlib
import importlib.util
import socket
import struct
import subprocess
import sys
import threading
from collections.abc import Callable, Iterator
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

RUNTIME_DIR = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
if str(RUNTIME_DIR) not in sys.path:
    sys.path.insert(0, str(RUNTIME_DIR))

PASSWORD = "AdminPass123"


def _load_script(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, RUNTIME_DIR / filename)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(name="rc")
def rc_fixture():
    return importlib.import_module("rcon_client")


# --- fake server ------------------------------------------------------------------------------


def packet(packet_id: int, packet_type: int, body: bytes = b"") -> bytes:
    """Build a packet by hand, independently of rcon_client.encode_packet."""
    payload = struct.pack("<ii", packet_id, packet_type) + body + b"\x00\x00"
    return struct.pack("<i", len(payload)) + payload


def recv_packet(conn: socket.socket) -> tuple[int, int, bytes]:
    size = struct.unpack("<i", recv_exact(conn, 4))[0]
    payload = recv_exact(conn, size)
    packet_id, packet_type = struct.unpack("<ii", payload[:8])
    return packet_id, packet_type, payload[8:-2]


def recv_exact(conn: socket.socket, count: int) -> bytes:
    data = b""
    while len(data) < count:
        chunk = conn.recv(count - len(data))
        if not chunk:
            raise ConnectionError("client closed")
        data += chunk
    return data


class FakeServer:
    """Accepts one connection per handler call and records what the client sent."""

    def __init__(self, handler: Callable[[socket.socket, FakeServer], None]) -> None:
        self.handler = handler
        self.received: list[tuple[int, int, bytes]] = []
        self.listener = socket.create_server(("127.0.0.1", 0))
        self.port = self.listener.getsockname()[1]
        self.thread = threading.Thread(target=self._serve, daemon=True)
        self.thread.start()

    def _serve(self) -> None:
        while True:
            try:
                conn, _ = self.listener.accept()
            except OSError:
                return
            with conn, contextlib.suppress(OSError):  # ConnectionError is an OSError
                self.handler(conn, self)

    def recv(self, conn: socket.socket) -> tuple[int, int, bytes]:
        received = recv_packet(conn)
        self.received.append(received)
        return received

    def close(self) -> None:
        self.listener.close()


@pytest.fixture
def serve() -> Iterator[Callable[..., FakeServer]]:
    servers: list[FakeServer] = []

    def start(handler: Callable[[socket.socket, FakeServer], None]) -> FakeServer:
        server = FakeServer(handler)
        servers.append(server)
        return server

    yield start
    for server in servers:
        server.close()


def ark_like(reply: bytes, *, empty_before_auth: bool = True, extra_before_reply: bytes = b""):
    """Handler that behaves like ARK: checks the password, answers one command."""

    def handler(conn: socket.socket, server: FakeServer) -> None:
        auth_id, _, password = server.recv(conn)
        if empty_before_auth:
            conn.sendall(packet(auth_id, 0))
        ok = password == PASSWORD.encode()
        conn.sendall(packet(auth_id if ok else -1, 2))
        if not ok:
            return
        command_id, _, _ = server.recv(conn)
        conn.sendall(extra_before_reply + packet(command_id, 0, reply))

    return handler


def closed_port() -> int:
    """Return a local port with no listener, so connecting to it is refused."""
    with socket.create_server(("127.0.0.1", 0)) as probe:
        return probe.getsockname()[1]


def run(rc, server: FakeServer, command: str = "saveworld", **kwargs) -> str:
    kwargs.setdefault("password", PASSWORD)
    kwargs.setdefault("timeout", 2)
    return rc.rcon_command(command, host="127.0.0.1", port=server.port, **kwargs)


# --- protocol ---------------------------------------------------------------------------------


def test_encode_packet_matches_the_source_rcon_layout(rc) -> None:
    assert rc.encode_packet(7, 2, "saveworld") == packet(7, 2, b"saveworld")


@pytest.mark.parametrize("empty_before_auth", [True, False], ids=["empty-packet-before-auth", "auth-reply-only"])
def test_command_returns_the_reply(rc, serve, empty_before_auth: bool) -> None:
    server = serve(ark_like(b"World Saved", empty_before_auth=empty_before_auth))
    assert run(rc, server) == "World Saved"
    assert [(t, body) for _, t, body in server.received] == [(3, PASSWORD.encode()), (2, b"saveworld")]


def test_ark_no_response_reply_is_returned_as_text(rc, serve) -> None:
    server = serve(ark_like(b"Server received, But no response!! \n"))
    assert run(rc, server, "ServerChat hello").startswith("Server received, But no response!!")


def test_wrong_password_raises_auth_error_and_sends_no_command(rc, serve) -> None:
    server = serve(ark_like(b"World Saved"))
    with pytest.raises(rc.RconAuthError, match="rejected"):
        run(rc, server, password="wrong")
    assert len(server.received) == 1


def test_unexpected_packets_before_the_reply_are_skipped(rc, serve) -> None:
    # Unknown types and ids crash some clients (conqp/rcon #34, #35); they must be ignored.
    noise = packet(99, 4, b"console line") + packet(-1, 0, b"broadcast") + packet(2, 77, b"odd type")
    server = serve(ark_like(b"World Saved", extra_before_reply=noise))
    assert run(rc, server) == "World Saved"


def test_endless_unexpected_packets_fail_instead_of_looping(rc, serve) -> None:
    server = serve(ark_like(b"never reached", extra_before_reply=packet(99, 0, b"noise") * 50))
    with pytest.raises(rc.RconError, match="without the expected reply"):
        run(rc, server)


def test_reply_split_across_tcp_segments_is_reassembled(rc, serve) -> None:
    def handler(conn: socket.socket, server: FakeServer) -> None:
        auth_id, _, _ = server.recv(conn)
        conn.sendall(packet(auth_id, 2))
        command_id, _, _ = server.recv(conn)
        for byte in packet(command_id, 0, b"Players: 3"):
            conn.sendall(bytes([byte]))

    assert run(rc, serve(handler), "ListPlayers") == "Players: 3"


def test_invalid_utf8_is_replaced_not_fatal(rc, serve) -> None:
    assert run(rc, serve(ark_like(b"caf\xe9"))) == "caf�"


@pytest.mark.parametrize("size", [-1, 3, 50 * 1024 * 1024], ids=["negative", "too-small", "too-large"])
def test_invalid_packet_size_raises(rc, serve, size: int) -> None:
    def handler(conn: socket.socket, server: FakeServer) -> None:
        server.recv(conn)
        conn.sendall(struct.pack("<i", size) + b"\x00" * 12)

    with pytest.raises(rc.RconError, match="invalid packet size"):
        run(rc, serve(handler))


def test_connection_closed_mid_reply_raises(rc, serve) -> None:
    def handler(conn: socket.socket, server: FakeServer) -> None:
        server.recv(conn)
        conn.sendall(packet(1, 2)[:6])

    with pytest.raises(rc.RconError, match="closed the connection"):
        run(rc, serve(handler))


def test_silent_server_times_out(rc, serve) -> None:
    def handler(conn: socket.socket, server: FakeServer) -> None:
        server.recv(conn)
        conn.recv(1)  # hold the connection open until the client gives up

    with pytest.raises(rc.RconError, match=r"timed out after 0\.3 seconds"):
        run(rc, serve(handler), timeout=0.3)


def test_refused_connection_raises(rc) -> None:
    with pytest.raises(rc.RconError, match="Failed to connect"):
        rc.rcon_command("saveworld", host="127.0.0.1", port=closed_port(), password=PASSWORD, timeout=1)


def test_missing_password_fails_without_connecting(rc, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("SERVER_ADMIN_PASSWORD", raising=False)
    monkeypatch.setattr(socket, "create_connection", lambda *_a, **_k: pytest.fail("must not connect"))
    with pytest.raises(rc.RconAuthError, match="not set"):
        rc.rcon_command("saveworld")


def test_non_numeric_rcon_port_is_an_rcon_error(rc, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", PASSWORD)
    monkeypatch.setenv("RCON_PORT", "abc")
    monkeypatch.setattr(socket, "create_connection", lambda *_a, **_k: pytest.fail("must not connect"))
    with pytest.raises(rc.RconError, match="RCON_PORT must be a port number"):
        rc.rcon_command("saveworld")


def test_connection_settings_default_to_the_environment(rc, serve, monkeypatch: pytest.MonkeyPatch) -> None:
    server = serve(ark_like(b"ok"))
    monkeypatch.setenv("RCON_HOST", "127.0.0.1")
    monkeypatch.setenv("RCON_PORT", str(server.port))
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", PASSWORD)
    assert rc.rcon_command("saveworld", timeout=2) == "ok"


# --- callers: no child process, so the password never reaches a command line ----------------


@pytest.fixture
def no_subprocess(monkeypatch: pytest.MonkeyPatch) -> None:
    def forbidden(*_args, **_kwargs):
        pytest.fail("RCON must not start a child process (its argv would expose the password)")

    monkeypatch.setattr(subprocess, "run", forbidden)
    monkeypatch.setattr(subprocess, "Popen", forbidden)


@pytest.fixture
def rcon_env(monkeypatch: pytest.MonkeyPatch) -> Callable[[FakeServer], None]:
    def point_at(server: FakeServer) -> None:
        monkeypatch.setenv("RCON_HOST", "127.0.0.1")
        monkeypatch.setenv("RCON_PORT", str(server.port))
        monkeypatch.setenv("SERVER_ADMIN_PASSWORD", PASSWORD)

    return point_at


def test_stop_script_sends_commands_in_process(serve, rcon_env, no_subprocess, capsys) -> None:
    stop = _load_script("ark_server_stop_rcon", "ark-server-stop.py")
    server = serve(ark_like(b"Server received, But no response!! "))
    rcon_env(server)
    assert stop.shutdown_server() is True
    assert "[rcon] DoExit: accepted by the server (no output)" in capsys.readouterr().out


@pytest.mark.parametrize("password", ["wrong", PASSWORD], ids=["auth-failure", "refused"])
def test_stop_script_reports_rcon_failures(serve, rcon_env, monkeypatch, capsys, password: str) -> None:
    stop = _load_script("ark_server_stop_rcon", "ark-server-stop.py")
    server = serve(ark_like(b""))
    rcon_env(server)
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", password)
    if password == PASSWORD:
        monkeypatch.setenv("RCON_PORT", str(closed_port()))
    assert stop.save_world() is False
    assert "[rcon] Error: saveworld failed:" in capsys.readouterr().err


def test_backup_save_retries_until_the_server_answers(serve, rcon_env, no_subprocess, monkeypatch) -> None:
    rcon_save = importlib.import_module("ark_backup.rcon_save")
    monkeypatch.setattr(rcon_save.time, "sleep", lambda _s: None)
    attempts: list[int] = []
    healthy = ark_like(b"World Saved")

    def flaky(conn: socket.socket, server: FakeServer) -> None:
        attempts.append(1)
        if len(attempts) == 1:
            return  # first attempt: connection dropped before the auth reply
        healthy(conn, server)

    rcon_env(serve(flaky))
    assert rcon_save.run_rcon_save() is True
    assert len(attempts) == 2


def test_backup_save_reports_a_non_numeric_port(monkeypatch, capsys) -> None:
    rcon_save = importlib.import_module("ark_backup.rcon_save")
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", PASSWORD)
    monkeypatch.setenv("RCON_PORT", "abc")
    monkeypatch.setenv("BACKUP_RCON_SAVE_MAX_RETRIES", "1")
    monkeypatch.setattr(socket, "create_connection", lambda *_a, **_k: pytest.fail("must not connect"))
    assert rcon_save.run_rcon_save() is False
    assert "RCON_PORT must be a port number (got 'abc')" in capsys.readouterr().err


def test_ark_rcon_entry_point(serve, rcon_env, monkeypatch, capsys) -> None:
    cli = _load_script("ark_rcon_cli", "ark-rcon.py")
    server = serve(ark_like(b"World Saved"))
    rcon_env(server)
    assert cli.main(["saveworld"]) == 0
    assert capsys.readouterr().out == "World Saved\n"
    assert cli.main([]) == 2
    monkeypatch.setenv("RCON_PORT", str(closed_port()))
    assert cli.main(["saveworld"]) == 1
    assert "Error:" in capsys.readouterr().err
