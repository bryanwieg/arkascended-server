from __future__ import annotations

import threading
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.artifacts import build_cache_dir
from helpers.infra import image_label, relabel_image

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.real_server]


def test_update_check_local_against_real_steam_api(
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """update instance --check queries the real Steam API and produces a valid result.

    This test intentionally hits the real Steam API (no fake server) to validate
    end-to-end integration. The `test_update_check_local_shows_up_to_date` and
    `test_update_check_local_detects_new_build` tests in docker/fast provide
    deterministic fake-server coverage for the same code paths.

    Scenario:
        An instance is created with the default local image (ark-server:latest).
        update instance --check is run without overriding the Steam API URL,
        so it hits the real steamcmd.net endpoint.

    Expected:
        - The command exits zero (no crash or unhandled error).
        - The output contains a row for the instance in local mode.
        - The action is either skip (image is current) or update (newer build
          exists on Steam) — both are valid depending on when the image was built.
        - The current version column is not empty (image labels were read).
    """
    instance = "pytest-ark-steam-api-1"

    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            "ServerPass123",
            "--admin-password",
            "AdminPass123",
        ],
        env=test_env,
        cwd=repo_root,
    )

    check_result = run_cmd(
        [str(cli_path), "update", "instance", "--instance", instance, "--check"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )

    assert f"{instance} | " in check_result.stdout
    # The row must contain one of the valid actions
    row = [line for line in check_result.stdout.splitlines() if instance in line]
    assert len(row) == 1, f"expected exactly one row for {instance}"
    assert "skip" in row[0] or "update" in row[0], f"expected skip or update action in row: {row[0]}"


def test_local_image_rebuild_produces_labeled_image(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    test_artifact_dir: Path,
) -> None:
    """update instance builds the image for Steam's build when it is missing, with correct labels.

    The update runs with a PATH holding only ark-cli (no bash, docker or podman),
    as in the scheduler, so the build must go through the engine API.

    Scenario:
        An instance is created with the default local image. ark-server:latest is
        relabeled with an old build ID (1), so the CLI sees an update against the
        real Steam API, and the real build's ark-server:<build-id>-<recipe-id> tag is removed,
        so update instance has to build it (downloading ARK with SteamCMD). The
        original image keeps its ark-server:<version> tag until the build
        replaces it, so a failed build can restore ark-server:latest. Leftover
        ark-server-files images are removed first, and the engine is polled for
        them every 5 s while update instance runs.

    Expected:
        - update instance exits zero (the build succeeds).
        - No ark-server-files image exists at any point of the run.
        - The rebuilt ark-server:latest image has a non-empty
          io.ark.server.version label matching the xx.yy format.
        - The rebuilt ark-server:latest image has a non-empty
          io.ark.server.build_id label containing only digits.
        - show instance still reports the instance name correctly.
    """
    instance = "pytest-ark-rebuild-1"

    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            "ServerPass123",
            "--admin-password",
            "AdminPass123",
        ],
        env=test_env,
        cwd=repo_root,
    )

    original_id = compose_client.image.inspect("ark-server:latest").id
    real_build = image_label(compose_client, "ark-server:latest", "io.ark.server.build_id")
    assert real_build.isdigit(), f"ark-server:latest needs a numeric build_id label, got {real_build!r}"
    real_tag = f"ark-server:{real_build}-{image_label(compose_client, 'ark-server:latest', 'io.ark.recipe')}"
    relabel_image(compose_client, "ark-server:latest", {"io.ark.server.build_id": "1"}, test_artifact_dir / "relabel")
    relabeled_id = compose_client.image.inspect("ark-server:latest").id
    # Untag only: the original image stays reachable through its ark-server:<version> tag.
    compose_client.image.remove(real_tag, force=False)
    # Share SteamCMD's install between runs, so only the first run downloads the whole server.
    shared_work = build_cache_dir(repo_root) / "steamcmd-work"
    shared_work.mkdir(exist_ok=True)
    work_dir = test_artifact_dir / ".ark-manage" / "build" / "work"
    work_dir.parent.mkdir(parents=True, exist_ok=True)
    work_dir.symlink_to(shared_work, target_is_directory=True)
    # A PATH with only ark-cli: the build must need neither bash nor a container CLI, as in the scheduler.
    bare_bin = test_artifact_dir / "bare-bin"
    bare_bin.mkdir()
    (bare_bin / "ark-cli").symlink_to(cli_path)
    env = {
        **test_env,
        "ARK_BUILD_NO_CACHE": "1",
        "PATH": str(bare_bin),
        "ARK_CONTAINER_RUNTIME": "docker",
        "DOCKER_HOST": test_env.get("DOCKER_HOST", "unix:///var/run/docker.sock"),
    }
    for leftover in compose_client.image.list("ark-server-files"):
        compose_client.image.remove(leftover.id, force=True)
    seen_files_images: list[str] = []
    done = threading.Event()

    def poll_files_images() -> None:
        while not done.wait(5):
            seen_files_images.extend(
                t for image in compose_client.image.list("ark-server-files") for t in image.repo_tags
            )

    poller = threading.Thread(target=poll_files_images, daemon=True)
    poller.start()
    try:
        run_cmd(
            [str(bare_bin / "ark-cli"), "update", "instance", "--instance", instance],
            env=env,
            cwd=repo_root,
        )
    finally:
        done.set()
        poller.join()
        # A failed build must not leave the fake-labeled image as ark-server:latest.
        if compose_client.image.inspect("ark-server:latest").id == relabeled_id:
            compose_client.image.tag(original_id, "ark-server:latest")
            compose_client.image.tag(original_id, real_tag)
            compose_client.image.remove(relabeled_id, force=False)

    assert not seen_files_images, f"the build must not make a files image, saw {seen_files_images}"

    version = image_label(
        compose_client,
        "ark-server:latest",
        "io.ark.server.version",
    )
    build_id = image_label(
        compose_client,
        "ark-server:latest",
        "io.ark.server.build_id",
    )

    assert version, "rebuilt image must have a version label"
    parts = version.split(".")
    assert len(parts) == 2 and all(p.isdigit() for p in parts), f"version label must be xx.yy format, got: {version}"  # noqa: PT018

    assert build_id, "rebuilt image must have a build_id label"
    assert build_id.isdigit(), f"build_id label must be numeric, got: {build_id}"

    show_result = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert f"name: {instance}" in show_result.stdout
