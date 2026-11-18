"""runtime_utils.server_command: the Proton command line shared by the start script and the INI generator."""

from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

_PROTON = "/opt/proton"


@pytest.fixture(name="ru_mod")
def ru_mod_fixture(monkeypatch: pytest.MonkeyPatch):
    runtime_path = str(Path(__file__).resolve().parents[3] / "container/scripts/runtime")
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    for key in ("MAP_NAME", "SESSION_NAME", "INSTANCE_NAME", "ASA_PORT", "PROTON_VERSION", "MOD_IDS"):
        monkeypatch.delenv(key, raising=False)
    monkeypatch.setenv("PROTON_PATH", _PROTON + "/")
    module = importlib.import_module("runtime_utils")
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)
    return module


def _command(url: str, *extra: str) -> list[str]:
    return [f"{_PROTON}/proton", "run", "./ArkAscendedServer.exe", url, *extra, "-servergamelog"]


@pytest.mark.parametrize(
    ("env", "extra", "expected"),
    [
        ({}, (), _command("TheIsland_WP?listen?Port=27015")),
        (
            {"MAP_NAME": "Aberration_WP", "INSTANCE_NAME": "Fallback Session", "ASA_PORT": "7777"},
            ("-NoBattlEye",),
            _command("Aberration_WP?listen?SessionName=Fallback Session?Port=7777", "-NoBattlEye"),
        ),
        (
            {"MAP_NAME": "ScorchedEarth_WP", "SESSION_NAME": "Primary Session", "INSTANCE_NAME": "Ignored Session"},
            (),
            _command("ScorchedEarth_WP?listen?SessionName=Primary Session?Port=27015"),
        ),
        (
            {"MOD_IDS": " 940975, 1386057 "},
            ("-NoBattlEye",),
            _command("TheIsland_WP?listen?Port=27015", "-mods=940975,1386057", "-NoBattlEye"),
        ),
    ],
    ids=["defaults", "instance-name-port-flag", "session-name-wins", "mod-ids"],
)
def test_server_command_builds_the_proton_command_line(
    ru_mod, monkeypatch: pytest.MonkeyPatch, env: dict[str, str], extra: tuple[str, ...], expected: list[str]
) -> None:
    """SESSION_NAME beats INSTANCE_NAME, the port defaults to 27015, and extra args go before -servergamelog."""
    for key, value in env.items():
        monkeypatch.setenv(key, value)
    assert ru_mod.server_command(*extra) == expected


def test_server_command_defaults_proton_path_from_version(ru_mod, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PROTON_PATH")
    monkeypatch.setenv("PROTON_VERSION", "9-20")
    assert ru_mod.server_command()[0] == "/home/ark-user/.steam/compatibilitytools.d/GE-Proton9-20/proton"


def test_server_command_warns_when_map_name_missing_wp(ru_mod, monkeypatch, capsys: pytest.CaptureFixture[str]) -> None:
    monkeypatch.setenv("MAP_NAME", "TheIsland")
    assert ru_mod.server_command()[3] == "TheIsland?listen?Port=27015"
    assert capsys.readouterr().err.startswith('[server-args] Warning: MAP_NAME "TheIsland" does not end in _WP.')


def test_server_command_no_warning_when_map_name_has_wp(
    ru_mod, monkeypatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setenv("MAP_NAME", "TheIsland_WP")
    ru_mod.server_command()
    assert capsys.readouterr().err == ""


@pytest.mark.parametrize("value", ["940975;1386057", "abc", "940975,", ",940975"])
def test_server_command_rejects_malformed_mod_ids(ru_mod, monkeypatch: pytest.MonkeyPatch, value: str) -> None:
    monkeypatch.setenv("MOD_IDS", value)
    with pytest.raises(ru_mod.ConfigError, match="MOD_IDS"):
        ru_mod.server_command()
