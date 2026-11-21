"""Static checks enforcing the logging & CLI output conventions.

These are mechanical rules derived from
docs/dev/reference/conventions.md#logging-and-cli-output (rule table)
and docs/dev/explanation/logging-architecture.md (rationale).

The ``except_paths`` list on each rule is the brownfield bridge:
known-legacy sites are exempt until remediated. Remove an entry from
the list as the corresponding site is fixed. The list is the
remediation checklist.

Rules that cannot be expressed as a grep (e.g. slog leaking to stdout,
JSON mode stdout purity, verbose tee completeness) live in Go runtime
tests under ``app/ark-cli/``, not here.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parent.parent


@dataclass(frozen=True)
class Rule:
    """One logging-discipline rule.

    ``paths`` and ``except_paths`` are repo-root-relative glob patterns
    (``Path.glob``-compatible). ``except_paths`` may also be exact
    relative paths for a single-file exemption.
    """

    name: str
    paths: tuple[str, ...]
    pattern: re.Pattern[str]
    except_paths: tuple[str, ...]
    message: str


# Domain packages must not write stdio directly or use CLI rendering
# primitives. See conventions.md §"Logging and CLI output".
PKG_ARK_STDIO_ALLOWLIST: tuple[str, ...] = (
    # Subprocess stdio plumbing; attaches streams deliberately.
    "app/pkg/ark/cmdrunner/runner.go",
    # Internal pipe consumer for container log streams.
    "app/pkg/ark/sdkbackend/compose.go",
    # Legacy sites pending remediation (see audit report, Gaps 2–3):  # noqa: RUF003
    "app/pkg/ark/schedule/state.go",
    "app/pkg/ark/host/kernel_config.go",
    "app/pkg/ark/host/port_output.go",
    "app/pkg/ark/runtime/privilege.go",
    # Runtime primitive owns subprocess stdio attachment.
    "app/pkg/ark/runtime/exec.go",
    # SDK conformance test helper uses os.Stderr for test logger output.
    "app/pkg/ark/sdkbackend_test/conformance_test.go",
)


# Container scripts emit through _logging.py; only the module itself writes stdio.
CONTAINER_PRINT_ALLOWLIST: tuple[str, ...] = ("container/scripts/runtime/_logging.py",)


RULES: tuple[Rule, ...] = (
    Rule(
        name="no-stdio-in-pkg-ark",
        paths=("app/pkg/ark/**/*.go",),
        # Flag fmt.Print*, fmt.Fprint*, and os.Stdout/os.Stderr outside
        # explicitly-allowed low-level files.
        pattern=re.compile(r"\bfmt\.(Print|Fprint(?:f|ln)?)\b|\bos\.(Stdout|Stderr)\b"),
        except_paths=PKG_ARK_STDIO_ALLOWLIST,
        message=(
            "Domain packages must not write stdio directly. Emit via the "
            "injected *slog.Logger, or return data to the handler. "
            "See docs/dev/reference/conventions.md#logging-and-cli-output."
        ),
    ),
    Rule(
        name="no-core-import-in-pkg-ark",
        paths=("app/pkg/ark/**/*.go",),
        # Domain packages must not import ark-cli's core rendering primitives.
        pattern=re.compile(r'"ark-cli/internal/core"'),
        except_paths=(),
        message=(
            "Domain packages in app/pkg/ark/ must not import "
            "ark-cli/internal/core. Handlers render; services compute. "
            "See docs/dev/reference/conventions.md#logging-and-cli-output."
        ),
    ),
    Rule(
        name="no-capture-false-cmdrunner",
        paths=("app/**/*.go",),
        # Capture: false is being migrated to Mode: Inherit (planned).
        # Allow the definition site (where Capture is declared) to mention it.
        pattern=re.compile(r"Capture:\s*false\b"),
        except_paths=(
            "app/pkg/ark/cmdrunner/runner.go",  # type definition and comment
        ),
        message=(
            "RunOptions{Capture: false} is being replaced by "
            "RunOptions{Mode: Inherit} so subprocess stdio flows through the "
            "verbose tee. See docs/dev/explanation/logging-architecture.md "
            "#subprocess-stdio."
        ),
    ),
    Rule(
        name="no-bare-print-in-container",
        paths=("container/scripts/runtime/**/*.py",),
        pattern=re.compile(r"^\s*print\("),
        except_paths=CONTAINER_PRINT_ALLOWLIST,
        message=(
            "Use emit_event / emit_warning / emit_error from _logging.py instead of bare "
            "print(). See docs/dev/explanation/logging-architecture.md"
            "#container-side-logging."
        ),
    ),
    Rule(
        name="no-scanner-text-as-slog-info",
        paths=("app/pkg/ark/**/*.go",),
        # backup/create.go surfaces subprocess stdout by promoting every
        # line to slog.Info. That produces N log events per invocation
        # and contaminates structured-log consumers. Collect into one event.
        pattern=re.compile(r"\blogger\.(Info|Warn|Error|Debug)\(\s*scanner\.Text\("),
        except_paths=(
            # Legacy site pending remediation (see audit report, Gap 9):
            "app/pkg/ark/backup/create.go",
        ),
        message=(
            "Do not promote raw subprocess stdout lines to structured log "
            "events. Collect the output and emit one structured event with "
            "the payload as an attribute. "
            "See docs/dev/explanation/logging-architecture.md."
        ),
    ),
)


def _expand(patterns: tuple[str, ...]) -> list[Path]:
    """Expand globs relative to the repo root, deduplicated and sorted."""
    files: set[Path] = set()
    for pat in patterns:
        files.update(REPO_ROOT.glob(pat))
    return sorted(files)


def _is_excepted(path: Path, patterns: tuple[str, ...]) -> bool:
    """Check if ``path`` is exempt. Patterns may be exact relative paths
    or ``Path.match``-compatible globs."""
    rel = path.relative_to(REPO_ROOT).as_posix()
    if rel in patterns:
        return True
    return any(Path(rel).match(p) for p in patterns)


def _find_violations(rule: Rule) -> list[str]:
    """Return ``file:line: content`` entries for each rule violation."""
    violations: list[str] = []
    for path in _expand(rule.paths):
        if _is_excepted(path, rule.except_paths):
            continue
        try:
            text = path.read_text(encoding="utf-8")
        except OSError, UnicodeDecodeError:
            continue
        for lineno, line in enumerate(text.splitlines(), start=1):
            if rule.pattern.search(line):
                rel = path.relative_to(REPO_ROOT).as_posix()
                violations.append(f"  {rel}:{lineno}: {line.rstrip()}")
    return violations


@pytest.mark.parametrize("rule", RULES, ids=[r.name for r in RULES])
def test_logging_discipline(rule: Rule) -> None:
    """Fail if any rule is violated. Each rule produces one aggregated report."""
    violations = _find_violations(rule)
    if violations:
        pytest.fail(
            f"{rule.name}: {rule.message}\n\nViolations ({len(violations)}):\n" + "\n".join(violations),
            pytrace=False,
        )


def test_rule_allowlists_reference_existing_files() -> None:
    """Each allowlist entry must point to a real file; stale entries
    accumulate silently and mask new violations."""
    stale: list[str] = []
    for rule in RULES:
        for entry in rule.except_paths:
            # Skip glob-style entries (contain wildcards).
            if any(ch in entry for ch in "*?["):
                continue
            target = REPO_ROOT / entry
            if not target.exists():
                stale.append(f"  rule={rule.name!r} entry={entry!r}")
    if stale:
        pytest.fail(
            "Stale allowlist entries (file no longer exists):\n"
            + "\n".join(stale)
            + "\n\nRemove the entry from except_paths in "
            "tests/test_logging_discipline.py.",
            pytrace=False,
        )
