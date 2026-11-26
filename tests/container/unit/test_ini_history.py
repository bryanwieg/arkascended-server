"""ini_history's revisions, file_at and packing, against a repository in tmp_path."""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

RUNTIME_DIR = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
if str(RUNTIME_DIR) not in sys.path:
    sys.path.insert(0, str(RUNTIME_DIR))

import ini_history as ih  # noqa: E402

_DAY = 86400
_NOV_1 = 1793491200  # 2026-11-01 00:00:00 UTC


def commit_at(monkeypatch, config_dir: Path, epoch: int, author: str, message: str, **files: str) -> None:
    """Write files (Game= / GameUserSettings= content) and commit them as author at epoch."""
    for stem, content in files.items():
        (config_dir / f"{stem}.ini").write_text(content)
    monkeypatch.setenv("GIT_AUTHOR_DATE", f"@{epoch} +0000")
    monkeypatch.setenv("GIT_COMMITTER_DATE", f"@{epoch} +0000")
    assert ih.commit(config_dir, author, message)


def test_revisions_filters_and_labels(tmp_path: Path, monkeypatch) -> None:
    def revisions(filename=None, *, user_only=False, limit=20, since=None, patch=False):
        return ih.revisions(tmp_path, filename, user_only=user_only, limit=limit, since=since, patch=patch)

    assert revisions() == ([], False)  # no repository

    history = [
        (_NOV_1 - 2 * _DAY, ih.UNKNOWN, "first seen", {"GameUserSettings": "[S]\nK=1\n"}),
        (_NOV_1 - 30, ih.ARK, "exit code 0 run", {"GameUserSettings": "[S]\nK=2\n"}),
        (_NOV_1 + 30, ih.CLI_SET, "[S] K", {"GameUserSettings": "[S]\nK=3\n"}),
        (_NOV_1 + 40, ih.ENV, "env", {"GameUserSettings": "[S]\nK=4\n"}),
        (_NOV_1 + 50, ih.CLI_RESTORE, "Game.ini to abc", {"Game": "[G]\nA=1\n"}),
        (_NOV_1 + 60, ih.OUTSIDE, "boot", {"Game": "[G]\nA=\xff\n", "GameUserSettings": "[S]\nK=5\n"}),
        (_NOV_1 + 70, ih.ARK, "exit code 0 run", {"Game": "[G]\nA=2\n"}),
        (_NOV_1 + 80, "someone else", "by hand", {"Game": "[G]\nA=3\n"}),
    ]
    for epoch, author, message, files in history:
        commit_at(monkeypatch, tmp_path, epoch, author, message, **files)
    (tmp_path / "Game.ini").write_bytes(b"[G]\nA=\xff\n")  # not UTF-8
    commit_at(monkeypatch, tmp_path, _NOV_1 + 90, ih.CLI_EDIT, "edited")

    rows, more = revisions()
    assert not more
    assert [(r["by"], r["how"], r["author"], r["message"]) for r in rows] == [
        ("user", "edited with ark-cli", ih.CLI_EDIT, "edited"),
        ("unknown", "someone else", "someone else", "by hand"),
        ("ARK", "written by ARK", ih.ARK, "exit code 0 run"),
        ("user", "changed outside ark-cli", ih.OUTSIDE, "boot"),
        ("user", "restored with ark-cli", ih.CLI_RESTORE, "Game.ini to abc"),
        ("user", "set from instance env", ih.ENV, "env"),
        ("user", "set with ark-cli", ih.CLI_SET, "[S] K"),
        ("ARK", "written by ARK", ih.ARK, "exit code 0 run"),
        ("unknown", "origin unknown", ih.UNKNOWN, "first seen"),
    ]
    assert rows[3]["files"] == ["Game.ini", "GameUserSettings.ini"]
    assert rows[3]["date"] == "2026-11-01 00:01:00 UTC"
    assert rows[3]["revision"] == ih._git(tmp_path, "rev-parse", "--short", "HEAD~3").strip()
    assert "patch" not in rows[0]

    limited, more = revisions(user_only=True, limit=3)
    assert more
    assert [r["message"] for r in limited] == ["edited", "by hand", "boot"]  # limit counts what is returned
    assert revisions(user_only=True, limit=7) == ([r for r in rows if r["author"] != ih.ARK], False)
    assert revisions(limit=0) == (rows, False)
    assert revisions(limit=8) == (rows[:8], True)

    assert [r["message"] for r in revisions(since="2026-11-01")[0]][-1] == "[S] K"  # 00:00:30 that day is in
    assert len(revisions(since="2026-11-01")[0]) == 7  # 23:59:30 the day before is not
    assert [r["message"] for r in revisions("Game.ini")[0]] == [
        "edited",
        "by hand",
        "exit code 0 run",
        "boot",
        "Game.ini to abc",
    ]

    patched, _more = revisions(limit=2, patch=True)
    assert "-A=3\n+A=�\n" in patched[0]["patch"]  # the non-UTF-8 byte is replaced, not an error
    assert "-A=2\n+A=3\n" in patched[1]["patch"]
    assert {k: v for k, v in patched[1].items() if k != "patch"} == rows[1]


