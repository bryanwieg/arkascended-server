from __future__ import annotations

import importlib.util
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]


def _load_entrypoint_mod():
    module_path = Path(__file__).resolve().parents[3] / "container/scripts/runtime/container-entrypoint.py"
    spec = importlib.util.spec_from_file_location("container_entrypoint", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"Unable to load module from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture(name="ep_mod")
def ep_mod_fixture():
    return _load_entrypoint_mod()


# ---------------------------------------------------------------------------
# _read_cgroup_mem_reservation_bytes (patching module-level Path constants)
# ---------------------------------------------------------------------------


def test_read_cgroup_mem_reservation_bytes_returns_none_for_cgroup_v2_max(
    ep_mod,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_read_cgroup_mem_reservation_bytes returns None when cgroup v2 reports 'max' (unlimited)."""
    fake_v2 = tmp_path / "memory.low"
    fake_v2.write_text("max\n", encoding="utf-8")
    monkeypatch.setattr(ep_mod, "CGROUP_V2_MEM_LOW", fake_v2)
    monkeypatch.setattr(ep_mod, "CGROUP_V1_MEM_SOFT_LIMIT", tmp_path / "nonexistent")
    assert ep_mod._read_cgroup_mem_reservation_bytes() is None


def test_read_cgroup_mem_reservation_bytes_returns_value_from_cgroup_v2(
    ep_mod,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_read_cgroup_mem_reservation_bytes returns the integer from cgroup v2 when it's a number."""
    fake_v2 = tmp_path / "memory.low"
    fake_v2.write_text("8589934592\n", encoding="utf-8")  # 8 GB
    monkeypatch.setattr(ep_mod, "CGROUP_V2_MEM_LOW", fake_v2)
    monkeypatch.setattr(ep_mod, "CGROUP_V1_MEM_SOFT_LIMIT", tmp_path / "nonexistent")
    assert ep_mod._read_cgroup_mem_reservation_bytes() == 8589934592


def test_read_cgroup_mem_reservation_bytes_returns_none_for_cgroup_v1_sentinel(
    ep_mod,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_read_cgroup_mem_reservation_bytes returns None for cgroup v1 very large value (no reservation set)."""
    sentinel = 1 << 62  # larger than this → no reservation set
    fake_v1 = tmp_path / "memory.soft_limit_in_bytes"
    fake_v1.write_text(str(sentinel + 1) + "\n", encoding="utf-8")
    monkeypatch.setattr(ep_mod, "CGROUP_V2_MEM_LOW", tmp_path / "nonexistent")
    monkeypatch.setattr(ep_mod, "CGROUP_V1_MEM_SOFT_LIMIT", fake_v1)
    assert ep_mod._read_cgroup_mem_reservation_bytes() is None


def test_read_cgroup_mem_reservation_bytes_returns_value_from_cgroup_v1(
    ep_mod,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_read_cgroup_mem_reservation_bytes returns the integer from cgroup v1 for a valid reservation."""
    limit = 4 * 1024**3  # 4 GB
    fake_v1 = tmp_path / "memory.soft_limit_in_bytes"
    fake_v1.write_text(str(limit) + "\n", encoding="utf-8")
    monkeypatch.setattr(ep_mod, "CGROUP_V2_MEM_LOW", tmp_path / "nonexistent")
    monkeypatch.setattr(ep_mod, "CGROUP_V1_MEM_SOFT_LIMIT", fake_v1)
    assert ep_mod._read_cgroup_mem_reservation_bytes() == limit


def test_read_cgroup_mem_reservation_bytes_returns_none_when_neither_cgroup_exists(
    ep_mod,
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_read_cgroup_mem_reservation_bytes returns None when neither cgroup file is present."""
    monkeypatch.setattr(ep_mod, "CGROUP_V2_MEM_LOW", tmp_path / "nonexistent_v2")
    monkeypatch.setattr(ep_mod, "CGROUP_V1_MEM_SOFT_LIMIT", tmp_path / "nonexistent_v1")
    assert ep_mod._read_cgroup_mem_reservation_bytes() is None


# ---------------------------------------------------------------------------
# check_memory_headroom (monkeypatching the private helpers)
# ---------------------------------------------------------------------------


_GB = 1024**3


@pytest.mark.parametrize(
    ("skip", "reservation", "available"),
    [
        ("1", 32 * _GB, 4 * _GB),  # ARK_SKIP_RAM_CHECK=1
        (None, None, 4 * _GB),  # no reservation
        (None, 0, 4 * _GB),  # cgroup v2 default, or runtime didn't apply it
        (None, 8 * _GB, 16 * _GB),  # enough RAM
        (None, 8 * _GB, 8 * _GB),  # boundary: available == reservation
        (None, 8 * _GB, None),  # MemAvailable unreadable
    ],
)
def test_check_memory_headroom_is_silent(
    ep_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], skip, reservation, available
) -> None:
    """check_memory_headroom prints nothing when skipped, unset, unreadable or RAM suffices."""
    if skip:
        monkeypatch.setenv("ARK_SKIP_RAM_CHECK", skip)
    else:
        monkeypatch.delenv("ARK_SKIP_RAM_CHECK", raising=False)
    monkeypatch.setattr(ep_mod, "_read_cgroup_mem_reservation_bytes", lambda: reservation)
    meminfo = {} if available is None else {"MemAvailable": available}
    monkeypatch.setattr(ep_mod, "_read_meminfo_bytes", lambda: meminfo)
    monkeypatch.setattr(ep_mod, "_memory_pressure", lambda _m: "pressure")
    assert ep_mod.check_memory_headroom() is None
    assert capsys.readouterr().err == ""


@pytest.mark.parametrize(
    ("pressure", "expected"),
    [
        (None, "balloon-memory VM"),
        ("swap 95% full", "memory pressure (swap 95% full). The server will likely be killed"),
    ],
)
def test_check_memory_headroom_warns_when_low(
    ep_mod, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], pressure, expected
) -> None:
    """check_memory_headroom warns (never blocks) when RAM < reservation, worded by memory pressure."""
    monkeypatch.delenv("ARK_SKIP_RAM_CHECK", raising=False)
    monkeypatch.setattr(ep_mod, "_read_cgroup_mem_reservation_bytes", lambda: 32 * _GB)
    monkeypatch.setattr(ep_mod, "_read_meminfo_bytes", lambda: {"MemAvailable": 4 * _GB})
    monkeypatch.setattr(ep_mod, "_memory_pressure", lambda _m: pressure)
    assert ep_mod.check_memory_headroom() is None
    err = capsys.readouterr().err
    assert "Warning: low host RAM: need 32.0 GB" in err
    assert "4.0 GB available" in err
    assert expected in err


def test_read_meminfo_bytes_parses_kb_fields(ep_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """_read_meminfo_bytes converts kB to bytes, keeps unitless counts and returns {} when unreadable."""
    meminfo = tmp_path / "meminfo"
    monkeypatch.setattr(ep_mod, "MEMINFO", meminfo)
    assert ep_mod._read_meminfo_bytes() == {}
    meminfo.write_text("MemTotal:       16384 kB\nMemAvailable:    2048 kB\nHugePages_Total:       0\n")
    assert ep_mod._read_meminfo_bytes() == {"MemTotal": 16384 * 1024, "MemAvailable": 2048 * 1024, "HugePages_Total": 0}


def test_memory_pressure_reads_psi_then_swap(ep_mod, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """_memory_pressure reports PSI stalls first, then swap over 90% full, else None."""
    psi = tmp_path / "memory"
    monkeypatch.setattr(ep_mod, "PSI_MEMORY", psi)
    full_swap = {"SwapTotal": 100, "SwapFree": 5}
    assert ep_mod._memory_pressure({}) is None  # no PSI file, no swap
    assert ep_mod._memory_pressure(full_swap) == "swap 95% full"
    assert ep_mod._memory_pressure({"SwapTotal": 100, "SwapFree": 50}) is None
    psi.write_text("some avg10=1.00 avg60=0 avg300=0 total=1\nfull avg10=0.00 avg60=0 avg300=0 total=1\n")
    assert ep_mod._memory_pressure({}) is None  # "some" stalls alone don't count
    psi.write_text("some avg10=3.00 avg60=0 avg300=0 total=1\nfull avg10=2.50 avg60=0 avg300=0 total=1\n")
    assert ep_mod._memory_pressure(full_swap) == "memory stalls in the last 10 s, PSI full avg10=2.50"


def _patch_volumes(ep_mod, monkeypatch, tmp_path, mounted: list[str]):
    save = tmp_path / "Saved"
    cluster = save / "clusters"
    cluster.mkdir(parents=True)
    monkeypatch.setattr(ep_mod, "SAVED_DIR", save)
    monkeypatch.setattr(ep_mod, "CLUSTER_DIR", cluster)
    monkeypatch.setattr(ep_mod, "VOLUME_PATHS", [save, cluster])
    mountinfo = tmp_path / "mountinfo"
    lines = ["22 1 0:21 / / rw,relatime - overlay overlay rw"]
    lines += [f"{30 + i} 22 8:1 /vol{i} {p} rw,relatime - ext4 /dev/sda1 rw" for i, p in enumerate(mounted)]
    mountinfo.write_text("\n".join(lines) + "\n")
    monkeypatch.setattr(ep_mod, "MOUNTINFO", mountinfo)
    return save, cluster


def test_check_volume_mounts_passes_when_both_volumes_mounted(ep_mod, tmp_path, monkeypatch) -> None:
    _patch_volumes(ep_mod, monkeypatch, tmp_path, [str(tmp_path / "Saved"), str(tmp_path / "Saved" / "clusters")])
    assert ep_mod.check_volume_mounts() is True


def test_check_volume_mounts_refuses_without_save_volume(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    """The image ships Saved/, so it exists and is writable even with no volume; that must not pass."""
    _patch_volumes(ep_mod, monkeypatch, tmp_path, [])
    assert ep_mod.check_volume_mounts() is False
    assert "no volume is mounted" in capsys.readouterr().err


def test_check_volume_mounts_warns_without_cluster_volume(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    _patch_volumes(ep_mod, monkeypatch, tmp_path, [str(tmp_path / "Saved")])
    assert ep_mod.check_volume_mounts() is True
    assert "cluster transfers will be lost" in capsys.readouterr().err


def test_mount_points_unescapes_spaces(ep_mod, tmp_path, monkeypatch) -> None:
    mountinfo = tmp_path / "mountinfo"
    mountinfo.write_text("30 22 8:1 / /data/my\\040saves rw - ext4 /dev/sda1 rw\n")
    monkeypatch.setattr(ep_mod, "MOUNTINFO", mountinfo)
    assert "/data/my saves" in ep_mod._mount_points()


def test_apply_rcon_settings_updates_changed_admin_password(ep_mod, tmp_path, monkeypatch) -> None:
    """A changed SERVER_ADMIN_PASSWORD reaches the INI on the next boot (not only on first boot)."""
    config = tmp_path / "GameUserSettings.ini"
    config.write_text("[ServerSettings]\nServerAdminPassword=old\nMaxPlayers=70\n")
    monkeypatch.setattr(ep_mod, "GAME_USER_SETTINGS", config)
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", "new")
    monkeypatch.delenv("RCON_PORT", raising=False)
    monkeypatch.setenv("SERVER_PASSWORD", "")
    assert ep_mod.apply_rcon_settings() is True
    text = config.read_text()
    assert "ServerAdminPassword=new" in text and "ServerAdminPassword=old" not in text  # noqa: PT018
    assert "RCONPort=27020" in text and "MaxPlayers=70" in text  # noqa: PT018


def test_apply_rcon_settings_warns_but_continues_without_password(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    """No admin password: warn and keep booting, leaving the INI alone."""
    config = tmp_path / "GameUserSettings.ini"
    monkeypatch.setattr(ep_mod, "GAME_USER_SETTINGS", config)
    monkeypatch.delenv("SERVER_ADMIN_PASSWORD", raising=False)
    assert ep_mod.apply_rcon_settings() is True
    assert not config.exists()
    assert "SERVER_ADMIN_PASSWORD is not set" in capsys.readouterr().err


@pytest.mark.parametrize("command", [["cat", "/home/ark-user/ARK_SERVER_VERSION"], ["/x/ark-restore.py"]])
def test_command_override_writes_nothing_to_stdout(ep_mod, monkeypatch, capsys, command: list[str]) -> None:
    """A passed command's stdout must be its own: build-image.sh reads `docker run <image> cat …` output."""
    executed: list[list[str]] = []

    def fake_execvp(file: str, args: list[str]) -> None:
        executed.append(args)
        raise SystemExit(0)  # stands in for the process being replaced

    monkeypatch.setattr(ep_mod, "RESTORE_SCRIPT", "/x/ark-restore.py")
    monkeypatch.setattr(ep_mod.os, "execvp", fake_execvp)
    monkeypatch.setattr(ep_mod.sys, "argv", ["container-entrypoint.py", *command])
    monkeypatch.setenv("ARK_MAINTENANCE_MODE", "")  # registers the original state so monkeypatch restores it
    monkeypatch.delenv("ARK_MAINTENANCE_MODE")

    with pytest.raises(SystemExit):
        ep_mod.main()

    assert executed == [command]
    assert capsys.readouterr().out == ""
    assert (ep_mod.os.environ.get("ARK_MAINTENANCE_MODE") == "restore") is (command == ["/x/ark-restore.py"])


def _patch_settings(ep_mod, monkeypatch, tmp_path: Path) -> tuple[Path, Path]:
    default = tmp_path / "config-defaults/GameUserSettings.ini"
    default.parent.mkdir()
    default.write_text("[ServerSettings]\r\nAutoSavePeriodMinutes=15\r\n")
    config = tmp_path / "Saved/Config/WindowsServer/GameUserSettings.ini"
    monkeypatch.setattr(ep_mod, "DEFAULT_GAME_USER_SETTINGS", default)
    monkeypatch.setattr(ep_mod, "GAME_USER_SETTINGS", config)
    monkeypatch.setattr("_logging._mirror_to_pid1", lambda *_a, **_k: None)
    return default, config


def test_install_default_settings_copies_the_default_when_missing(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    """A volume without GameUserSettings.ini (new, bind-mounted or restored) gets the image's default."""
    default, config = _patch_settings(ep_mod, monkeypatch, tmp_path)
    assert ep_mod.install_default_settings() is True
    assert config.read_bytes() == default.read_bytes()
    # No leftover temp file; the default is the history's first commit, as ARK's (it generated it).
    assert sorted(p.name for p in config.parent.iterdir()) == [".git", "GameUserSettings.ini"]
    assert _log(config.parent) == ["ARK server: image default installed (generated by ARK at image build)"]
    assert "[entrypoint] no GameUserSettings.ini found; installed the image's default." in capsys.readouterr().out


def test_install_default_settings_leaves_an_existing_file_alone(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    _, config = _patch_settings(ep_mod, monkeypatch, tmp_path)
    config.parent.mkdir(parents=True)
    config.write_text("[ServerSettings]\nMaxPlayers=10\n")
    assert ep_mod.install_default_settings() is True
    assert config.read_text() == "[ServerSettings]\nMaxPlayers=10\n"
    assert capsys.readouterr().out == ""


def test_install_default_settings_fails_when_the_copy_fails(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    default, _ = _patch_settings(ep_mod, monkeypatch, tmp_path)
    default.unlink()
    assert ep_mod.install_default_settings() is False
    assert "[entrypoint] Error: failed to install the default GameUserSettings.ini:" in capsys.readouterr().err


def test_default_startup_records_files_as_found_then_env_settings(ep_mod, tmp_path, monkeypatch) -> None:
    """Boot commits the files as found (origin unknown on a volume without history), then the RCON settings."""
    _default, config = _patch_settings(ep_mod, monkeypatch, tmp_path)
    config.parent.mkdir(parents=True)
    config.write_text("[ServerSettings]\nMaxPlayers=70\n")
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", "AdminPass123")
    for name in ("check_memory_headroom", "check_host_kernel_params"):
        monkeypatch.setattr(ep_mod, name, lambda: None)
    monkeypatch.setattr(ep_mod, "check_volume_mounts", lambda: True)

    def no_exec(*_a):
        raise RuntimeError("exec")

    monkeypatch.setattr(ep_mod.os, "execv", no_exec)
    with pytest.raises(RuntimeError, match="exec"):
        ep_mod.run_default_startup()

    assert _log(config.parent) == [
        "instance env: RCON and password settings from the environment",
        "unknown origin: files as first seen; origin unknown",
    ]
    import ini_history

    first = ini_history._git(config.parent, "show", "HEAD~1:GameUserSettings.ini")
    assert first == "[ServerSettings]\nMaxPlayers=70\n"  # as found, before the RCON rewrite

    # Next boot: a change made while stopped is recorded as such before anything else.
    config.write_text(config.read_text() + "XPMultiplier=2\n")
    with pytest.raises(RuntimeError, match="exec"):
        ep_mod.run_default_startup()
    # (The RCON rewrite then moves its keys back to the end of the section: an env commit on top.)
    assert _log(config.parent)[:2] == [
        "instance env: RCON and password settings from the environment",
        "outside ark-cli: changed while the instance was stopped",
    ]


def _log(config_dir: Path) -> list[str]:
    import ini_history

    return ini_history._git(config_dir, "log", "--format=%an: %s").splitlines()


def test_history_failure_only_warns_and_boot_goes_on(ep_mod, tmp_path, monkeypatch, capsys) -> None:
    """A broken git (missing binary, full disk) must never stop the server from starting."""
    _default, config = _patch_settings(ep_mod, monkeypatch, tmp_path)
    monkeypatch.setenv("SERVER_ADMIN_PASSWORD", "AdminPass123")
    monkeypatch.setenv("PATH", str(tmp_path / "no-bin"))  # no git
    for name in ("check_memory_headroom", "check_host_kernel_params"):
        monkeypatch.setattr(ep_mod, name, lambda: None)
    monkeypatch.setattr(ep_mod, "check_volume_mounts", lambda: True)

    def exec_server(*_a):
        raise RuntimeError("exec")

    monkeypatch.setattr(ep_mod.os, "execv", exec_server)
    with pytest.raises(RuntimeError, match="exec"):
        ep_mod.run_default_startup()
    assert "ServerAdminPassword=AdminPass123" in config.read_text()
    assert "[entrypoint] Warning: could not record the INI files' history: git" in capsys.readouterr().err
