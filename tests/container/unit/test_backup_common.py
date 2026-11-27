from __future__ import annotations

import importlib
import subprocess
import sys
import tempfile
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_backup_common():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("ark_backup.common")


@pytest.fixture(name="common_mod")
def common_mod_fixture():
    return _load_backup_common()


# ---------------------------------------------------------------------------
# archive_args
# ---------------------------------------------------------------------------


def test_archive_args_produces_plain_tar_command(common_mod) -> None:
    """archive_args returns a plain tar -cf command at low priority for a .tar output path."""
    output = Path("/home/ark-user/maintenance/backup.tar")
    args = common_mod.archive_args(output)
    assert args[:7] == ["nice", "-n", "19", "ionice", "-c", "3", "tar"]
    assert "-cf" in args
    assert str(output) in args


def test_archive_args_produces_gzip_tar_command(common_mod) -> None:
    """archive_args returns a tar -czf command for a .tar.gz output path."""
    output = Path("/home/ark-user/maintenance/backup.tar.gz")
    args = common_mod.archive_args(output)
    assert "-czf" in args
    assert str(output) in args


def test_archive_args_produces_zstd_tar_command(common_mod) -> None:
    """archive_args compresses a .tar.zst output with zstd level 9."""
    output = Path("/home/ark-user/maintenance/backup.tar.zst")
    args = common_mod.archive_args(output)
    assert args[args.index("-I") + 1] == "zstd -9"
    assert str(output) in args


def test_archive_args_raises_for_unsupported_extension(common_mod) -> None:
    """archive_args raises ValueError when the output path has an unsupported extension."""
    with tempfile.TemporaryDirectory() as tmp, pytest.raises(ValueError, match="output must end with"):
        common_mod.archive_args(Path(tmp) / "backup.zip")


def test_archive_args_includes_save_dir_as_source(common_mod) -> None:
    """archive_args uses SAVED_DIR as the -C argument for all supported extensions."""
    with tempfile.TemporaryDirectory() as tmp:
        for ext in (".tar", ".tar.gz", ".tar.zst"):
            args = common_mod.archive_args(Path(tmp) / f"backup{ext}")
            assert "-C" in args
            # The source directory following -C must be a string form of SAVED_DIR.
            save_dir_idx = args.index("-C")
            assert "ShooterGame/Saved" in args[save_dir_idx + 1]


def test_archive_args_exact_gzip_command(common_mod) -> None:
    """archive_args keeps its exact instance command: excludes Logs and clusters, archives Saved."""
    output = Path("/home/ark-user/maintenance/backup.tar.gz")
    assert common_mod.archive_args(output) == [
        "nice",
        "-n",
        "19",
        "ionice",
        "-c",
        "3",
        "tar",
        "-czf",
        str(output),
        "--exclude=./Logs",
        "--exclude=./clusters",
        "-C",
        str(common_mod.SAVED_DIR),
        ".",
    ]


def test_tar_create_args_writes_to_dest_with_output_compression(common_mod) -> None:
    """tar_create_args picks compression from output's name but writes to dest."""
    output = Path("/out/cluster.tar.zst")
    dest = Path("/out/cluster.tar.zst.partial")
    args = common_mod.tar_create_args(output, Path("/data"), ["./skip"], dest=dest)
    assert args == [
        "nice",
        "-n",
        "19",
        "ionice",
        "-c",
        "3",
        "tar",
        "-I",
        "zstd -9",
        "-cf",
        str(dest),
        "--exclude=./skip",
        "-C",
        "/data",
        ".",
    ]


def _fake_completed(stdout: str = "", stderr: str = "", returncode: int = 0) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(args=[], returncode=returncode, stdout=stdout, stderr=stderr)


# ---------------------------------------------------------------------------
# combined_process_output
# ---------------------------------------------------------------------------


def test_combined_process_output_concatenates_stdout_and_stderr(common_mod) -> None:
    """combined_process_output concatenates stdout then stderr and strips surrounding whitespace."""
    result = _fake_completed(stdout="out text", stderr="err text")
    assert common_mod.combined_process_output(result) == "out texterr text"


def test_combined_process_output_strips_outer_whitespace(common_mod) -> None:
    """combined_process_output strips leading and trailing whitespace from the combined string."""
    result = _fake_completed(stdout="  hello  ", stderr="  world  ")
    assert common_mod.combined_process_output(result) == "hello    world"


def test_combined_process_output_returns_empty_for_no_output(common_mod) -> None:
    """combined_process_output returns an empty string when both stdout and stderr are empty."""
    result = _fake_completed(stdout="", stderr="")
    assert common_mod.combined_process_output(result) == ""


def test_combined_process_output_handles_only_stderr(common_mod) -> None:
    """combined_process_output returns the stderr content when stdout is empty."""
    result = _fake_completed(stdout="", stderr="error message")
    assert common_mod.combined_process_output(result) == "error message"


# ---------------------------------------------------------------------------
# process_failure_detail
# ---------------------------------------------------------------------------


def test_process_failure_detail_prefers_stderr(common_mod) -> None:
    """process_failure_detail returns stderr content when it is non-empty."""
    result = _fake_completed(stdout="stdout", stderr="stderr message")
    assert common_mod.process_failure_detail(result, "fallback") == "stderr message"


def test_process_failure_detail_falls_back_to_stdout(common_mod) -> None:
    """process_failure_detail returns stdout when stderr is empty."""
    result = _fake_completed(stdout="stdout message", stderr="")
    assert common_mod.process_failure_detail(result, "fallback") == "stdout message"


def test_process_failure_detail_returns_fallback_when_both_empty(common_mod) -> None:
    """process_failure_detail returns the fallback string when both stdout and stderr are empty."""
    result = _fake_completed(stdout="", stderr="")
    assert common_mod.process_failure_detail(result, "fallback message") == "fallback message"


def test_archive_args_excludes_logs_before_source(common_mod) -> None:
    """archive_args excludes Logs/ (written by the live server mid-archive) ahead of the source operand."""
    for name in ("backup.tar", "backup.tar.gz", "backup.tar.zst"):
        args = common_mod.archive_args(Path("/home/ark-user/maintenance") / name)
        assert args.index("--exclude=./Logs") < args.index(".")


def test_archive_args_excludes_shared_cluster_dir(common_mod) -> None:
    """Saved/clusters is a volume shared by every instance, so instance backups leave it out."""
    for name in ("backup.tar", "backup.tar.gz", "backup.tar.zst"):
        args = common_mod.archive_args(Path("/home/ark-user/maintenance") / name)
        assert args.index("--exclude=./clusters") < args.index(".")


# ---------------------------------------------------------------------------
# invalid settings
# ---------------------------------------------------------------------------


def test_backup_main_returns_2_for_invalid_setting(
    common_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    """A bad BACKUP_* integer is reported once and the backup exits 2 without archiving."""
    backup_main = importlib.import_module("ark_backup.main")
    monkeypatch.setattr(backup_main, "SAVED_DIR", tmp_path)
    monkeypatch.setattr(sys, "argv", ["ark-backup.py", "--output", str(tmp_path / "out/backup.tar")])
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)
    monkeypatch.setenv("BACKUP_VERIFY_MAX_RETRIES", "0")

    assert backup_main.main() == 2
    err = capsys.readouterr().err
    assert "BACKUP_VERIFY_MAX_RETRIES must be an integer of at least 1 (got '0')." in err
    assert "[backup] Error: result: FAILURE exit_code=2 reason=invalid configuration" in err
    assert not (tmp_path / "out/backup.tar").exists()
