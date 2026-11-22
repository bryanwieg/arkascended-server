# Conventions

Project-specific coding rules. Rationale lives in
[design-decisions.md](../explanation/design-decisions.md) and
[logging-architecture.md](../explanation/logging-architecture.md).

## Reuse before writing

- When a framework enforces a rule (output channels, test helpers, lint
  config), extend the framework. Don't route around it with raw
  `subprocess.run`, `fmt.Println` or a one-off test fixture.
- When no existing pattern fits, record why (code comment, commit message or
  [design-decisions.md](../explanation/design-decisions.md)), so the next
  person doesn't "fix" it back.

## CLI design

- Syntax: `ark-cli <verb> <object> [options]`.
- Targets are flags, never positional: `--instance` (existing instance),
  `--name` (thing being created, or a schedule), `--backup`, `--port`. Build the
  flag set with `core.NewCommandFlagSet` and parse with
  `core.ParseNoPositionalArgs`.
- Reuse existing flag names before adding new ones: `--restart` (not
  `--restart-now`), `--force`, `--json`, `--dry-run`, `--timeout`,
  `--compose-file`, `--no-<x>` to turn off a default.
- Register a command in `CommandRegistry`
  (`app/ark-cli/internal/cmd/registry_entries.go`). Handler signature:
  `func(*core.AppDeps, []string) error`.
- Handlers reach Docker, the filesystem and other external state only through
  `d.*` seams (`AppDeps`, `app/ark-cli/internal/core/deps.go`), and delegate
  domain logic to services in `app/pkg/ark/`. Steps:
  [add-cli-command.md](../how-to/add-cli-command.md).

## Compose editing

