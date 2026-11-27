from __future__ import annotations

import importlib.util
import subprocess
import sys
import tarfile
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_cluster_backup_mod():
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/ark-cluster-backup.py"
    spec = importlib.util.spec_from_file_location("ark_cluster_backup", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load module from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(name="mod")
def mod_fixture(tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    module = _load_cluster_backup_mod()
    cluster = tmp_path / "clusters"
    cluster.mkdir()
    monkeypatch.setattr(module, "CLUSTER_DIR", cluster)
    monkeypatch.setattr(module, "RETRY_DELAY_SECONDS", 0)
    return module


def _run(mod, monkeypatch: pytest.MonkeyPatch, output: Path) -> int:
    monkeypatch.setattr(sys, "argv", ["ark-cluster-backup.py", "--output", str(output)])
    return mod.main()


def _members(archive: Path) -> set[str]:
    with tarfile.open(archive) as tar:
        return {name.removeprefix("./") for name in tar.getnames()}


def test_backup_archives_cluster_files(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A cluster with files produces a complete archive and no .partial file."""
    (mod.CLUSTER_DIR / "cid").mkdir()
    (mod.CLUSTER_DIR / "cid/player1").write_text("v1")
    output = tmp_path / "out/cluster-backup.tar.gz"
    assert _run(mod, monkeypatch, output) == 0
    assert "cid/player1" in _members(output)
    assert not output.with_name(output.name + ".partial").exists()


def test_backup_of_empty_cluster_succeeds(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """An empty cluster directory is a valid backup."""
    output = tmp_path / "cluster-backup.tar"
    assert _run(mod, monkeypatch, output) == 0
    assert output.is_file()


def test_backup_skips_restore_work_dirs(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Leftover restore staging/previous directories are not archived."""
    (mod.CLUSTER_DIR / ".restore-staging").mkdir()
    (mod.CLUSTER_DIR / ".restore-staging/leftover").write_text("x")
    (mod.CLUSTER_DIR / "keep").write_text("k")
    output = tmp_path / "cluster-backup.tar.gz"
    assert _run(mod, monkeypatch, output) == 0
    members = _members(output)
    assert "keep" in members
    assert not any(".restore-staging" in name for name in members)


def _fake_tar_create(monkeypatch: pytest.MonkeyPatch, mod, codes: list[int]) -> list[int]:
    """Make the archive-creating tar call return the given codes in turn; listing calls run for real."""
    real_run = subprocess.run
    calls: list[int] = []

    def fake_run(cmd, **kwargs):
        if "-tf" in cmd:
            return real_run(cmd, **kwargs)
        code = codes[len(calls)]
        calls.append(code)
        if code == 0:
            return real_run(cmd, **kwargs)
        Path(cmd[cmd.index("-czf") + 1]).write_text("half")
        return subprocess.CompletedProcess(cmd, code, "", "file changed as we read it")

    monkeypatch.setattr(mod.subprocess, "run", fake_run)
    return calls


def test_backup_fails_when_files_keep_changing(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """Three 'file changed' exits fail the backup and leave nothing behind."""
    calls = _fake_tar_create(monkeypatch, mod, [1, 1, 1])
    output = tmp_path / "cluster-backup.tar.gz"
    assert _run(mod, monkeypatch, output) == 1
    assert len(calls) == 3
    assert list(tmp_path.iterdir()) == [mod.CLUSTER_DIR]


def test_backup_retries_after_a_change(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """One 'file changed' exit is retried and the second attempt succeeds."""
    calls = _fake_tar_create(monkeypatch, mod, [1, 0])
    output = tmp_path / "cluster-backup.tar.gz"
    assert _run(mod, monkeypatch, output) == 0
    assert calls == [1, 0]
    assert output.is_file()


def test_backup_does_not_retry_other_tar_errors(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A fatal tar exit (2) fails immediately without retrying."""
    calls = _fake_tar_create(monkeypatch, mod, [2])
    assert _run(mod, monkeypatch, tmp_path / "cluster-backup.tar.gz") == 1
    assert calls == [2]


def test_backup_fails_without_cluster_dir(mod, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A missing cluster directory is an error."""
    monkeypatch.setattr(mod, "CLUSTER_DIR", tmp_path / "missing")
    assert _run(mod, monkeypatch, tmp_path / "cluster-backup.tar.gz") == 1
