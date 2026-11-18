#!/usr/bin/env python3
from __future__ import annotations

import argparse
import os
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent / "tests"))
from helpers.artifacts import create_suite_run_dir, ensure_python, repo_root, tests_root, wipe_test_artifacts
from helpers.batch_runner import SUITE_TITLES, Batch, print_summary, run_batch

REPO_ROOT = repo_root()
DEFAULT_PYTHON = REPO_ROOT / ".venv" / "bin" / "python"

ALL_BATCHES = (
    Batch("unit", "unit-runtime-log-rotation", 120, ("tests/container/unit/test_log_rotation.py",)),
    Batch("unit", "unit-rcon-client", 60, ("tests/container/unit/test_rcon_client.py",)),
    Batch(
        "container-fast",
        "scripts-server-oom-report",
        60,
        ("tests/container/integration/fast/test_server_oom_report.py",),
    ),
    Batch(
        "container-slow",
        "scripts-startup-and-rcon",
        900,
        (
            "tests/container/integration/slow/test_ark_server_start_script.py",
            "tests/container/integration/slow/test_rcon.py",
        ),
    ),
    Batch(
        "container-slow",
        "scripts-start-init-and-stop-script",
        1200,
        (
            "tests/container/integration/slow/test_ark_server_start_integrated_init.py",
            "tests/container/integration/slow/test_ark_server_stop_script.py",
        ),
    ),
    Batch(
        "container-slow",
        "scripts-backup-script",
        1800,
        ("tests/container/integration/slow/test_ark_backup_script.py",),
    ),
    Batch(
        "container-slow",
        "scripts-restore-script",
        1200,
        ("tests/container/integration/slow/test_ark_restore_script.py",),
    ),
    Batch(
        "container-slow",
        "scripts-start-backup-stop-workflow",
        1200,
        ("tests/container/integration/slow/test_ark_start_backup_stop_workflow.py",),
    ),
    Batch(
        "container-slow",
        "scripts-dirty-stop",
        1800,
        ("tests/container/integration/slow/test_dirty_stop.py",),
    ),
    Batch(
        "container-slow",
        "scripts-runtime-signal-shutdown",
        1200,
        ("tests/container/integration/slow/test_runtime_signal_shutdown.py",),
    ),
    Batch(
        "cli-fast",
        "cli-reconfigure-ports",
        120,
        ("tests/app/ark_cli/integration/docker/fast/test_reconfigure_ports.py",),
    ),
    Batch(
        "cli-fast",
        "cli-update-local-check",
        120,
        ("tests/app/ark_cli/integration/docker/fast/test_update_local_check.py",),
    ),
    Batch(
        "cli-slow",
        "cli-manage-start-stop",
        1200,
        ("tests/app/ark_cli/integration/docker/slow/test_manage_start_stop.py",),
    ),
    Batch(
        "cli-slow",
        "cli-manage-backup-restore",
        2400,
        (
            "tests/app/ark_cli/integration/docker/slow/test_manage_backup_restore.py",
            "tests/app/ark_cli/integration/docker/slow/test_cluster_backup_restore.py",
        ),
    ),
    Batch(
        "cli-slow",
        "cli-update-local-check-slow",
        1200,
        ("tests/app/ark_cli/integration/docker/slow/test_update_local_check.py",),
    ),
    Batch(
        "cli-slow",
        "cli-reconfigure-ports-slow",
        900,
        ("tests/app/ark_cli/integration/docker/slow/test_reconfigure_ports.py",),
    ),
    Batch(
        "cli-slow",
        "cli-update-instance-rollout",
        900,
        ("tests/app/ark_cli/integration/docker/slow/test_update_instance_rollout.py",),
    ),
    Batch(
        "cli-slow",
        "cli-update-instance-rollback",
        1200,
        ("tests/app/ark_cli/integration/docker/slow/test_update_instance_rollback.py",),
    ),
)

