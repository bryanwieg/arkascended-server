# SDK Backend

One backend (`app/pkg/ark/sdkbackend/`) implements `ark.ContainerEngine` and
`ark.ComposeService` for both Docker and Podman.

## The SDK backend

Built on Go libraries, not the `docker` binary:

- `SDKEngine`: Docker Engine client (`github.com/moby/moby/client`).
- `SDKCompose`: Compose library (`github.com/docker/compose/v5`), initialized
  through a `docker/cli` command object.

There is no Podman SDK. Podman is reached through its Docker-compatible REST
API: the same client code talks to whichever socket `DOCKER_HOST` points at.
Both clients read `DOCKER_HOST` (and the TLS variables) like the docker CLI does.

Minimum daemon versions, checked at startup via the server version API:
Docker 28, Podman 5.

## Runtime selection

`ark-cli` picks the runtime at startup (`ResolveContainerRuntime`):

1. `--runtime` flag
2. `ARK_CONTAINER_RUNTIME`
3. PATH probe, `docker` before `podman`

The chosen binary must be on PATH. Then, per runtime:

| Runtime | Socket | Failure |
|---------|--------|---------|
| docker | `DOCKER_HOST` if set, else `/var/run/docker.sock` | error if neither |
| podman | `$XDG_RUNTIME_DIR/podman/podman.sock` (fallback `/run/user/<uid>/...`); `ark-cli` sets `DOCKER_HOST` to it | rootful-only socket (`/run/podman/podman.sock`): error, rootless only. No socket: error suggesting `systemctl --user start podman.socket` |

`ark-web` has no runtime flag: it uses the Docker socket if present, else the
rootless Podman socket, with the same rootful rejection.

## Why conformance tests

Podman's compatibility API is not byte-for-byte identical to Docker's. The
shared contract tests in `app/pkg/ark/conformance/` run against both sockets,
so a gap shows up as a failing test, not as a bug in one runtime. Podman runs
are skipped when no rootless socket is available.

See the [backend reference](../reference/backends.md) for interface methods,
constructors and the test inventory, and
[Add a backend method](../how-to/add-backend-method.md) to extend the
interfaces.
