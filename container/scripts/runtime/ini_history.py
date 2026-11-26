"""Git history of the instance's INI files: who wrote each line, and when.

A repository in the INI directory (Saved/Config/WindowsServer/.git, so backups and restores carry
it) tracks Game.ini and GameUserSettings.ini only. Every writer commits under its own author name:

  ARK server       the image default (ARK generated it at build) and the server's own writes
  instance env     the RCON and password settings the entrypoint applies from the environment
  ark-cli set ini  ark-ini.py set
  ark-cli unset ini  ark-ini.py unset
  ark-cli edit ini the editor run by `ark-cli edit ini`
  outside ark-cli  changes found uncommitted (a hand edit or a restored file), committed when found
  unknown origin   the files as first seen, when the repository is created on an existing volume

origins() turns `git blame` into a label per line, e.g. "written by ARK 2026-11-19 00:01:26 UTC".
"""

from __future__ import annotations

import os
import subprocess
from datetime import UTC, datetime
from pathlib import Path

TRACKED = ("Game.ini", "GameUserSettings.ini")
ARK = "ARK server"
ENV = "instance env"
CLI_SET = "ark-cli set ini"
CLI_UNSET = "ark-cli unset ini"
CLI_EDIT = "ark-cli edit ini"
OUTSIDE = "outside ark-cli"
UNKNOWN = "unknown origin"
_LABELS = {
    ARK: "written by ARK",
    ENV: "set from instance env",
    CLI_SET: "set with ark-cli",
    CLI_UNSET: "unset with ark-cli",
    CLI_EDIT: "edited with ark-cli",
    OUTSIDE: "changed outside ark-cli",
}
_NOT_COMMITTED = "Not Committed Yet"  # git blame's author for lines changed since the last commit


class HistoryError(Exception):
    """git failed; the message says which command and why."""


def _git(config_dir: Path, *args: str, author: str = UNKNOWN) -> str:
    email = author.lower().replace(" ", "-") + "@ark-ascended.invalid"
    env = {
        **os.environ,
        "GIT_AUTHOR_NAME": author,
        "GIT_AUTHOR_EMAIL": email,
        "GIT_COMMITTER_NAME": author,
        "GIT_COMMITTER_EMAIL": email,
        "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": os.devnull,
        # Never use a repository above the INI directory (no .git of its own yet = no history).
        "GIT_CEILING_DIRECTORIES": str(config_dir.parent),
    }
    # safe.directory: the volume may have been chowned on the host; this repo is ours either way.
    cmd = ["git", "-c", f"safe.directory={config_dir}", "-C", str(config_dir), *args]
    try:
        result = subprocess.run(cmd, env=env, capture_output=True, text=True, check=False)
    except OSError as exc:
        raise HistoryError(f"git {args[0]} failed: {exc}") from exc
    if result.returncode != 0:
        raise HistoryError(f"git {args[0]} failed: {result.stderr.strip() or result.returncode}")
    return result.stdout


def _ensure_repo(config_dir: Path) -> bool:
    """Create the repository if missing (only the tracked files are visible to it); return True if created."""
    if (config_dir / ".git").is_dir():
        return False
    config_dir.mkdir(parents=True, exist_ok=True)
    _git(config_dir, "init", "-q", "-b", "main")
    exclude = "*\n" + "".join(f"!{name}\n" for name in TRACKED)
    (config_dir / ".git/info").mkdir(parents=True, exist_ok=True)
    (config_dir / ".git/info/exclude").write_text(exclude, encoding="utf-8")
    return True


def commit(config_dir: Path, author: str, message: str) -> bool:
    """Commit the tracked files' current state as author; return True if anything changed.

    A missing repository is created first; its first commit is the files as found.
    """
    if not any((config_dir / name).is_file() for name in TRACKED) and not (config_dir / ".git").is_dir():
        return False
    _ensure_repo(config_dir)
    _git(config_dir, "add", "-A", "--", ".")
    if not _git(config_dir, "status", "--porcelain", "--", *TRACKED).strip():
        return False
    _git(config_dir, "commit", "-q", "--no-verify", "-m", message, author=author)
    return True


def commit_outside_changes(config_dir: Path, why: str) -> bool:
    """Commit uncommitted changes (or, with no repository yet, the files as found) before our own write.

    A brand-new repository's first commit is of unknown origin; later uncommitted changes were
    made outside ark-cli (by hand on the volume, by a restore of an untracked backup, ...).
    """
    new = not (config_dir / ".git").is_dir()
    if new:
        return commit(config_dir, UNKNOWN, "files as first seen; origin unknown")
    return commit(config_dir, OUTSIDE, why)


def _stamp(epoch: str) -> str:
    return datetime.fromtimestamp(int(epoch), UTC).strftime("%Y-%m-%d %H:%M:%S UTC")


def origins(config_dir: Path, filename: str) -> dict[int, str]:
    """Return {line number: origin label} for every line of filename, from git blame.

    Without a repository (or for a file it has never seen), every line is "origin unknown".
    """
    path = config_dir / filename
    if not path.is_file():
        return {}
    count = len(path.read_bytes().splitlines())
    try:
        out = _git(config_dir, "blame", "--porcelain", "--", filename)
    except HistoryError:
        return dict.fromkeys(range(1, count + 1), "origin unknown")
    labels: dict[int, str] = {}
    commits: dict[str, dict[str, str]] = {}
    current: dict[str, str] = {}
    line_no = 0
    for row in out.splitlines():
        if row.startswith("\t"):
            labels[line_no] = _label(current)
            continue
        head, _, rest = row.partition(" ")
        if len(head) == 40 and rest:
            current = commits.setdefault(head, {})
            line_no = int(rest.split()[1])
        elif head in ("author", "author-time"):
            current[head] = rest
    return labels


def _label(info: dict[str, str]) -> str:
    author, when = info.get("author", ""), _stamp(info.get("author-time", "0"))
    if author == _NOT_COMMITTED:
        return "changed outside ark-cli (not yet recorded)"
    if author == UNKNOWN:
        return f"origin unknown (present {when})"
    return f"{_LABELS.get(author, author)} {when}"


def last_change(config_dir: Path, filename: str) -> str:
    """One line on the file's last recorded change, e.g. 'written by ARK 2026-... (exit code 0 run)'."""
    try:
        out = _git(config_dir, "log", "-1", "--format=%an%x00%at%x00%s", "--", filename).strip()
    except HistoryError:
        out = ""
    if not out:
        return f"There is no recorded history of {filename}."
    author, epoch, subject = out.split("\0", 2)
    return f"Last change to {filename}: {_label({'author': author, 'author-time': epoch})} ({subject})."
