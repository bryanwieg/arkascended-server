from __future__ import annotations

import importlib
import sys
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_archive_validation():
    runtime_dir = Path(__file__).resolve().parents[3] / "container/scripts/runtime"
    runtime_path = str(runtime_dir)
    if runtime_path not in sys.path:
        sys.path.insert(0, runtime_path)
    return importlib.import_module("ark_backup.archive_validation")


@pytest.fixture(name="av_mod")
def av_mod_fixture():
    return _load_archive_validation()


# ---------------------------------------------------------------------------
# normalize_member
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("./SavedArks/save.ark", "SavedArks/save.ark"),
        ("SavedArks/save.ark", "SavedArks/save.ark"),
        ("./", ""),
        ("Logs/ShooterGame.log", "Logs/ShooterGame.log"),
    ],
)
def test_normalize_member_strips_leading_dotslash(av_mod, raw: str, expected: str) -> None:
    """normalize_member removes a leading './' prefix and returns other paths unchanged."""
    assert av_mod.normalize_member(raw) == expected


# ---------------------------------------------------------------------------
# archive_members_from_tar_list
# ---------------------------------------------------------------------------


def test_archive_members_from_tar_list_parses_standard_output(av_mod) -> None:
    """archive_members_from_tar_list parses tar -tf output into ArchiveMember namedtuples."""
    tar_output = (
        "./SavedArks/TheIsland_WP/save.ark\n./Logs/ShooterGame.log\n./Config/WindowsServer/GameUserSettings.ini\n"
    )
    members = av_mod.archive_members_from_tar_list(tar_output)
    assert len(members) == 3
    normalized = {m.normalized for m in members}
    assert "SavedArks/TheIsland_WP/save.ark" in normalized
    assert "Logs/ShooterGame.log" in normalized
    assert "Config/WindowsServer/GameUserSettings.ini" in normalized


def test_archive_members_from_tar_list_skips_blank_lines(av_mod) -> None:
    """archive_members_from_tar_list ignores blank lines in tar output."""
    tar_output = "\n./SavedArks/save.ark\n\n"
    members = av_mod.archive_members_from_tar_list(tar_output)
    assert len(members) == 1


def test_archive_members_from_tar_list_preserves_raw_name(av_mod) -> None:
    """archive_members_from_tar_list stores the original raw name alongside the normalized name."""
    tar_output = "./SavedArks/save.ark\n"
    members = av_mod.archive_members_from_tar_list(tar_output)
    assert members[0].raw == "./SavedArks/save.ark"
    assert members[0].normalized == "SavedArks/save.ark"


def test_archive_members_from_tar_list_returns_empty_for_empty_output(av_mod) -> None:
    """archive_members_from_tar_list returns an empty list for empty tar output."""
    assert av_mod.archive_members_from_tar_list("") == []


# ---------------------------------------------------------------------------
# archive_has_savedarks_content
# ---------------------------------------------------------------------------


def test_archive_has_savedarks_content_returns_true_when_present(av_mod) -> None:
    """archive_has_savedarks_content returns True when at least one member is under SavedArks/."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        ArchiveMember(raw="./SavedArks/save.ark", normalized="SavedArks/save.ark"),
        ArchiveMember(raw="./Logs/ShooterGame.log", normalized="Logs/ShooterGame.log"),
    ]
    assert av_mod.archive_has_savedarks_content(members) is True


def test_archive_has_savedarks_content_returns_false_when_absent(av_mod) -> None:
    """archive_has_savedarks_content returns False when no member is under SavedArks/."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        ArchiveMember(raw="./Logs/ShooterGame.log", normalized="Logs/ShooterGame.log"),
        ArchiveMember(raw="./Config/GameUserSettings.ini", normalized="Config/GameUserSettings.ini"),
    ]
    assert av_mod.archive_has_savedarks_content(members) is False


def test_archive_has_savedarks_content_returns_false_for_empty_list(av_mod) -> None:
    """archive_has_savedarks_content returns False for an empty member list."""
    assert av_mod.archive_has_savedarks_content([]) is False


def test_archive_has_savedarks_content_requires_exact_prefix(av_mod) -> None:
    """archive_has_savedarks_content requires the 'SavedArks/' prefix, not just substring."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        # 'savedarks' lowercase is not a match (prefix is case-sensitive).
        ArchiveMember(raw="./savedarks/save.ark", normalized="savedarks/save.ark"),
    ]
    assert av_mod.archive_has_savedarks_content(members) is False


# ---------------------------------------------------------------------------
# sqlite_candidates (backup version — same logic as restore but in backup module)
# ---------------------------------------------------------------------------


def test_sqlite_candidates_returns_ark_files_under_savedarks(av_mod) -> None:
    """sqlite_candidates includes .ark files under SavedArks/ and excludes all others."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        ArchiveMember(raw="./SavedArks/save.ark", normalized="SavedArks/save.ark"),
        ArchiveMember(raw="./Logs/ShooterGame.log", normalized="Logs/ShooterGame.log"),
        ArchiveMember(raw="./Config/GameUserSettings.ini", normalized="Config/GameUserSettings.ini"),
    ]
    candidates = av_mod.sqlite_candidates(members)
    assert len(candidates) == 1
    assert candidates[0].normalized == "SavedArks/save.ark"


def test_sqlite_candidates_includes_sqlite_and_db_under_savedarks(av_mod) -> None:
    """sqlite_candidates includes .sqlite and .db files under SavedArks/."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        ArchiveMember(raw="SavedArks/world.sqlite", normalized="SavedArks/world.sqlite"),
        ArchiveMember(raw="SavedArks/sidecar.db", normalized="SavedArks/sidecar.db"),
        ArchiveMember(raw="SavedArks/readme.txt", normalized="SavedArks/readme.txt"),
    ]
    candidates = av_mod.sqlite_candidates(members)
    normalized = {c.normalized for c in candidates}
    assert "SavedArks/world.sqlite" in normalized
    assert "SavedArks/sidecar.db" in normalized
    assert "SavedArks/readme.txt" not in normalized


def test_sqlite_candidates_returns_sorted_results(av_mod) -> None:
    """sqlite_candidates returns candidates in sorted order by normalized name."""
    ArchiveMember = av_mod.ArchiveMember  # noqa: N806
    members = [
        ArchiveMember(raw="SavedArks/Z.ark", normalized="SavedArks/Z.ark"),
        ArchiveMember(raw="SavedArks/A.ark", normalized="SavedArks/A.ark"),
    ]
    candidates = av_mod.sqlite_candidates(members)
    assert candidates[0].normalized == "SavedArks/A.ark"
    assert candidates[1].normalized == "SavedArks/Z.ark"
