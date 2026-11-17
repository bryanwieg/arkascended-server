# Conventions

Coding conventions, design rules, and safety defaults for this repo.

## Research before implementing

Before writing new code for any task, check for existing patterns and
utilities first. This repo has shared helpers, framework functions, and
established approaches across every domain — tests, logging, compose
editing, CLI commands, container scripts. Reusing them keeps the codebase
consistent and avoids the drift that comes from hand-rolling solutions that
duplicate or contradict what already exists.

- **Grep before you write.** If you need a helper, a pattern, or an approach,
  search for it first. The answer often already exists in the codebase or
  docs.
- **Follow established patterns.** When a domain has a documented pattern
  (test helpers, logging channels, compose editing, CLI handler structure),
  use it. Don't invent a parallel approach because the existing one wasn't
  discovered.
- **Extend, don't bypass.** When a framework enforces a rule (logging
  channels, test helpers, enforcement hooks), the fix is to extend the
  framework — not to work around it with raw `subprocess.run`, direct
  `fmt.Println`, or a bespoke test fixture.
- **Document exceptions.** When no existing pattern applies and you must
  create something new, note the reasoning. A comment in the code, a
  design-decisions entry, or a commit message explaining *why* the existing
  approach didn't fit prevents the next agent from re-deriving the same
  choice — or worse, "fixing" it to match a pattern it was deliberately
  departing from.

## CLI design

- Command syntax is `ark-cli <verb> <object> [options]` — keep new commands
  consistent with this pattern.
- Targets are flags, not positional arguments: `--instance` for an existing
  instance, `--name` for something being created or a schedule, `--backup`,
  `--port`. Parse with `core.ParseNoPositionalArgs`.
- Route a new command by adding an entry to `CommandRegistry` in
  `app/ark-cli/internal/cmd/registry_entries.go`. The handler signature is
  `func(*core.AppDeps, []string) error`.

## Compose editing

