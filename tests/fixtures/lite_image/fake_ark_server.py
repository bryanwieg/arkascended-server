"""Stand-in for ArkAscendedServer.exe in the lite test image: the parts the runtime scripts and ark-cli see.

Started by the fake proton as `proton run ./ArkAscendedServer.exe <url> ... -servergamelog`, so
its command line contains ArkAscendedServer.exe and the scripts treat it as the server process.

- Writes ShooterGame.log with the markers the CLI and scripts wait for (started, advertising,
  World Save Complete, Log file closed).
- Serves RCON on 27020 with the ServerAdminPassword from GameUserSettings.ini, as ARK does:
  saveworld, DoExit, ListPlayers; anything else gets ARK's generic reply.
- On DoExit, rewrites GameUserSettings.ini as ARK does on shutdown: here it bumps
  [ServerSettings] FakeArkShutdowns. FAKE_ARK_INI_DUPLICATE=<key> also makes it write that key
  twice, like an ARK build that duplicates a key.

Settings: FAKE_ARK_READY_DELAY seconds between the started and advertising markers (default 1).
"""

from __future__ import annotations

import os
import socket
import struct
import sys
import threading
import time
from datetime import UTC, datetime
from pathlib import Path

SAVED = Path("/home/ark-user/ark-server/ShooterGame/Saved")
LOG = SAVED / "Logs/ShooterGame.log"
GUS = SAVED / "Config/WindowsServer/GameUserSettings.ini"
RCON_PORT = 27020
GENERIC_REPLY = "Server received, But no response!! "

_log_lock = threading.Lock()
_exit = threading.Event()


def log(message: str) -> None:
    stamp = datetime.now(UTC).strftime("%Y.%m.%d-%H.%M.%S:%f")[:-3]
    with _log_lock, LOG.open("a", encoding="utf-8") as f:
        f.write(f"[{stamp}][  0]{message}\n")


def admin_password() -> str:
    for line in GUS.read_text(encoding="utf-8", errors="replace").splitlines() if GUS.is_file() else []:
        key, _, value = line.partition("=")
        if key.strip().lower() == "serveradminpassword":
            return value.strip()
    return ""


def write_settings_on_shutdown() -> None:
    """Rewrite the file the way ARK does: its own keys replaced, not appended."""
    lines = GUS.read_text(encoding="utf-8").splitlines() if GUS.is_file() else ["[ServerSettings]"]
    duplicate = os.environ.get("FAKE_ARK_INI_DUPLICATE", "")
    count = 0
    kept = []
    for line in lines:
        key, _, value = line.partition("=")
        if key == "FakeArkShutdowns":
            count = int(value or 0)
        elif not (duplicate and key == duplicate):
            kept.append(line)
    section = next((i for i, line in enumerate(kept) if line.strip() == "[ServerSettings]"), None)
    if section is None:
        kept.append("[ServerSettings]")
        section = len(kept) - 1
    extra = [f"FakeArkShutdowns={count + 1}"]
    if duplicate:
        extra += [f"{duplicate}=ark-a", f"{duplicate}=ark-b"]
    kept[section + 1 : section + 1] = extra
    GUS.write_text("\n".join(kept) + "\n", encoding="utf-8")


def packet(packet_id: int, packet_type: int, body: str) -> bytes:
    payload = struct.pack("<ii", packet_id, packet_type) + body.encode() + b"\x00\x00"
    return struct.pack("<i", len(payload)) + payload


def read_packet(conn: socket.socket) -> tuple[int, int, str] | None:
    header = conn.recv(4, socket.MSG_WAITALL)
    if len(header) < 4:
        return None
    (size,) = struct.unpack("<i", header)
    payload = conn.recv(size, socket.MSG_WAITALL)
    packet_id, packet_type = struct.unpack("<ii", payload[:8])
    return packet_id, packet_type, payload[8:].rstrip(b"\x00").decode(errors="replace")


def run_command(command: str) -> str:
    name = command.split(" ", 1)[0].lower()
    if name == "saveworld":
        log("World Save Complete")
        return "World Saved"
    if name == "listplayers":
        return "No Players Connected"
    if name == "doexit":
        _exit.set()
        return "Exiting... "
    return GENERIC_REPLY


def serve_client(conn: socket.socket) -> None:
    with conn:
        authed = False
        while (pkt := read_packet(conn)) is not None:
            packet_id, packet_type, body = pkt
            if packet_type == 3:  # AUTH
                authed = bool(body) and body == admin_password()
                conn.sendall(packet(packet_id if authed else -1, 2, ""))
            elif packet_type == 2 and authed:  # EXECCOMMAND
                conn.sendall(packet(packet_id, 0, run_command(body)))
            else:
                return


def serve_rcon() -> None:
    server = socket.create_server(("0.0.0.0", RCON_PORT), reuse_port=True)  # noqa: S104
    while True:
        conn, _addr = server.accept()
        threading.Thread(target=serve_client, args=(conn,), daemon=True).start()


def main() -> int:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    log(f"Fake ARK server, args: {' '.join(sys.argv[1:])}")
    threading.Thread(target=serve_rcon, daemon=True).start()
    log("Server: 'FakeMap' has successfully started!")
    time.sleep(float(os.environ.get("FAKE_ARK_READY_DELAY", "1")))
    log("Server has completed startup and is now advertising for join.")
    _exit.wait()
    time.sleep(0.5)  # let the DoExit reply go out first
    write_settings_on_shutdown()
    log(f"Log file closed, {datetime.now(UTC):%m/%d/%y %H:%M:%S}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
