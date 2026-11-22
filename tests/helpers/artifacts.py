from __future__ import annotations

import os
import re
import shutil
import subprocess
import time
from pathlib import Path

import pytest


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def test_tmp_dir(root: Path) -> Path:
    """Returns the temp directory for tests, creating if needed."""
    tmp = root / ".ark-tmp"
    tmp.mkdir(parents=True, exist_ok=True)
    return tmp


def _artifacts_dir(root: Path) -> Path:
    # All test logs, state files, compose snapshots, and run artifacts
    # must live under .tests/ — never use .ark-tmp, /tmp, or any other
    # directory for test framework output. This ensures cleanup and
    # retention logic sees everything in one place.
    base = os.environ.get("ARK_TEST_ARTIFACTS_DIR", ".tests")
    path = Path(base)
    if not path.is_absolute():
        path = root / path
    return path


def tests_root(root: Path) -> Path:
    path = _artifacts_dir(root)
    path.mkdir(parents=True, exist_ok=True)
    return path


def sanitize_path_component(value: str) -> str:
    sanitized = re.sub(r"[^A-Za-z0-9._-]+", "-", value).strip("-")
    return sanitized or "unknown"


def create_suite_run_dir(root: Path) -> Path:
    """Create a YYMMDD-HHMMSS-suiterun directory and return its path."""
    timestamp = time.strftime("%y%m%d-%H%M%S", time.localtime())
    suite_dir = tests_root(root) / f"{timestamp}-suiterun"
    suffix = 1
    while suite_dir.exists():
        suffix += 1
        suite_dir = tests_root(root) / f"{timestamp}-suiterun-{suffix}"
    suite_dir.mkdir(parents=True, exist_ok=True)
    return suite_dir


def ensure_python(python_bin: str) -> str:
    """Resolve a Python binary as a shell would and return its absolute path. Raise SystemExit if unusable.

    A bare name is looked up on PATH; a path is relative to the current directory (batches run from the repo
    root, so an unresolved relative path would point somewhere else).
    """
    found = shutil.which(python_bin) if os.sep not in python_bin else python_bin
    path = Path(found).absolute() if found else None
    if path is None or not path.exists() or not os.access(path, os.X_OK):
        raise SystemExit(f"ERROR: pytest Python not found or not executable: {python_bin}\n")
    return str(path)


def test_node_dir_name(nodeid: str) -> str:
    return sanitize_path_component(nodeid.replace("::", "__").replace("/", "_"))


def _outcome_label(outcome: str, phase: str) -> str:
    if outcome == "passed":
        return "completed" if phase in ("setup", "teardown") else "passed"
    return "failed"


def write_pytest_report_artifact(item: pytest.Item, artifact_dir: Path) -> None:
    report_path = artifact_dir / "pytest.log"
    sections: list[str] = [f"nodeid: {item.nodeid}"]
    wrote_phase = False

    call_report = getattr(item, "rep_call", None)
    if call_report is not None:
        result = "PASSED" if call_report.passed else "FAILED"
        sections.append(f"result: {result}")
    else:
        sections.append("result: (no call phase recorded)")

    for phase in ("setup", "call", "teardown"):
        report = getattr(item, f"rep_{phase}", None)
        if report is None:
            continue
        wrote_phase = True
        label = _outcome_label(report.outcome, phase)
        sections.append("")
        sections.append(f"== {phase}: {label} ==")
        duration = getattr(report, "duration", None)
        if isinstance(duration, float):
            sections.append(f"duration_seconds: {duration:.3f}")
        longreprtext = getattr(report, "longreprtext", "")
        if isinstance(longreprtext, str) and longreprtext.strip():
            sections.append("")
            sections.append("longrepr:")
            sections.append(longreprtext.rstrip())
        report_sections = getattr(report, "sections", [])
        for section_name, content in report_sections:
            if not content or not content.strip():
                continue
            sections.append("")
            sections.append(f"{section_name}:")
            sections.append(content.rstrip())

    if not wrote_phase:
        sections.append("")
        sections.append("No phase reports were recorded for this test item.")

    report_path.write_text("\n".join(sections) + "\n", encoding="utf-8")


def artifacts_retention() -> int:
    """Number of recent run directories to keep: ARK_TEST_ARTIFACTS_RETENTION, default 7."""
    return int(os.environ.get("ARK_TEST_ARTIFACTS_RETENTION", "7"))


def wipe_test_artifacts(root: Path, preserve_recent: int = 7) -> None:
    """Delete test artifacts but preserve the N most recent run directories."""
    artifacts_dir = _artifacts_dir(root)
    resolved = artifacts_dir.resolve()
    protected = {root.resolve(), (root / "tests").resolve()}
    if resolved in protected or resolved.parent == resolved:
        raise SystemExit(f"Refusing to wipe unsafe test artifacts path: {artifacts_dir}")
    if not artifacts_dir.exists():
        artifacts_dir.mkdir(parents=True, exist_ok=True)
        return

    # Identify run directories (timestamp-named folders)
    run_dirs = sorted(
        [d for d in artifacts_dir.iterdir() if d.is_dir()],
        key=lambda d: d.name,
    )

    # Preserve the most recent N directories and the .run-state directory
    dirs_to_keep = set(run_dirs[-preserve_recent:]) if preserve_recent > 0 else set()
    dirs_to_keep.add(artifacts_dir / ".run-state")

    # Remove everything else
    for item in artifacts_dir.iterdir():
        if item in dirs_to_keep:
            continue
        if item.is_dir():
            shutil.rmtree(item)
        else:
            item.unlink()

    # Ensure directory exists (in case we deleted everything)
    artifacts_dir.mkdir(parents=True, exist_ok=True)


def prune_test_artifacts(root: Path, retention: int = 7) -> None:
    """Keep only the N most recent test run directories under the artifacts root."""
    if retention <= 0:
        return
    artifacts_dir = tests_root(root)
    run_dirs = sorted(
        [d for d in artifacts_dir.iterdir() if d.is_dir()],
        key=lambda d: d.name,
    )
    for old_dir in run_dirs[:-retention]:
        shutil.rmtree(old_dir)


def cli_path(root: Path) -> Path:
    return root / "app" / "ark-cli" / "ark-cli"


def ensure_cli_binary(root: Path) -> Path:
    """Build ark-cli from the current source and return its path.

    Always rebuilds (Go only recompiles what changed, so an up-to-date binary takes about a
    second): reusing any existing binary let CLI tests silently run stale code, including the
    container files the binary bundles for `update instance` local builds.
    """
    path = cli_path(root)
    if not shutil.which("go"):
        if path.exists():
            return path
        pytest.skip("go is required to build ark-cli for integration tests.")
    env = os.environ.copy()
    env["GOCACHE"] = env.get("GOCACHE", str(repo_root() / ".go-cache"))
    env["GOTMPDIR"] = env.get("GOTMPDIR", str(repo_root() / ".ark-tmp"))
    env["CGO_ENABLED"] = "0"  # static, so the binary runs in the scratch scheduler image
    Path(env["GOTMPDIR"]).mkdir(parents=True, exist_ok=True)  # go refuses a missing GOTMPDIR (fresh CI checkout)
    subprocess.run(
        ["go", "build", "-o", str(path)],
        check=True,
        cwd=str(root / "app" / "ark-cli"),
        env=env,
    )
    return path