- All compose file reads go through `ComposeService.LoadProject`; all writes
  go through `ComposeService.SaveProject` (see `app/pkg/ark/compose_service.go`).
  The `LoadProject` → mutate → `SaveProject` round-trip is the only sanctioned
  compose I/O path. Do not load or save compose files directly — direct file
  I/O bypasses project name derivation (causing "project name must not be
  empty" on Docker Compose v5) and loses the AST-preservation logic.
- Do not hand-roll YAML string manipulation in command handlers. Hand-rolled
  strings break YAML quoting, produce unreviewable diffs, and bypass the
  AST-preservation logic that keeps user comments and custom fields intact.
- Validate all inputs that touch compose service names or filesystem paths
  before using them. Invalid names produce confusing downstream errors in
  Docker API calls or compose edits that are hard to trace back to the source.

## Code style

- Add brief inline comments to non-obvious code blocks so the intent is clear
  at a glance. The bar for "non-obvious" is whether a reader unfamiliar with
  the domain would need more than a few seconds to understand the purpose.
  Do not comment code that is self-evident from naming and structure.
- Keep files focused on a single responsibility and small enough to read as a
  unit. Large "jack-of-all-trades" files that mix unrelated concerns are harder
  to navigate, review, and test — split by responsibility when a file loses
  that focus. Apply the same principle to functions: each should do one thing
  clearly. Extract helpers when logic is reused (2+ call sites), but do not
  create single-use helpers or new files just to hit a line target.
- Python code must pass `ruff check .` and `ruff format --check .` (config:
  `ruff.toml`; CI enforces both). Without a local ruff, run the image:
  `docker run --rm -v "$PWD:$PWD" -w "$PWD" ghcr.io/astral-sh/ruff:0.17.5 check .`
  Existing findings were baselined as `# noqa: <code>` comments when ruff was
  adopted; when you touch such a line, fix it and drop the noqa (`RUF100`
  flags stale ones). Don't add new `noqa`s without a reason in the comment.
  `T20` rejects `print()` in container scripts: use
  `emit_event`/`emit_warning`/`emit_error`.
- Go code must pass `golangci-lint` (config: `.golangci.yml`; CI fails on new
  findings only). Run it before pushing:
  `golangci-lint run ./app/ark-cli/... ./app/pkg/ark/... ./app/ark-web/...`
  (install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`).
  List the modules explicitly — `go.work` has no root module, so `./...`
  matches nothing. `forbidigo` rejects `fmt.Print*` because user output must go
  through `core.Infof`/`Infoln`/`Warnf`. Suppress a finding only with
  `//nolint:<linter> // <reason>`.

## Prefer immutability

Immutable-style code is easier to follow because each variable has one meaning
for its entire lifetime — no need to trace reassignments to understand the
current value. This reduces bugs from unintended mutation and makes diffs
easier to review.

Guidelines (apply judgment across Go, Python, and Bash):

- Give variables their final value at the point of declaration when practical.
- Prefer early returns, guard clauses, or single expressions over declaring a
  variable and reassigning it through branches.
- Use language-appropriate constant forms (`const` in Go, `ALL_CAPS` in Python,
  `readonly` in Bash) for values that truly never change.
- When mutation is the clearest approach — such as accumulating in a loop,
  defaulting a None parameter in Python, or building a string incrementally —
  use it. Clarity wins over dogma.

## Error handling

- **Sentinel errors** for cross-boundary conditions: use `ark.ErrNotFound`,
  `ark.ErrConflict`, and `ark.ErrTimeout` (defined in `app/pkg/ark/errors.go`)
  when the caller needs to branch on the error type. Wrap them with
  `fmt.Errorf("doing X: %w", err)` to add context.
- **User-facing errors** in handlers: prefix with `"Error: "` and use sentence
  case. Example: `"Error: instance name is required."` This matches the CLI
  output contract that errors start with `Error:`.
- **Domain services** return errors without the `"Error: "` prefix. The handler
  adds the prefix when displaying to the user.
- **`core.Die(message string)`** logs the formatted message and exits with
  code 1. The caller formats the message — it does not accept an `error` value.
  Use only in `Main()` or top-level initialization — never inside domain
  services or handlers that can be tested.
- **Validation errors** should be returned from the handler, not printed directly.
  Let the `Main()` function handle error display.

## Validation

- **Validate at the boundary.** Handlers validate user inputs before calling
  domain services. Domain services validate invariants that must hold regardless
  of the caller.
- **Existing validators** are in `app/pkg/ark/runtime/`:
  - `ValidateComposeName(value, label)` — reject invalid compose service names
  - `ValidateYAMLScalar(value, label)` — reject values with newlines or special chars
  - `ValidatePassword(value, label)` — enforce minimum password requirements
  - `ParsePort(value, label string)` — parse and validate port numbers, using `label` in error messages
  - `ParseNonNegativeInt(value, label)` — parse non-negative integers
- **Error format** for validation: `"Error: <label> <detail>."` — e.g.,
  `"Error: instance name may only contain ..."`.

## CLI output formatting

- **Text tables** for multi-row data: use `core.PrintTable(rows)` (space-aligned,
  ANSI-aware) or `core.PrintPipeTable(rows)` (pipe-separated with horizontal rule).
- **Key-value detail** for single-instance info: use `core.PrintDetailKV(rows)`.
- **JSON output**: add a `--json` boolean flag. Define a struct with `json` tags in
  `app/ark-cli/internal/core/json_output.go` and call
  `core.PrintJSONOutput(v)`.
- **Never use `fmt.Print*`** for user-facing output. Always use
  `core.Infof`/`core.Infoln`/`core.Warnf` to respect the output channel discipline.

## Context threading

All domain services in `app/pkg/ark/` accept `context.Context` as the first
argument for signal-aware cancellation — when the user hits Ctrl+C or a
timeout expires, the context is cancelled and long-running operations (Docker
commands, health checks, backup) can exit promptly rather than hanging
indefinitely. Command handlers thread the context from the CLI entry point
through `d.Ctx` (the context field on `*core.AppDeps`). When adding a new domain
service method, include `ctx context.Context` as the first parameter.

## Logging and CLI output

The CLI uses two output channels, pinned to message kind:

| Kind | Call through | Stream | In `--log-file` tee? |
|---|---|---|---|
| User text | `core.Infof` / `core.Infoln` | stdout | yes (`[cli-stdout]`) |
| Structured output | `core.PrintJSONOutput(v)` | stdout | yes (`[json]`) |
| Warning | `core.Warnf` / `core.Warnln` | stderr | yes (`[cli-stderr]`) |
| Error (terminal) | `core.Die(message)` | stderr + exit≠0 | yes (`[cli-stderr]`) |
| Prompt | `core.Promptf` | stderr | no (transient) |
| Operational event | injected `*slog.Logger` (`Debug`/`Info`/`Warn`) | stderr (text handler) | yes (`[event]`) |
| Spinner frame | `compose.spinnerTerminal*` | stdout (TTY only) | no (transient) |

Load-bearing rules:

- **CLI handlers never call `d.Logger.*` for user-visible text.** The
  slog logger is for operational events. If you are tempted to write
  `d.Logger.Info("status: stopped")` you meant `core.Infof("status:
  stopped\n")`.
- **`app/pkg/ark/` services never write to `os.Stdout` / `os.Stderr`
  directly and never call `core.*`.** Domain packages return data;
  handlers render it. Services emit operational events via the injected
  `*slog.Logger` only.
- **Enforcement:** `logging_test.go` validates JSON-output conventions
  and channel routing (slog to stderr, not stdout).
  `tests/test_logging_discipline.py` enforces the mechanically-checkable
  rules: no direct stdio in domain packages, no `core` imports in
  `pkg/ark`, and no bare `print()` in container scripts (with allowlists
  for legacy sites pending migration). Other discipline rules (e.g., no
  `fmt.Print*` in CLI handlers) are code-review conventions. When adding
  a new rule or relaxing an existing one, update both the enforcement
  tests and this table.

Container scripts emit all output through
`container/scripts/runtime/_logging.py` — use `emit_event(phase, msg)`
for stdout, `emit_warning(phase, msg)` and `emit_error(phase, msg)` for
stderr (they add the `Warning:`/`Error:` prefix; don't type it in `msg`).
`emit_event`, `emit_warning` and
`emit_error` mirror output to the PID1 file descriptor so
it appears in both `docker compose exec` and `docker logs`. Do not use
`print()` directly in container scripts — the `no-bare-print-in-container`
rule in `test_logging_discipline.py` catches violations; only `_logging.py`
itself is exempt.

For rationale, alternative designs considered, and the full rendering-mode
description (verbosity, `--json`, `--log-file`, TTY detection),
see
[explanation/logging-architecture.md](../explanation/logging-architecture.md).

## Safety and defaults

- Default to repo-local paths for backups, restores, and managed state.
- Validate inputs that touch the filesystem or compose services at the
  boundary; do not rely on downstream code to catch bad values.
- Avoid network access and external dependencies unless the feature explicitly
  requires them.
- Follow security best practices in Dockerfile and container scripts; the
  server runs as `ark-user`, not root.

## Temp Directory Pattern

Use `ark.TempDir(prefix)` from `pkg/ark/tmpdir.go` instead of `os.MkdirTemp`.
Test code should use `t.TempDir()` for isolation.