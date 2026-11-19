"""The lite test image: real runtime scripts and a fake ARK server, no ARK files.

Tests that don't need the real server mark themselves `@pytest.mark.lite`; the `test_env`
fixtures then point ARK_TEST_IMAGE at this image, so the usual helpers (container_instance_config,
started_container_instance, start_container_instance) run against it unchanged.

The image is tagged with a hash of its inputs, so it is built once and rebuilt only when the
runtime scripts or tests/fixtures/lite_image change.
"""

from __future__ import annotations

import hashlib
import os
import shutil
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
FIXTURE_DIR = REPO_ROOT / "tests/fixtures/lite_image"
RUNTIME_DIR = REPO_ROOT / "container/scripts/runtime"


def _context_files() -> dict[str, Path]:
    """Map each build-context path to its source file."""
    files = {p.name: p for p in FIXTURE_DIR.iterdir() if p.is_file()}
    for path in sorted(RUNTIME_DIR.rglob("*")):
        if path.is_file() and "__pycache__" not in path.parts:
            files[f"runtime/{path.relative_to(RUNTIME_DIR)}"] = path
    return files


def lite_image_tag() -> str:
    """Return ark-server:lite-<hash of the build inputs>.

    The ark-server repository because ark-cli only starts ark-server images (it keeps any existing
    ark-server:<tag> as is); the lite- tag keeps it clear of ark-server:latest and real builds.
    """
    digest = hashlib.sha256()
    for name, path in sorted(_context_files().items()):
        digest.update(name.encode() + b"\0" + path.read_bytes() + b"\0")
    return f"ark-server:lite-{digest.hexdigest()[:12]}"


def ensure_lite_image() -> str:
    """Build the lite image if this runtime doesn't have it yet; return its tag."""
    from python_on_whales import DockerClient

    runtime = os.environ.get("ARK_CONTAINER_RUNTIME", "docker")
    dc = DockerClient(client_binary="podman") if runtime == "podman" else DockerClient()
    tag = lite_image_tag()
    if dc.image.exists(tag):
        return tag
    # A staged context keeps the build from sending the whole repo (and .tests/) to the daemon.
    context = REPO_ROOT / ".tests/lite-image-context"
    shutil.rmtree(context, ignore_errors=True)
    for name, path in _context_files().items():
        (context / name).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(path, context / name)
    dc.build(context, tags=[tag])
    shutil.rmtree(context, ignore_errors=True)
    return tag


def image_for_test(request: pytest.FixtureRequest) -> str | None:
    """The lite image for a test marked `lite`, else None (the configured ARK image)."""
    return ensure_lite_image() if request.node.get_closest_marker("lite") else None
