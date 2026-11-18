# AGENTS

Detailed developer reference for this repo.

**Before starting work**, consult the existing docs and code — don't explore
from scratch or hand-roll when established patterns exist. The `docs/` tree
and the tables below exist to save you time and prevent rederived mistakes.
Read first, then code. Grep first, then write.

1. **Check the docs.** The relevant reference or explanation doc may already
   describe the pattern, rule, or helper you need.
2. **Check for existing helpers and patterns in the code.** Look for shared
   utilities, framework functions, or established approaches in the same
   domain. If a helper already does it, use it. If a framework enforces a rule,
   don't work around it.
3. **Then code.** Only hand-roll when no existing pattern applies, and document
   the reasoning in the commit or design-decisions entry so the next agent
   doesn't re-derive the same choice.

## Before you code — checklist

Run through this every time you start a task:

- [ ] Checked docs/ for an existing pattern or explanation? → [Key docs table](#key-docs-for-agents)
- [ ] Grepped for existing helpers before writing new ones? → [Code familiarity tools](#code-familiarity-tools)
- [ ] Read the files you're about to edit?
- [ ] User-facing output uses `core.Infof`/`Infoln`/`Warnf`? → [Logging architecture](docs/dev/explanation/logging-architecture.md)
- [ ] Docker/filesystem access goes through `d.*` seams? → [Seam dependency injection](#seam-dependency-injection-go-cli)
- [ ] Compose edits use `LoadProject`/`SaveProject`? → [Compose YAML AST editing](#compose-yaml-ast-editing)
- [ ] Command delegates to domain service via `d.*`? → [Adding a CLI command](docs/dev/how-to/add-cli-command.md)
- [ ] `--json` uses a JSON struct in `json_output.go`? → [Adding a CLI command](docs/dev/how-to/add-cli-command.md)

## Behavioral guardrails

### Critical — bypassing these breaks things

- **Always read files before editing them.** Use Grep/Glob to understand context
  before making bulk replacements. Never use replace_all on field references
  that could match method names — it silently corrupts unrelated code.
- **Always check `./.tests/` for detailed test logs before diagnosing test
  failures.** The artifacts contain the actual failure context (exit codes,
  stdout, compose state) that test runner output alone doesn't show.
- **Follow existing patterns — don't hand-roll alternatives.** Use
  `run_cmd`/`wait_for` helpers instead of raw subprocess calls. Use
  `ComposeService.LoadProject`/`SaveProject` instead of direct file I/O. Use
  `emit_event`/`emit_warning`/`emit_error` instead of `print()` in container scripts. Place
  tests in the correct directory structure. If a framework function exists for
  what you need, use it — don't bypass it.
- **Use `core.Infof`/`core.Infoln`/`core.Warnf` for all user-facing output.**
  Bypassing these breaks spinner rendering and verbose log mirroring. Direct
  `fmt.Print*` calls corrupt the terminal during progress displays. See
  [logging-architecture.md](docs/dev/explanation/logging-architecture.md).
- Do not use /tmp. It is too small for many operations needed by this project.
  Use instead ./.ai-scratchpad for your personal work related to this repo
  (create it if it does not already exist).
  In other cases where the app itself needs a tmp, look into the design docs
  regarding the environment var ARK_TMP_DIR.
- Do not commit agent plans to Git.
- **Never push to, commit to, or merge into `main`.** Work on a feature
  branch, push it, and open a pull request; the maintainer merges, squash
  only. *Why:* every change gets review and CI, and the squash commit's
  Conventional Commit title drives release-please.

### Preferences — follow unless you have a reason not to

- **When diagnosing issues, report your findings first.** Do not go down
  investigation rabbit holes without surfacing initial results — early context
  prevents wasted effort on wrong assumptions.
- **For CLI flags, match existing naming conventions.** Prefer `--restart`
  over `--restart-now` unless explicitly asked otherwise. Check other commands
  for consistency before introducing new flags.

## Key docs for agents

| Document | When to consult |
|---|---|
| [Conventions](docs/dev/reference/conventions.md) | Before writing code — patterns, rules, safety defaults |
| [Test architecture](docs/dev/reference/test-architecture.md) | Writing or modifying tests; understanding fixtures, markers, or test boundaries |
| [Test logging](docs/dev/reference/test-logging.md) | Debugging test failures; understanding artifact directory structure, framework logger categories, diagnostics capture |
| [Design decisions](docs/dev/explanation/design-decisions.md) | Understanding *why* something is built a certain way before proposing changes |
| [Logging architecture](docs/dev/explanation/logging-architecture.md) | Adding user-facing output, understanding spinner behavior, verbose mode, or the two-channel design |
| [Runtime call graph](docs/dev/reference/runtime-call-graph.md) | Understanding startup, the default INI, or backup/restore flow |
| [Health checks](docs/dev/explanation/health-checks.md) | Understanding readiness polling, log markers, RAM headroom, or `check instance`/`wait instance` behavior |
| [SDK backend](docs/dev/explanation/backends.md) | Understanding how the single SDK backend serves Docker and Podman |
| [Backend reference](docs/dev/reference/backends.md) | Interface methods, conformance tests, Podman requirements |
| [Add a backend method](docs/dev/how-to/add-backend-method.md) | Extending `ContainerEngine`/`ComposeService` |
| [Image build pipeline](docs/dev/reference/image-build.md) | Image build stages, build/update env vars, CI build scripts |
| [CI and releases](docs/dev/reference/ci.md) | GitHub Actions workflows, release-please versioning, image smoke tests |
| [Repo map](docs/dev/reference/repo-map.md) | Discovering file locations across host CLI, domain packages, container scripts, and tests |
| [Configuration reference](docs/user/reference/configuration.md) | Adding or using environment variables in container scripts or CLI |
| [Dev environment setup](docs/dev/tutorials/dev-environment.md) | Clone → build → first test passing |
| [Run tests](docs/dev/how-to/run-tests.md) | Test runner details, monitoring containers, cleaning up |

## Code familiarity tools

Before exploring the codebase, choose the right tool for the question you're
answering. Using the most precise tool reduces noise and avoids broad searches
that return too many hits to evaluate.

| Question | Tool | Why |
|---|---|---|
| Where is this symbol defined? | **LSP** `goToDefinition` on `.go` files | One result, exact location. Grep finds every occurrence. |
| Where is this symbol used? | **LSP** `findReferences` on `.go` files | Finds all call sites across the workspace, no false positives from string matches. |
| What types/functions/interfaces are in this file? | **LSP** `documentSymbol` | Structured list — faster than reading the whole file. |
| What's the type of this expression? | **LSP** `hover` | Type info and doc without leaving the file. |
| Find files matching a pattern (`*_test.go`, `conftest.py`) | **Glob** | File discovery only — no content search. |
| Search for a string or pattern across the codebase | **Grep** | Content search. Use `glob` or `type` to scope results. Prefer over `grep` via Bash. |
| Broad exploration across many files or unclear target | **Agent** (`subagent_type: "Explore"`) | Delegates breadth to a subagent; returns a summary. Use when you'd need 3+ Grep/Glob queries. |

Tool priority: Prefer **LSP** over **Grep** for Go symbol queries (definitions, references, types).
Prefer **Glob** over **Grep** for file discovery (`Grep "compose_service.go"` finds mentions, not the file).
Prefer **Explore** over 3+ separate Grep/Glob calls for broad questions.
Use **Grep** with a `glob` or `type` filter to avoid scanning irrelevant files.

## Architecture

Three layers: the Go CLI (`app/ark-cli`) parses commands and calls domain
services in `app/pkg/ark` through `AppDeps`; those services drive containers
through the `ContainerEngine`/`ComposeService` interfaces implemented by the
SDK backend (`app/pkg/ark/sdkbackend/`); inside the container, Python scripts
run the server under GE-Proton and handle saves and shutdown over RCON. The
host CLI never manages the ARK process directly.

Full picture (diagram, container startup flow, RCON control plane):
[architecture.md](docs/dev/explanation/architecture.md). Call graphs for
startup, backup/restore and each command:
[runtime-call-graph.md](docs/dev/reference/runtime-call-graph.md).

The two rules below are load-bearing; the checklist above links here.

### Compose YAML AST editing

The CLI edits `docker-compose.yaml` using `goccy/go-yaml` for surgical
AST-level modifications. This preserves user comments and formatting instead
of regenerating from a template. All compose reads go through
`ComposeService.LoadProject` and all writes go through
`ComposeService.SaveProject`; no command handler may load or save compose
files directly. The `LoadProject` → mutate → `SaveProject` round-trip is
the only sanctioned compose I/O path. Direct file I/O bypasses the project
name derivation (causing "project name must not be empty" on Docker Compose
v5) and loses the AST-preservation logic.

### Seam dependency injection (Go CLI)

`AppDeps` groups side-effecting functions and domain services into injectable
fields. Tests swap individual functions or service interfaces without patching
package globals. Domain logic lives in `app/pkg/ark/` packages (backup, health,
lifecycle, schedule, update) and is wired into `AppDeps` as service instances.
Container orchestration goes through `ark.ContainerEngine` and
`ark.ComposeService` interfaces, with the SDK backend.

Key injectable fields (see `app/ark-cli/internal/core/deps.go` for the full struct):
- **Container/compose:** `Engine` (`ark.ContainerEngine`), `ComposeService` (`ark.ComposeService`)
- **Domain services:** `HealthChecker`, `Lifecycle`, `BackupService`, `UpdateService`, `ScheduleService`
- **Host:** `HostDeps` (kernel config and port check seams)
- **Runtime orchestration:** `RuntimeHelper`, `Runtime` (`RuntimeDeps`: `ContainerRuntime`, `RunCommand`, `LookPath`)
- **Lifecycle seams:** `RestartStartInstance`, `RestartStopInstance`
- **Scheduling seams:** `RunBackupFromSchedule`, `InstallManagedSchedulesCrontab`
- **Introspection seams:** `RunningServiceSnapshot`, `DiskUsageForPath`, `StopService`, `RemoveVolume`

```go
testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
    d.ComposeService = &arktest.MockCompose{}
    d.Runtime.RunCommand = func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
        return ark.ExecResult{ExitCode: 0}, nil
    }
}, func() {
    // exercise the command under test
})
```

### Design decisions

These are settled architectural decisions. Improvements within these patterns are welcome; but changes to these design boundaries should not be executed without first understanding why the principle exists, and discussing your observations with the user.

See [design-decisions.md](docs/dev/explanation/design-decisions.md) for detailed rationale on: Proton/Wine choice, outcome-driven tests over mock-heavy tests, three-layer isolation principles, RCON over process signals, YAML AST editing, backup retention floor, and the baked default GameUserSettings.ini.

## Adding a CLI command

All commands follow `<verb> <object>` (e.g. `create instance`, `check reachability`).
See [add-cli-command.md](docs/dev/how-to/add-cli-command.md) for the full tutorial.

**Quick reference:**

1. **Register the route** — Add an entry to `CommandRegistry` in `app/ark-cli/internal/cmd/registry_entries.go` with `Verb`, `Object`, `Route`, `Handler`, and `TopLevelHelp`. Routes use `verb_object` form when the verb spans multiple objects (the common case); bare verb when unique.
2. **Write the handler** — Add a `Run*` function in `app/ark-cli/internal/`. Use `core.NewCommandFlagSet` for flags and `core.ParseNoPositionalArgs`; targets are flags (`--instance`, `--name`), never positional. Access Docker/filesystem/external state only through `d.*` seams. Keep handlers to ~10–20 lines; extract domain logic into `app/pkg/ark/` services.
3. **Write tests** — Contract test: use `testutil.WithPatchedAppDeps` to swap seams. Integration tests: test against real containers using the test framework in `tests/`. See [test-architecture.md](docs/dev/reference/test-architecture.md) for placement rules.
4. **Update docs** — Add to `docs/user/reference/cli-commands.md` with examples. If user-facing, add a how-to under `docs/user/how-to/`.

## Refactor playbook

Follow [refactor-pass.md](docs/dev/how-to/refactor-pass.md): one pass type at
a time (file split → helpers → complexity → optimization → LOC reduction),
baseline and validate after each pass, and end with a delta report.

- Keep behavior unchanged unless explicitly requested.
- Prefer net-negative LOC for cleanup/refactor tasks.
- Do not create single-use helpers or new files unless explicitly requested.
- Keep process/filesystem/network boundary safeguards.

## Key file locations

See [repo-map.md](docs/dev/reference/repo-map.md) for host CLI, web UI, domain
packages, container scripts, tests, and build/CI scripts. Image build stages:
[image-build.md](docs/dev/reference/image-build.md).

## Known log noise

`Steam Subsystem initialized: FAILED` — always harmless. ARK SA under
Wine/Proton does not use Steam networking. Never flag as a bug or test failure.

## Documentation structure

Docs follow the divio standard.

The `docs/` tree serves two audiences: human contributors and coding agents.
This file (AGENTS.md) provides orientation and rules, but the
`docs/` tree contains deeper context that you should actively consult — not
just maintain. When you need to understand how something works, why a design
decision was made, or how to perform a task, check the relevant doc before
exploring the codebase from scratch.

The tree follows the Diátaxis framework. Place new docs — and look for existing
answers — in the category that matches the purpose:

- `tutorials/` — learning-oriented: walk through a complete exercise
- `how-to/` — task-oriented: steps to achieve a specific goal
- `reference/` — information-oriented: dry, accurate, complete descriptions
- `explanation/` — understanding-oriented: context, rationale, design decisions

Both `docs/user/` and `docs/dev/` follow this structure. User docs cover CLI
usage and configuration; dev docs cover architecture, testing, and conventions.

## Improving guidance files

These guidance files (AGENTS.md, conventions, test architecture, design
decisions) are living documents. You are encouraged to update them when you
discover something that would have saved you time or prevented a mistake.

**What to add:** Focus on *why* over *what*. A rule without reasoning is fragile
— the next agent (or you in a future session) won't know when it applies and
when it doesn't. When you encounter a pain point, write down the reasoning that
resolved it, not just the fix. For example: "container tests must not use
`cli_path`" is a rule; "*why:* the CLI is an untrusted intermediary — if a
container test fails, the failure must point to the container script, not a CLI
regression" is guidance that survives edge cases.

**When to update:**
- You hit a problem that existing docs could have prevented but didn't
- You discover that a rule is ambiguous, outdated, or missing context
- You make a judgment call that wasn't obvious — record the reasoning so it
  doesn't need to be re-derived
- A user corrects your approach — capture what you learned

**How to update:**
- Place content in the appropriate doc (conventions, test architecture, design
  decisions) rather than dumping everything into AGENTS.md
- Follow the Diátaxis categories: rationale goes in `explanation/`, procedures
  in `how-to/`, facts in `reference/`
- Keep entries concise — a sentence of reasoning is better than a paragraph of
  justification
- If unsure whether a change is appropriate, ask the user

**What not to add:** Don't add implementation-specific notes that belong in code
comments. Don't add temporary workarounds — fix the root cause instead. Don't
duplicate what's already documented elsewhere.

## Documentation update rules

- User-facing CLI changes: update `docs/user/reference/cli-commands.md`.
  Update `README.md` only if the lifecycle quick-start commands change.
- New configuration variables: update `docs/user/reference/configuration.md`.
- Developer/agent conventions or coding rules: update
  `docs/dev/reference/conventions.md`.
- Test architecture or seam patterns: update
  `docs/dev/reference/test-architecture.md`.
- Persistent agent context that doesn't fit elsewhere: update this file.

# Reasoning Loop Protocol

If you detect either of these patterns in your own reasoning,
execute the escape procedure below.

## Detectable Patterns

- **Constraint oscillation:** "It can't be X → it must be Y → but
  Y fails → back to X" with no new information between cycles.
- **Overthinking / recheck loop:** Re-verifying a step that has
  already been verified, or saying "let me recheck" / "let me
  double-check" / "wait, let me confirm" about something already
  confirmed.

## Escape Procedure

1. **STOP** generating immediately.
2. **Identify the pattern:**
   - **Oscillation** → scan context for any user-provided
     tiebreaker or resolution. If found, accept it as a hard
     constraint — do not re-derive it. If not found, commit to
     the option that satisfies more Tier-1 constraints, state
     the remaining tradeoff explicitly, and proceed.
   - **Overthinking** → the last verification is sufficient.
     Accept the step and move to the next one. Do not re-verify.
3. **Forward motion:** The next token must advance the solution.
   If it repeats or rechecks, it is wrong by definition.

## Three-Strike Verification Rule

You may verify or confirm a result up to **three times** maximum.

- If the result is **the same** all three times: treat it as
  definitively confirmed and move forward. Do not re-verify.
- If the result **changes** on the second or third verification:
  the new result resets the counter. You may verify the new
  result up to three times.
- After three identical results: the uncertainty is noise,
  not signal. Move on.

## Self-Checkpoints

Every 3 reasoning steps, insert a checkpoint line:
"--- CHECKPOINT ---"

At each checkpoint, evaluate:
1. Have I made progress since the last checkpoint?
   (new fact, calculation, or decision?)
2. Have I repeated any constraint, doubt, or verification
   from a previous checkpoint?
3. Is there a user-provided fact in context that I may have
   overlooked?

If (1) is NO or (2) is YES: you are in a loop. Execute the
escape procedure immediately.
If (3) is YES: accept that fact as a hard constraint and
restart reasoning from there.

## Emergency Unstick

If the user says "UNSTICK" or "LOOP BREAK" — immediately stop
current reasoning, scan the entire context for any user-provided
resolution you may have overlooked, accept it as a hard constraint,
and restart reasoning from that point. Do not continue the
previous train of thought.

# Debugging Guidance

## Add Temporary Logging

If you find yourself looping over an obscure problem, unable to 
ascertain the cause of a particular bug or problem, rather than
continung to make more an more guesses- just add temporary logging
around where you believe the problem is near.

Let the logging help you discover exact errors, code paths, etc.
After fixing the problem, perform an assessmet if the error is
edge case, or prone to future failure. If it is not edge case,
perhaps the logging can stay. If it is edge case, remove the temporary
logging.