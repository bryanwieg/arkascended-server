# Backend Reference

Interfaces, constructors and conformance tests for the SDK backend
(`app/pkg/ark/sdkbackend/`). Background:
[SDK backend explanation](../explanation/backends.md).

## Interfaces

All methods take `context.Context` first. Backends map native errors to the
sentinels in `app/pkg/ark/errors.go` for `errors.Is`: `ErrNotFound` (missing
container, image or volume) and `ErrConflict` (in use).

### ContainerEngine

`app/pkg/ark/engine.go`.

| Method | Behavior |
|--------|----------|
| `InspectState` | Runtime state of a container (`Running`, `Restarting`, `ExitCode`, `StartedAt`). `ErrNotFound` if missing |
| `RunContainer` | Run a `RunContainerRequest` to completion (binds, user, env), stream output, return the exit code; always removes the container and first removes a leftover one of the same name. `ErrNotFound` if the image is missing; on cancel stops it and wraps `ctx.Err()` |
| `RemoveContainer` | Force-remove a container. `ErrNotFound` if missing |
| `ContainerLogs` | Reader for combined stdout/stderr |
| `ImageExists` | Whether an image exists locally |
| `ImageLabels` | OCI labels of an image. `ErrNotFound` if missing |
| `PullImage` | Pull a reference anonymously; returns errors reported during the pull |
| `BuildImage` | Build an image from a `BuildRequest` |
| `ReadFileFromImage` | Read a file via a throwaway container (removed afterwards) |
| `CopyFromImage` | Copy a file or directory out of an image to a host path, like `docker create` + `cp` + `rm`. `ErrNotFound` if the image or path is missing |
| `ImageTags` | Local tags of a repository, full reference to image ID |
| `RemoveImage` | Remove one tag, no force. Removing the last tag of an image a container uses returns `ErrConflict`; `ErrNotFound` if the reference is missing |
| `ImagesInUse` | IDs of images any container uses (running or stopped, any project) |
| `PruneDangling` | Remove untagged images and unused build cache |
| `Rootless` | Whether the engine runs rootless (`/info` SecurityOptions) |
| `VolumeMountpoint` | Host path of a named volume; `("", false)` if missing |
| `RemoveVolume` | Remove a named volume. `ErrNotFound` if missing, `ErrConflict` if in use |
| `CopyFromContainer` | Copy a file or directory from a container to a host path |

### ComposeService

`app/pkg/ark/compose_service.go`.

| Group | Methods |
|-------|---------|
| Project | `LoadProject`, `SaveProject` |
| Lifecycle | `Up`, `Stop`, `Kill`, `Down`, `Remove`, `Create` |
| Status | `RunningServices`, `ExitedServices`, `AllServices`, `ServiceContainers` (container name and state per service) |
| Exec | `Exec`, `ExecInteractive`, `RunOneOff`, `RunOneOffDetached`, `RunOneOffInteractive` |
| Logs | `Logs` |
| Copy | `CopyFromService` |

`SaveProject` writes only changes to a service's image, `cpu_shares` (set,
not removed), environment, labels and ports, and the top-level `x-ark-schedules` block (replaced whole, or
appended at the end of the file when absent, `$` written as `$$`), as line edits that keep the rest of the file byte for byte, and
refuses anything else (see
[design decisions](../explanation/design-decisions.md#why-yaml-ast-editing-instead-of-template-replacement)).
New services are added with `runtime.InsertService` on the raw YAML instead.

## Constructors

| Constructor | Socket |
|-------------|--------|
| `NewSDKEngine(logger)`, `NewSDKCompose(logger)` | `DOCKER_HOST` or the default socket |
| `NewSDKEngineWithSocket(logger, socket)`, `NewSDKComposeWithSocket(logger, socket)` | Explicit socket URI. The compose variant sets `DOCKER_HOST` process-wide |

For Podman, `app/ark-cli/app.go` finds the rootless socket with
`host.HasPodmanService()`, sets `DOCKER_HOST`, then calls the default
constructors. The `WithSocket` variants are used by the Podman conformance
tests.

## Conformance tests

Shared contract tests in `app/pkg/ark/conformance/`:
`conformance.TestContainerEngine(t, engine)` and
`conformance.TestComposeService(t, svc)`. Runners in
`app/pkg/ark/sdkbackend_test/`. All tagged `//go:build integration`; they need
a real daemon.

| Test | Target | Skips when |
|------|--------|------------|
| `TestSDKEngineConformance` | Docker (default socket) | No reachable daemon |
| `TestSDKComposeConformance` | Docker (default socket) | No reachable daemon |
| `TestContainerEngine_Podman` | Podman (rootless socket) | No rootless socket, or init fails |
| `TestComposeService_Podman` | Podman (rootless socket) | No rootless socket, or init fails |

```bash
cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
```

## Podman requirements

- Podman 5.0+ (Docker 28.0+), checked by `ValidateDaemonVersion`.
- A compose provider: `podman compose version` must succeed
  (`RuntimeHelper.ValidateComposeProvider`, run on every command).
- Rootless only: `$XDG_RUNTIME_DIR/podman/podman.sock` (falls back to
  `/run/user/<uid>` when `XDG_RUNTIME_DIR` is unset).
- System service running: `systemctl --user start podman.socket`.
- Rootful socket `/run/podman/podman.sock` alone is detected and rejected.

The SDK reaches Podman through its Docker-compatible API; the same backend
code serves both runtimes.
