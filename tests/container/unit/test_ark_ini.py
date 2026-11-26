"""ark-ini.py (get/set) and the ARK-write tracking in ini_settings, against files in tmp_path."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

RUNTIME_DIR = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
if str(RUNTIME_DIR) not in sys.path:
    sys.path.insert(0, str(RUNTIME_DIR))

import ini_history as ih  # noqa: E402


@pytest.fixture(name="ark_ini")
def ark_ini_fixture(tmp_path: Path, monkeypatch):
    spec = importlib.util.spec_from_file_location("ark_ini", RUNTIME_DIR / "ark-ini.py")
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    monkeypatch.setattr(
        module, "FILES", {"game": tmp_path / "Game.ini", "gameusersettings": tmp_path / "GameUserSettings.ini"}
    )
    monkeypatch.setattr(module, "ark_server_running", lambda: False)
    return module


@pytest.fixture(autouse=True)
def _git_dates(monkeypatch) -> None:
    """Pin commit times, so origin labels are exact: 1970-01-02 00:00:00 UTC."""
    monkeypatch.setenv("GIT_AUTHOR_DATE", "@86400 +0000")
    monkeypatch.setenv("GIT_COMMITTER_DATE", "@86400 +0000")


_WHEN = "1970-01-02 00:00:00 UTC"


def log(config_dir: Path) -> list[str]:
    """Commit authors and subjects, newest first."""
    return ih._git(config_dir, "log", "--format=%an: %s").splitlines()


def run(ark_ini, capsys, *argv: str) -> tuple[int, str, str]:
    code = ark_ini.main(list(argv))
    out, err = capsys.readouterr()
    return code, out, err


@pytest.mark.parametrize(
    ("content", "section", "key", "value", "want", "old"),
    [
        (
            "[ServerSettings]\r\n; note\r\nA=1\r\nMaxPlayers=70\r\nB=2\r\n",
            "ServerSettings",
            "maxplayers",
            "20",
            "[ServerSettings]\r\n; note\r\nA=1\r\nmaxplayers=20\r\nB=2\r\n",
            ["70"],
        ),
        ("[S]\nK=a\nX=1\nK = b\n[T]\nK=c\n", "S", "K", "z", "[S]\nK=z\nX=1\n[T]\nK=c\n", ["a", "b"]),
        ("[S]\nA=1\n\n[T]\nB=2\n", "S", "K", "v", "[S]\nA=1\nK=v\n\n[T]\nB=2\n", []),
        (
            "[S]\nA=1",
            "/Script/ShooterGame.ShooterGameMode",
            "K",
            "",
            "[S]\nA=1\n\n[/Script/ShooterGame.ShooterGameMode]\nK=\n",
            [],
        ),
        ("", "S", "K", "v", "[S]\nK=v\n", []),
        ("[S]\n;K=old\n", "S", "K", "v", "[S]\n;K=old\nK=v\n", []),
        ("[S]\nK=1\n[T]\n[s]\nK=2\n", "S", "K", "v", "[S]\nK=v\n[T]\n[s]\nK=2\n", ["1"]),
    ],
    ids=[
        "crlf-comments-order",
        "collapse",
        "missing-key",
        "missing-section",
        "empty",
        "comment-not-a-match",
        "repeated-section-first-only",
    ],
)
def test_set_value(ark_ini, content, section, key, value, want, old) -> None:
    assert ark_ini.set_value(content, section, key, value) == (want, old)


def test_set_reports_change_as_json_and_keeps_mode(ark_ini, capsys, tmp_path: Path) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nMaxPlayers=70\n")
    gus.chmod(0o640)
    code, out, err = run(
        ark_ini,
        capsys,
        "set",
        "--file",
        "gameusersettings",
        "--section",
        "[ServerSettings]",
        "--key",
        "MaxPlayers",
        "--value",
        "20",
    )
    assert (code, err) == (0, "")
    assert json.loads(out) == {
        "file": "GameUserSettings.ini",
        "section": "ServerSettings",
        "key": "MaxPlayers",
        "value": "20",
        "old": ["70"],
        "changed": True,
        "removed": 0,
    }
    assert gus.read_text() == "[ServerSettings]\nMaxPlayers=20\n"
    assert gus.stat().st_mode & 0o777 == 0o640


def test_set_creates_missing_game_ini_but_not_gameusersettings(ark_ini, capsys, tmp_path: Path) -> None:
    code, _out, _err = run(ark_ini, capsys, "set", "--file", "game", "--section", "S", "--key", "K", "--value", "v")
    assert code == 0
    assert (tmp_path / "Game.ini").read_text() == "[S]\nK=v\n"
    assert (tmp_path / "Game.ini").stat().st_mode & 0o777 == 0o644

    code, _out, err = run(
        ark_ini, capsys, "set", "--file", "gameusersettings", "--section", "S", "--key", "K", "--value", "v"
    )
    assert code == 1
    assert "doesn't exist yet" in err
    assert not (tmp_path / "GameUserSettings.ini").exists()


@pytest.mark.parametrize(
    ("args", "message"),
    [
        (("--section", "ServerSettings", "--key", "ServerAdminPassword", "--value", "x"), "--admin-password"),
        (("--section", "ServerSettings", "--key", "rconenabled", "--value", "False"), "always on"),
        (("--section", "S[x", "--key", "K", "--value", "v"), "--section"),
        (("--section", "S", "--key", "K=1", "--value", "v"), "--key"),
        (("--section", "S", "--key", "K", "--value", "a\nb"), "line breaks"),
    ],
)
def test_set_refuses_bad_requests(ark_ini, capsys, tmp_path: Path, args, message) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\n")
    code, _out, err = run(ark_ini, capsys, "set", "--file", "gameusersettings", *args)
    assert code == 1
    assert err.startswith("Error:")
    assert message in err
    assert gus.read_text() == "[ServerSettings]\n"


def test_set_refuses_while_server_runs(ark_ini, capsys, tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setattr(ark_ini, "ark_server_running", lambda: True)
    code, _out, err = run(ark_ini, capsys, "set", "--file", "game", "--section", "S", "--key", "K", "--value", "v")
    assert code == 1
    assert "server is running" in err
    assert not (tmp_path / "Game.ini").exists()


def test_get_prints_file_or_missing_error(ark_ini, capsys, tmp_path: Path) -> None:
    (tmp_path / "Game.ini").write_text("[S]\r\nK=v\r\n")
    assert run(ark_ini, capsys, "get", "--file", "game") == (0, "[S]\r\nK=v\r\n", "")
    code, _out, err = run(ark_ini, capsys, "get", "--file", "gameusersettings")
    assert code == 1
    assert "GameUserSettings.ini doesn't exist yet" in err


def test_set_keeps_crlf_bom_and_non_utf8_bytes(ark_ini, capsys, tmp_path: Path) -> None:
    game = tmp_path / "Game.ini"
    game.write_bytes(b"\xef\xbb\xbf[S]\r\nName=caf\xe9\r\nK=1\r\n")
    code, _out, err = run(ark_ini, capsys, "set", "--file", "game", "--section", "S", "--key", "K", "--value", "2")
    assert (code, err) == (0, "")
    assert game.read_bytes() == b"\xef\xbb\xbf[S]\r\nName=caf\xe9\r\nK=2\r\n"


def test_replace_all_on_game_ini_collapses_list_lines(ark_ini, capsys, tmp_path: Path) -> None:
    game = tmp_path / "Game.ini"
    game.write_text("[S]\nOverrideNamedEngramEntries=a\nOverrideNamedEngramEntries=b\n")
    code, out, _err = run(
        ark_ini,
        capsys,
        "set",
        "--file",
        "game",
        "--section",
        "S",
        "--key",
        "OverrideNamedEngramEntries",
        "--value",
        "c",
        "--replace-all",
    )
    assert (code, json.loads(out)["removed"]) == (0, 1)
    assert game.read_text() == "[S]\nOverrideNamedEngramEntries=c\n"
    assert not (tmp_path / "Game.ini.bak").exists()


def test_get_key_works_while_server_runs_and_needs_both_names(ark_ini, capsys, tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setattr(ark_ini, "ark_server_running", lambda: True)
    (tmp_path / "Game.ini").write_text("[S]\nK=v\n")
    code, out, _err = run(ark_ini, capsys, "get", "--file", "game", "--section", "S", "--key", "K")
    assert (code, json.loads(out)["lines"]) == (0, [{"line": 2, "value": "v", "origin": "origin unknown"}])

    with pytest.raises(SystemExit) as usage:
        ark_ini.main(["get", "--file", "game", "--key", "K"])
    assert usage.value.code == 2
    assert "both --section and --key" in capsys.readouterr().err


def test_duplicates_refused_with_origin_then_replace_all(ark_ini, capsys, tmp_path: Path) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nK=ark\n")
    ih.commit(tmp_path, ih.ARK, "written by the server during its run (exit code 0)")
    gus.write_text("[ServerSettings]\nK=ark\nK=restored\n")  # e.g. a hand edit, not recorded yet
    args = ("set", "--file", "gameusersettings", "--section", "ServerSettings", "--key", "K", "--value", "new")

    code, out, err = run(ark_ini, capsys, *args)
    assert (code, out) == (1, "")
    assert "K appears 2 times" in err
    assert f"line 2: K=ark  (written by ARK {_WHEN})" in err
    assert "line 3: K=restored  (changed outside ark-cli (not yet recorded))" in err
    assert f"Last change to GameUserSettings.ini: written by ARK {_WHEN} (written by the server" in err
    assert gus.read_text() == "[ServerSettings]\nK=ark\nK=restored\n"

    code, out, _err = run(ark_ini, capsys, *args, "--replace-all")
    result = json.loads(out)
    assert (code, result["removed"], result["old"]) == (0, 1, ["ark", "restored"])
    assert gus.read_text() == "[ServerSettings]\nK=new\n"
    # The hand edit is committed as such before set's own write, so the history holds the previous file.
    assert ih._git(tmp_path, "show", "HEAD~1:GameUserSettings.ini") == "[ServerSettings]\nK=ark\nK=restored\n"
    assert log(tmp_path) == [
        "ark-cli set ini: [ServerSettings] K=new",
        "outside ark-cli: changed outside ark-cli; found before set ini",
        "ARK server: written by the server during its run (exit code 0)",
    ]
    assert ih.origins(tmp_path, "GameUserSettings.ini")[2] == f"set with ark-cli {_WHEN}"
    assert ih._git(tmp_path, "ls-files").split() == ["GameUserSettings.ini"]


def test_duplicates_origin_unknown_without_history(ark_ini, capsys, tmp_path: Path) -> None:
    (tmp_path / "Game.ini").write_text("[S]\nK=1\nK=2\n")
    code, _out, err = run(ark_ini, capsys, "set", "--file", "game", "--section", "S", "--key", "K", "--value", "3")
    assert code == 1
    assert err.count("(origin unknown)") == 2
    assert "There is no recorded history of Game.ini." in err


def test_history_baseline_then_outside_changes(ark_ini, capsys, tmp_path: Path) -> None:
    """Files from before tracking are 'origin unknown'; later unrecorded changes are 'outside ark-cli'."""
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nK=old\n")
    (tmp_path / "Engine.ini").write_text("[x]\n")  # not tracked
    assert ih.commit_outside_changes(tmp_path, "boot")
    gus.write_text("[ServerSettings]\nK=old\nK=added\n")
    assert ih.commit_outside_changes(tmp_path, "boot")
    assert not ih.commit_outside_changes(tmp_path, "boot")  # nothing new

    code, out, _err = run(
        ark_ini, capsys, "get", "--file", "gameusersettings", "--section", "ServerSettings", "--key", "K"
    )
    assert code == 0
    assert [line["origin"] for line in json.loads(out)["lines"]] == [
        f"origin unknown (present {_WHEN})",
        f"changed outside ark-cli {_WHEN}",
    ]
    assert log(tmp_path) == ["outside ark-cli: boot", "unknown origin: files as first seen; origin unknown"]
    assert ih._git(tmp_path, "ls-files").split() == ["GameUserSettings.ini"]


def test_track_records_edit_and_outside_changes(ark_ini, capsys, tmp_path: Path) -> None:
    game = tmp_path / "Game.ini"
    game.write_text("[S]\nA=1\n")
    assert run(ark_ini, capsys, "track", "--by", "outside") == (0, "", "")
    game.write_text("[S]\nA=1\nB=2\n")
    assert run(ark_ini, capsys, "track", "--by", "edit") == (0, "", "")
    assert log(tmp_path) == [
        "ark-cli edit ini: edited with ark-cli edit ini",
        "unknown origin: files as first seen; origin unknown",
    ]
    assert ih.origins(tmp_path, "Game.ini")[3] == f"edited with ark-cli {_WHEN}"


def test_server_run_commits_only_when_files_changed(tmp_path: Path, monkeypatch) -> None:
    spec = importlib.util.spec_from_file_location("ark_server_start", RUNTIME_DIR / "ark-server-start.py")
    assert spec is not None
    assert spec.loader is not None
    start = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(start)
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nA=1\n")
    ih.commit(tmp_path, ih.ENV, "RCON and password settings from the environment")
    monkeypatch.setattr(start, "GAME_USER_SETTINGS", gus)

    gus.write_text("[ServerSettings]\nA=1\n")  # rewritten with the same content: nothing to record
    start.record_server_ini_write(0)
    gus.write_text("[ServerSettings]\nA=2\n")
    start.record_server_ini_write(2)
    assert log(tmp_path) == [
        "ARK server: written by the server during its run (exit code 2)",
        "instance env: RCON and password settings from the environment",
    ]


def test_get_key_returns_lines_with_origins_and_history(ark_ini, capsys, tmp_path: Path) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nK=ark\nMaxPlayers=70\n")
    ih.commit(tmp_path, ih.ARK, "image default installed (generated by ARK at image build)")
    gus.write_text("[ServerSettings]\nK=ark\nMaxPlayers=70\nK=restored\n")
    ih.commit(tmp_path, ih.OUTSIDE, "changed while the instance was stopped")
    args = ("get", "--file", "gameusersettings", "--section", "[ServerSettings]")

    code, out, err = run(ark_ini, capsys, *args, "--key", "k")
    assert (code, err) == (0, "")
    reply = json.loads(out)
    assert (reply["file"], reply["section"], reply["key"]) == ("GameUserSettings.ini", "ServerSettings", "k")
    assert reply["lines"] == [
        {"line": 2, "value": "ark", "origin": f"written by ARK {_WHEN}"},
        {"line": 4, "value": "restored", "origin": f"changed outside ark-cli {_WHEN}"},
    ]
    assert reply["history"] == (
        f"Last change to GameUserSettings.ini: changed outside ark-cli {_WHEN} (changed while the instance was stopped)."
    )

    code, out, _err = run(ark_ini, capsys, *args, "--key", "Missing")
    assert (code, json.loads(out)["lines"]) == (0, [])


def test_set_same_value_writes_and_commits_nothing(ark_ini, capsys, tmp_path: Path) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nMaxPlayers=20\n")
    ih.commit(tmp_path, ih.ARK, "image default installed (generated by ARK at image build)")
    before = gus.stat().st_mtime_ns
    code, out, _err = run(
        ark_ini,
        capsys,
        "set",
        "--file",
        "gameusersettings",
        "--section",
        "ServerSettings",
        "--key",
        "MaxPlayers",
        "--value",
        "20",
    )
    assert (code, json.loads(out)["changed"]) == (0, False)
    assert gus.stat().st_mtime_ns == before
    assert log(tmp_path) == ["ARK server: image default installed (generated by ARK at image build)"]


def test_set_reports_a_git_failure_as_an_error(ark_ini, capsys, tmp_path: Path, monkeypatch) -> None:
    (tmp_path / "Game.ini").write_text("[S]\nK=1\n")

    def broken(*_a, **_k):
        raise ih.HistoryError("git commit failed: disk full")

    monkeypatch.setattr(ih, "commit_outside_changes", broken)
    code, out, err = run(ark_ini, capsys, "set", "--file", "game", "--section", "S", "--key", "K", "--value", "2")
    assert (code, out, err) == (1, "", "Error: git commit failed: disk full\n")


def test_missing_key_goes_after_a_last_line_without_newline(ark_ini) -> None:
    assert ark_ini.set_value("[S]\nA=1", "S", "K", "v") == ("[S]\nA=1\nK=v\n", [])


def test_get_missing_game_ini_says_how_to_create_it(ark_ini, capsys) -> None:
    code, _out, err = run(ark_ini, capsys, "get", "--file", "game")
    assert code == 1
    assert "Game.ini doesn't exist yet. Use 'set ini' or 'edit ini' to create it." in err


def test_history_without_git_or_files(tmp_path: Path, monkeypatch) -> None:
    """No git binary: commits raise HistoryError; reads fall back to 'unknown' instead of failing."""
    (tmp_path / "Game.ini").write_text("[S]\nK=1\n")
    assert ih.origins(tmp_path, "GameUserSettings.ini") == {}  # no such file
    assert ih.last_change(tmp_path, "Game.ini") == "There is no recorded history of Game.ini."
    monkeypatch.setenv("PATH", str(tmp_path / "no-bin"))
    with pytest.raises(ih.HistoryError, match="git init failed"):
        ih.commit(tmp_path, ih.ARK, "x")
    assert ih.origins(tmp_path, "Game.ini") == {1: "origin unknown", 2: "origin unknown"}


_UNSET = ("unset", "--file", "gameusersettings", "--section", "ServerSettings", "--key")


def test_unset_removes_key_and_commits(ark_ini, capsys, tmp_path: Path) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    before = b"[ServerSettings]\r\n; note\r\nA=1\r\nMaxPlayers=70\r\nB=2\r\n[T]\r\nMaxPlayers=5\r\n"
    gus.write_bytes(before)
    ih.commit(tmp_path, ih.ARK, "image default")
    gus.write_bytes(before + b"C=3\r\n")  # a hand edit, not recorded yet
    code, out, err = run(ark_ini, capsys, *_UNSET, "maxplayers")
    assert (code, err) == (0, "")
    assert json.loads(out) == {
        "file": "GameUserSettings.ini",
        "section": "ServerSettings",
        "key": "maxplayers",
        "old": ["70"],
        "changed": True,
    }
    assert gus.read_bytes() == b"[ServerSettings]\r\n; note\r\nA=1\r\nB=2\r\n[T]\r\nMaxPlayers=5\r\nC=3\r\n"
    assert not (tmp_path / "GameUserSettings.ini.bak").exists()
    assert log(tmp_path) == [
        "ark-cli unset ini: [ServerSettings] maxplayers",
        "outside ark-cli: changed outside ark-cli; found before unset ini",
        "ARK server: image default",
    ]


def test_unset_missing_key_is_no_change(ark_ini, capsys, tmp_path: Path) -> None:
    code, _out, err = run(ark_ini, capsys, *_UNSET, "K")
    assert code == 1
    assert "GameUserSettings.ini doesn't exist yet" in err

    gus = tmp_path / "GameUserSettings.ini"
    gus.write_text("[ServerSettings]\nA=1\n")
    for args in (
        (*_UNSET, "K"),  # missing key
        ("unset", "--file", "gameusersettings", "--section", "Other", "--key", "A"),  # missing section
        ("unset", "--file", "game", "--section", "S", "--key", "K"),  # missing Game.ini
    ):
        code, out, _err = run(ark_ini, capsys, *args)
        reply = json.loads(out)
        assert (code, reply["old"], reply["changed"]) == (0, [], False)
    assert gus.read_text() == "[ServerSettings]\nA=1\n"
    assert sorted(p.name for p in tmp_path.iterdir()) == ["GameUserSettings.ini"]  # no Game.ini or .git


def test_unset_refuses_env_keys_duplicates_and_running_server(ark_ini, capsys, tmp_path: Path, monkeypatch) -> None:
    gus = tmp_path / "GameUserSettings.ini"
    content = "[ServerSettings]\nRCONPort=27020\nK=ark\nK=restored\n"
    gus.write_text(content)
    ih.commit(tmp_path, ih.ARK, "written by the server during its run (exit code 0)")

    code, out, err = run(ark_ini, capsys, *_UNSET, "rconPORT")
    assert (code, out) == (1, "")
    assert "set from the instance's environment" in err
    assert "ark-cli reconfigure instance --rcon-port" in err

    code, out, err = run(ark_ini, capsys, *_UNSET, "K")
    assert (code, out) == (1, "")
    assert "K appears 2 times" in err
    assert f"line 3: K=ark  (written by ARK {_WHEN})" in err
    assert f"line 4: K=restored  (written by ARK {_WHEN})" in err
    assert f"Last change to GameUserSettings.ini: written by ARK {_WHEN}" in err
    assert "--all" in err

    monkeypatch.setattr(ark_ini, "ark_server_running", lambda: True)
    code, _out, err = run(ark_ini, capsys, *_UNSET, "K", "--all")
    assert code == 1
    assert "ark-cli unset ini --restart" in err
    assert gus.read_text() == content

    monkeypatch.setattr(ark_ini, "ark_server_running", lambda: False)
    code, out, _err = run(ark_ini, capsys, *_UNSET, "K", "--all")
    result = json.loads(out)
    assert (code, result["old"]) == (0, ["ark", "restored"])
    assert gus.read_text() == "[ServerSettings]\nRCONPort=27020\n"
    assert ih._git(tmp_path, "show", "HEAD~1:GameUserSettings.ini") == content


def test_history_stays_packed(ark_ini, capsys, tmp_path: Path) -> None:
    """A few hundred `set ini` runs leave a small packed .git that still holds every revision unchanged."""
    (tmp_path / "GameUserSettings.ini").write_text("[ServerSettings]\n" + "Filler=some value\n" * 300)
    ih.commit(tmp_path, ih.ARK, "image default")
    fmt = "--format=%H %an %at %s"
    seen = [ih._git(tmp_path, "log", "-1", fmt).strip()]
    for n in range(250):
        code, _out, err = run(
            ark_ini,
            capsys,
            "set",
            "--file",
            "gameusersettings",
            "--section",
            "ServerSettings",
            "--key",
            "MaxPlayers",
            "--value",
            str(n),
        )
        assert (code, err) == (0, "")
        seen.append(ih._git(tmp_path, "log", "-1", fmt).strip())

    git_dir = tmp_path / ".git"
    assert ih._git(tmp_path, "log", fmt).splitlines() == seen[::-1]
    assert len(set(seen)) == 251
    assert sum(f.stat().st_size for f in git_dir.rglob("*") if f.is_file()) < 300_000
    assert list(git_dir.glob("objects/pack/*.pack"))
    assert int(ih._git(tmp_path, "count-objects").split()[0]) <= ih._PACK_AFTER
    assert not (git_dir / "logs").exists()
    assert not (git_dir / "hooks").exists()
    ih._git(tmp_path, "fsck", "--strict")
