#!/usr/bin/env python3.14
"""Read or change one of the instance's INI files. ark-cli's only way into them.

Usage:
  ark-ini.py get --file <game|gameusersettings> [--section <s> --key <k>]
  ark-ini.py set --file <game|gameusersettings> --section <s> --key <k> --value <v> [--replace-all]
  ark-ini.py unset --file <game|gameusersettings> --section <s> --key <k> [--all]
  ark-ini.py track --by <edit|outside>

get prints the file; with --section and --key, one JSON object instead:
  {"file", "section", "key", "lines": [{"line", "value", "origin"}, ...], "history": str}
  (lines is empty when the key isn't set; origin and history as in set's refusal below).
set prints one JSON object on stdout:
  {"file", "section", "key", "value", "old": [...], "changed": bool, "removed": int, "backup": str|null}
unset removes every line of the key under set's rules (--all where set has --replace-all), always
keeps <file>.bak, and prints one JSON object on stdout:
  {"file", "section", "key", "old": [...], "changed": bool, "backup": str|null}
Errors go to stderr as one "Error: ..." message. Exit codes: 0 ok, 1 refused or failed, 2 usage.

set refuses while the server runs (ARK rewrites GameUserSettings.ini when it stops), refuses the
keys the entrypoint sets from the environment, and refuses a key that appears more than once
unless --replace-all is given: some keys are lists ARK reads line by line. The refusal lists each
line with who wrote it and when, from the files' git history (ini_history).

track records a change in that history: `--by edit` after ark-cli's editor, `--by outside`
(changes nobody recorded) before it. set records its own write the same way.
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import ini_history
from runtime_utils import GAME_USER_SETTINGS, ark_server_running

FILES = {
    "game": GAME_USER_SETTINGS.with_name("Game.ini"),
    "gameusersettings": GAME_USER_SETTINGS,
}
# Set from the instance's env on every start (container-entrypoint.apply_rcon_settings).
ENV_KEYS = {
    "rconenabled": "RCON is always on: the stop and backup scripts need it.",
    "rconport": "Set it with: ark-cli reconfigure instance --rcon-port.",
    "serveradminpassword": "Set it with: ark-cli reconfigure instance --admin-password.",
    "serverpassword": "Set it with: ark-cli reconfigure instance --server-password.",
}


class IniError(Exception):
    """A refusal or failure; the message is shown to the user as is."""


def _header(line: str) -> str | None:
    text = line.strip().lstrip("\ufeff")
    if text.startswith("[") and text.endswith("]"):
        return text[1:-1].strip()
    return None


def _entry(line: str) -> tuple[str, str] | None:
    text = line.strip()
    if text.startswith(";") or "=" not in text:
        return None
    key, value = text.split("=", 1)
    return key.strip(), value.strip()


def find_key(lines: list[str], section: str, key: str) -> tuple[int, int, int, list[int]]:
    """Return (section start, section end, last non-blank line, matching line indexes); start is -1 if missing.

    Section and key match case-insensitively, as UE does. Only the first section with the name counts.
    """
    start, end = -1, len(lines)
    for i, line in enumerate(lines):
        name = _header(line)
        if name is None:
            continue
        if start >= 0:
            end = i
            break
        if name.lower() == section.lower():
            start = i
    matches, last = [], start
    for i in range(start + 1, end) if start >= 0 else ():
        if lines[i].strip():
            last = i
        entry = _entry(lines[i])
        if entry and entry[0].lower() == key.lower():
            matches.append(i)
    return start, end, last, matches


def set_value(content: str, section: str, key: str, value: str) -> tuple[str, list[str]]:
    """Set key=value in section; return the new content and the key's previous values.

    Every existing line for the key is replaced by one at the first one's position; a missing key
    goes after the section's last non-blank line, a missing section at the end of the file.
    Comments, order and the file's line ending (CRLF or LF) are kept.
    """
    eol = "\r\n" if "\r\n" in content else "\n"
    lines = content.splitlines(keepends=True)
    start, _end, last, matches = find_key(lines, section, key)
    new_line = f"{key}={value}{eol}"
    if start < 0:
        if lines and not lines[-1].endswith("\n"):
            lines[-1] += eol
        if lines and lines[-1].strip():
            lines.append(eol)
        return "".join([*lines, f"[{section}]{eol}", new_line]), []
    old = [_entry(lines[i])[1] for i in matches]  # type: ignore[index]
    if not matches:
        if not lines[last].endswith("\n"):
            lines[last] += eol
        lines.insert(last + 1, new_line)
        return "".join(lines), old
    lines[matches[0]] = new_line
    for i in reversed(matches[1:]):
        del lines[i]
    return "".join(lines), old


def key_lines(path: Path, lines: list[str], matches: list[int]) -> tuple[list[dict], str]:
    """Return each line for the key with who wrote it and when, and the file's last recorded change.

    The origin comes from the files' git history (ini_history): "written by ARK <when>",
    "set with ark-cli <when>", "changed outside ark-cli <when>", "origin unknown (present <when>)", ...
    """
    labels = ini_history.origins(path.parent, path.name)
    entries = [
        {"line": i + 1, "value": _entry(lines[i])[1], "origin": labels.get(i + 1, "origin unknown")}  # type: ignore[index]
        for i in matches
    ]
    return entries, ini_history.last_change(path.parent, path.name)


def write_atomic(path: Path, content: str) -> None:
    """Replace path with content in one rename, keeping its permissions (0644 for a new file)."""
    mode = path.stat().st_mode & 0o777 if path.exists() else 0o644
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        "w", delete=False, encoding="utf-8", errors="surrogateescape", newline="", dir=path.parent
    ) as tmp:
        tmp.write(content)
    Path(tmp.name).chmod(mode)
    Path(tmp.name).replace(path)


def clean_names(section: str, key: str) -> tuple[str, str]:
    """Check a section and key name; return them cleaned (brackets and spaces dropped)."""
    section = section.strip().removeprefix("[").removesuffix("]").strip()
    key = key.strip()
    if not section or any(c in section for c in "[]\r\n"):
        raise IniError("Error: --section must be one section name, e.g. ServerSettings.")
    if not key or any(c in key for c in "=[];\r\n"):
        raise IniError("Error: --key may not be empty or contain '=', '[', ']', ';' or line breaks.")
    return section, key


def validate(file_key: str, section: str, key: str, value: str) -> tuple[str, str]:
    """Check a set request; return the cleaned section and key."""
    section, key = clean_names(section, key)
    if any(c in value for c in "\r\n"):
        raise IniError("Error: --value may not contain line breaks.")
    if file_key == "gameusersettings" and section.lower() == "serversettings" and key.lower() in ENV_KEYS:
        raise IniError(
            f"Error: {key} is set from the instance's environment on every start, so a change here "
            f"would be overwritten. {ENV_KEYS[key.lower()]}"
        )
    return section, key


def cmd_get(file_key: str) -> str:
    path = FILES[file_key]
    if not path.is_file():
        raise IniError(missing_message(file_key))
    return path.read_text(encoding="utf-8", errors="replace", newline="")


def cmd_get_key(file_key: str, section: str, key: str) -> dict:
    """Every line for one key, with origins. Reading is safe while the server runs."""
    section, key = clean_names(section, key)
    path = FILES[file_key]
    lines = cmd_get(file_key).splitlines(keepends=True)
    _s, _e, _l, matches = find_key(lines, section, key)
    entries, history = key_lines(path, lines, matches)
    return {"file": path.name, "section": section, "key": key, "lines": entries, "history": history}


def missing_message(file_key: str) -> str:
    if file_key == "gameusersettings":
        return "Error: GameUserSettings.ini doesn't exist yet. Start the instance once to install the defaults."
    return "Error: Game.ini doesn't exist yet. Use 'set ini' or 'edit ini' to create it."


def cmd_set(file_key: str, section: str, key: str, value: str, *, replace_all: bool) -> dict:
    section, key = validate(file_key, section, key, value)
    if ark_server_running():
        raise IniError("Error: the server is running; stop it first (ark-cli set ini --restart does this for you).")
    path = FILES[file_key]
    # GameUserSettings.ini must come from the image's defaults on first start; a hand-made one would block that.
    if file_key == "gameusersettings" and not path.is_file():
        raise IniError(missing_message(file_key))
    # surrogateescape: bytes that aren't valid UTF-8 are written back unchanged.
    original = path.read_text(encoding="utf-8", errors="surrogateescape", newline="") if path.is_file() else ""
    lines = original.splitlines(keepends=True)
    _s, _e, _l, matches = find_key(lines, section, key)
    if len(matches) > 1 and not replace_all:
        entries, history = key_lines(path, lines, matches)
        listing = "\n".join(f"  line {e['line']}: {key}={e['value']}  ({e['origin']})" for e in entries)
        raise IniError(
            f"Error: {key} appears {len(matches)} times in [{section}] of {path.name}:\n{listing}\n{history}\n"
            "Some ARK keys are lists with one line per entry, and setting one value would delete the others. "
            "Use 'edit ini' to change one line, or rerun with --replace-all to replace them all with one line."
        )
    updated, old = set_value(original, section, key, value)
    result = {
        "file": path.name,
        "section": section,
        "key": key,
        "value": value,
        "old": old,
        "changed": updated != original,
        "removed": max(len(old) - 1, 0),
        "backup": None,
    }
    if not result["changed"]:
        return result
    # Changes nobody recorded (hand edits, restores) get their own commit, so they aren't blamed on us.
    ini_history.commit_outside_changes(path.parent, "changed outside ark-cli; found before set ini")
    if result["removed"]:
        backup = path.with_name(path.name + ".bak")
        shutil.copy2(path, backup)
        result["backup"] = str(backup)
    write_atomic(path, updated)
    ini_history.commit(path.parent, ini_history.CLI_SET, f"[{section}] {key}={value}")
    return result


def cmd_unset(file_key: str, section: str, key: str, *, remove_all: bool) -> dict:
    section, key = validate(file_key, section, key, "")
    if ark_server_running():
        raise IniError("Error: the server is running; stop it first (ark-cli unset ini --restart does this for you).")
    path = FILES[file_key]
    if file_key == "gameusersettings" and not path.is_file():
        raise IniError(missing_message(file_key))
    # surrogateescape: bytes that aren't valid UTF-8 are written back unchanged.
    original = path.read_text(encoding="utf-8", errors="surrogateescape", newline="") if path.is_file() else ""
    lines = original.splitlines(keepends=True)
    _s, _e, _l, matches = find_key(lines, section, key)
    if len(matches) > 1 and not remove_all:
        entries, history = key_lines(path, lines, matches)
        listing = "\n".join(f"  line {e['line']}: {key}={e['value']}  ({e['origin']})" for e in entries)
        raise IniError(
            f"Error: {key} appears {len(matches)} times in [{section}] of {path.name}:\n{listing}\n{history}\n"
            "Some ARK keys are lists with one line per entry, and unset would delete them all. "
            "Use 'edit ini' to remove one line, or rerun with --all to remove every line."
        )
    old = [_entry(lines[i])[1] for i in matches]  # type: ignore[index]
    result = {"file": path.name, "section": section, "key": key, "old": old, "changed": bool(matches), "backup": None}
    if not matches:
        return result
    # Changes nobody recorded (hand edits, restores) get their own commit, so they aren't blamed on us.
    ini_history.commit_outside_changes(path.parent, "changed outside ark-cli; found before unset ini")
    backup = path.with_name(path.name + ".bak")
    shutil.copy2(path, backup)
    write_atomic(path, "".join(line for i, line in enumerate(lines) if i not in matches))
    ini_history.commit(path.parent, ini_history.CLI_UNSET, f"[{section}] {key}")
    return {**result, "backup": str(backup)}


def cmd_track(by: str) -> None:
    """Commit the INI files as changed by `by`: edit (after ark-cli's editor) or outside (before it)."""
    config_dir = FILES["gameusersettings"].parent
    if by == "edit":
        ini_history.commit(config_dir, ini_history.CLI_EDIT, "edited with ark-cli edit ini")
    else:
        ini_history.commit_outside_changes(config_dir, "changed outside ark-cli; found before edit ini")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(prog="ark-ini.py")
    sub = parser.add_subparsers(dest="command", required=True)
    get = sub.add_parser("get")
    get.add_argument("--file", required=True, choices=FILES)
    get.add_argument("--section")
    get.add_argument("--key")
    put = sub.add_parser("set")
    put.add_argument("--file", required=True, choices=FILES)
    put.add_argument("--section", required=True)
    put.add_argument("--key", required=True)
    put.add_argument("--value", required=True)
    put.add_argument("--replace-all", action="store_true")
    unset = sub.add_parser("unset")
    unset.add_argument("--file", required=True, choices=FILES)
    unset.add_argument("--section", required=True)
    unset.add_argument("--key", required=True)
    unset.add_argument("--all", action="store_true")
    track = sub.add_parser("track")
    track.add_argument("--by", required=True, choices=("edit", "outside"))
    args = parser.parse_args(argv)
    if args.command == "get" and (args.section is None) != (args.key is None):
        parser.error("get needs both --section and --key, or neither")
    return args


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    try:
        if args.command == "get" and args.key is not None:
            sys.stdout.write(json.dumps(cmd_get_key(args.file, args.section, args.key)) + "\n")
        elif args.command == "get":
            sys.stdout.write(cmd_get(args.file))
        elif args.command == "track":
            cmd_track(args.by)
        elif args.command == "unset":
            result = cmd_unset(args.file, args.section, args.key, remove_all=args.all)
            sys.stdout.write(json.dumps(result) + "\n")
        else:
            result = cmd_set(args.file, args.section, args.key, args.value, replace_all=args.replace_all)
            sys.stdout.write(json.dumps(result) + "\n")
    except IniError as exc:
        sys.stderr.write(f"{exc}\n")
        return 1
    except (OSError, ini_history.HistoryError) as exc:
        sys.stderr.write(f"Error: {exc}\n")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
