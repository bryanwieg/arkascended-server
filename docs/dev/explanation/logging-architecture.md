# Logging and CLI output architecture

How the CLI, domain packages, and container scripts emit text; which
channel to use for what; and why the design is shaped this way. For the
channel-rule table operators and authors reach for first, see
[conventions.md — Logging and CLI output](../reference/conventions.md#logging-and-cli-output).

## Core idea

Two channels, one discipline, three rendering modes.

1. **User-facing text channel** — stdout for results and progress; stderr
   for warnings, errors, and prompts. Spinner-aware. Every substring
   assertion in the integration test suite targets this channel.
2. **Operational event channel** — an injected `*slog.Logger` at the
   `pkg/ark` boundary. Writes to stderr via a text handler; level gates
   visibility. Never used for user-facing text.

Rendering modes — default text output, `--json` for structured output, and an
orthogonal `--verbose` / `--log-file PATH` — adjust how these channels
render but never swap them.

The three-layer architecture (CLI → `pkg/ark` → container) is preserved:
domain services only emit operational events through the injected
logger; they never reach into `os.Stdout` / `os.Stderr` or call the
CLI's `core.*` primitives. Handlers render, services compute.

## Message kinds

Every output has exactly one kind. The channel is determined by kind, not
by preference or location.

| Kind | What it is | Audience |
|---|---|---|
| User text | status, results, progress narration, tables, YAML-ish detail blocks | human in terminal |
| Structured output | single JSON (later: YAML) document representing the command's result | scripts, dashboards |
| Warning | actionable problem; operation continues or succeeds with caveats | human |
| Error | terminal failure | human |
| Prompt | synchronous input request | human |
| Operational event | what the system did internally (`exec cmd=…`, `backup-started svc=…`) | operator debugging |
| Spinner frame | transient terminal UI; never persisted | human (TTY only) |

## Why two channels instead of one

Unified single-channel models (everything structured, consumer demuxes)
were considered and rejected:

- Integration tests parse substrings from process stdout/stderr. Putting
  `"Created schedule nightly"` inside a JSON envelope would force every
  assertion to become a JSON decode. The test suite has ~50 such
  assertions; rewriting them is a large churn with no gain.
- Operators pipe `ark-cli show list` into `grep`, `awk`, or their eyes.
  Human-readable text on stdout is the contract they already expect.
- Structured event emission (slog) is still useful for debugging
  subprocess traces and domain events — separating it prevents contamination
  of the human channel and keeps `-v` mode meaningful.

The cost is one discipline rule: handlers must not emit user-facing text
through `slog`, and domain services must not emit at all except via
`slog`. Static and runtime checks enforce this (see
[conventions.md](../reference/conventions.md#logging-and-cli-output)).

## Rendering modes

### Format

- `--json` — boolean flag. When set, stdout is reserved for a single
  JSON document. No progress narration on stdout; status goes to
  stderr if emitted. Commands with no structured result reject `--json`
  at flag parse time.

### Verbosity

- default — slog level `Info`. Lifecycle events on stderr.
- `--verbose` / `-v` — slog level `Debug`. Subprocess trace visible;
  `verboseLogf` timestamped lines enabled.

### Log-file tee

`--log-file PATH` — tees everything in temporal order with tags. When
`--verbose` is enabled, the tee also captures user-facing writes:

- `[cli-stdout]` — user text and structured output
- `[cli-stderr]` — warnings and errors
- `[docker-logs]` — test-infra capture of `docker compose logs`

Works with or without `--verbose` and any `--format`. An operator can
combine `--json --log-file out.log` to get clean JSON on stdout
and a complete trace in the file.

### TTY detection

Spinners auto-disable when stdout is not a TTY, when `TERM=dumb`, or
when `--verbose` is set. Phase transitions render as discrete `Infof`
lines in those cases.

## PID1 file descriptor mirror (load-bearing)

Container-side scripts invoked under `docker compose exec` write to the
exec caller's stdio, not to `docker logs`. To surface the same event in
both streams, `container/scripts/runtime/_logging.py` opens
`/proc/1/fd/1` or `/proc/1/fd/2` and writes the bytes a second time.

The canonical logging module is `container/scripts/runtime/_logging.py`,
exposing `emit_event`, `emit_warning` and `emit_error`.

This is how integration tests can assert the same `"[backup] starting
backup workflow"` string in both `backup_result.stdout` (from
`exec`) and `backup_logs.stdout` (from `docker logs`). Removing the
mirror would silently break the `docker logs` assertion only. The
mechanism is **load-bearing** and should not be removed without a
replacement.

When the caller script *is* PID 1 (e.g. `ark-server-start.py`), the
mirror is a no-op — writing to its own fd 1/2 is harmless.

## Subprocess stdio

`cmdrunner.Runner.Run` captures stdout and stderr into
`RunResult.Stdout` / `RunResult.Stderr` by default (`Capture: true`).
When `Capture` is `false`, the subprocess inherits the caller's stdio
streams directly. Used for long-running commands (image builds,
crontab installs) where the operator needs live output.

Inherited stdio must not run concurrently with a spinner. The spinner
mechanism and the inherited subprocess both want exclusive control of
the terminal; callers orchestrate this by suppressing the spinner for
the subprocess's duration.

## Container-side logging

All container runtime scripts emit through
`container/scripts/runtime/_logging.py`:

```python
emit_event(phase, msg)  # stdout + PID1 mirror; "[{phase}] {msg}"
emit_warning(phase, msg)  # stderr + PID1 mirror; "[{phase}] Warning: {msg}"
emit_error(phase, msg)  # stderr + PID1 mirror; "[{phase}] Error: {msg}"
```

Phases in use: `entrypoint`, `server`, `shutdown`, `log-rotation`,
`server-args`, `server-oom`, `stop`, `rcon`, `backup`, `archive`, `restore`,
and `ini-defaults` (the image build's INI generator). Integration tests assert on these
`[phase]` lines, so renaming a phase means updating its tests.

`no-bare-print-in-container` in `tests/test_logging_discipline.py` rejects
`print()` everywhere except `_logging.py`. A few places write raw stdio on
purpose:

- `ark-rcon.py` writes plain stderr: readiness polls call it every few
  seconds, and the CLI reads its stdout as the RCON reply.
- `ark-server-start.py` writes the crash log body raw after its
  `[server] Crash log detected …` line.
- `container-entrypoint.sh` (bash) echoes two lines before Python starts.

## JSON output contract

Each command that produces a structured result declares a schema struct
in `app/ark-cli/internal/core/json_output.go`. The text and JSON
renderers share one data source: the handler computes once, then either
prints text lines or marshals JSON. This keeps the two representations
from drifting.

Field names are part of the schema and stable. Field order and tag
semantics (`omitempty` vs always present) are enforced by committed
golden files under `testdata/`.

No doc currently promises a stable JSON schema to external consumers.
Golden-file tests track shape; promising stability is a future decision.

## Non-goals

- No additional log levels, handler formats, or transports beyond
  stdout/stderr + optional file tee. Container scripts have exactly three
  levels (event, warning, error) through `_logging.py`; the functions add
  the `Warning:`/`Error:` prefix so call sites never type it.
- No replacement of the spinner UI or two-phase start rendering.
- No rewrite of container-side Python to use the `logging` module.
- No change to the three-layer boundary: `pkg/ark` remains ignorant
  of the CLI's rendering primitives.

## Future work

- `cmd/ark-web/` (anticipated in `core/logging.go:11`) will inject a
  JSON-handler `*slog.Logger` to the same `pkg/ark` services. The
  domain packages need no changes — they already emit only through the
  injected logger.
- The `core.*` package-var seam in `internal/core/logging.go` works
  today but is fragile. A later cleanup may replace it with an
  `OutputSink` interface field on `AppDeps`. This does not affect the
  architecture described above.
