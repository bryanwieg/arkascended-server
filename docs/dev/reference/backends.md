# Backend Reference

Interfaces, constructors, and conformance tests for the SDK backend
(`app/pkg/ark/sdkbackend/`). For background, see the
[SDK backend explanation](../explanation/backends.md).

## Interfaces

### ContainerEngine

Defined in `app/pkg/ark/engine.go`. Methods:

- `InspectState` — check if a container is running
- `RemoveContainer` — remove a stopped container
- `ContainerLogs` — fetch container logs
- `ImageExists` — check if an image is available locally
- `PullImage` — pull an image from its registry (anonymously); errors from the pull are returned
- `ImageLabels` — read labels from an image
- `BuildImage` — build an image from a Dockerfile
- `ReadFileFromImage` — read a file from an image's filesystem
- `ImageTags` — list a repository's local tags and their image IDs
- `RemoveImage` — remove one tag without force; never deletes an image a container uses (`ErrConflict`)
- `PruneDangling` — remove untagged images and build cache nothing uses
- `VolumeMountpoint` — get the mount path of a named volume
- `RemoveVolume` — remove a named volume
- `CopyFromContainer` — copy a file from a running container

All methods take `context.Context` as the first argument. Backends map native
errors to sentinel `ErrNotFound` for `errors.Is` branching.

### ComposeService

Defined in `app/pkg/ark/compose_service.go`. Methods grouped by concern:

- **Project**: `LoadProject`, `SaveProject` — parse and write compose YAML.
  `SaveProject` writes only changes to a service's image, environment, labels
  and ports, as line edits that keep the rest of the file byte for byte, and
  refuses anything else (see
  [design decisions](../explanation/design-decisions.md#why-yaml-ast-editing-instead-of-template-replacement))
- **Lifecycle**: `Up`, `Stop`, `Kill`, `Down`, `Remove`, `Create` — container
  lifecycle
- **Status**: `RunningServices`, `ExitedServices`, `AllServices` — query state
- **Exec**: `Exec`, `ExecInteractive`, `RunOneOff`, `RunOneOffDetached`,
  `RunOneOffInteractive` — run commands in or alongside containers
- **Logs**: `Logs` — stream or fetch container logs
- **Copy**: `CopyFromService` — copy files from a service

## Constructors

- Default constructors use `DOCKER_HOST`. When the runtime is Podman, `app.go`
  detects the socket via `host.HasPodmanService()` and sets `DOCKER_HOST`
  before initializing the SDK.
- `NewSDKEngineWithSocket(logger, socketPath)` and
  `NewSDKComposeWithSocket(logger, socketPath)` take an explicit socket path,
  bypassing `DOCKER_HOST`. Used by the Podman conformance tests.

## Conformance tests

Shared contract tests live in `app/pkg/ark/conformance/`; the runners live in
`app/pkg/ark/sdkbackend_test/`. All are tagged `//go:build integration` and
exercise a real daemon.

| Test | Target |
|------|--------|
| `TestSDKEngineConformance` | Docker (default socket) |
| `TestSDKComposeConformance` | Docker (default socket) |
| `TestContainerEngine_Podman` | Podman (detected socket) |
| `TestComposeService_Podman` | Podman (detected socket) |

Podman tests auto-skip when the Podman socket is not available. Run with:

```bash
cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
```

## Podman requirements

- Podman 5.0+
- Rootless Podman only (`$XDG_RUNTIME_DIR/podman/podman.sock`)
- Podman system service running (`systemctl --user start podman.socket`)
- Rootful Podman (`/run/podman/podman.sock`) is detected but rejected

The SDK talks to Podman through its Docker v1.40 compatibility API. Container
inspect/remove/logs, image inspect/labels/build, volume inspect/remove, compose
load/save/up/down/stop/kill, and compose exec/run/logs/copy all use standard
Docker API endpoints that Podman implements.