# CI's publish gate: a healthy start and a clean stop against the new image.
# Kept out of ALL_BATCHES so local "all" runs don't repeat these tests.
SMOKE_BATCHES = (
    Batch(
        "smoke",
        "scripts-healthy-start-and-stop",
        1200,
        (
            "tests/container/integration/slow/test_ark_server_start_script.py",
            "tests/container/integration/slow/test_ark_server_stop_script.py",
        ),
    ),
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run repo test batches from one root command.")
    parser.add_argument(
        "--suite",
        choices=(
            "all",
            "smoke",
            "unit",
            "container",
            "container-fast",
            "container-slow",
            "cli",
            "cli-fast",
            "cli-slow",
            "fast",
            "slow",
        ),
        default="all",
        help="Which suite group to run.",
    )
    parser.add_argument(
        "--python",
        dest="python_bin",
        default=os.environ.get("TEST_BATCH_PYTHON", str(DEFAULT_PYTHON)),
        help="Python interpreter used for pytest. Defaults to TEST_BATCH_PYTHON or ./.venv/bin/python.",
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List the selected batches without running them.",
    )
    parser.add_argument(
        "pytest_args",
        nargs=argparse.REMAINDER,
        help="Extra pytest args to append to every batch. Prefix with -- to separate.",
    )
    return parser.parse_args()


def selected_batches(suite: str) -> list[Batch]:  # noqa: PLR0911
    if suite == "smoke":
        return list(SMOKE_BATCHES)
    if suite == "all":
        return list(ALL_BATCHES)
    if suite == "fast":
        return [batch for batch in ALL_BATCHES if batch.suite.endswith("-fast")]
    if suite == "slow":
        return [batch for batch in ALL_BATCHES if batch.suite.endswith("-slow")]
    if suite == "container":
        return [batch for batch in ALL_BATCHES if batch.suite.startswith("container")]
    if suite == "cli":
        return [batch for batch in ALL_BATCHES if batch.suite.startswith("cli")]
    return [batch for batch in ALL_BATCHES if batch.suite == suite]


def normalize_pytest_args(values: list[str]) -> list[str]:
    if values and values[0] == "--":
        return values[1:]
    return values


def main() -> int:
    args = parse_args()
    batches = selected_batches(args.suite)
    extra_pytest_args = normalize_pytest_args(args.pytest_args)
    ensure_python(args.python_bin, REPO_ROOT)

    if args.list:
        for batch in batches:
            print(f"{batch.suite:9} {batch.label:40} {' '.join(batch.paths)}")
        return 0

    if not batches:
        print("No batches selected.")
        return 0

    wipe_test_artifacts(REPO_ROOT)
    os.environ["ARK_TEST_WIPE_STATE"] = "1"
    title = SUITE_TITLES.get(args.suite, "All Tests") if args.suite != "all" else "All Tests"
    print(title, flush=True)
    print(f"Test artifacts: {tests_root(REPO_ROOT)}", flush=True)
    # Create suite-run directory for aggregating all batch artifacts
    suite_dir = create_suite_run_dir(REPO_ROOT)
    suite_lines: list[str] = []
    results: list[tuple[Batch, str, int]] = []
    total = len(batches)
    for index, batch in enumerate(batches, start=1):
        batch_env = {
            "ARK_TEST_RUN_DIR": str(suite_dir),
            "ARK_TEST_BATCH_LABEL": batch.label,
        }
        status, duration = run_batch(
            index, total, batch, args.python_bin, ("-q",), extra_pytest_args, REPO_ROOT, batch_env
        )
        results.append((batch, status, duration))
        suite_lines.append(f"{status:<8} {batch.label:<40} {duration:>6}s\n")
    # Write suite-level summary log
    suite_log = suite_dir / "suite.log"
    with open(suite_log, "w", encoding="utf-8") as f:  # noqa: PTH123
        f.write(f"Suite: {title}\n")
        f.write(f"Started: {time.strftime('%y%m%d-%H%M%S', time.localtime())}\n")
        f.write(f"{'STATUS':<8} {'LABEL':<40} {'DURATION':>6}\n")
        f.write(f"{'-' * 8} {'-' * 40} {'-' * 6}\n")
        f.writelines(suite_lines)
    return print_summary(results)


if __name__ == "__main__":
    raise SystemExit(main())
