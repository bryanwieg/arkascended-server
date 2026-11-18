"""Shared constants, argument parsing, logging, and utilities for the backup package."""

from __future__ import annotations

import argparse
import subprocess
from collections.abc import Sequence
from pathlib import Path

from runtime_utils import CLUSTER_DIR_NAME, SAVED_DIR


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments for the backup script."""
    parser = argparse.ArgumentParser(usage="%(prog)s --output <path>")
    parser.add_argument("--output", required=True, help="Archive output path (.tar, .tar.gz, or .tar.zst)")
    return parser.parse_args()


def tar_create_args(output: Path, source_dir: Path, excludes: Sequence[str], dest: Path | None = None) -> list[str]:
    """Return tar arguments archiving source_dir, compressed by output's suffix, written to dest (default output)."""
    output_name = output.name
    target = str(dest or output)
    source = [*(f"--exclude={item}" for item in excludes), "-C", str(source_dir), "."]
    if output_name.endswith(".tar"):
        return ["tar", "-cf", target, *source]
    if output_name.endswith(".tar.gz"):
        return ["tar", "-czf", target, *source]
    if output_name.endswith(".tar.zst"):
        return ["tar", "--zstd", "-cf", target, *source]
    raise ValueError(f"output must end with .tar, .tar.gz, or .tar.zst: {output}")


def archive_args(output: Path) -> list[str]:
    """Return the tar command arguments for the given output archive path and extension."""
    # The live server appends to Logs/ constantly; tar exits 1 ("file changed as we read it") on it.
    return tar_create_args(output, SAVED_DIR, ["./Logs", f"./{CLUSTER_DIR_NAME}"])


def combined_process_output(result: subprocess.CompletedProcess[str]) -> str:
    """Return stdout and stderr of a completed process concatenated and stripped."""
    return f"{result.stdout}{result.stderr}".strip()


def process_failure_detail(result: subprocess.CompletedProcess[str], fallback: str) -> str:
    """Return the most descriptive failure message from a process result, falling back to a provided string."""
    detail = (result.stderr or result.stdout).strip()
    if detail:
        return detail
    return fallback
