"""set ini / show ini against a running instance on the lite image (fake ARK server)."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path

import pytest
from helpers.compose_client import container_running, start_container_instance
from helpers.infra import build_container_instance_config
from python_on_whales import DockerClient

pytestmark = [pytest.mark.container, pytest.mark.lite]


def test_set_ini_restart_applies_change_and_explains_ark_duplicates(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    compose_client: DockerClient,
) -> None:
    """set ini needs --restart on a running instance, applies the change across stop/start, and labels ARK's repeats.

    Scenario:
        A lite instance whose fake server writes DupKey twice on every shutdown is started.
        set ini runs without --restart, then with it (MaxPlayers 70 -> 20); show ini prints the
        file; set ini --restart on DupKey hits the two lines the server wrote at that stop.

    Expected:
        - Without --restart: refused, pointing at --restart.
        - With --restart: "70" -> "20" reported, instance running again, show ini has MaxPlayers=20.
        - DupKey: refused, both lines labelled "written by ARK", instance running again.
    """
    cfg = build_container_instance_config(test_env, extra_env={"FAKE_ARK_INI_DUPLICATE": "DupKey"})
    instance = cfg["instance"]
    start_container_instance(dc=compose_client, test_env=test_env, repo_root=repo_root, **cfg)

    def cli(*args: str, check: bool = True):
        base = [str(cli_path), *args[:2], "--instance", instance, "--file", "gus"]
        return run_cmd([*base, *args[2:]], env=test_env, cwd=repo_root, check=check, capture_output=True)

    set_max = ("set", "ini", "--section", "ServerSettings", "--key", "MaxPlayers", "--value", "20")
    refused = cli(*set_max, check=False)
    assert refused.returncode != 0
    assert "--restart" in refused.stderr

    assert 'MaxPlayers: "70" -> "20"' in cli(*set_max, "--restart").stdout
    assert container_running(compose_client, instance)
    assert "MaxPlayers=20" in cli("show", "ini").stdout

    dup = cli("set", "ini", "--section", "ServerSettings", "--key", "DupKey", "--value", "x", "--restart", check=False)
    assert dup.returncode != 0
    assert dup.stderr.count("(written by ARK 20") == 2, dup.stderr
    assert "Last change to GameUserSettings.ini: written by ARK 20" in dup.stderr
    assert container_running(compose_client, instance)
