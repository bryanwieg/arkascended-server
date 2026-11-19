from __future__ import annotations

import re
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.fakes import steam_api_server
from helpers.infra import image_label

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.real_server]


def test_manage_update_instance_rollout(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
) -> None:
    """update instance --rollout restarts a local-image instance to the same image and leaves it running.

    Scenario:
        An instance using the local ark-server:latest image is created and
        started. update instance --check is run to confirm the check table
        is printed correctly for a local-mode image. update instance --rollout
        is then run, which triggers a rolling restart. The fake Steam API
        reports the image's own build ID, so no rebuild happens. show compose and
        validate compose are checked afterwards, and check instance confirms
        the server is still healthy.

    Expected:
        - update instance --check exits zero and the output contains the
          expected table header "instance | current | target | action | note"
          and a row for the instance.
        - update instance --rollout exits zero.
        - show compose reports compose_file, services: 1, and the instance name.
        - validate compose reports "Compose config valid:".
        - check instance exits zero, confirming the server is running and healthy
          after the rollout.
        - show instance reports status: running.
    """
    instance = "pytest-ark-update-1"
    admin_password = "AdminPass123"
    server_password = "ServerPass123"

    run_cmd(
        [
            str(cli_path),
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            server_password,
            "--admin-password",
            admin_password,
        ],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "start", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )
    run_cmd(
        [str(cli_path), "wait", "instance", "--instance", instance, "--for", "started"],
        env=test_env,
        cwd=repo_root,
    )

    show_before = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert re.search(rf"name:\s+{re.escape(instance)}", show_before.stdout)
    assert re.search(r"status:\s+running", show_before.stdout)

    # Report the image's own build ID so the CLI sees no update and --rollout restarts the
    # instance on the same image without a rebuild (a rebuild with a fake value would download
    # ARK with SteamCMD to build one).
    current_build_id = image_label(compose_client, "ark-server:latest", "io.ark.server.build_id")
    assert current_build_id.isdigit(), f"ark-server:latest needs a numeric build_id label, got {current_build_id!r}"
    with steam_api_server(build_id=current_build_id) as steam_url:
        env = dict(test_env)
        env["ARK_STEAM_API_URL"] = steam_url

        update_check = run_cmd(
            [
                str(cli_path),
                "update",
                "instance",
                "--instance",
                instance,
                "--check",
            ],
            env=env,
            cwd=repo_root,
            capture_output=True,
        )
        assert "instance | current | target | action | note" in update_check.stdout
        assert f"{instance} |" in update_check.stdout

        run_cmd(
            [
                str(cli_path),
                "update",
                "instance",
                "--instance",
                instance,
                "--rollout",
            ],
            env=env,
            cwd=repo_root,
        )

    show_compose_result = run_cmd(
        [str(cli_path), "show", "compose"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "compose_file:" in show_compose_result.stdout
    assert "services: 1" in show_compose_result.stdout
    assert f"service_names: {instance}" in show_compose_result.stdout

    validate_result = run_cmd(
        [str(cli_path), "validate", "compose"],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert "Compose config valid:" in validate_result.stdout

    run_cmd(
        [str(cli_path), "check", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
    )

    show_after = run_cmd(
        [str(cli_path), "show", "instance", "--instance", instance],
        env=test_env,
        cwd=repo_root,
        capture_output=True,
    )
    assert re.search(r"status:\s+running", show_after.stdout)
