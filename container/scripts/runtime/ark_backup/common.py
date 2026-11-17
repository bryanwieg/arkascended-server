"""Shared constants, argument parsing, logging, and utilities for the backup package."""

from __future__ import annotations

import argparse
import subprocess
from pathlib import Path

from runtime_utils import CLUSTER_DIR_NAME, SAVED_DIR


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments for the backup script."""
    parser = argparse.ArgumentParser(usage="%(prog)s --output <path>")
    parser.add_argument("--output", required=True, help="Archive output path (.tar, .tar.gz, or .tar.zst)")
    return parser.parse_args()


def archive_args(output: Path) -> list[str]:
    """Return the tar command arguments for the given output archive path and extension."""
    output_name = output.name
    # The live server appends to Logs/ constantly; tar exits 1 ("file changed as we read it") on it.
    source = ["--exclude=./Logs", f"--exclude=./{CLUSTER_DIR_NAME}", "-C", str(SAVED_DIR), "."]
    if output_name.endswith(".tar"):
        return ["tar", "-cf", str(output), *source]
    if output_name.endswith(".tar.gz"):
        return ["tar", "-czf", str(output), *source]
    if output_name.endswith(".tar.zst"):
        return ["tar", "--zstd", "-cf", str(output), *source]
    raise ValueError(f"output must end with .tar, .tar.gz, or .tar.zst: {output}")


def combined_process_output(result: subprocess.CompletedProcess[str]) -> str:
    """Return stdout and stderr of a completed process concatenated and stripped."""
    return f"{result.stdout}{result.stderr}".strip()


def process_failure_detail(result: subprocess.CompletedProcess[str], fallback: str) -> str:
    """Return the most descriptive failure message from a process result, falling back to a provided string."""
    detail = (result.stderr or result.stdout).strip()
    if detail:
        return detail
    return fallback
