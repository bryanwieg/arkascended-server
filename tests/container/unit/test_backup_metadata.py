from __future__ import annotations

import importlib
import json
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_metadata():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("ark_backup.metadata")


@pytest.fixture(name="meta_mod")
def meta_mod_fixture():
    return _load_metadata()


def test_build_metadata_dict_includes_all_fields(meta_mod, monkeypatch):
    monkeypatch.setenv("SESSION_NAME", "test-server")
    monkeypatch.setenv("MAP_NAME", "TheIsland_WP")
    monkeypatch.setenv("ASA_PORT", "27015")
    monkeypatch.setenv("TZ", "America/New_York")
    data = meta_mod.build_metadata_dict()
    assert data["instance_name"] == "test-server"
    assert data["map_name"] == "TheIsland_WP"
    assert data["environment"]["ASA_PORT"] == "27015"
    assert data["environment"]["TZ"] == "America/New_York"
    assert "server_version" in data
    assert "build_id" in data
    assert "backup_timestamp" in data


def test_write_and_remove_backup_metadata(meta_mod, monkeypatch, tmp_path):
    monkeypatch.setattr(meta_mod, "SAVED_DIR", tmp_path)
    monkeypatch.setenv("SESSION_NAME", "test-server")
    monkeypatch.setenv("MAP_NAME", "TheIsland")
    meta_mod.write_backup_metadata()
    metadata_path = tmp_path / "backup-metadata.json"
    assert metadata_path.is_file()
    data = json.loads(metadata_path.read_text())
    assert data["instance_name"] == "test-server"
    meta_mod.remove_backup_metadata()
    assert not metadata_path.exists()


def test_remove_backup_metadata_idempotent(meta_mod, monkeypatch, tmp_path):
    monkeypatch.setattr(meta_mod, "SAVED_DIR", tmp_path)
    meta_mod.remove_backup_metadata()
    meta_mod.remove_backup_metadata()


def test_read_file_safe_returns_empty_for_missing(meta_mod):
    result = meta_mod.read_file_safe(Path("/nonexistent/file"))
    assert result == ""


def test_build_metadata_dict_defaults_empty(meta_mod, monkeypatch):
    monkeypatch.delenv("SESSION_NAME", raising=False)
    monkeypatch.delenv("MAP_NAME", raising=False)
    monkeypatch.delenv("ASA_PORT", raising=False)
    monkeypatch.delenv("TZ", raising=False)
    data = meta_mod.build_metadata_dict()
    assert data["instance_name"] == ""
    assert data["map_name"] == ""
    assert data["environment"]["ASA_PORT"] == ""


def test_read_backup_metadata_from_archive(meta_mod, monkeypatch, tmp_path):
    monkeypatch.setenv("SESSION_NAME", "test-server")
    monkeypatch.setenv("MAP_NAME", "TheIsland")
    save_dir = tmp_path / "saved"
    save_dir.mkdir()
    monkeypatch.setattr(meta_mod, "SAVED_DIR", save_dir)
    meta_mod.write_backup_metadata()
    archive_path = tmp_path / "backup.tar.gz"
    import subprocess

    subprocess.run(["tar", "-czf", str(archive_path), "-C", str(save_dir), "."], check=True)
    meta_mod.remove_backup_metadata()
    result = meta_mod.read_backup_metadata(archive_path)
    assert result is not None
    assert result["instance_name"] == "test-server"
    assert result["map_name"] == "TheIsland"


def test_read_backup_metadata_returns_none_for_missing(meta_mod, tmp_path):
    empty_dir = tmp_path / "empty"
    empty_dir.mkdir()
    archive_path = tmp_path / "empty.tar"
    import subprocess

    subprocess.run(["tar", "-cf", str(archive_path), "-C", str(empty_dir), "."], check=True)
    result = meta_mod.read_backup_metadata(archive_path)
    assert result is None


def test_metadata_included_in_backup_archive(meta_mod, monkeypatch, tmp_path):
    save_dir = tmp_path / "saved"
    save_dir.mkdir()
    (save_dir / "SavedArks").mkdir()
    (save_dir / "SavedArks" / "test.ark").write_text("fake save data")
    monkeypatch.setattr(meta_mod, "SAVED_DIR", save_dir)
    monkeypatch.setenv("SESSION_NAME", "test-server")
    monkeypatch.setenv("MAP_NAME", "TheIsland_WP")

    meta_mod.write_backup_metadata()
    try:
        archive_path = tmp_path / "backup.tar.gz"
        import subprocess

        subprocess.run(
            ["tar", "-czf", str(archive_path), "-C", str(save_dir), "."],
            check=True,
        )
        result = subprocess.run(
            ["tar", "-tf", str(archive_path)],
            capture_output=True,
            text=True,
            check=True,
        )
        members = result.stdout.splitlines()
        assert any("backup-metadata.json" in m for m in members), (
            f"backup-metadata.json not found in archive; members: {members}"
        )
        metadata = meta_mod.read_backup_metadata(archive_path)
        assert metadata is not None
        assert metadata["instance_name"] == "test-server"
        assert metadata["map_name"] == "TheIsland_WP"
        assert "backup_timestamp" in metadata
    finally:
        meta_mod.remove_backup_metadata()
