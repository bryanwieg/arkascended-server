"""ark-server-install.sh against a fake steamcmd: failure reasons and build-ID checks.

The fake replays one scripted run per call (exit code, output, build ID written to the
app manifest) and records each call's arguments, so tests see what the script asked for.
"""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
INSTALL = REPO_ROOT / "container/files/scripts/installer/ark-server-install.sh"
MANIFEST = "steamapps/appmanifest_2430930.acf"

FAKE_STEAMCMD = r"""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
state = Path(os.environ["FAKE_STEAMCMD_STATE"])
runs = json.loads(state.read_text())
run = runs.pop(0)
state.write_text(json.dumps(runs))
with open(os.environ["FAKE_STEAMCMD_CALLS"], "a") as calls:
    calls.write(json.dumps(sys.argv[1:]) + "\n")
install_dir = Path(sys.argv[sys.argv.index("+force_install_dir") + 1])
if run.get("build_id"):
    (install_dir / "steamapps").mkdir(parents=True, exist_ok=True)
    (install_dir / "steamapps/appmanifest_2430930.acf").write_text(
        '"AppState"\n{\n\t"buildid"\t\t"%s"\n}\n' % run["build_id"])
print(run.get("output", ""))
sys.exit(run.get("exit", 0))
"""


def run_install(tmp_path: Path, runs: list[dict], **env: str) -> tuple[subprocess.CompletedProcess, list[list[str]]]:
    """Run the install script with a fake steamcmd that replays runs; return the result and its calls."""
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
        "ARK_INSTALL_DIR": str(tmp_path / "server"),
        "ARK_FAILURE_FILE": str(tmp_path / "failure"),
        "FAKE_STEAMCMD_STATE": str(tmp_path / "runs.json"),
        "FAKE_STEAMCMD_CALLS": str(calls),
        **env,
    }
    result = subprocess.run(["bash", str(INSTALL)], env=full_env, capture_output=True, text=True, check=False)
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


def test_state_failure_names_the_state_in_one_line(tmp_path: Path) -> None:
    result, _ = run_install(tmp_path, [{"exit": 8, "output": STATE_0X6}])
    assert result.returncode == 1
    assert (tmp_path / "failure").read_text() == "SteamCMD download failed: state is 0x6\n"
    # SteamCMD's output is shown once (tee), not repeated on stderr.
    assert STATE_0X6 not in result.stderr


def test_unknown_failure_reports_the_exit_code(tmp_path: Path) -> None:
    result, _ = run_install(tmp_path, [{"exit": 5, "output": "something else"}])
    assert result.returncode == 1
    assert (tmp_path / "failure").read_text() == "SteamCMD download failed: exit code 5\n"
