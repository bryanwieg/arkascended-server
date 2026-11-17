"""Helpers for building the container image during tests."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path


def image_exists(image: str, runtime: str = "") -> bool:
    """Check if a container image exists, optionally for a specific runtime."""
    try:
        from python_on_whales import DockerClient

        rt = runtime or os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
        kwargs = {}
        if rt == "podman":
            kwargs["client_binary"] = "podman"
        dc = DockerClient(**kwargs)
        return len(dc.images(image)) > 0
    except Exception:  # noqa: BLE001
        return False


def build_server_image(image: str, runtime: str = "") -> None:
    """Build the container image via build-image.sh, optionally for a runtime."""
    repo_root = Path(__file__).resolve().parents[2]
    build_script = repo_root / ".github" / "build-image.sh"
    rt = runtime or os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    env = os.environ.copy()
    env["CONTAINER_RUNTIME"] = rt
    env["FINAL_TAG"] = image
    subprocess.run(
        ["bash", str(build_script)],
        check=True,
        capture_output=True,
        env=env,
    )