- Read compose files only through `ComposeService.LoadProject` and write them
  only through `ComposeService.SaveProject` (`app/pkg/ark/compose_service.go`).
  Direct file I/O skips project-name derivation ("project name must not be
  empty" on Compose v5) and the `goccy/go-yaml` AST edits that keep user
  comments and custom fields.
- Exception: adding or removing a service (`create instance`, `remove
  instance`, `restore backup --new-instance`, `enable`/`disable scheduler`).
  `SaveProject` refuses those, so edit the text with the `runtime` helpers
  (`InsertService`, `RemoveServiceBlock`, `RemoveVolumeEntry`) and write it
  with `compose.WriteComposeText`, which renames the edit over the file only
  after it loads.
- No hand-built YAML strings in handlers.

## Container access

- Reach container internals only through the runtime scripts in
  `container/scripts/runtime/`, run via `ComposeService.Exec` (running) or
  `RunOneOff` (stopped). No inline shell, `cat`, `mv`, `rm` or file parsing of
  container paths in Go. Why:
  [architecture.md](../explanation/architecture.md#container-script-boundary).
- A script returns results on stdout (JSON when the host parses it), problems
  on stderr through `_logging.py`, and a non-zero exit on failure. The caller
  reads `ExecResult`. A short CLI-facing script (`ark-rcon.py`, `ark-ini.py`)
  instead writes one plain `Error: …` message to stderr, which the CLI shows
  as is; it is a reply to the caller, not a container log line.
- The script, not the CLI, enforces its safety rules (`ark-ini.py set` refuses
  while the server runs, for example), so no caller can skip them.
- Known exceptions, to be moved behind scripts: `edit ini` (inline file prep
  before the editor), backup artifact cleanup (`rm -f`), and the
  helper-container backup glue
  (`sh -c "ark-backup.py … && touch … && sleep 600"`).

## Lint

- Python: `ruff check .` and `ruff format --check .` (config `ruff.toml`; CI
  runs ruff 0.17.5). Without a local ruff:
  `docker run --rm -v "$PWD:$PWD" -w "$PWD" ghcr.io/astral-sh/ruff:0.17.5 check .`
  - Pre-existing findings are baselined as `# noqa: <code>`. Fix and drop the
    noqa when you touch the line (`RUF100` flags stale ones). New `noqa`s need
    a reason in the comment.
  - `T20` rejects `print()` outside `tests/` and `run-tests.py`. Container
    scripts use `emit_event`/`emit_warning`/`emit_error`.
- Go: `golangci-lint` (config `.golangci.yml`; CI reports new findings only).
  Run before pushing:
  `golangci-lint run ./app/ark-cli/... ./app/pkg/ark/... ./app/ark-web/... ./container/...`
  (install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`).
  - List the modules: `go.work` has no root module, so `./...` matches
    nothing.
  - `forbidigo` rejects `fmt.Print*` (exempt: `app/ark-cli/internal/core/`).
  - Suppress only with `//nolint:<linter> // <reason>`.

## Error handling

- Sentinels `ark.ErrNotFound`, `ark.ErrConflict`, `ark.ErrTimeout`
  (`app/pkg/ark/errors.go`) for conditions callers branch on. Wrap with
  `fmt.Errorf("doing X: %w", err)`.
- `Main()` (`app/ark-cli/app.go`) prints a returned error verbatim via
  `core.Die(err.Error())` and adds no prefix. So the outermost error a handler
  returns must read `Error: <sentence-case message>.`, e.g.
  `Error: instance name is required.`
- Errors meant to be wrapped with `%w` omit the prefix. Validators in
  `app/pkg/ark/runtime/` and some domain services return already-prefixed
  errors; return those unwrapped.
- `core.Die(message string)` writes via `core.Warnln` and exits 1. It takes a
  string, not an `error`. Call it only from `Main()`/top-level init, never from
  handlers or services.
- Handlers return errors; they don't print them.

## Validation

Handlers validate user input before calling services; services validate their
own invariants. Validators in `app/pkg/ark/runtime/compose.go` (all take a
`label` used in the message, format `Error: <label> <detail>.`):

| Function | Checks |
|---|---|
| `ValidateComposeName(value, label)` | non-empty; `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$` |
| `ValidateYAMLScalar(value, label)` | no newline/tab, no `: `, no ` #` |
| `ValidatePassword(value, label)` | ASCII letters and digits only |
| `ParsePort(value, label)` | integer 1–65535 |
| `ParseNonNegativeInt(value, label)` | integer ≥ 0 |

## CLI output formatting

- Multi-row: `core.PrintTable(rows)` (space-aligned, ANSI-aware) or
  `core.PrintPipeTable(rows)` (pipe-separated, header rule).
- Single-item detail: `core.PrintDetailKV(rows)`.
- `--json`: define a tagged struct in
  `app/ark-cli/internal/core/json_output.go`, print with
  `core.PrintJSONOutput(v)`.

## Context threading

Service methods in `app/pkg/ark/` that touch containers, subprocesses or the
network take `ctx context.Context` first, so Ctrl+C and timeouts cancel them.
Handlers pass `d.Ctx`.

## Logging and CLI output

| Kind | Call through | Stream | In `-v FILE` log? |
|---|---|---|---|
| User text | `core.Infof` / `core.Infoln` | stdout | yes (`[cli-stdout]`) |
| Structured output | `core.PrintJSONOutput(v)` | stdout | no |
| Warning | `core.Warnf` / `core.Warnln` | stderr | yes (`[cli-stderr]`) |
| Error (terminal) | `core.Die(message)` | stderr + exit 1 | yes (`[cli-stderr]`) |
| Prompt | `core.Promptf` | stderr | no (transient) |
| Operational event | injected `*slog.Logger` (`Debug`/`Info`/`Warn`) | stderr (text handler) | no |
| Spinner frame | `compose` spinner (`start_progress_spinner.go`) | stdout (TTY only) | no (transient) |

Rules:

- Handlers never use `d.Logger.*` for user-visible text; that's
  `core.Infof`. slog is for operational events.
- `app/pkg/ark/` never writes to `os.Stdout`/`os.Stderr` and never imports
  `core`. Services return data and log via the injected `*slog.Logger`;
  handlers render.
- Enforcement:
  - `app/ark-cli/internal/core/logging_test.go`: slog goes to stderr, default
    stream routing of `Infof`/`Warnf`/`Warnln`/`Promptf`, JSON output.
  - `tests/test_logging_discipline.py`: no stdio in `pkg/ark`, no `core`
    import in `pkg/ark`, no `RunOptions{Capture: false}`, no
    `scanner.Text()` lines promoted to slog events, no bare `print()` in
    `container/scripts/runtime/`. Each has an allowlist for legacy sites.
  - `forbidigo` (see [Lint](#lint)) for `fmt.Print*` in the CLI.
  - Adding or relaxing a rule: update the enforcement and this section.

Container scripts write all output through
`container/scripts/runtime/_logging.py`: `emit_event(phase, msg)` (stdout),
`emit_warning(phase, msg)` / `emit_error(phase, msg)` (stderr; they add the
`Warning:`/`Error:` prefix, so leave it out of `msg`). All three mirror to
PID 1's fds so output shows in both `docker compose exec` and `docker logs`.

Rendering modes (verbosity, `--json`, `-v FILE`, TTY detection) and
rationale: [logging-architecture.md](../explanation/logging-architecture.md).

## Tests

- Shell out with the `run_cmd` fixture (`tests/helpers/diagnostics.py`) and
  poll with `wait_for` (`tests/helpers/infra.py`), not raw `subprocess` calls
  or sleep loops. They log commands and polls into the test artifacts.
- Placement, fixtures and markers:
  [test-architecture.md](test-architecture.md).

## Safety and defaults

- Default to repo-local paths for backups, restores and managed state.
- Temp dirs: `ark.TempDir(prefix)` (`app/pkg/ark/tmpdir.go`; `ARK_TMP_DIR`,
  else `./.ark-tmp/`), not `os.MkdirTemp`. Tests use `t.TempDir()`.
- No network access or new external dependencies unless the feature needs
  them.
- The server runs as `ark-user` (uid 3000), not root.
