#!/usr/bin/env python3.14
"""Record why the next stop happens, e.g. "updating to 98.9", for the stop's player warnings.

Usage: ark-stop-reason.py <reason...>
ark-cli runs this just before it stops the instance; ark-server-stop.py names the reason in its
countdown and removes the file. A separate script (not a flag on ark-server-stop.py) so that an
image older than this script fails with "not found" instead of running a stop.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from runtime_utils import STOP_REASON_FILE


def main(argv: list[str]) -> int:
    reason = " ".join(argv).strip()
    if not reason:
        sys.stderr.write("Usage: ark-stop-reason.py <reason...>\n")
        return 2
    STOP_REASON_FILE.parent.mkdir(parents=True, exist_ok=True)
    STOP_REASON_FILE.write_text(reason + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
