"""ark-server-install.sh and download.sh against a fake steamcmd.

The fake replays one scripted run per call (exit code, output, build ID written to the
app manifest, depot manifest written to Steam's depotcache) and records each call's
arguments and the depot manifests it found, so tests see what SteamCMD was given.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from pathlib import Path

import pytest

pytestmark = [pytest.mark.unit]

REPO_ROOT = Path(__file__).resolve().parents[3]
INSTALL = REPO_ROOT / "container/files/scripts/installer/ark-server-install.sh"
DOWNLOAD = REPO_ROOT / "container/files/download.sh"
MANIFEST = "steamapps/appmanifest_2430930.acf"

FAKE_STEAMCMD = r"""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
state = Path(os.environ["FAKE_STEAMCMD_STATE"])
runs = json.loads(state.read_text())
run = runs.pop(0)
state.write_text(json.dumps(runs))
depots = Path(os.environ["HOME"]) / ".local/share/Steam/depotcache"
seen = sorted(p.name for p in depots.iterdir()) if depots.is_dir() else []
install_dir = Path(sys.argv[sys.argv.index("+force_install_dir") + 1])
manifest = (install_dir / "steamapps/appmanifest_2430930.acf").is_file()
with open(os.environ["FAKE_STEAMCMD_CALLS"], "a") as calls:
    calls.write(json.dumps({"args": sys.argv[1:], "depots": seen, "manifest": manifest}) + "\n")
if run.get("depot"):
    depots.mkdir(parents=True, exist_ok=True)
    (depots / run["depot"]).write_text("manifest")
if run.get("build_id"):
    (install_dir / "steamapps").mkdir(parents=True, exist_ok=True)
    (install_dir / "steamapps/appmanifest_2430930.acf").write_text(
        '"AppState"\n{\n\t"buildid"\t\t"%s"\n}\n' % run["build_id"])
print(run.get("output", ""))
sys.exit(run.get("exit", 0))
"""


def run_install(
    tmp_path: Path, runs: list[dict], script: Path = INSTALL, args: tuple[str, ...] = (), **env: str
) -> tuple[subprocess.CompletedProcess, list[dict]]:
    """Run script with a fake steamcmd that replays runs; return the result and steamcmd's calls."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir(exist_ok=True)
    fake = bin_dir / "steamcmd"
    fake.write_text(FAKE_STEAMCMD)
    fake.chmod(0o755)
    (tmp_path / "runs.json").write_text(json.dumps(runs))
    calls = tmp_path / "calls.jsonl"
    calls.unlink(missing_ok=True)
    full_env = {
        **os.environ,
        "PATH": f"{bin_dir}:{os.environ['PATH']}",
        "TMPDIR": str(tmp_path),
        "HOME": str(tmp_path / "home"),
        "ARK_INSTALL_DIR": str(tmp_path / "server"),
        "ARK_FAILURE_FILE": str(tmp_path / "failure"),
        "FAKE_STEAMCMD_STATE": str(tmp_path / "runs.json"),
        "FAKE_STEAMCMD_CALLS": str(calls),
        **env,
    }
    result = subprocess.run(["bash", str(script), *args], env=full_env, capture_output=True, text=True, check=False)
    lines = calls.read_text().splitlines() if calls.exists() else []
    return result, [json.loads(line) for line in lines]


STATE_0X6 = "Error! App '2430930' state is 0x6 after update job."


def test_success_with_matching_build_id(tmp_path: Path) -> None:
    result, calls = run_install(tmp_path, [{"build_id": "200"}], ARK_STEAM_BUILD_ID="200")
    assert result.returncode == 0, result.stderr
    assert len(calls) == 1
    assert not (tmp_path / "failure").exists()


def test_build_id_mismatch_fails_with_reason(tmp_path: Path) -> None:
    result, _ = run_install(tmp_path, [{"build_id": "201"}], ARK_STEAM_BUILD_ID="200")
    assert result.returncode == 1
    reason = "Downloaded ARK build 201, but the build was keyed to 200; rebuild to pick up the new key."
    assert (tmp_path / "failure").read_text().strip() == reason


def installed(tmp_path: Path, build_id: str) -> Path:
    """Lay down an installed build's app manifest; return its path."""
    manifest = tmp_path / "server" / MANIFEST
    manifest.parent.mkdir(parents=True)
    manifest.write_text(f'"AppState"\n{{\n\t"buildid"\t\t"{build_id}"\n}}\n')
    return manifest


def test_state_failure_names_the_state_in_one_line(tmp_path: Path) -> None:
    """A fresh install (no app manifest) has nothing to fall back from."""
    result, calls = run_install(tmp_path, [{"exit": 8, "output": STATE_0X6}])
    assert result.returncode == 1
    assert len(calls) == 1
    assert (tmp_path / "failure").read_text() == "SteamCMD download failed: state is 0x6\n"
    # SteamCMD's output is shown once (tee), not repeated on stderr.
    assert STATE_0X6 not in result.stderr


def test_state_0x6_falls_back_to_validate_without_the_app_manifest(tmp_path: Path) -> None:
    manifest = installed(tmp_path, "100")
    runs = [{"exit": 8, "output": STATE_0X6}, {"build_id": "200"}]
    result, calls = run_install(tmp_path, runs, ARK_STEAM_BUILD_ID="200")
    assert result.returncode == 0, result.stderr
    assert [c["manifest"] for c in calls] == [True, False]
    assert all("validate" in c["args"] for c in calls)
    assert "Warning: SteamCMD could not update from the installed build (state 0x6" in result.stderr
    assert '"200"' in manifest.read_text()
    assert not manifest.with_name(manifest.name + ".before-fallback").exists()


