# Refactor Pass Playbook

Pass types, their order, and how to run them without growing the codebase.

## Pass Types

### 1) Optimization Pass

- Goal: better runtime or operational efficiency, same behavior.
- Typical work: fewer subprocess and file I/O calls; reuse parsed data instead
  of re-reading; tighter polling and retry loops.
- Success: lower runtime, CPU, memory or I/O; existing tests pass.
- Guardrails: no change to public CLI semantics; keep safety checks at
  process/filesystem boundaries.

### 2) Cognitive Complexity Reduction Pass

- Goal: functions under the lint thresholds:
  - Go: `gocyclo`, `min-complexity: 30` in [`.golangci.yml`](../../../.golangci.yml)
    (`_test.go` excluded).
  - Python: ruff `PLR0911`/`PLR0912`/`PLR0915` (return, branch and statement
    counts), enabled by `PL` in [`ruff.toml`](../../../ruff.toml).
- Typical work: flatten nested branches; early returns instead of deep
  conditionals; helpers for repeated branch patterns.
- Success: complexity at or under threshold; test behavior unchanged.
- Guardrails: prefer branch simplification over new helpers; no one-line
  helpers that only move code around.

### 3) Helper Implementation Pass

- Goal: remove duplication by extracting shared logic.
- Typical work: shared env parsing, retry/backoff, subprocess result
  formatting.
- Success: each helper used from 2+ call sites; fewer repeated blocks.
- Guardrails: no single-use helpers; keep helper scope narrow and
  domain-specific. Check for an existing helper first.

### 4) File Split Pass

- Goal: break large modules into focused units.
- Typical work: separate orchestration from utility logic; group by concern.
  Example: `container/scripts/runtime/ark_backup/` (`save_markers`,
  `rcon_save`, `archive_validation`, ...).
- Success: smaller files, clearer boundaries; entrypoints unchanged.
- Guardrails: split by responsibility, not line count; no circular imports.

### 5) LOC Reduction Pass

- Goal: fewer lines, same behavior and safety.
- Typical work: remove dead code; inline single-use wrappers; merge tiny
  helpers into callers; dedupe messages, constants and utility logic. Use the
  [Code Reduction Audit](#code-reduction-audit) checklist.
- Success: net negative LOC; no test regressions.
- Guardrails: keep filesystem/process/network error handling and validations
  that protect external state.

## Recommended Order

When all pass types are needed:

1. File Split Pass (only if file size is already a problem).
2. Helper Implementation Pass (shared logic only).
3. Cognitive Complexity Reduction Pass.
4. Optimization Pass.
5. LOC Reduction Pass (final cleanup and flattening).

Why: structure changes first make later edits safer; the final LOC pass
removes helper bloat the earlier passes added.

## Execution Process

### Step 0: Define Constraints Up Front

- Behavior that must not change.
- Test scope (unit, container, CLI, integration).
- Budget targets, e.g. `-100 LOC`, max complexity per function.

### Step 1: Baseline Snapshot

Capture before editing:

- Line counts of target files (`wc -l <files>`).
- Complexity findings, if that is a target:

  ```bash
  golangci-lint run --enable-only gocyclo ./app/ark-cli/... ./app/pkg/ark/... ./app/ark-web/... ./container/...
  ruff check --select PLR0911,PLR0912,PLR0915 .
  ```

- Pass/fail state of the tests for the target area.

### Step 2: Plan By Pass

Per pass: scope (files/functions), expected net LOC (`+`, `-`, neutral), and
the validation to run after it.

### Step 3: Execute One Pass At A Time

- One objective per pass.
- Each pass reviewable and measurable on its own.
- Validate before starting the next.

### Step 4: Validate

- Build and lint: `go vet`/`golangci-lint` for Go, `ruff check .` and
  `ruff format --check .` for Python (see
  [conventions](../reference/conventions.md)).
- Targeted tests for the touched code; integration tests when runtime-critical
  paths changed. See [run-tests.md](run-tests.md).

### Step 5: Delta Report

After each pass: files changed, net LOC delta, complexity delta (if tracked),
tests run and results.

## Operational Policy For Agent Sessions

This doc is the canonical playbook; AGENTS.md carries a summary and links
here. To keep agent sessions from growing the codebase:

1. Require an explicit pass mode per request (`complexity`, `optimization`,
   `loc-reduction`, ...).
2. Require a net LOC target for cleanup requests.
3. No new helpers unless used from 2+ call sites in the same change.
4. No new files unless explicitly requested.
5. End with a delta report (`added`, `deleted`, `net`, tests run).

Prompt suffix:

```text
Pass mode: LOC reduction.
Constraints: no behavior changes; no new files; no single-use helpers.
Success criteria: net negative LOC and tests pass.
Report: changed files + net LOC + tests run.
```

## Code Reduction Audit

Prompt/checklist for finding low-value complexity. Per finding: line range,
what it does, why it costs more than it gives, the simpler form.

Look for:

- **Wrappers** that only call one other function: call the target directly.
- **Single-use intermediate variables** named like what they hold: inline.
- **Error handling for impossible cases** the caller, type system or
  surrounding code already rules out.
- **Diagnostic variables** used only in a log or assert that isn't earning its
  keep: remove both.
- **Helpers built for one call site**: inline, delete the helper.
- **Docstrings longer than the function body**: one line, or none if the
  signature says it.
- **Retry/fallback logic** that duplicates the caller's.
- **Over-structured data** (struct, NamedTuple, dataclass, dict) passing 2–3
  values through one call: use plain return values.
- **Comments that restate the code** without a why.

Bar for removal: can it go without changing observable behavior or error
handling at system boundaries? If yes, cut it.
