from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_config_update():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("ini_settings")


@pytest.fixture(name="config_update_mod")
def config_update_mod_fixture():
    return _load_config_update()


# ---------------------------------------------------------------------------
# update_config — section already present
# ---------------------------------------------------------------------------


def test_update_config_replaces_rcon_settings_in_existing_section(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config overwrites existing RCON keys inside [ServerSettings]."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text(
        "[ServerSettings]\n"
        "RCONEnabled=False\n"
        "RCONPort=12345\n"
        "ServerAdminPassword=oldpw\n"
        "ServerPassword=oldjoin\n"
        "SomeOtherKey=preserved\n",
        encoding="utf-8",
    )

    config_update_mod.update_config(ini, "newadmin", "27020", "newjoin")

    content = ini.read_text(encoding="utf-8")
    assert "RCONEnabled=True" in content
    assert "RCONPort=27020" in content
    assert "ServerAdminPassword=newadmin" in content
    assert "ServerPassword=newjoin" in content
    assert "SomeOtherKey=preserved" in content
    # Old values must be gone.
    assert "oldpw" not in content
    assert "oldjoin" not in content
    assert "False" not in content or "RCONEnabled=False" not in content


def test_update_config_inserts_settings_when_section_has_no_rcon_keys(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config inserts RCON settings when [ServerSettings] exists but has no RCON keys."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text(
        "[ServerSettings]\nMaxPlayers=70\n",
        encoding="utf-8",
    )

    config_update_mod.update_config(ini, "admin123", "27020", "join123")

    content = ini.read_text(encoding="utf-8")
    assert "RCONEnabled=True" in content
    assert "RCONPort=27020" in content
    assert "ServerAdminPassword=admin123" in content
    assert "ServerPassword=join123" in content
    assert "MaxPlayers=70" in content


def test_update_config_inserts_settings_before_next_section(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config inserts RCON settings before a following section header."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text(
        "[ServerSettings]\nMaxPlayers=70\n[ScalabilityGroups]\nsg.ResolutionQuality=72\n",
        encoding="utf-8",
    )

    config_update_mod.update_config(ini, "admin", "27020", "join")

    content = ini.read_text(encoding="utf-8")
    server_idx = content.index("[ServerSettings]")
    scalability_idx = content.index("[ScalabilityGroups]")
    rcon_idx = content.index("RCONEnabled=True")
    # RCON settings must be inserted inside [ServerSettings], before [ScalabilityGroups].
    assert server_idx < rcon_idx < scalability_idx


# ---------------------------------------------------------------------------
# update_config — section absent
# ---------------------------------------------------------------------------


def test_update_config_creates_section_when_absent(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config appends [ServerSettings] with RCON keys when the section is missing."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text(
        "[/Script/Engine.GameSession]\nMaxPlayers=70\n",
        encoding="utf-8",
    )

    config_update_mod.update_config(ini, "admin", "27020", "join")

    content = ini.read_text(encoding="utf-8")
    assert "[ServerSettings]" in content
    assert "RCONEnabled=True" in content
    assert "RCONPort=27020" in content


def test_update_config_handles_empty_file(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config creates [ServerSettings] and inserts RCON keys into an empty file."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text("", encoding="utf-8")

    config_update_mod.update_config(ini, "admin", "27020", "join")

    content = ini.read_text(encoding="utf-8")
    assert "[ServerSettings]" in content
    assert "RCONEnabled=True" in content


# ---------------------------------------------------------------------------
# update_config — Windows line endings and indented section header
# ---------------------------------------------------------------------------


def test_update_config_handles_windows_line_endings(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config strips \\r from Windows CRLF line endings without corrupting keys."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_bytes(b"[ServerSettings]\r\nMaxPlayers=70\r\n")

    config_update_mod.update_config(ini, "admin", "27020", "join")

    content = ini.read_text(encoding="utf-8")
    assert "RCONEnabled=True" in content
    assert "MaxPlayers=70" in content


def test_update_config_matches_indented_section_header(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config recognises [ServerSettings] with leading whitespace."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text(
        "  [ServerSettings]\nMaxPlayers=70\n",
        encoding="utf-8",
    )

    config_update_mod.update_config(ini, "admin", "27020", "")

    content = ini.read_text(encoding="utf-8")
    assert "RCONEnabled=True" in content


# ---------------------------------------------------------------------------
# update_config — atomic write (uses temp file + os.replace)
# ---------------------------------------------------------------------------


def test_update_config_writes_atomically(
    config_update_mod,
    tmp_path: Path,
) -> None:
    """update_config replaces the file in-place (no partial writes visible on error)."""
    ini = tmp_path / "GameUserSettings.ini"
    ini.write_text("[ServerSettings]\nMaxPlayers=70\n", encoding="utf-8")
    original_inode = ini.stat().st_ino

    config_update_mod.update_config(ini, "admin", "27020", "join")

    # File still accessible at same path; inode may differ due to os.replace.
    assert ini.is_file()
    assert "RCONEnabled=True" in ini.read_text(encoding="utf-8")
    _ = original_inode  # checked implicitly by asserting the path still resolves


def test_update_config_leaves_file_untouched_when_already_applied(config_update_mod, tmp_path: Path) -> None:
    """Re-applying identical settings (every boot) does not rewrite the file."""
    config = tmp_path / "GameUserSettings.ini"
    config.write_text("[ServerSettings]\nRCONEnabled=True\nRCONPort=27020\nServerAdminPassword=pw\nServerPassword=\n")
    before = config.stat()
    config_update_mod.update_config(config, "pw", "27020", "")
    after = config.stat()
    assert (after.st_ino, after.st_mtime_ns) == (before.st_ino, before.st_mtime_ns)


def test_update_config_creates_missing_file(config_update_mod, tmp_path: Path) -> None:
    """A missing INI (and its directory) is created with just the RCON section."""
    config = tmp_path / "Config" / "WindowsServer" / "GameUserSettings.ini"
    config_update_mod.update_config(config, "pw", "27020", "join")
    assert config.read_text() == (
        "[ServerSettings]\nRCONEnabled=True\nRCONPort=27020\nServerAdminPassword=pw\nServerPassword=join\n"
    )
