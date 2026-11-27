"""Outcome tests for the CI change groups (.github/changed.sh) and the workflow structure."""

from __future__ import annotations

import subprocess
from pathlib import Path

import pytest
import yaml

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

    changed_bytes = group_hash(tmp_path, "python")
    (tmp_path / "a.py").chmod(0o755)
    commit(tmp_path, {})
    assert group_hash(tmp_path, "python") != changed_bytes  # mode-only change

    for args in (("nope", "a.py"), ("hash", "nope"), ("python",)):  # last: no origin/main
        result = changed(*args, cwd=tmp_path)
        assert result.returncode == 2, args
        assert result.stderr, args


WORKFLOWS = CHANGED.parent / "workflows"
REQUIRED = {
    "python.yml": ("ruff",),
    "cli.yml": ("test", "lint"),
    "image.yml": ("unit", "smoke"),
    "version.yml": ("version",),
}
BOT_WORKFLOWS = ("proton-update.yml", "go-update.yml", "base-refresh.yml", "auto-merge.yml")


def workflow(name: str) -> dict:
    data = yaml.safe_load((WORKFLOWS / name).read_text())
    data["on"] = data.pop(True)  # YAML 1.1 reads the `on` key as true
    return data


def runs(job: dict) -> str:
    return "\n".join(step.get("run", "") for step in job["steps"])


def test_required_checks_always_report() -> None:
    for name, jobs in REQUIRED.items():
        data = workflow(name)
        assert set(jobs) <= set(data["jobs"]), name
        assert "pull_request" in data["on"], name
        assert not {"paths", "paths-ignore"} & set(data["on"]["pull_request"] or {}), name
        assert "workflow_dispatch" in data["on"], name


def test_required_jobs_fail_when_changes_fails() -> None:
    for name, jobs in REQUIRED.items():
        for job_name in jobs:
            job = workflow(name)["jobs"][job_name]
            if "changes" not in job.get("needs", ()):
                continue
            # Without !cancelled() a failed need skips the job, and a skipped check passes.
            assert "!cancelled()" in job["if"], job_name
            assert "needs.changes.result != 'success'" in job["if"], job_name
            first = job["steps"][0]
            assert "needs.changes.result != 'success'" in first["if"], job_name
            assert "exit 1" in first["run"], job_name


def test_no_push_runs_checks() -> None:
    for name in REQUIRED:
        push = workflow(name)["on"].get("push")
        if name == "image.yml":
            assert push == {"branches": ["main"], "paths": ["container/base.dockerfile"]}
        else:
            assert push is None, name


def test_pass_markers_per_job() -> None:
    savers: dict[str, list[str]] = {}
    lookups: dict[str, list[str]] = {}
    for path in sorted(WORKFLOWS.glob("*.yml")):
        for job_name, job in workflow(path.name)["jobs"].items():
            for step in job.get("steps", ()):
                key = step.get("with", {}).get("key", "")
                if not key.startswith("passed-"):
                    continue
                prefix = key.split("${{")[0]
                if step["uses"].startswith("actions/cache/save@"):
                    savers.setdefault(prefix, []).append(f"{path.name}:{job_name}")
                    assert prefix == f"passed-{job_name}-", (path.name, job_name)
                else:
                    lookups.setdefault(prefix, []).append(f"{path.name}:{job_name}")
                    assert step.get("with", {}).get("lookup-only") is True, prefix
                    assert prefix == f"passed-{step['id']}-", prefix
                    output = job["outputs"][f"{step['id']}-passed"]
                    assert output == f"${{{{ steps.{step['id']}.outputs.cache-hit }}}}", prefix
    assert savers.keys() == lookups.keys()
    for prefix, jobs in savers.items():
        assert len(jobs) == 1, (prefix, jobs)
        assert len(lookups[prefix]) == 1, (prefix, lookups[prefix])
    for name, jobs in REQUIRED.items():
        for job_name in jobs:
            gate = workflow(name)["jobs"][job_name].get("if", "")
            assert ("needs.changes" not in gate) or f"needs.changes.outputs.{job_name}-passed" in gate, job_name


def test_smoke_concurrency_per_ref() -> None:
    assert "github.ref" in workflow("image.yml")["jobs"]["smoke"]["concurrency"]["group"]


def test_bot_workflows_dispatch_checks() -> None:
    checks = ("python.yml", "cli.yml", "image.yml", "version.yml")
    for name in BOT_WORKFLOWS:
        scripts = [runs(job) for job in workflow(name)["jobs"].values()]
        assert any("gh workflow run" in s and all(c in s for c in checks) for s in scripts), name
    assert workflow("auto-merge.yml")["on"]["schedule"] == [{"cron": "0 * * * *"}]


def test_auto_merge_ignores_forks() -> None:
    script = "\n".join(runs(job) for job in workflow("auto-merge.yml")["jobs"].values())
    selection = next(line for line in script.splitlines() if "isCrossRepository" in line and "|" in line)
    assert "(.isCrossRepository | not)" in selection
    assert '.author.login == "app/dependabot"' in selection
    assert 'if [[ "$trusted" != true ]]' in script


def test_publish_base_on_main() -> None:
    job = workflow("image.yml")["jobs"]["publish-base"]
    assert job["if"] == "github.ref == 'refs/heads/main'"
    assert "needs" not in job


def test_smoke_gated_and_daily() -> None:
    data = workflow("image.yml")
    gate = data["jobs"]["smoke"]["if"]
    assert "needs.changes.outputs.smoke-work == 'true'" in gate
    assert "github.event_name == 'schedule'" in gate
    (schedule,) = data["on"]["schedule"]
    minute, hour, *rest = schedule["cron"].split()
    assert minute.isdigit(), schedule
    assert hour.isdigit(), schedule
    assert rest == ["*", "*", "*"], schedule
