from __future__ import annotations

import re
import socket
from pathlib import Path

from helpers.framework_logger import log_framework


def pick_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("", 0))
        return sock.getsockname()[1]


def assign_random_ports(compose_path: Path) -> None:
    compose_text = compose_path.read_text(encoding="utf-8")
    log_framework("compose", "assigning random ports to %s", compose_path)
    asa_pattern = r'"(\d+):\d+/udp"'
    # Optional host-IP prefix: the default template publishes RCON on 127.0.0.1 only.
    rcon_pattern = r'"((?:\d+\.\d+\.\d+\.\d+:)?)(\d+):27020/tcp"'
    asa_old_ports = re.findall(asa_pattern, compose_text)
    rcon_old_ports = [port for _, port in re.findall(rcon_pattern, compose_text)]
    if not asa_old_ports or not rcon_old_ports:
        raise AssertionError("Expected at least one ASA and one RCON port mapping.")
    if len(asa_old_ports) != len(rcon_old_ports):
        raise AssertionError("Expected ASA and RCON port mapping counts to match.")

    service_count = len(asa_old_ports)
    attempts = 0
    while True:
        attempts += 1
        if attempts > 200:
            raise AssertionError("Unable to allocate incrementing host port blocks for compose services.")
        asa_start = pick_free_port()
        rcon_start = pick_free_port()
        if asa_start + service_count - 1 > 65535 or rcon_start + service_count - 1 > 65535:
            continue
        asa_block = {asa_start + offset for offset in range(service_count)}
        rcon_block = {rcon_start + offset for offset in range(service_count)}
        if asa_block.isdisjoint(rcon_block):
            break
    log_framework(
        "compose",
        "port ranges: asa=%d-%d, rcon=%d-%d",
        asa_start,
        asa_start + service_count - 1,
        rcon_start,
        rcon_start + service_count - 1,
    )

    asa_next = asa_start
    asa_port_map: dict[str, int] = {}

    def replace_asa(match: re.Match[str]) -> str:
        nonlocal asa_next
        old = match.group(1)
        new = asa_next
        asa_port_map[old] = new
        asa_next += 1
        return f'"{new}:{new}/udp"'

    updated_text = re.sub(asa_pattern, replace_asa, compose_text)

    for old_port, new_port in asa_port_map.items():
        updated_text = updated_text.replace(f"- ASA_PORT={old_port}", f"- ASA_PORT={new_port}")
        updated_text = updated_text.replace(f'ark.port.asa: "{old_port}"', f'ark.port.asa: "{new_port}"')

    rcon_next = rcon_start
    rcon_port_map: dict[str, int] = {}

    def replace_rcon(match: re.Match[str]) -> str:
        nonlocal rcon_next
        prefix, old = match.group(1), match.group(2)
        new = rcon_next
        rcon_port_map[old] = new
        rcon_next += 1
        return f'"{prefix}{new}:27020/tcp"'

    updated_text = re.sub(rcon_pattern, replace_rcon, updated_text)

    for old_port, new_port in rcon_port_map.items():
        updated_text = updated_text.replace(f'ark.port.rcon: "{old_port}"', f'ark.port.rcon: "{new_port}"')

    compose_path.write_text(updated_text, encoding="utf-8")


def enable_no_battleye(compose_path: Path) -> None:
    compose_text = compose_path.read_text(encoding="utf-8")
    updated_text, replacements = re.subn(
        r"(- NoBattlEye=)false",
        r"\1true",
        compose_text,
    )
    if replacements == 0:
        raise AssertionError("Expected to find NoBattlEye=false in compose file to enable it for tests.")
    compose_path.write_text(updated_text, encoding="utf-8")


def enable_skip_ram_check(compose_path: Path) -> None:
    compose_text = compose_path.read_text(encoding="utf-8")
    updated_text, replacements = re.subn(
        r"(- ARK_SKIP_RAM_CHECK=)0",
        r"\g<1>1",
        compose_text,
    )
    if replacements == 0:
        raise AssertionError("Expected to find ARK_SKIP_RAM_CHECK=0 in compose file to enable it for tests.")
    compose_path.write_text(updated_text, encoding="utf-8")


def use_lite_image(compose_path: Path, image: str) -> None:
    """Point services that `create instance` gave ark-server:latest at the lite image.

    Only for lite tests (see helpers.lite_image): ark-cli always writes ark-server:latest, and
    retagging the lite image as latest would overwrite a real ARK image on the host.
    """
    compose_text = compose_path.read_text(encoding="utf-8")
    updated_text = re.sub(r"^(\s+image:\s+)ark-server:latest\s*$", rf"\g<1>{image}", compose_text, flags=re.MULTILINE)
    compose_path.write_text(updated_text, encoding="utf-8")


