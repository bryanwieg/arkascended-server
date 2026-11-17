from __future__ import annotations

import json
import re
from collections.abc import Callable
from pathlib import Path
from typing import TYPE_CHECKING

import pytest
from helpers.fakes import steam_api_server
from helpers.infra import image_label

if TYPE_CHECKING:
    from python_on_whales import DockerClient

pytestmark = [pytest.mark.container]


def test_update_instance_rollout_rolls_back_broken_image(
    compose_client: DockerClient,
    test_env: dict[str, str],
    slow_test_protection,
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    test_artifact_dir: Path,
) -> None:
    """update instance --rollout puts a running instance back on its image when the new one fails.

    Scenario:
        An instance is created and started on the real image (pinned to
        ark-server:<real>). A broken image, ark-server:<real+1>, is built from it
        with an entrypoint that exits at once. The fake Steam API reports
        <real+1>, so update instance finds that image already built and rolls
        the instance out to it.

    Expected:
        - update instance --rollout exits non-zero and says it rolled back to
          ark-server:<real>.
        - The compose file points the instance at ark-server:<real> again.
        - check instance reports result: healthy.
        - The update state records last_attempt.result rolled_back with a
          pre-update backup that exists.
    """
    instance = "pytest-ark-rollback-1"
    for args in (
        [
            "create",
            "instance",
            "--name",
            instance,
            "--server-password",
            "ServerPass123",
            "--admin-password",
            "AdminPass123",
        ],
        ["start", "instance", "--instance", instance],
        ["wait", "instance", "--instance", instance, "--for", "started"],
    ):
        run_cmd([str(cli_path), *args], env=test_env, cwd=repo_root)

    real = image_label(compose_client, "ark-server:latest", "io.ark.server.build_id")
    assert real.isdigit(), f"ark-server:latest needs a numeric build_id label, got {real!r}"
    fake = str(int(real) + 1)
    broken = f"ark-server:{fake}"
    build_dir = test_artifact_dir / "broken-image"
    build_dir.mkdir(parents=True, exist_ok=True)
    (build_dir / "Dockerfile").write_text(
        f'FROM ark-server:latest\nLABEL io.ark.server.build_id="{fake}"\nENTRYPOINT ["/bin/false"]\n',
        encoding="utf-8",
    )
    compose_client.buildx.build(str(build_dir), tags=[broken], load=True)
    try:
        with steam_api_server(build_id=fake) as steam_url:
            env = dict(test_env)
            env["ARK_STEAM_API_URL"] = steam_url
            update = run_cmd(
                [str(cli_path), "update", "instance", "--instance", instance, "--rollout"],
                env=env,
                cwd=repo_root,
                capture_output=True,
                check=False,
            )
        assert update.returncode == 1, f"update must fail, got {update.returncode}:\n{update.stdout}\n{update.stderr}"
        assert f"rolled back to ark-server:{real}" in update.stdout + update.stderr

        compose_file = Path(test_env["COMPOSE_FILE"])
        assert re.search(rf"image:\s+ark-server:{real}\b", compose_file.read_text(encoding="utf-8"))

        check = run_cmd(
            [str(cli_path), "check", "instance", "--instance", instance],
            env=test_env,
            cwd=repo_root,
            capture_output=True,
        )
        assert "result: healthy" in check.stdout

        state = json.loads(
            (compose_file.parent / ".ark-manage" / "state" / "updates" / f"{instance}.json").read_text(encoding="utf-8")
        )
        assert state["last_attempt"]["result"] == "rolled_back"
        assert Path(state["last_attempt"]["backup"]).is_file()
    finally:
        compose_client.image.remove(broken, force=True)
