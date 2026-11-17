# Developer Documentation

For contributors and coding agents working in this repo.

## Tutorials — guided onboarding

- [Development environment setup](tutorials/dev-environment.md) — clone → build → first test passing

## How-to guides — task recipes

- [Add a CLI command](how-to/add-cli-command.md) — add a new `ark-cli` verb/object
- [Run tests](how-to/run-tests.md) — test suites, runner commands, and cleanup
- [Refactor pass playbook](how-to/refactor-pass.md) — pass types, execution order, code reduction audit
- [Add a backend method](how-to/add-backend-method.md) — extend `ContainerEngine`/`ComposeService` with conformance coverage

## Reference — complete technical information

- [Repo map](reference/repo-map.md) — key file locations and directory layout
- [Conventions](reference/conventions.md) — coding rules, logging, validation, safety defaults
- [Test architecture](reference/test-architecture.md) — test layers, seam injection, env vars
- [Runtime call graph](reference/runtime-call-graph.md) — Mermaid flow diagrams for startup/shutdown, the default INI build step, and backup/restore
- [Test logging](reference/test-logging.md) — per-test artifact log formats and framework trace semantics
- [Backend reference](reference/backends.md) — interface methods, constructors, conformance tests, Podman requirements
- [Image build pipeline](reference/image-build.md) — image flavors, build stages, build/update env vars
- [Performance testing](reference/performance-testing/README.md) — Proton/Wine benchmark scripts and findings

## Explanation — background and concepts

- [Architecture](explanation/architecture.md) — three-layer architecture, startup flow, control plane
- [Design decisions](explanation/design-decisions.md) — rationale behind key architectural choices
- [Health checks](explanation/health-checks.md) — readiness polling, log markers, RAM headroom
- [Logging architecture](explanation/logging-architecture.md) — two-channel output, PID1 mirror, rendering modes
- [SDK backend](explanation/backends.md) — how one SDK backend serves Docker and Podman
