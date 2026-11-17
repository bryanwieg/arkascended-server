from __future__ import annotations

import os
import shlex
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Batch:
    suite: str
    label: str
    timeout_seconds: int
    paths: tuple[str, ...]


SUITE_TITLES = {
    "unit": "Unit Tests",
    "container": "Container Integration Tests",
    "container-fast": "Container Fast Integration Tests",
    "container-slow": "Container Slow Integration Tests",
    "cli": "CLI Integration Tests",
    "cli-fast": "CLI Fast Integration Tests",
    "cli-slow": "CLI Slow Integration Tests",
    "fast": "Fast Integration Tests (all)",
    "slow": "Slow Integration Tests (all)",
    "smoke": "Smoke Tests (healthy start and stop)",
}


def run_batch(
    index: int,
    total: int,
    batch: Batch,
    python_bin: str,
    pytest_flags: tuple[str, ...],
    extra_pytest_args: list[str],
    repo_root: Path,
    env_override: dict[str, str] | None = None,
) -> tuple[str, int]:
    command = [python_bin, "-m", "pytest", *batch.paths, *pytest_flags, *extra_pytest_args]
    print(f"\n==> [{index:02d}/{total:02d}] {batch.label}", flush=True)
    print(f"    suite: {batch.suite}", flush=True)
    timeout_str = f"    timeout: {batch.timeout_seconds}s" if batch.timeout_seconds > 0 else "    timeout: none"
    print(timeout_str, flush=True)
    print(f"    command: {shlex.join(command)}", flush=True)

    batch_env = os.environ.copy()
    if env_override:
        batch_env.update(env_override)

    start = time.time()
    try:
        completed = subprocess.run(
            command,
            cwd=repo_root,
            check=False,
            capture_output=True,
            text=True,
            env=batch_env,
            timeout=batch.timeout_seconds if batch.timeout_seconds > 0 else None,
        )
        rc = completed.returncode
        stdout = completed.stdout or ""
        stderr = completed.stderr or ""
        if stdout:
            sys.stdout.write(stdout)
        if stderr:
            sys.stderr.write(stderr)
        if rc == 0:
            last_lines = stdout.strip().splitlines()[-5:]
            all_skipped = any("dependency" in line for line in last_lines)
            status = "SKIP" if all_skipped else "PASS"
        else:
            status = f"FAIL({rc})"
    except subprocess.TimeoutExpired as exc:
        # Keep what the batch printed before it was killed; it's the only clue.
        for output, stream in ((exc.stdout, sys.stdout), (exc.stderr, sys.stderr)):
            if output:
                stream.write(output if isinstance(output, str) else output.decode(errors="replace"))
        rc = 124
        status = "FAIL(124)"

    duration = int(time.time() - start)
    print(f"    result: {status} in {duration}s", flush=True)
    return status, duration


def print_summary(results: list[tuple[Batch, str, int]]) -> int:
    print("\n=== Summary ===")
    failed = 0
    skipped = 0
    for idx, (batch, status, duration) in enumerate(results, start=1):
        print(f"{idx:2d}. {batch.label:<40} {status:<8} {duration:>6}s")
        if status == "FAIL(1)" or status.startswith("FAIL"):
            failed += 1
        elif status == "SKIP":
            skipped += 1
    passed = len(results) - failed - skipped
    print(f"Batches: {len(results)} total, {passed} passed, {failed} failed, {skipped} skipped")
    return 1 if failed else 0
