"""INI commands (set, get, show, unset, history, restore) against a running instance on the lite image (fake ARK server)."""

from __future__ import annotations

import json
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
        set ini runs without --restart, then with it (MaxPlayers 70 -> 20); show ini and get ini
        read it back (get also lists the server's two DupKey lines); set ini --restart on DupKey
        hits the two lines the server wrote at that stop.

    Expected:
        - Without --restart: refused, pointing at --restart.
        - With --restart: "70" -> "20" reported, instance running again, show ini has MaxPlayers=20,
          get ini prints 20 and lists both DupKey lines as written by ARK.
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
    get = ("get", "ini", "--section", "ServerSettings", "--json", "--key")
    assert [line["value"] for line in json.loads(cli(*get, "MaxPlayers").stdout)["lines"]] == ["20"]
    dup_lines = json.loads(cli(*get, "DupKey").stdout)["lines"]
    assert all(line["origin"].startswith("written by ARK 20") for line in dup_lines), dup_lines
    assert len(dup_lines) == 2, dup_lines

    dup = cli("set", "ini", "--section", "ServerSettings", "--key", "DupKey", "--value", "x", "--restart", check=False)
    assert dup.returncode != 0
    assert dup.stderr.count("(written by ARK 20") == 2, dup.stderr
    assert "Last change to GameUserSettings.ini: written by ARK 20" in dup.stderr
    assert container_running(compose_client, instance)


def test_unset_ini_removes_keys_with_restart_and_all(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    compose_client: DockerClient,
) -> None:
    """unset ini needs --restart on a running instance, removes the key across stop/start, and needs --all for repeats.

    Scenario:
        A lite instance whose fake server writes DupKey twice on every shutdown is started.
        unset ini runs on MaxPlayers without --restart, then with it; then on DupKey with
        --restart, without and with --all.

    Expected:
        - Without --restart: refused, pointing at --restart; MaxPlayers is still 70.
        - With --restart: "70" reported as removed, instance running again, MaxPlayers has no line.
        - DupKey without --all: refused, pointing at --all, instance running again.
        - DupKey with --all: both values reported as removed, instance running again, no DupKey line.
    """
    cfg = build_container_instance_config(test_env, extra_env={"FAKE_ARK_INI_DUPLICATE": "DupKey"})
    instance = cfg["instance"]
    start_container_instance(dc=compose_client, test_env=test_env, repo_root=repo_root, **cfg)

    def cli(*args: str, check: bool = True):
        base = [str(cli_path), *args[:2], "--instance", instance, "--file", "gus", "--section", "ServerSettings"]
        return run_cmd([*base, *args[2:]], env=test_env, cwd=repo_root, check=check, capture_output=True)

    def values(key: str) -> list[str]:
        return [line["value"] for line in json.loads(cli("get", "ini", "--json", "--key", key).stdout)["lines"]]

    refused = cli("unset", "ini", "--key", "MaxPlayers", check=False)
    assert refused.returncode != 0
    assert "--restart" in refused.stderr
    assert values("MaxPlayers") == ["70"]

    assert 'MaxPlayers: removed "70"' in cli("unset", "ini", "--key", "MaxPlayers", "--restart").stdout
    assert container_running(compose_client, instance)
    assert values("MaxPlayers") == []

    dup = cli("unset", "ini", "--key", "DupKey", "--restart", check=False)
    assert dup.returncode != 0
    assert "--all" in dup.stderr
    assert container_running(compose_client, instance)

    assert 'DupKey: removed "ark-a", "ark-b"' in cli("unset", "ini", "--key", "DupKey", "--restart", "--all").stdout
    assert container_running(compose_client, instance)
    assert values("DupKey") == []


def test_list_ini_history_shows_user_and_ark_revisions(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    compose_client: DockerClient,
) -> None:
    """list ini-history shows the fake server's writes and ark-cli's own, and its flags narrow the list.

    Scenario:
        A lite instance is started and MaxPlayers is set with --restart, so the history holds the
        fake server's write at that stop and the set. list ini-history then runs as a table, with
        --json, --user-only, --limit 1, --since and --patch.

    Expected:
        - Table: the header, one row per revision of --json (newest first, UTC dates), then the
          note that ARK's writes are included.
        - The set is by "user", "set with ark-cli"; the server's write is by "ARK".
        - --user-only: no ARK row and no note. --limit 1: the newest row and a hint at older ones.
        - --since a far future date: no history. --patch: the set's diff.
    """
    cfg = build_container_instance_config(test_env)
    instance = cfg["instance"]
    start_container_instance(dc=compose_client, test_env=test_env, repo_root=repo_root, **cfg)

    def cli(*args: str) -> str:
        cmd = [str(cli_path), *args[:2], "--instance", instance, *args[2:]]
        return run_cmd(cmd, env=test_env, cwd=repo_root, check=True, capture_output=True).stdout

    set_args = ("--file", "gus", "--section", "ServerSettings", "--key", "MaxPlayers", "--value", "20", "--restart")
    cli("set", "ini", *set_args)

    revisions = json.loads(cli("list", "ini-history", "--json"))["revisions"]
    assert [r["date"] for r in revisions] == sorted((r["date"] for r in revisions), reverse=True)
    assert all(r["date"].endswith(" UTC") and r["by"] in ("ARK", "user", "unknown") for r in revisions), revisions
    set_row = next(r for r in revisions if r["how"] == "set with ark-cli")
    assert (set_row["by"], set_row["files"]) == ("user", ["GameUserSettings.ini"]), set_row
    assert "MaxPlayers=20" in set_row["message"]
    assert any(r["by"] == "ARK" for r in revisions), revisions

    table = cli("list", "ini-history").splitlines()
    assert table[0].split() == ["REVISION", "DATE", "BY", "HOW", "FILES", "MESSAGE"]
    assert [line.split()[0] for line in table[1:-1]] == [r["revision"] for r in revisions]
    assert "ARK's own writes" in table[-1]
    assert "--user-only" in table[-1]

    user_only = cli("list", "ini-history", "--user-only")
    assert set_row["revision"] in user_only
    assert " ARK " not in user_only
    assert "--user-only" not in user_only

    limited = cli("list", "ini-history", "--limit", "1").splitlines()
    assert limited[1].split()[0] == revisions[0]["revision"]
    assert len(limited) == 4
    assert "--limit 0" in limited[-1]
    assert "--since" in limited[-1]

    assert f"No INI history recorded for {instance}." in cli("list", "ini-history", "--since", "2999-01-01")
    assert "+MaxPlayers=20" in cli("list", "ini-history", "--patch", "--user-only", "--file", "gus")


def test_restore_ini_brings_back_a_revision(
    test_env: dict[str, str],
    repo_root: Path,
    cli_path: Path,
    run_cmd: Callable[..., object],
    compose_client: DockerClient,
) -> None:
    """restore ini needs --restart on a running instance, writes the file as a revision had it, and records it.

    Scenario:
        A lite instance is started and MaxPlayers is set from 70 to 20 with --restart. restore ini
        then targets the revision just before that set: without --restart, with it, with a
        revision the history doesn't have, and for Game.ini, which no revision has.

    Expected:
        - Without --restart: refused, pointing at --restart; MaxPlayers is still 20.
        - With --restart: restore reported, instance running again, MaxPlayers is 70 and the
          newest user revision is "restored with ark-cli".
        - Unknown revision: refused, GameUserSettings.ini unchanged.
        - Game.ini: refused, instance running again, Game.ini still missing.
    """
    cfg = build_container_instance_config(test_env)
    instance = cfg["instance"]
    start_container_instance(dc=compose_client, test_env=test_env, repo_root=repo_root, **cfg)

    def cli(*args: str, check: bool = True):
        cmd = [str(cli_path), *args[:2], "--instance", instance, *args[2:]]
        return run_cmd(cmd, env=test_env, cwd=repo_root, check=check, capture_output=True)

    def user_revisions() -> list[dict]:
        return json.loads(cli("list", "ini-history", "--json", "--file", "gus", "--user-only").stdout)["revisions"]

    cli(
        "set",
        "ini",
        "--file",
        "gus",
        "--section",
        "ServerSettings",
        "--key",
        "MaxPlayers",
        "--value",
        "20",
        "--restart",
    )
    revisions = json.loads(cli("list", "ini-history", "--json", "--file", "gus", "--limit", "0").stdout)["revisions"]
    set_index = next(i for i, r in enumerate(revisions) if r["how"] == "set with ark-cli")
    before_set = revisions[set_index + 1]["revision"]

    refused = cli("restore", "ini", "--file", "gus", "--revision", before_set, check=False)
    assert refused.returncode != 0
    assert "--restart" in refused.stderr
    assert "MaxPlayers=20" in cli("show", "ini", "--file", "gus").stdout

    restored = cli("restore", "ini", "--file", "gus", "--revision", before_set, "--restart")
    assert f"GameUserSettings.ini restored from {before_set}." in restored.stdout
    assert container_running(compose_client, instance)
    gus = cli("show", "ini", "--file", "gus").stdout
    assert "MaxPlayers=70" in gus
    assert user_revisions()[0]["how"] == "restored with ark-cli"

    unknown = cli("restore", "ini", "--file", "gus", "--revision", "deadbeef", "--restart", check=False)
    assert unknown.returncode != 0
    assert f"revision deadbeef is not in the INI history of {instance}" in unknown.stderr
    assert cli("show", "ini", "--file", "gus").stdout == gus

    no_file = cli("restore", "ini", "--file", "game", "--revision", before_set, "--restart", check=False)
    assert no_file.returncode != 0
    assert "has no Game.ini at revision" in no_file.stderr
    assert container_running(compose_client, instance)
    assert cli("show", "ini", "--file", "game", check=False).returncode != 0
