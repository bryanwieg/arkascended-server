"""Git history of the instance's INI files: who wrote each line, and when.

A repository in the INI directory (Saved/Config/WindowsServer/.git, so backups and restores carry
it) tracks Game.ini and GameUserSettings.ini only. Every writer commits under its own author name:

  ARK server       the image default (ARK generated it at build) and the server's own writes
  instance env     the RCON and password settings the entrypoint applies from the environment
  ark-cli set ini  ark-ini.py set
  ark-cli unset ini  ark-ini.py unset
  ark-cli edit ini the editor run by `ark-cli edit ini`
  ark-cli restore ini  a file brought back to an earlier revision
  outside ark-cli  changes found uncommitted (a hand edit or a restored file), committed when found
  unknown origin   the files as first seen, when the repository is created on an existing volume

origins() turns `git blame` into a label per line, e.g. "written by ARK 2026-11-19 00:01:26 UTC".
revisions() lists the commits and file_at() reads a file as a revision had it. The repository keeps
no reflog and no hooks, and commit() packs it, so it stays small; no revision is ever dropped.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from datetime import UTC, datetime
from pathlib import Path

from _logging import emit_warning

TRACKED = ("Game.ini", "GameUserSettings.ini")
ARK = "ARK server"
ENV = "instance env"
CLI_SET = "ark-cli set ini"
CLI_UNSET = "ark-cli unset ini"
CLI_EDIT = "ark-cli edit ini"
CLI_RESTORE = "ark-cli restore ini"
OUTSIDE = "outside ark-cli"
UNKNOWN = "unknown origin"
_LABELS = {
    ARK: "written by ARK",
    ENV: "set from instance env",
    CLI_SET: "set with ark-cli",
    CLI_UNSET: "unset with ark-cli",
    CLI_EDIT: "edited with ark-cli",
    CLI_RESTORE: "restored with ark-cli",
    OUTSIDE: "changed outside ark-cli",
}
_NOT_COMMITTED = "Not Committed Yet"  # git blame's author for lines changed since the last commit
_PACK_AFTER = 50  # loose objects (a revision adds about three) before commit packs the repository


class HistoryError(Exception):
    """git failed; the message says which command and why."""


def _run(config_dir: Path, args: tuple[str, ...], author: str, *, text: bool) -> str | bytes:
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
    # No reflog: it would only keep unreachable objects alive, and nothing here rewrites history.
    options = ["-c", f"safe.directory={config_dir}", "-c", "core.logAllRefUpdates=false"]
    cmd = ["git", *options, "-C", str(config_dir), *args]
    try:
        result = subprocess.run(cmd, env=env, capture_output=True, text=text, check=False)
    except OSError as exc:
        raise HistoryError(f"git {args[0]} failed: {exc}") from exc
    if result.returncode != 0:
        stderr = result.stderr if text else result.stderr.decode("utf-8", "replace")
        raise HistoryError(f"git {args[0]} failed: {stderr.strip() or result.returncode}")
    return result.stdout


def _git(config_dir: Path, *args: str, author: str = UNKNOWN) -> str:
    return _run(config_dir, args, author, text=True)


def _git_bytes(config_dir: Path, *args: str) -> bytes:
    """Like _git, but the output's exact bytes (text mode translates CRLF and needs UTF-8)."""
    return _run(config_dir, args, UNKNOWN, text=False)


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
    try:
        _pack(config_dir)
    except (HistoryError, OSError) as exc:
        # The commit itself succeeded; the next commit packs again.
        emit_warning("ini-history", f"could not pack the INI history in {config_dir}: {exc}")
    return True


def _pack(config_dir: Path) -> None:
    """Drop the reflog and hooks, and pack the objects once enough are loose. Every revision stays."""
    for name in ("logs", "hooks"):
        if (config_dir / ".git" / name).is_dir():
            shutil.rmtree(config_dir / ".git" / name)
    loose = int(_git(config_dir, "count-objects").split()[0])
    if loose > _PACK_AFTER:
        _git(config_dir, "gc", "-q")


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


def _by(author: str) -> str:
    if author == ARK:
        return "ARK"
    return "user" if author in _LABELS else "unknown"


def revisions(
    config_dir: Path, filename: str | None, *, user_only: bool, limit: int, since: str | None, patch: bool
) -> tuple[list[dict], bool]:
    """Return (revisions newest first, whether older ones exist beyond limit).

    filename keeps the revisions that changed that file, user_only drops the ARK server's, since
    (YYYY-MM-DD) keeps those from 00:00:00 UTC that day on; limit 0 returns all. patch adds each
    revision's diff. Without a repository there are no revisions.
    """
    if not (config_dir / ".git").is_dir():
        return [], False
    since_epoch = 0 if since is None else datetime.strptime(since, "%Y-%m-%d").replace(tzinfo=UTC).timestamp()
    out = _git(config_dir, "log", "--name-only", "--format=%x1e%h%x00%an%x00%at%x00%s%x00")
    rows = [chunk.split("\0", 4) for chunk in out.split("\x1e")[1:]]
    found = [
        {
            "revision": revision,
            "date": _stamp(epoch),
            "by": _by(author),
            "how": "origin unknown" if author == UNKNOWN else _LABELS.get(author, author),
            "author": author,
            "files": files.split(),
            "message": subject,
        }
        for revision, author, epoch, subject, files in rows
        if int(epoch) >= since_epoch
        and not (user_only and author == ARK)
        and (filename is None or filename in files.split())
    ]
    kept = found[:limit] if limit else found
    if not patch:
        return kept, len(found) > len(kept)
    show = ("show", "--format=", "--no-color")
    patched = [
        {**row, "patch": _git_bytes(config_dir, *show, row["revision"], "--", *TRACKED).decode("utf-8", "replace")}
        for row in kept
    ]
    return patched, len(found) > len(kept)


def file_at(config_dir: Path, revision: str, filename: str) -> bytes:
    """Return filename's exact bytes as revision had it; HistoryError if there is no such revision or file."""
    try:
        return _git_bytes(config_dir, "show", "--end-of-options", f"{revision}^{{commit}}:{filename}")
    except HistoryError as exc:
        raise HistoryError(f"the INI history has no {filename} at revision {revision!r}: {exc}") from exc
