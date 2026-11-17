"""Merge the environment-driven RCON and password settings into GameUserSettings.ini."""

from __future__ import annotations

import os
import re
import tempfile
from pathlib import Path

SERVER_SECTION_PATTERN = re.compile(r"^[\t ]*\[ServerSettings\][\t ]*$")
ANY_SECTION_PATTERN = re.compile(r"^\[.*\]$")
SERVER_SETTING_PATTERN = re.compile(r"^(RCONEnabled|RCONPort|ServerAdminPassword|ServerPassword)=")


def _insert_settings(
    output_lines: list[str], admin_password: str, rcon_port: str, server_password: str, inserted: bool
) -> bool:
    """Append the four RCON setting lines to output_lines if not already inserted; return True when done."""
    if inserted:
        return True
    output_lines.append("RCONEnabled=True")
    output_lines.append(f"RCONPort={rcon_port}")
    output_lines.append(f"ServerAdminPassword={admin_password}")
    output_lines.append(f"ServerPassword={server_password}")
    return True


def update_config(config_file: Path, admin_password: str, rcon_port: str, server_password: str) -> None:
    """Rewrite GameUserSettings.ini to enforce RCON settings, replacing any existing values atomically.

    A missing file is created with just the [ServerSettings] section; ARK adds its defaults around it.
    The file is left untouched when it already holds these values, so per-boot calls are no-ops.
    """
    original = config_file.read_text(encoding="utf-8", errors="ignore") if config_file.is_file() else ""
    lines = original.splitlines()
    output_lines: list[str] = []
    in_server_section = False
    found_server_section = False
    inserted = False

    for raw_line in lines:
        line = raw_line.rstrip("\r")
        if SERVER_SECTION_PATTERN.match(line):
            in_server_section = True
            found_server_section = True
            output_lines.append(line)
            continue
        if ANY_SECTION_PATTERN.match(line):
            if in_server_section:
                inserted = _insert_settings(output_lines, admin_password, rcon_port, server_password, inserted)
            in_server_section = False
            output_lines.append(line)
            continue
        if in_server_section and SERVER_SETTING_PATTERN.match(line):
            continue
        output_lines.append(line)

    if not found_server_section:
        output_lines.append("[ServerSettings]")
        inserted = _insert_settings(output_lines, admin_password, rcon_port, server_password, inserted)
    elif in_server_section:
        _insert_settings(output_lines, admin_password, rcon_port, server_password, inserted)

    content = "\n".join(output_lines) + "\n"
    if content == original:
        return
    config_file.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile("w", delete=False, encoding="utf-8", dir=config_file.parent) as tmp_file:
        tmp_file.write(content)
        tmp_name = tmp_file.name
    os.replace(tmp_name, config_file)  # noqa: PTH105
