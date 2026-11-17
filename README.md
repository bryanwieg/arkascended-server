# ARK Docker

Docker image and management CLI for ARK: Survival Ascended dedicated servers
on Linux. Handles server lifecycle, backups, scheduling, and updates via a
simple `ark-cli` command-line tool.

## Prerequisites

- Linux host with about 24 GB RAM
- About 60-70 GB free disk (current and previous image at ~25 GB each, ~12 GB
  SteamCMD cache, and the build peak)
- Docker Engine with Compose v2 plugin and BuildKit/buildx
- Internet access to Steam (`api.steamcmd.net` and the Steam CDN) and `ghcr.io`
  (the ARK-free base image). Without `ghcr.io` the base is built locally, which
  needs GitHub (GE-Proton), Launchpad (deadsnakes PPA), NuGet (XAudio2) and
  Ubuntu mirrors
- `ark-cli` binary on your PATH (see below)

## Setup

**Preferred:** download the `ark-cli` binary from the
[GitHub releases page](https://github.com/bryanwieg/arkascended-server/releases) and place it on your `PATH`.

**Until releases are available** — clone the repo and build from source
(requires the Go version in `go.work`):

```sh
git clone https://github.com/bryanwieg/arkascended-server.git && cd arkascended-server
./.github/build-cli.sh   # writes app/ark-cli/ark-cli; put it on your PATH
```

Apply required kernel settings for Proton/Wine:

```sh
sudo ark-cli fix host-kernel-config
```

The server image is built on your host the first time you start an
instance (about 15-25 minutes). Nothing is pulled from a registry. See
[How updates work](docs/user/explanation/updates.md).

## Quick start

Create and start a server (the first start builds the image):

```sh
ark-cli create instance --name my-server \
  --server-password "ChangeMe" \
  --admin-password "MyAdminPass"

ark-cli start instance --instance my-server
```

Core lifecycle commands:

```sh
ark-cli stop instance --instance my-server      # graceful stop (saveworld, then exit)
ark-cli restart instance --instance my-server   # stop then start
ark-cli save instance --instance my-server      # trigger in-game saveworld
ark-cli logs instance --instance my-server      # tail container logs
ark-cli create backup --instance my-server      # create a backup archive
ark-cli check instance --instance my-server     # check health and ready status
```

## Documentation

- **New to ARK server admin?** → [docs/user/tutorials/first-server.md](docs/user/tutorials/first-server.md)
- **Full CLI reference** → [docs/user/reference/cli-commands.md](docs/user/reference/cli-commands.md)
- **Configuration variables** → [docs/user/reference/configuration.md](docs/user/reference/configuration.md)
- **All user guides** → [docs/user/](docs/user/)
- **Contributing** → [docs/dev/tutorials/dev-environment.md](docs/dev/tutorials/dev-environment.md)

## Disclaimer

This project is not affiliated with Studio Wildcard or Snail Games. ARK server
files are downloaded from Steam on your own machine and used under the
ARK: Survival Ascended Dedicated Server EULA (personal, non-commercial use).
No ARK files are published or redistributed by this project.

## License

Copyright (C) 2026 bryanwieg

This project is licensed under the GNU Affero General Public License v3.0
only (`AGPL-3.0-only`). See [LICENSE](LICENSE) for the full text.

Bundled third-party assets (htmx, Pico CSS) and Go dependencies remain under
their own licenses. The published base image
(`ghcr.io/bryanwieg/ark-server-base`) also contains GE-Proton (including Wine),
Ubuntu packages and Microsoft's XAudio2 redistributable, each under its own
license. The ARK: Survival Ascended server files are not part of this project
and are covered by their own EULA (see the disclaimer above).

Commercial licenses for use outside the terms of the AGPL are available; ask in
[GitHub Discussions](https://github.com/bryanwieg/arkascended-server/discussions).
