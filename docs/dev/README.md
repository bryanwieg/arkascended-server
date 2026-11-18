# Developer Documentation

For contributors and coding agents.

## Tutorials

- [Development environment setup](tutorials/dev-environment.md): clone, build, first passing test

## How-to guides

- [Add a CLI command](how-to/add-cli-command.md): new `ark-cli` verb/object
- [Add a backend method](how-to/add-backend-method.md): extend `ContainerEngine`/`ComposeService` with conformance coverage
- [Run tests](how-to/run-tests.md): suites, runner, cleanup
- [Refactor pass playbook](how-to/refactor-pass.md): pass types, execution order, code reduction audit

## Reference

- [Repo map](reference/repo-map.md): key file locations
- [Conventions](reference/conventions.md): coding rules, logging, validation, safety defaults
- [Test architecture](reference/test-architecture.md): test layers, seam injection, env vars
- [Test logging](reference/test-logging.md): per-test artifact layout, framework trace semantics
- [Runtime call graph](reference/runtime-call-graph.md): startup/shutdown, default INI build, backup/restore flows
- [Backend reference](reference/backends.md): interfaces, constructors, conformance tests, Podman requirements
- [Image build pipeline](reference/image-build.md): base and server-files images, layer cache, build/update env vars
- [CI and releases](reference/ci.md): workflows, versioning, smoke tests, Dependabot, automated updates
- [Performance testing](reference/performance-testing/README.md): Proton/Wine benchmark scripts and findings

## Explanation

- [Architecture](explanation/architecture.md): three layers, container startup, RCON control plane
- [Design decisions](explanation/design-decisions.md): rationale behind key choices
- [Health checks](explanation/health-checks.md): readiness polling, log markers, RAM headroom
- [Logging architecture](explanation/logging-architecture.md): two channels, PID1 fd mirror, rendering modes
- [SDK backend](explanation/backends.md): one SDK backend for Docker and Podman
