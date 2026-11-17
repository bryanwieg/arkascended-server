# Refactor Pass Playbook

This document defines the common pass types used in this repo and the process
for executing them with predictable outcomes (especially controlling codebase
growth).

## Pass Types

### 1) Optimization Pass

Goal:
- Improve runtime or operational efficiency without changing behavior.

Typical work:
- Reduce subprocess/file I/O calls.
- Reuse parsed data instead of re-reading.
- Tighten polling loops and retry behavior.

Primary success signals:
- Lower runtime duration, CPU, memory, or I/O.
- No behavior regressions in existing tests.

Guardrails:
- Do not change public CLI semantics.
- Keep safety checks at process/filesystem boundaries.

### 2) Cognitive Complexity Reduction Pass

Goal:
- Reduce function complexity to pass static analysis thresholds (for example gocyclo or a linter).

Typical work:
- Flatten nested branches.
- Introduce helper functions for repeated branch patterns.
- Replace deeply nested conditionals with early returns.

Primary success signals:
- Function complexity at or below threshold.
- Test behavior unchanged.

Guardrails:
- Avoid one-line indirection helpers that only move code around.
- Prefer branch simplification over helper explosion.

### 3) Helper Implementation Pass

Goal:
- Remove duplication by extracting shared logic.

Typical work:
- Extract shared env parsing.
- Extract shared retry/backoff behavior.
- Extract shared subprocess result formatting.

Primary success signals:
- Same logic used from at least 2 call sites.
- Fewer repeated code blocks.

Guardrails:
- Do not create helpers used only once.
- Keep helper scope narrow and domain-specific.

### 4) File Split Pass

Goal:
- Break large modules into focused units that are easier to reason about and test.

Typical work:
- Separate orchestration from utility logic.
- Group code by concern (for example `save_markers`, `rcon_save`, `archive_validation`).

Primary success signals:
- Smaller files and clearer boundaries.
- Existing entrypoints still work unchanged.

Guardrails:
- Split by responsibility, not arbitrarily by line count.
- Keep imports simple; avoid circular dependencies.

### 5) LOC Reduction Pass

Goal:
- Reduce total lines while preserving behavior and safety.

Typical work:
- Remove dead code.
- Inline single-use wrappers.
- Merge low-value tiny helpers into callers.
- Deduplicate repeated messages/constants/utility logic.

Primary success signals:
- Net negative LOC.
- No test regressions.

Guardrails:
- Keep boundary defenses (filesystem/process/network error handling).
- Do not remove validations that protect external state.

## Recommended Order

When all pass types are needed, run in this order:

1. File Split Pass (only if file size is already problematic).
2. Helper Implementation Pass (shared logic only).
3. Cognitive Complexity Reduction Pass.
4. Optimization Pass.
5. LOC Reduction Pass (last cleanup and flattening).

Why this order:
- Early structure changes make later edits safer.
- Final LOC pass removes helper bloat introduced by previous passes.

## Execution Process

### Step 0: Define Constraints Up Front

Before editing, define:
- Behavior constraints (what must not change).
- Test scope (unit, integration, lifecycle).
- Budget targets (for example `-100 LOC`, max complexity per function).

### Step 1: Baseline Snapshot

Capture:
- Current line counts per file.
- Current complexity warnings (if applicable).
- Current failing/passing test baseline for target area.

### Step 2: Plan By Pass

For each pass, state:
- Scope (files/functions).
- Expected net LOC effect (`+`, `-`, or neutral).
- Validation to run after the pass.

### Step 3: Execute One Pass At A Time

Rules:
- Do not mix objectives in a single pass.
- Keep each pass reviewable and measurable.
- Validate after each pass before continuing.

### Step 4: Validate

At minimum:
- Syntax/compile checks.
- Targeted tests for touched workflows.
- Integration tests for runtime-critical paths when changed.

### Step 5: Delta Report

Report after each pass:
- Files changed.
- Net LOC delta.
- Complexity delta (if tracked).
- Test results.

## Operational Policy For Agent Sessions

Use this policy when working with Claude Code or other agent sessions to avoid codebase growth:

1. Require explicit pass mode per request (`complexity`, `optimization`, `loc-reduction`, etc.).
2. Require net LOC target for cleanup requests.
3. Forbid new helpers unless used in 2+ call sites in same change.
4. Forbid new files unless explicitly requested.
5. Require a final delta report (`added`, `deleted`, `net`).

Suggested prompt suffix:

```text
Pass mode: LOC reduction.
Constraints: no behavior changes; no new files; no single-use helpers.
Success criteria: net negative LOC and tests pass.
Report: changed files + net LOC + tests run.
```

---

## Code Reduction Audit

Use this prompt to scan code for low-value complexity and find opportunities
to reduce lines and complexity without changing observable behavior.

For each finding, state the line range, what it does, why it adds more
complexity than value, and the simpler form.

Look specifically for:

- **Wrapper functions** that do nothing but call one other function — delete
  them, call the target directly
- **Intermediate variables** used exactly once and named identically to what
  they hold — inline them
- **Error handling for impossible cases** — conditions the calling framework,
  type system, or surrounding code already guarantees can't occur
- **Diagnostic variables** only used in a log or assert — if the log/assert
  isn't earning its keep, remove both
- **Helpers and utilities built for one call site** — inline the logic, delete
  the helper
- **Docstrings longer than the function body** — trim to one line or remove if
  the signature is self-evident
- **Retry/fallback logic** that duplicates what the caller already does
- **Over-structured data** (NamedTuple, dataclass, dict) used to pass 2–3
  values through a single call — use plain return values
- **Comments that restate the code** without adding why

The bar for removal: *can this be removed or collapsed without changing
observable behavior or error handling at system boundaries?* If yes, cut it.

---

## Default Refactor Pass Policy

This doc is the canonical refactor playbook; AGENTS.md carries only a summary
and the hard constraints, and links here.

When executing a refactor pass by default:

- Follow the [recommended order](#recommended-order) and the constraints in
  [Step 0](#step-0-define-constraints-up-front).
- Capture a baseline before starting.
- Execute one pass at a time; validate after each.
- Provide a delta report (files changed, LOC delta, tests run/results).