def test_state_0x6_fallback_failing_restores_the_app_manifest(tmp_path: Path) -> None:
    manifest = installed(tmp_path, "100")
    before = manifest.read_text()
    runs = [{"exit": 8, "output": STATE_0X6}, {"exit": 8, "output": STATE_0X6}]
    result, calls = run_install(tmp_path, runs)
    assert result.returncode == 1
    assert len(calls) == 2  # the fallback runs once
    assert manifest.read_text() == before
    assert not manifest.with_name(manifest.name + ".before-fallback").exists()
    reason = "SteamCMD download failed: state is 0x6 (also after the validate fallback)\n"
    assert (tmp_path / "failure").read_text() == reason


def test_unknown_failure_reports_the_exit_code(tmp_path: Path) -> None:
    result, _ = run_install(tmp_path, [{"exit": 5, "output": "something else"}])
    assert result.returncode == 1
    assert (tmp_path / "failure").read_text() == "SteamCMD download failed: exit code 5\n"


def run_download(tmp_path: Path, runs: list[dict]) -> tuple[subprocess.CompletedProcess, list[dict]]:
    """Run download.sh on work dir tmp_path/work with the fake steamcmd."""
    return run_install(tmp_path, runs, script=DOWNLOAD, args=(str(tmp_path / "work"),))


def test_download_keeps_the_depot_cache_between_runs(tmp_path: Path) -> None:
    depots = tmp_path / "work/cache/depotcache"

    result, calls = run_download(tmp_path, [{"build_id": "100", "depot": "2430931_100.manifest"}])
    assert result.returncode == 0, result.stderr
    assert calls[0]["depots"] == []
    assert sorted(p.name for p in depots.iterdir()) == ["2430931_100.manifest"]
    # The image still gets SteamCMD's client files, depotcache included.
    assert (tmp_path / "work/out/steam-client/depotcache/2430931_100.manifest").is_file()

    # A new SteamCMD home (a new container) still finds the installed build's manifest.
    shutil.rmtree(tmp_path / "home")
    result, calls = run_download(tmp_path, [{"build_id": "200", "depot": "2430931_200.manifest"}])
    assert result.returncode == 0, result.stderr
    assert calls[0]["depots"] == ["2430931_100.manifest"]
    assert sorted(p.name for p in depots.iterdir()) == ["2430931_100.manifest", "2430931_200.manifest"]


def test_download_failure_keeps_the_previous_depot_cache(tmp_path: Path) -> None:
    depots = tmp_path / "work/cache/depotcache"
    depots.mkdir(parents=True)
    (depots / "2430931_100.manifest").write_text("manifest")
    result, _ = run_download(tmp_path, [{"exit": 5, "output": "boom"}])
    assert result.returncode != 0
    assert [p.name for p in depots.iterdir()] == ["2430931_100.manifest"]
    assert (tmp_path / "work/failure").read_text() == "SteamCMD download failed: exit code 5\n"


BUILD = REPO_ROOT / "container/files/build.sh"
FAKE_DOCKER = r"""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ["FAKE_DOCKER_CALLS"], "a") as calls:
    calls.write(json.dumps(args) + "\n")
if args[:2] == ["image", "inspect"]:
    sys.exit(0 if args[2] in os.environ.get("FAKE_DOCKER_IMAGES", "").split() else 1)
if args[0] == "create":
    print("seed-container")
elif args[0] == "cp":
    dest = Path(args[2])
    dest.mkdir(parents=True)
    (dest / "2430931_100.manifest").write_text("manifest")
"""


def run_build(tmp_path: Path, images: str) -> tuple[subprocess.CompletedProcess, list[list[str]]]:
    """Run build.sh with a fake docker that knows images (space-separated); return its calls."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir(exist_ok=True)
    fake = bin_dir / "docker"
    fake.write_text(FAKE_DOCKER)
    fake.chmod(0o755)
    calls = tmp_path / "docker-calls.jsonl"
    calls.unlink(missing_ok=True)
    env = {
        **os.environ,
        "PATH": f"{bin_dir}:{os.environ['PATH']}",
        "ARK_STEAM_BUILD_ID": "200",
        "WORK_DIR": str(tmp_path / "work"),
        "FAKE_DOCKER_CALLS": str(calls),
        "FAKE_DOCKER_IMAGES": images,
    }
    result = subprocess.run(["bash", str(BUILD)], env=env, capture_output=True, text=True, check=False)
    return result, [json.loads(line) for line in calls.read_text().splitlines()]


def test_build_seeds_a_missing_depot_cache_from_the_last_image(tmp_path: Path) -> None:
    result, calls = run_build(tmp_path, "ark-server:latest")
    assert result.returncode == 0, result.stderr
    assert ["create", "ark-server:latest"] in calls
    assert (tmp_path / "work/cache/depotcache/2430931_100.manifest").is_file()
    assert ["rm", "seed-container"] in calls
    # Seeded before the download runs.
    assert calls.index(["create", "ark-server:latest"]) < next(i for i, c in enumerate(calls) if c[0] == "run")


def test_build_does_not_seed_an_existing_depot_cache(tmp_path: Path) -> None:
    (tmp_path / "work/cache/depotcache").mkdir(parents=True)
    result, calls = run_build(tmp_path, "ark-server:latest")
    assert result.returncode == 0, result.stderr
    assert not any(c[0] == "create" for c in calls)


def test_build_without_a_seed_image_skips_seeding(tmp_path: Path) -> None:
    result, calls = run_build(tmp_path, "")
    assert result.returncode == 0, result.stderr
    assert not any(c[0] == "create" for c in calls)
    assert not (tmp_path / "work/cache/depotcache").exists()
