"""Outcome test for the release version rules (.github/release-version.py)."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

SCRIPT = Path(__file__).resolve().parents[1] / ".github" / "release-version.py"


def run(*args: str, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(SCRIPT), *args], cwd=cwd, capture_output=True, text=True, check=False)


def next_version(title: str, tag: str = "cli-v0.11.0") -> str:
    result = run("next", "--title", title, "--latest-tag", tag)
    assert result.returncode == 0, result.stderr
    return result.stdout.strip()


def check(tmp_path: Path, title: str, body: str) -> subprocess.CompletedProcess[str]:
    body_file = tmp_path / "body.md"
    body_file.write_text(body)
    return run("check", "--title", title, "--body-file", str(body_file), "--latest-tag", "cli-v0.11.0")


def test_release_version_rules(tmp_path: Path) -> None:
    assert next_version("feat: x") == "0.12.0"
    assert next_version("feat(cli): x") == "0.12.0"
    for kind in ("fix", "perf", "revert", "deps", "build"):
        assert next_version(f"{kind}: x") == "0.11.1", kind
    assert next_version("build(deps): x") == "0.11.1"
    assert next_version("fix(cli)!: x") == "0.12.0"
    assert next_version("feat!: x", "cli-v1.4.2") == "2.0.0"
    for title in ("ci: x", "chore(deps): x", "docs: x", "feat(web): x", "Add a thing"):
        assert next_version(title) == "", title

    good = {
        ("feat: x", "Summary.\n\nRelease: 0.12.0\n"),
        ("fix: x", "Release: 0.11.1"),
        ("ci: x", "No release here."),
        ("feat(web): x", ""),
    }
    for title, body in good:
        result = check(tmp_path, title, body)
        assert result.returncode == 0, (title, result.stderr)
    bad = {
        ("feat: x", "Release: 0.11.1", "Release: 0.12.0"),
        ("feat: x", "", "Release: 0.12.0"),
        ("ci: x", "Release: 0.11.1", "no Release: line"),
        ("Add a thing", "", "Conventional Commit"),
        ("fix: x", "Release: 0.11.1\nRelease: 0.11.1", "at most one"),
        ("fix: x", "Release: v0.11.1", "malformed"),
    }
    for title, body, reason in bad:
        result = check(tmp_path, title, body)
        assert result.returncode == 1, (title, body)
        assert reason in result.stderr, (title, body, result.stderr)

    (tmp_path / "body.md").write_text("Release: 1.2\n")
    assert run("parse", "--body-file", str(tmp_path / "body.md")).returncode == 1
    (tmp_path / "body.md").write_text("Release: 0.12.0\n")
    assert run("parse", "--body-file", str(tmp_path / "body.md")).stdout == "0.12.0\n"

    repo = tmp_path / "repo"
    repo.mkdir()
    subprocess.run(["git", "init", "-q"], cwd=repo, check=True)
    assert run("latest", cwd=repo).returncode == 1
    subprocess.run(
        ["git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "c"],
        cwd=repo,
        check=True,
    )
    for tag in ("cli-v0.9.0", "cli-v0.11.0", "cli-v0.10.0"):
        subprocess.run(["git", "tag", tag], cwd=repo, check=True)
    assert run("latest", cwd=repo).stdout == "cli-v0.11.0\n"