def test_file_at_reads_old_content_and_refuses_unknown(tmp_path: Path) -> None:
    old = b"[S]\r\nK=\xff\xfe\r\n"
    (tmp_path / "GameUserSettings.ini").write_bytes(old)
    ih.commit(tmp_path, ih.ARK, "image default")
    (tmp_path / "GameUserSettings.ini").write_bytes(b"[S]\nK=new\n")
    ih.commit(tmp_path, ih.CLI_SET, "[S] K")
    (first, second), _more = ih.revisions(tmp_path, None, user_only=False, limit=0, since=None, patch=False)

    assert ih.file_at(tmp_path, second["revision"], "GameUserSettings.ini") == old
    assert ih.file_at(tmp_path, first["revision"], "GameUserSettings.ini") == b"[S]\nK=new\n"

    blob = ih._git(tmp_path, "rev-parse", "HEAD:GameUserSettings.ini").strip()
    for revision, filename in [
        ("0123abc", "GameUserSettings.ini"),  # unknown
        (blob, "GameUserSettings.ini"),  # not a commit
        ("--output=x", "GameUserSettings.ini"),  # not an option
        (first["revision"], "Game.ini"),  # absent at that revision
    ]:
        with pytest.raises(ih.HistoryError, match=f"no {filename} at revision '{revision}'"):
            ih.file_at(tmp_path, revision, filename)
    assert not (tmp_path / "x").exists()


def test_pack_failure_does_not_fail_commit(tmp_path: Path, monkeypatch, capsys) -> None:
    monkeypatch.setattr(ih, "_PACK_AFTER", 0)
    real_git = ih._git

    def failing_gc(config_dir: Path, *args: str, author: str = ih.UNKNOWN) -> str:
        if args[0] == "gc":
            raise ih.HistoryError("git gc failed: disk full")
        return real_git(config_dir, *args, author=author)

    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[S]\nK=1\n")
    with monkeypatch.context() as patched:
        patched.setattr(ih, "_git", failing_gc)
        assert ih.commit(tmp_path, ih.ARK, "first")
    assert capsys.readouterr().err.count("[ini-history] Warning: could not pack the INI history") == 1
    assert not list(tmp_path.glob(".git/objects/pack/*.pack"))

    gus.write_text("[S]\nK=2\n")
    assert ih.commit(tmp_path, ih.CLI_SET, "second")  # packs again, and now it works
    assert capsys.readouterr().err == ""
    assert list(tmp_path.glob(".git/objects/pack/*.pack"))
    assert ih._git(tmp_path, "log", "--format=%an: %s").splitlines() == ["ark-cli set ini: second", "ARK server: first"]
    assert ih.file_at(tmp_path, "HEAD~1", "GameUserSettings.ini") == b"[S]\nK=1\n"
