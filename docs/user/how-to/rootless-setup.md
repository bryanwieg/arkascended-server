# Rootless Docker Setup

How to run the ARK server stack under a non-root account: either rootful
Docker with a dedicated `ark` user (Option A) or rootless Docker (Option B).
To decide which, see [Rootful vs. rootless Docker](../explanation/rootful-vs-rootless.md).

## Option A: Rootful Docker with a dedicated ark user (recommended for most)

This keeps the standard rootful daemon but isolates the server under a
dedicated user account, avoiding running it as your login user (UID 1000).

### Create the ark user

```sh
sudo useradd -m -s /bin/bash ark
sudo usermod -aG docker ark
```

### Set up the repo as the ark user

```sh
sudo -u ark -i
# now running as ark
git clone --recursive <repo-url> ~/ark-docker
cd ~/ark-docker
./.github/build-cli.sh
```

### Run ark-cli as the ark user

All `ark-cli` commands should be run as the `ark` user:

```sh
sudo -u ark ark-cli start instance --instance my-server
# or switch to the ark user:
sudo -u ark -i
ark-cli start instance --instance my-server
```

## Option B: Rootless Docker

### Prerequisites

- Docker Engine with rootless support (Docker 20.10+)
- `uidmap` package installed (`sudo apt install uidmap` on Debian/Ubuntu)
- A user account (your own, not root)

### Enable rootless Docker

Follow the official Docker rootless setup for your distribution. For most
systems:

```sh
dockerd-rootless-setuptool.sh install
```

This installs a user-level systemd service. Enable it to start on login:

```sh
systemctl --user enable docker
systemctl --user start docker
```

### Set the Docker socket path

Rootless Docker uses a per-user socket. Export the path in your shell profile:

```sh
export DOCKER_HOST=unix:///run/user/$(id -u)/docker.sock
```

Add this to `~/.bashrc` or `~/.zshrc` so it persists.

### Verify Docker is working

```sh
docker version
docker compose version
```

### Install and use ark-cli

The rest of the setup is identical to the rootful path:

```sh
git clone --recursive <repo-url> ~/ark-docker
cd ~/ark-docker
./.github/build-cli.sh
./.github/build-image.sh
ark-cli create instance --name my-server --server-password "ChangeMe" --admin-password "AdminPass"
ark-cli start instance --instance my-server
```

### Kernel settings with rootless Docker

The `fix host-kernel-config` command writes to `/etc/sysctl.d/` and requires
root regardless of your Docker mode:

```sh
sudo ark-cli fix host-kernel-config
```

### Ports below 1024

Rootless Docker cannot bind to ports below 1024 by default. ARK uses port
27015 which is above 1024, so this is not an issue for standard setups.
If you need a port below 1024, see the Docker rootless documentation for
`net.ipv4.ip_unprivileged_port_start`.

## Verifying your setup

After setup, confirm everything works:

```sh
docker version                          # Docker is accessible
ark-cli check host-kernel-config        # kernel settings are correct
ark-cli create instance --name test-server \
  --server-password test \
  --admin-password test
ark-cli show instance --instance test-server       # compose was created
```
