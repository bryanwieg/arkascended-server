from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_restore_mod():
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/ark-restore.py"
    spec = importlib.util.spec_from_file_location("ark_restore", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load module from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(name="restore_mod")
def restore_mod_fixture():
    return _load_restore_mod()


# ---------------------------------------------------------------------------
# is_unsafe_path
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "path",
    [
        "/etc/passwd",  # absolute
        "../escape",  # leading traversal
        "safe/../escape",  # mid-path traversal
        "..",  # exactly ".."
        "dir/..",  # ends with /..,
        "a/b/../..",  # double traversal ending
    ],
)
def test_is_unsafe_path_rejects_dangerous_paths(restore_mod, path: str) -> None:
    """is_unsafe_path returns True for paths that could escape the target directory."""
    assert restore_mod.is_unsafe_path(path) is True


@pytest.mark.parametrize(
    "path",
    [
        "SavedArks/TheIsland_WP/TheIsland_WP.ark",
        "./SavedArks/bootstrap.arkprofile",
        "Config/WindowsServer/GameUserSettings.ini",
        "Logs/ShooterGame.log",
        ".",
        "./",
        "plain-file.txt",
    ],
)
def test_is_unsafe_path_allows_safe_paths(restore_mod, path: str) -> None:
    """is_unsafe_path returns False for relative paths that stay within the extract root."""
    assert restore_mod.is_unsafe_path(path) is False


# ---------------------------------------------------------------------------
# swap_blocker / swap_in
# ---------------------------------------------------------------------------


def _saved(tmp_path: Path) -> Path:
    save_dir = tmp_path / "Saved"
    (save_dir / "SavedArks").mkdir(parents=True)
    (save_dir / "SavedArks/old.ark").write_text("old world")
    (save_dir / "Config").mkdir()
    (save_dir / "clusters").mkdir()
    (save_dir / "clusters/transfer").write_text("shared")
    return save_dir


def test_swap_in_replaces_everything_but_clusters(restore_mod, tmp_path: Path) -> None:
    """The staged saves replace the old ones; clusters/ and its contents are left alone."""
    save_dir = _saved(tmp_path)
    staging = save_dir / restore_mod.STAGING_NAME
    (staging / "SavedArks").mkdir(parents=True)
    (staging / "SavedArks/new.ark").write_text("restored world")

    restore_mod.swap_in(save_dir, staging)

    assert sorted(p.name for p in save_dir.iterdir()) == ["SavedArks", "clusters"]
    assert [p.name for p in (save_dir / "SavedArks").iterdir()] == ["new.ark"]
    assert (save_dir / "clusters/transfer").read_text() == "shared"


def test_swap_blocker_allows_a_normal_save_dir(restore_mod, tmp_path: Path) -> None:
    assert restore_mod.swap_blocker(_saved(tmp_path)) is None


def test_swap_blocker_refuses_after_an_interrupted_swap(restore_mod, tmp_path: Path) -> None:
    """A leftover .restore-previous may hold the only copy of the earlier saves, so restore stops."""
    save_dir = _saved(tmp_path)
    (save_dir / restore_mod.PREVIOUS_NAME).mkdir()
    assert "interrupted" in restore_mod.swap_blocker(save_dir)


def test_swap_blocker_refuses_mounted_subdirectories(restore_mod, tmp_path: Path, monkeypatch) -> None:
    """A mount point can't be renamed; clusters/ (always a mount) is the only one allowed."""
    save_dir = _saved(tmp_path)
    monkeypatch.setattr(restore_mod.os.path, "ismount", lambda path: Path(path).name in {"clusters", "Config"})
    assert restore_mod.swap_blocker(save_dir) == f"cannot swap mounted directories under {save_dir}: Config"


def test_failed_extraction_leaves_the_saves_unchanged(restore_mod, tmp_path: Path, monkeypatch, capsys) -> None:
    """tar failing partway (disk full, I/O error) must not touch the live saves; the staging copy is removed."""
    save_dir = _saved(tmp_path)
    archive = tmp_path / "backup.tar.gz"
    archive.write_bytes(b"validated elsewhere")
    monkeypatch.setattr(restore_mod, "SAVED_DIR", save_dir)
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)
    monkeypatch.setenv("ARK_MAINTENANCE_MODE", "restore")
    monkeypatch.setattr(restore_mod, "ark_server_running", lambda: False)
    monkeypatch.setattr(restore_mod, "list_archive_entries", lambda _a: ["./SavedArks/new.ark"])
    monkeypatch.setattr(restore_mod, "validate_archive_contents", lambda _a: True)
    monkeypatch.setattr(restore_mod, "read_backup_metadata", lambda _a: None)
    monkeypatch.setattr(
        restore_mod.sys,
        "argv",
        ["ark-restore.py", "--archive", str(archive), "--pre-restore", str(tmp_path / "pre/pre-restore.tar.gz")],
    )

    class _Result:
        def __init__(self, returncode: int) -> None:
            self.returncode = returncode

    def fake_tar(cmd: list[str], **_kwargs) -> _Result:
        if "-xf" not in cmd:
            return _Result(0)  # the pre-restore backup
        staged = Path(cmd[cmd.index("-C") + 1])
        (staged / "SavedArks").mkdir()
        (staged / "SavedArks/new.ark").write_text("half written")
        return _Result(2)

    monkeypatch.setattr(restore_mod.subprocess, "run", fake_tar)

    assert restore_mod.main() == 2
    assert [p.name for p in (save_dir / "SavedArks").iterdir()] == ["old.ark"]
    assert not (save_dir / restore_mod.STAGING_NAME).exists()
    assert "the existing saves are unchanged" in capsys.readouterr().err
