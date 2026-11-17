"""Minimal Source RCON client for the ARK server, using only the standard library.

Replaces the rcon-cli binary so the admin password never appears in a process's command
line (any host user can read those with ps). One connection per command: authenticate,
send the command, read one reply packet, close. That matches gorcon/rcon, which ARK is
known to work with.

Protocol (https://developer.valvesoftware.com/wiki/Source_RCON_Protocol): each packet is
<int32 size><int32 id><int32 type><body>\\0\\0, little-endian, where size counts everything
after itself.
"""

from __future__ import annotations

import os
import socket
import struct

AUTH = 3
AUTH_RESPONSE = 2
EXECCOMMAND = 2
RESPONSE_VALUE = 0

_AUTH_ID = 1
_COMMAND_ID = 2
_MIN_SIZE = 10  # id + type + two NUL terminators
_MAX_SIZE = 1024 * 1024  # far beyond any ARK reply; a larger size means a desynced stream
_MAX_SKIPPED_PACKETS = 16


class RconError(Exception):
    """RCON connection, protocol or timeout failure."""


class RconAuthError(RconError):
    """The server rejected the admin password."""


def encode_packet(packet_id: int, packet_type: int, body: str) -> bytes:
    """Return one RCON packet with a UTF-8 body."""
    payload = struct.pack("<ii", packet_id, packet_type) + body.encode() + b"\x00\x00"
    return struct.pack("<i", len(payload)) + payload


def _read_exact(sock: socket.socket, count: int) -> bytes:
    data = b""
    while len(data) < count:
        chunk = sock.recv(count - len(data))
        if not chunk:
            raise RconError("RCON server closed the connection mid-reply.")
        data += chunk
    return data


def read_packet(sock: socket.socket) -> tuple[int, int, str]:
    """Read one packet and return (id, type, body). Undecodable bytes are replaced, not fatal."""
    (size,) = struct.unpack("<i", _read_exact(sock, 4))
    if not _MIN_SIZE <= size <= _MAX_SIZE:
        raise RconError(f"RCON server sent an invalid packet size ({size}).")
    payload = _read_exact(sock, size)
    packet_id, packet_type = struct.unpack("<ii", payload[:8])
    return packet_id, packet_type, payload[8:].rstrip(b"\x00").decode(errors="replace")


def _read_matching(sock: socket.socket, wanted_id: int, wanted_type: int) -> tuple[int, str]:
    """Return (id, body) of the next packet of wanted_type whose id is wanted_id (-1 too for auth).

    Other packets (an empty RESPONSE_VALUE before the auth reply, server console messages,
    unknown types) are skipped, so an unexpected packet doesn't abort the command.
    """
    accepted = {wanted_id, -1} if wanted_type == AUTH_RESPONSE else {wanted_id}
    for _ in range(_MAX_SKIPPED_PACKETS + 1):
        packet_id, packet_type, body = read_packet(sock)
        if packet_type == wanted_type and packet_id in accepted:
            return packet_id, body
    raise RconError(f"RCON server sent {_MAX_SKIPPED_PACKETS} packets without the expected reply.")


def rcon_command(
    command: str,
    *,
    host: str | None = None,
    port: int | None = None,
    password: str | None = None,
    timeout: float = 10.0,
) -> str:
    """Run one RCON command and return the server's reply text.

    host, port and password default to RCON_HOST (localhost), RCON_PORT (27020) and
    SERVER_ADMIN_PASSWORD. Raises RconAuthError for a wrong password and RconError for
    anything else (refused, timed out, malformed reply).
    """
    host = host or os.environ.get("RCON_HOST", "localhost")
    if not port:
        env_port = os.environ.get("RCON_PORT", "27020")
        try:
            port = int(env_port)
        except ValueError:
            raise RconError(f"RCON_PORT must be a port number (got {env_port!r}).") from None
    password = os.environ.get("SERVER_ADMIN_PASSWORD", "") if password is None else password
    if not password:
        raise RconAuthError("SERVER_ADMIN_PASSWORD is not set.")
    if not command:
        raise RconError("RCON command is empty.")
    try:
        with socket.create_connection((host, port), timeout=timeout) as sock:
            sock.sendall(encode_packet(_AUTH_ID, AUTH, password))
            auth_id, _ = _read_matching(sock, _AUTH_ID, AUTH_RESPONSE)
            if auth_id == -1:
                raise RconAuthError("RCON authentication failed: the server rejected SERVER_ADMIN_PASSWORD.")
            sock.sendall(encode_packet(_COMMAND_ID, EXECCOMMAND, command))
            return _read_matching(sock, _COMMAND_ID, RESPONSE_VALUE)[1]
    except TimeoutError as exc:
        raise RconError(f"RCON command timed out after {timeout:g} seconds ({host}:{port}).") from exc
    except OSError as exc:
        raise RconError(f"Failed to connect to RCON server at {host}:{port}: {exc.strerror or exc}.") from exc
