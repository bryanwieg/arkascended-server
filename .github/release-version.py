#!/usr/bin/env python3
"""ark-cli release versions: the next version from a PR title, and the PR's `Release: X.Y.Z` line.

check --title T --body-file F --latest-tag TAG   exit 0, or 1 with the reason
next --title T --latest-tag TAG                  print the next version, or nothing
parse --body-file F                              print the stated version, or nothing
latest                                           print the highest local cli-v* tag
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

NUM = r"(0|[1-9]\d*)"
VERSION = re.compile(rf"{NUM}\.{NUM}\.{NUM}")
TAG = re.compile(rf"cli-v{VERSION.pattern}")
TITLE = re.compile(r"(?P<type>[a-z]+)(\((?P<scope>[^()]+)\))?(?P<breaking>!)?: \S")
RELEASE_LINE = re.compile(r"^\s*Release:", re.MULTILINE)
MINOR = {"feat"}
PATCH = {"fix", "perf", "revert", "deps", "build"}
# app/ark-web is not part of the ark-cli release.
EXCLUDED_SCOPES = {"web"}


class InvalidError(Exception):
    pass


def parse_tag(tag: str) -> tuple[int, ...]:
    match = TAG.fullmatch(tag)
    if not match:
        raise InvalidError(f"latest tag {tag!r} is not cli-vX.Y.Z")
    return tuple(int(part) for part in match.groups())


def next_version(title: str, latest_tag: str) -> str:
    major, minor, patch = parse_tag(latest_tag)
    match = TITLE.match(title)
    if not match:
        return ""
    kind = match["type"]
    if match["scope"] in EXCLUDED_SCOPES or (kind not in MINOR | PATCH and not match["breaking"]):
        return ""
    if match["breaking"]:
        return f"{major + 1}.0.0" if major >= 1 else f"{major}.{minor + 1}.0"
    if kind in MINOR:
        return f"{major}.{minor + 1}.0"
    return f"{major}.{minor}.{patch + 1}"


def stated_version(body: str) -> str:
    lines = [line.strip() for line in body.splitlines() if RELEASE_LINE.match(line)]
    if len(lines) > 1:
        raise InvalidError(f"{len(lines)} Release: lines; state at most one")
    if not lines:
        return ""
    match = re.fullmatch(rf"Release: ({VERSION.pattern})", lines[0])
    if not match:
        raise InvalidError(f"malformed {lines[0]!r}; expected 'Release: X.Y.Z'")
    return match[1]


def check(title: str, body: str, latest_tag: str) -> None:
    if not TITLE.match(title):
        raise InvalidError(f"title {title!r} is not a Conventional Commit (type(scope)!: subject)")
    expected = next_version(title, latest_tag)
    stated = stated_version(body)
    if stated != expected:
        want = f"the line 'Release: {expected}'" if expected else "no Release: line"
        raise InvalidError(f"title {title!r} after {latest_tag} needs {want} in the PR description")


def latest() -> str:
    tags = subprocess.run(["git", "tag", "-l", "cli-v*"], capture_output=True, text=True, check=True).stdout.split()
    versions = [tag for tag in tags if TAG.fullmatch(tag)]
    if not versions:
        raise InvalidError("no cli-vX.Y.Z tag")
    return max(versions, key=parse_tag)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="command", required=True)
    check_cmd = sub.add_parser("check")
    check_cmd.add_argument("--title", required=True)
    check_cmd.add_argument("--body-file", type=Path, required=True)
    check_cmd.add_argument("--latest-tag", required=True)
    next_cmd = sub.add_parser("next")
    next_cmd.add_argument("--title", required=True)
    next_cmd.add_argument("--latest-tag", required=True)
    parse_cmd = sub.add_parser("parse")
    parse_cmd.add_argument("--body-file", type=Path, required=True)
    sub.add_parser("latest")
    args = parser.parse_args()
    try:
        if args.command == "check":
            check(args.title, args.body_file.read_text(), args.latest_tag)
            return 0
        result = {
            "next": lambda: next_version(args.title, args.latest_tag),
            "parse": lambda: stated_version(args.body_file.read_text()),
            "latest": latest,
        }[args.command]()
    except InvalidError as error:
        sys.stderr.write(f"release-version: {error}\n")
        return 1
    if result:
        sys.stdout.write(result + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
