from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest


def repo_root() -> Path:
    return Path(__file__).resolve().parents[1]


def test_build_cli() -> None:
    """Validate the ark-cli binary is available, building it if not.

    If the binary is missing, builds it with go build.
    If Go is not installed or the build fails, the test fails with
    a clear message before any integration tests run.
    """
    root = repo_root()
    bin_path = root / "app" / "ark-cli" / "ark-cli"

    if bin_path.exists():
        return

    if not shutil.which("go"):
        pytest.fail("Go is required to build ark-cli. Install Go and ensure it's in PATH, then re-run.")

    env = os.environ.copy()
    env["GOCACHE"] = env.get("GOCACHE", str(root / ".go-cache"))
    env["GOTMPDIR"] = env.get("GOTMPDIR", str(root / ".ark-tmp"))

    subprocess.run(
        ["go", "build", "-o", str(bin_path)],
        check=True,
        cwd=str(root / "app" / "ark-cli"),
        env=env,
    )

    assert bin_path.exists(), f"Build completed but binary not found at {bin_path}"
