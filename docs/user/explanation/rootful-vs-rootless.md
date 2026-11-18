# Rootful vs. Rootless Docker

Which Docker mode to run the stack under, and what each mode changes in this project.

## The short answer

- Home or small-community server: rootful Docker with a dedicated `ark` user. Fewer moving parts.
- Shared host or security-sensitive setup: rootless Docker.
- Podman: rootless only. ark-cli refuses a rootful Podman socket.

## What rootful Docker means

`docker` group membership is root-equivalent, as usual. The server process inside
the container runs as the unprivileged `ark-user` (image `USER`), not root.

Project specifics:

- ark-cli finds the daemon via `DOCKER_HOST`, else `/var/run/docker.sock`.
- On Docker, commands that touch an instance check that the invoking user
  (`SUDO_USER`, else `USER`) is in the `docker` group. If not, ark-cli runs
  `usermod -aG docker <user>` (via `sudo` when not root) and asks for a re-login.

## What rootless Docker means

Daemon and containers run in the owning user's namespace; a container escape
stops at that account.

Project specifics:

- Set `DOCKER_HOST` (e.g. `unix:///run/user/$(id -u)/docker.sock`). Without it
  ark-cli only looks at `/var/run/docker.sock` and fails with "Docker socket not
  found".
- The `docker` group check above still runs for the Docker runtime. If the host
  has a `docker` group and you are not in it, ark-cli adds you with `sudo usermod`,
  which also grants access to any rootful daemon on the host.
- `.github/build-image.sh` detects rootless Docker/Podman and runs the SteamCMD
  download without `--user`, so downloaded files stay owned by you.
- `fix host-kernel-config` writes `/etc/sysctl.d/` and needs root in either mode.
- Ports: ARK uses 27015+ (RCON 27020+), so the below-1024 bind limit does not apply.

## Recommendation by use case

| Situation | Recommendation |
|---|---|
| Home server, you own the machine | Rootful + dedicated `ark` user |
| Shared server with multiple users | Rootless (limits blast radius) |
| VPS or cloud host, single purpose | Either; rootful has fewer moving parts |
| Security-sensitive or production | Rootless |
| Podman host | Rootless Podman (rootful unsupported) |

## Setup guides

- [Rootful with dedicated user](../how-to/rootless-setup.md#option-a-rootful-docker-with-a-dedicated-ark-user-recommended-for-most)
- [Rootless Docker](../how-to/rootless-setup.md#option-b-rootless-docker)