def disable_restart_policy(compose_path: Path) -> None:
    compose_text = compose_path.read_text(encoding="utf-8")
    log_framework("compose", "disabling restart policy in %s", compose_path)
    updated_text, replacements = re.subn(
        r"^\s+restart:\s+unless-stopped\s*$",
        '    restart: "no"',
        compose_text,
        flags=re.MULTILINE,
    )
    if replacements != 1:
        raise AssertionError("Expected to override restart policy once.")
    compose_path.write_text(updated_text, encoding="utf-8")


def inject_test_suite_label(compose_path: Path) -> None:
    """Add ``io.ark.test-suite: "true"`` label to every ``labels:`` section.

    Finds all ``labels:`` keys at standard compose indent levels (service
    labels and volume labels) and inserts the test label as the first
    entry. Also adds ``labels:`` sections to named volumes that don't
    already have one.

    Call after compose file generation so the production template stays
    free of test-only metadata.

    Idempotent: if the label already exists anywhere in the file, no
    changes are made.
    """
    compose_text = compose_path.read_text(encoding="utf-8")
    if "io.ark.test-suite" in compose_text:
        return
    log_framework("compose", "injecting io.ark.test-suite label into %s", compose_path)
    lines = compose_text.splitlines()
    result: list[str] = []
    in_volumes_top_level = False
    for line in lines:
        result.append(line)
        if line == "    labels:":
            result.append('      io.ark.test-suite: "true"')
        elif line == "volumes:":
            in_volumes_top_level = True
        elif in_volumes_top_level:
            stripped = line.lstrip()
            if not stripped or stripped.startswith("#"):
                continue
            indent = len(line) - len(stripped)
            if indent == 0:
                in_volumes_top_level = False
            elif indent == 2 and stripped.endswith(":"):
                result.append("    labels:")
                result.append('      io.ark.test-suite: "true"')
    compose_path.write_text("\n".join(result) + "\n", encoding="utf-8")


def asa_port_from_compose(compose_path: Path, instance: str) -> str:
    """Return the ark.port.asa label value from the compose file."""
    text = compose_path.read_text(encoding="utf-8")
    match = re.search(r"ark\.port\.asa:\s*\"?(\d+)\"?", text)
    assert match, f"ark.port.asa label not found in {compose_path}"
    return match.group(1)


def write_single_service_compose(
    compose_path: Path,
    *,
    instance: str,
    image: str,
    admin_password: str,
    server_password: str,
    extra_env: dict[str, str] | None = None,
    mem_reservation: str = "12G",
    entrypoint: str | None = None,
) -> None:
    asa_port = pick_free_port()
    rcon_port = pick_free_port()
    while rcon_port == asa_port:
        rcon_port = pick_free_port()

    env_lines = [
        "      - TZ=America/Los_Angeles",
        "      - MAP_NAME=TheIsland_WP",
        f"      - SESSION_NAME={instance}",
        f"      - SERVER_ADMIN_PASSWORD={admin_password}",
        f"      - SERVER_PASSWORD={server_password}",
        f"      - ASA_PORT={asa_port}",
        "      - NoBattlEye=true",
        "      - ARK_SKIP_RAM_CHECK=1",
    ]
    for key, value in (extra_env or {}).items():
        env_lines.append(f"      - {key}={value}")

    service_lines = [
        "services:",
        f"  {instance}:",
        f"    image: {image}",
        f"    container_name: {instance}",
    ]
    if entrypoint:
        service_lines.append(f"    entrypoint: {entrypoint}")
    service_lines.extend(
        [
            '    restart: "no"',
            "    init: true",
            "    stop_grace_period: 15m",
            "    stop_signal: SIGTERM",
            "    environment:",
            *env_lines,
            "    ports:",
            f'      - "{asa_port}:{asa_port}/udp"',
            f'      - "127.0.0.1:{rcon_port}:27020/tcp"',
            "    labels:",
            '      io.ark.test-suite: "true"',
            f'      ark.port.asa: "{asa_port}"',
            f'      ark.port.rcon: "{rcon_port}"',
            f"    mem_reservation: {mem_reservation}",
            "    volumes:",
            f'      - "{instance}-saved:/home/ark-user/ark-server/ShooterGame/Saved"',
            f'      - "{instance}-cluster:/home/ark-user/ark-server/ShooterGame/Saved/clusters"',
            "volumes:",
            f"  {instance}-saved:",
            "    labels:",
            '      io.ark.test-suite: "true"',
            f"  {instance}-cluster:",
            "    labels:",
            '      io.ark.test-suite: "true"',
            "",
        ]
    )

    compose_path.write_text(
        "\n".join(service_lines),
        encoding="utf-8",
    )
    log_framework(
        "compose",
        "wrote single-service compose to %s (instance=%s, asa_port=%d, rcon_port=%d)",
        compose_path,
        instance,
        asa_port,
        rcon_port,
    )
