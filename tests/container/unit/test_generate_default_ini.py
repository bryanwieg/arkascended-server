"""installer/generate-default-ini.py: the pure helpers that shape the image's default GameUserSettings.ini."""

from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

_SCRIPT = Path(__file__).resolve().parents[3] / "container/scripts/installer/generate-default-ini.py"


@pytest.fixture(name="gen")
def gen_fixture():
    spec = importlib.util.spec_from_file_location("generate_default_ini", _SCRIPT)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# Shapes of the two files ARK writes (from a probe run): partial first, complete on clean shutdown.
_PARTIAL = "[ScalabilityGroups]\nsg.ShadowQuality=1\n\n[Startup]\nFSRQualityMode=4\n"
_COMPLETE = (
    ";METADATA=(Diff=true, UseCommands=true)\r\n"
    "[ServerSettings]\r\n"
    "AutoSavePeriodMinutes=15\r\n"
    "SessionName=kept outside SessionSettings\r\n"
    "\r\n"
    "[SessionSettings]\r\n"
    "SessionName=ARK #230446\r\n"
    "\r\n"
    "[/Script/Engine.GameSession]\r\n"
    "MaxPlayers=70\r\n"
)


def test_strip_session_name_removes_only_the_session_settings_name(gen) -> None:
    stripped = gen.strip_session_name(_COMPLETE)
    assert "SessionName=ARK #230446" not in stripped
    assert "SessionName=kept outside SessionSettings\r\n" in stripped
    assert "[SessionSettings]\r\n\r\n[/Script/Engine.GameSession]\r\n" in stripped
    assert stripped == _COMPLETE.replace("SessionName=ARK #230446\r\n", "")


def test_strip_session_name_leaves_files_without_it_unchanged(gen) -> None:
    assert gen.strip_session_name(_PARTIAL) == _PARTIAL


def test_is_complete_tells_the_shutdown_file_from_the_partial_one(gen) -> None:
    assert gen.is_complete(_COMPLETE) is True
    assert gen.is_complete(_PARTIAL) is False
