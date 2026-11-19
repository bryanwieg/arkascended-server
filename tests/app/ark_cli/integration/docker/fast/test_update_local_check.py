from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.fakes import steam_api_server
from helpers.infra import image_label

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.real_server]


def test_update_check_local_shows_up_to_date(
    compose_client: DockerClient,
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """update instance --check reports skip when the local image build ID matches Steam.

    Scenario:
        An instance is created with the default local image (ark-server:latest).
        A fake Steam API server is started that returns the same build ID as the
        local image. update instance --check is run with the Steam API URL
        overridden to point at the fake server.

    Expected:
        - The output contains a row for the instance in local mode.
        - The action is skip or the message indicates the image is up to date.
        - The compose file is not modified.
    """
    instance = "pytest-ark-local-check-1"

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

    compose_path = Path(test_env["COMPOSE_FILE"])
    compose_before = compose_path.read_text(encoding="utf-8")

    current_build_id = image_label(
        compose_client,
        "ark-server:latest",
        "io.ark.server.build_id",
    )
    assert current_build_id, "local image must have a build_id label"

    with steam_api_server(build_id=current_build_id) as steam_url:
        env = dict(test_env)
        env["ARK_STEAM_API_URL"] = steam_url

        check_result = run_cmd(
            [str(cli_path), "update", "instance", "--instance", instance, "--check"],
            env=env,
            cwd=repo_root,
            capture_output=True,
        )

    assert f"{instance} | " in check_result.stdout
    assert "skip" in check_result.stdout or "up to date" in check_result.stdout

    compose_after = compose_path.read_text(encoding="utf-8")
    assert compose_after == compose_before, "compose file should not change on --check"


def test_update_check_local_detects_new_build(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """update instance --check reports update when the Steam build ID differs from local.

    Scenario:
        An instance is created with the default local image. A fake Steam API
        server is started that returns a different build ID (99999999) from the
        local image. update instance --check is run.

    Expected:
        - The output contains a row for the instance in local mode with action update.
        - The compose file is not modified (check-only mode).
    """
    instance = "pytest-ark-local-check-2"

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

    compose_path = Path(test_env["COMPOSE_FILE"])
    compose_before = compose_path.read_text(encoding="utf-8")

    with steam_api_server(build_id="99999999") as steam_url:
        env = dict(test_env)
        env["ARK_STEAM_API_URL"] = steam_url

        check_result = run_cmd(
            [str(cli_path), "update", "instance", "--instance", instance, "--check"],
            env=env,
            cwd=repo_root,
            capture_output=True,
        )

    assert f"{instance} | " in check_result.stdout
    assert "update" in check_result.stdout

    compose_after = compose_path.read_text(encoding="utf-8")
    assert compose_after == compose_before, "compose file should not change on --check"
