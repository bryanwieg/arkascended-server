# Logging and CLI output architecture

Which channel each kind of output uses, and why. Channel-rule table and
enforcement: [conventions.md — Logging and CLI output](../reference/conventions.md#logging-and-cli-output).

## Core idea

Two channels:

1. **User-facing text**: stdout for results and progress; stderr for
   warnings, errors and prompts. Spinner-aware. Integration tests assert
   substrings on this channel.
2. **Operational events**: an injected `*slog.Logger` passed into
   `pkg/ark` services. Text handler on stderr in the CLI
   (`core.NewCLILogger`); level gates visibility. Never carries
   user-facing text.

`--json` and `--verbose` change how output renders, not which channel it
uses.

Layer boundary: `pkg/ark` services emit only through the injected logger.
They never write `os.Stdout`/`os.Stderr` or call the CLI's `core.*`
printers. Handlers render, services compute.

## Message kinds

Each output has one kind; the kind decides the channel.

| Kind | What it is | Audience |
|---|---|---|
| User text | status, results, progress, tables, detail blocks | human in terminal |
| Structured output | one JSON document for the command's result | scripts, dashboards |
| Warning | actionable problem; operation continues | human |
| Error | terminal failure | human |
| Prompt | synchronous input request | human |
| Operational event | what the system did internally (`cmd args=…`) | operator debugging |
| Spinner frame | transient terminal UI; never persisted | human (TTY only) |

## Why two channels instead of one

A single structured channel (consumer demuxes) was rejected:

- Integration tests match substrings in process stdout/stderr. A JSON
  envelope would turn every assertion into a decode.
- Operators pipe `ark-cli show list` into `grep`/`awk`. Readable text on
  stdout is the expected contract.
- slog stays useful for subprocess traces and domain events; keeping it
  separate stops it leaking into the human channel and keeps `-v`
  meaningful.

Cost: handlers must not send user text through slog, and domain services
emit only through slog. `tests/test_logging_discipline.py` and the lint
config enforce this (see
[conventions.md](../reference/conventions.md#logging-and-cli-output)).

## Rendering modes

### Format

`--json` is a per-command flag, defined only on commands with a
structured result (`show`/`list` commands). Elsewhere it fails flag
parsing as an unknown flag. With `--json`, stdout carries one JSON
document only.

### Verbosity

- Default: slog level `Info`.
- `-v` / `--verbose` (global, before the verb): slog level `Debug`, so
  subprocess traces show; `verboseLogf` lines (prefixed `[HH:MM:SS.mmm]`)
  are enabled on stderr.

### Log-file tee

There is no `--log-file` flag. The log file is an argument to the
verbose flag: `-v FILE`, `-v=FILE` or `--verbose=FILE` (append mode,
parent dirs created). A file without verbose mode is not possible.

The file receives:

- every `verboseLogf` line (same bytes as stderr);
- user text from `core.Infof`/`Infoln`, tagged `[cli-stdout]`;
- warnings from `core.Warnf`, tagged `[cli-stderr]`.

slog events are not written to the file; they go to stderr only. JSON
documents (`core.PrintJSONOutput`) and prompts are not mirrored.

`core.TaggedTeeWriter` (`internal/core/tagged_tee.go`) is a line-tagging
writer built for this tee but not yet wired in; the current mirror is
`mirrorCLIToVerboseFile` in `app/ark-cli/verbose.go`.

### TTY detection

Spinners turn off when stdout is not a character device, when
`TERM=dumb`, or with `--verbose`. Progress then prints as plain
`core.Infof` lines (e.g. `Starting ark server: asa1 started`).

## PID1 file descriptor mirror (load-bearing)

Scripts run via `docker compose exec` write to the exec caller's stdio,
not to `docker logs`. `_logging.py` writes each line a second time to
`/proc/1/fd/1` or `/proc/1/fd/2` so it shows in both.

Integration tests depend on this: they assert `[backup] starting backup
workflow` in both the exec output and `docker logs`. Removing the mirror
breaks only the `docker logs` side, silently. Do not remove it without a
replacement.

The mirror is skipped when the caller's stream already is PID 1's (PID 1
itself, or a child sharing its output such as the stop script), so lines
are not doubled in `docker logs`. It is best-effort: `/proc/1` errors are
ignored.

## Subprocess stdio

Two runners: `cmdrunner.Runner.Run` (`RunOptions`) and
`Runtime.RunCommand` (`ark.CommandOptions`). Both capture stdout/stderr
into the result only when `Capture: true`; the zero value inherits the
CLI's stdio. Inherit is used where the operator must see or answer the
subprocess, e.g. `RuntimeHelper.RunWithPrivs` (sudo password prompt);
`cmdrunner.RunInteractive` always inherits. Inherited output bypasses
the verbose tee.

An inheriting subprocess must not run while a spinner is active; both
want the terminal. Callers keep them apart.

## Container-side logging

All container runtime scripts emit through
`container/scripts/runtime/_logging.py`:

```python
emit_event(phase, msg)  # stdout + PID1 mirror; "[{phase}] {msg}"
emit_warning(phase, msg)  # stderr + PID1 mirror; "[{phase}] Warning: {msg}"
emit_error(phase, msg)  # stderr + PID1 mirror; "[{phase}] Error: {msg}"
```

Phases in use: `entrypoint`, `server`, `server-args`, `server-oom`,
`shutdown`, `stop`, `log-rotation`, `rcon`, `backup`, `archive`,
`restore`, `cluster-backup`, and `ini-defaults` (image build INI
generator). Tests assert on `[phase]` lines; renaming a phase means
updating its tests.

`no-bare-print-in-container` in `tests/test_logging_discipline.py`
rejects `print()` in `container/scripts/runtime/` except `_logging.py`.
Deliberate raw stdio:

- `ark-rcon.py`: RCON reply on plain stdout (the CLI parses it), usage
  and errors on plain stderr. Readiness polls call it every few seconds.
- `ark-server-start.py`: crash log body written raw after its
  `[server] Crash log detected …` line.
- `container-entrypoint.sh` (bash): one `Entrypoint: …` line on stderr
  before Python starts.

## Scheduler service log

`ark-cli run scheduler` runs as the `ark-scheduler` container, so its
user-facing lines are the container log (`docker compose logs ark-scheduler`);
it has no TTY, so no spinner. Its loops live in `pkg/ark`
(`schedule.Runner`, `health.Monitor`) and take an injected `Logf` that the
handler wires to `core.Infof`: each line is meant for the operator reading the
log (`START`/`SUCCESS`/`FAIL` per run, `Restarting <instance>: ...`). The
monitor logs its `Health: ...` line only when the set of failing checks
changes, so a steady state does not add a line a minute. The health report
itself goes to a file and `GET /health`, not to the log.

## JSON output contract

Each command with a structured result has a struct in
`app/ark-cli/internal/core/json_output.go`. The handler computes once,
then prints text or marshals the struct, so the two forms cannot drift.
`check scheduler --json` prints `health.Report` (alias `SchedulerHealthJSON`),
the same document as `health.json` and `GET /health`.

Field names are part of the schema. Tests (`*_json_test.go` in
`app/ark-cli`, `internal/show`, `internal/schedule`) decode the output
into the struct, check fields, and check no text-mode output leaks into
stdout. There are no golden files. No doc promises schema stability to
external consumers yet.

## ark-web

`app/ark-web` builds its own `*slog.Logger` (JSON handler, stderr, level
`Info`) and injects it into the same `pkg/ark` services. It does not use
the CLI's `core.*` printers; the services needed no changes.

## Non-goals

- No extra log levels, handler formats or transports beyond
  stdout/stderr and the verbose log file. Container scripts have three
  levels (event, warning, error); the functions add the
  `Warning:`/`Error:` prefix.
- No replacement of the spinner UI or two-phase start rendering.
- No move of container-side Python to the `logging` module.
- `pkg/ark` stays ignorant of CLI rendering primitives.

## Possible cleanup

The `core.*` package-var seam in `internal/core/logging.go` (assigned in
`app/ark-cli/utils.go`) works but is fragile. It could become an
`OutputSink` field on `AppDeps` without changing the design above.
