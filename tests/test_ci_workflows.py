"""Outcome tests for the CI change groups (.github/changed.sh)."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

CHANGED = Path(__file__).resolve().parents[1] / ".github" / "changed.sh"
GROUPS = ("python", "go", "unit", "smoke")


def changed(group: str, *files: str, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["bash", str(CHANGED), group, *files], cwd=cwd, capture_output=True, text=True, check=False)


def has_work(group: str, *files: str) -> bool:
    result = changed(group, *files)
    assert result.returncode == 0, result.stderr
    return {"true": True, "false": False}[result.stdout.strip()]


def test_change_groups() -> None:
    in_group = {
        "python": ["tests/test_canary.py", "ruff.toml", ".github/workflows/python.yml", ".github/changed.sh"],
        "go": [
            "app/pkg/ark/compose.go",
            "container/embed.go",
            "go.work",
            "go.work.sum",
            ".golangci.yml",
            ".github/workflows/cli.yml",
            ".github/changed.sh",
        ],
        "unit": [
            "container/cmd/main.go",
            "tests/container/unit/test_x.py",
            "tests/app/ark_cli/integration/test_x.py",
            "tests/fixtures/lite_image/Dockerfile",
            "app/ark-cli/main.go",
            "app/pkg/ark/x.go",
            ".github/workflows/release.yml",
            ".github/changed.sh",
            ".github/release-version.py",
            "tests/test_ci_workflows.py",
            "tests/test_release_version.py",
        ],
    }
    outside = {
        "python": ["app/pkg/ark/x.go", "container/dockerfile", "go.work"],
        "go": ["tests/test_canary.py", "ruff.toml", "run-tests.py", ".github/workflows/python.yml"],
        "unit": ["tests/test_canary.py", "app/ark-web/main.go", "ruff.toml", "go.work"],
    }
    for group, paths in in_group.items():
        for path in paths:
            assert has_work(group, "README.md", path), (group, path)
        assert not has_work(group, *outside[group]), group

    docs_only = ("README.md", "docs/user/how-to/x.png", "app/ark-cli/NOTES.md", "container/CHANGELOG.md")
    for group in GROUPS:
        assert not has_work(group, *docs_only), group


def test_smoke_paths() -> None:
    smoke = [
        "container/base.dockerfile",
        "container/dockerfile",
        "container/files/build.sh",
        "container/files/download.sh",
        "container/files/steamcmd.dockerfile",
        "container/files/scripts/installer/install.sh",
        "container/scripts/installer/generate-default-ini.py",
        ".github/build-image.sh",
        ".github/workflows/image.yml",
        ".github/changed.sh",
        *(
            f"container/scripts/runtime/{name}"
            for name in (
                "container-entrypoint.py",
                "container-entrypoint.sh",
                "ark-server-start.py",
                "ark-server-stop.py",
                "ark-stop-reason.py",
                "rcon_client.py",
                "runtime_utils.py",
                "_logging.py",
                "ini_settings.py",
                "ini_history.py",
                "ark_backup/save_markers.py",
            )
        ),
        "tests/container/integration/slow/test_ark_server_start_script.py",
        "tests/container/integration/slow/test_ark_server_stop_script.py",
        "tests/conftest.py",
        "tests/container/conftest.py",
        "tests/container/integration/conftest.py",
        "tests/.test-order.yaml",
        "tests/helpers/lite_image.py",
        "pytest.ini",
        "run-tests.py",
        "requirements-dev.txt",
    ]
    not_smoke = [
        *(
            f"container/scripts/runtime/{name}"
            for name in ("ark-backup.py", "ark-restore.py", "ark-ini.py", "ark-rcon.py", "ark_backup/archive.py")
        ),
        "container/dockerfile.labels",
        "container/scripts/installer/ark-image-labels.sh",
        "container/cmd/main.go",
        "app/ark-cli/main.go",
        "tests/container/unit/test_x.py",
        "tests/container/integration/slow/test_other.py",
    ]
    for path in smoke:
        assert has_work("smoke", path), path
        assert has_work("unit", path), path  # smoke implies unit
    for path in not_smoke:
        assert not has_work("smoke", path), path
    assert not has_work("smoke", *not_smoke)


def git(repo: Path, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=repo, capture_output=True, text=True, check=True).stdout


def commit(repo: Path, files: dict[str, str]) -> None:
    for name, text in files.items():
        (repo / name).parent.mkdir(parents=True, exist_ok=True)
        (repo / name).write_text(text)
    git(repo, "add", "-A")
    git(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "c")


def group_hash(repo: Path, group: str) -> str:
    result = changed("hash", group, cwd=repo)
    assert result.returncode == 0, result.stderr
    return result.stdout.strip()


def test_group_hash(tmp_path: Path) -> None:
    git(tmp_path, "init", "-q")
    commit(tmp_path, {"a.py": "1\n", "app/x.go": "1\n", "README.md": "1\n"})
    before = {group: group_hash(tmp_path, group) for group in GROUPS}

    commit(tmp_path, {"README.md": "2\n", "docs/x.txt": "2\n"})
    assert {group: group_hash(tmp_path, group) for group in GROUPS} == before

    commit(tmp_path, {"app/x.go": "2\n"})
    assert group_hash(tmp_path, "python") == before["python"]
    assert group_hash(tmp_path, "go") != before["go"]

    commit(tmp_path, {"a.py": "2\n"})
    assert group_hash(tmp_path, "python") != before["python"]

    for args in (("nope", "a.py"), ("hash", "nope"), ("python",)):  # last: no origin/main
        result = changed(*args, cwd=tmp_path)
        assert result.returncode == 2, args
        assert result.stderr, args
