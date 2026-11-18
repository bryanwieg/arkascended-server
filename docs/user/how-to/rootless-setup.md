# Rootless Docker Setup

Run the server stack under a non-root account: rootful Docker with a dedicated
`ark` user (Option A) or rootless Docker (Option B). Which to pick:
[Rootful vs. rootless Docker](../explanation/rootful-vs-rootless.md).
Rootless Podman is also supported (`--runtime podman` or
`ARK_CONTAINER_RUNTIME=podman`); see
[Configuration](../reference/configuration.md).

Both options: install `ark-cli` on the `PATH` (see the
[README](../../../README.md#setup)) and apply kernel settings as root. These
write to `/etc/sysctl.d/` regardless of Docker mode:

```sh
sudo ark-cli fix host-kernel-config
```

## Option A: Rootful Docker with a dedicated ark user (recommended for most)

```sh
sudo useradd -m -s /bin/bash ark
sudo usermod -aG docker ark
sudo -iu ark      # run all ark-cli commands as ark, from the compose directory
```

Docker group check: with the Docker runtime, `ark-cli` checks that the user
in `SUDO_USER` (else `USER`) is in the `docker` group, and if not runs
`sudo usermod -aG docker <user>`. Under `sudo -u ark` / `sudo -iu ark`,
`SUDO_USER` is your admin account, not `ark`. If that account is not in the
`docker` group, the command tries `sudo` as `ark` and fails. Either add the
admin account to `docker` or unset `SUDO_USER` in the `ark` shell.

## Option B: Rootless Docker

Set up rootless Docker per
[Docker's rootless mode docs](https://docs.docker.com/engine/security/rootless/),
including `loginctl enable-linger` so the daemon survives logout.

Project-specific requirements:

- **`DOCKER_HOST` must be set.** `ark-cli` uses `DOCKER_HOST` if set, else
  `/var/run/docker.sock`. It ignores Docker contexts, so
  `docker context use rootless` alone is not enough. Persist in your shell
  profile:

  ```sh
  export DOCKER_HOST=unix:///run/user/$(id -u)/docker.sock
  ```

- **Docker group.** The group check from Option A also runs here. If the
  host has a `docker` group (for example, the rootful package is also
  installed) and your user is not in it, `ark-cli` adds your user with
  `sudo usermod`. That grants rootful daemon access, which rootless setups
  usually want to avoid.
- **Ports.** Defaults (game 27015/UDP and up, RCON from 27020 bound to
  127.0.0.1) are above 1024. Ports below 1024 need
  `net.ipv4.ip_unprivileged_port_start` (see Docker's docs).

## Verify

```sh
ark-cli check host-kernel-config
ark-cli create instance --name test-server --server-password test --admin-password test
ark-cli start instance --instance test-server   # first start builds the image
```
