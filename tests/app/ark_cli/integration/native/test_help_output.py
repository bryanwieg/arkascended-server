from __future__ import annotations

from pathlib import Path

_KEY_VERBS = [
    "create instance",
    "start instance",
    "stop instance",
    "restart instance",
    "create backup",
    "restore backup",
    "check host-kernel-config",
    "check reachability",
    "list backups",
    "show schedule",
]


def test_dash_help_lists_key_commands(
    cli_path: Path,
    run_cmd_native,
) -> None:
    """Top-level --help output includes all key verb/object commands.

    Scenario:
        The CLI is invoked with --help and the output is checked for the
        presence of every core verb/object pair (create instance, start
        instance, etc.).

    Expected:
        - The command exits zero.
        - Each key verb/object string appears in the combined stdout/stderr.
    """
    result = run_cmd_native(
        [str(cli_path), "--help"],
        capture_output=True,
        timeout=10,
    )
    assert result.returncode == 0
    output = result.stdout or result.stderr
    for verb in _KEY_VERBS:
        assert verb in output, f"missing verb in --help output: {verb}"


def test_create_dash_help_lists_subcommands(
    cli_path: Path,
    run_cmd_native,
) -> None:
    """The 'create' subcommand --help lists its objects (instance, backup).

    Scenario:
        The CLI is invoked with 'create --help' and the output is inspected
        for the subcommand objects that 'create' supports.

    Expected:
        - The output contains both 'instance' and 'backup'.
    """
    result = run_cmd_native(
        [str(cli_path), "create", "--help"],
        check=False,
        capture_output=True,
        timeout=10,
    )
    output = result.stdout or result.stderr
    assert "instance" in output
    assert "backup" in output


def test_check_dash_help_lists_subcommands(
    cli_path: Path,
    run_cmd_native,
) -> None:
    """The 'check' subcommand --help lists its objects (instance, host-kernel-config).

    Scenario:
        The CLI is invoked with 'check --help' and the output is inspected
        for the subcommand objects that 'check' supports.

    Expected:
        - The output contains 'instance' or 'host-kernel-config'.
    """
    result = run_cmd_native(
        [str(cli_path), "check", "--help"],
        check=False,
        capture_output=True,
        timeout=10,
    )
    output = result.stdout or result.stderr
    assert "instance" in output or "host-kernel-config" in output
