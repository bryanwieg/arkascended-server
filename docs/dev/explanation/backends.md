# SDK Backend

The CLI interacts with Docker/Podman through a single SDK backend implementation.
This page explains how it reaches both Docker and Podman.

## The SDK backend

The **SDK backend** (`app/pkg/ark/sdkbackend/`) uses the Docker and Podman Go SDKs
directly, avoiding subprocess overhead. It implements the full `ContainerEngine` and
`ComposeService` interfaces.

When the runtime is Podman, the SDK backend connects to Podman's Docker-compatible
API socket (`$XDG_RUNTIME_DIR/podman/podman.sock` for rootless) via the
`DOCKER_HOST` environment variable. Podman exposes a Docker v1.40 compatibility layer
through its REST API, allowing the Docker SDK to operate against Podman transparently.
Alternatively, the `NewSDKEngineWithSocket` and `NewSDKComposeWithSocket` constructors
accept an explicit socket path.

`ark.ContainerEngine` and `ark.ComposeService` are implemented by the SDK backend.
Shared contract tests in `app/pkg/ark/conformance/` are run from
`app/pkg/ark/sdkbackend_test/` against both Docker (default) and Podman (when
the Podman socket is available).

## Runtime selection

The CLI selects the container runtime at startup via `ResolveContainerRuntime`:

1. Explicit `--runtime` flag
2. `ARK_CONTAINER_RUNTIME` environment variable
3. PATH probe (prefers `docker` over `podman`)

When using the SDK backend with Podman:
- The Podman system service must be running (`systemctl --user start podman.socket`)
- The SDK client connects via `DOCKER_HOST` set to the Podman socket path
- Only rootless Podman is supported (rootful at `/run/podman/podman.sock` is not supported)
- Podman's Docker compatibility layer handles the API translation

## Why conformance tests

Docker and Podman both speak the Docker API, but Podman's compatibility layer
is not byte-for-byte identical. The conformance suite runs the same contract
tests against both sockets, so a gap shows up as a failing conformance test
rather than a user-facing bug in one runtime.

See the [backend reference](../reference/backends.md) for the interface
methods and test inventory, and
[Add a backend method](../how-to/add-backend-method.md) to extend the
interfaces.
