#!/usr/bin/env python3.14
"""Run one RCON command against the local ARK server and print the reply.

Usage: ark-rcon.py <command> [args...]   (e.g. ark-rcon.py saveworld)
Connection settings come from RCON_HOST, RCON_PORT and SERVER_ADMIN_PASSWORD, so the
password never appears on a command line. Exit codes: 0 ok, 1 RCON error, 2 usage.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from rcon_client import RconError, rcon_command


def main(argv: list[str]) -> int:
    if not argv:
        sys.stderr.write("Usage: ark-rcon.py <command> [args...]\n")
        return 2
    try:
        reply = rcon_command(" ".join(argv))
    except RconError as exc:
        # Plain stderr, not emit_error: readiness polls call this every few seconds.
        sys.stderr.write(f"Error: {exc}\n")
        return 1
    sys.stdout.write(reply + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
