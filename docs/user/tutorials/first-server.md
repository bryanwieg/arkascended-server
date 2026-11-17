# Your First ARK Server

This tutorial takes you from nothing to a running ARK: Survival Ascended
dedicated server. No prior Docker experience required beyond having it
installed.

Estimated time: 40–60 minutes (mostly waiting for the image build and server start).

## What you need before starting

- A Linux host with about 24 GB of RAM and 60-70 GB of free disk
- Docker with BuildKit/buildx
- Internet access to Steam and `ghcr.io` (the image is built on your machine;
  without `ghcr.io` its base is built too, which needs GitHub, Launchpad, NuGet
  and Ubuntu mirrors)
- Docker Engine installed and running
- Docker Compose v2 plugin (`docker compose version` should work)
- `ark-cli` binary on your PATH — download from the GitHub releases page, or
  build from source with Go 1.25.6+ (see Step 1)
- Your user in the `docker` group, or root access

## Step 1: Get the repo and build the CLI

```sh
git clone https://github.com/bryanwieg/arkascended-server.git && cd arkascended-server
./.github/build-cli.sh
```

This builds the `ark-cli` binary at `app/ark-cli/ark-cli`. Add that directory
to your `PATH` (or copy the binary somewhere on it) so the `ark-cli` commands
below work.

All `ark-cli` commands in this guide are run from the repo directory.

## Step 2: Apply required kernel settings

ARK runs via Proton/Wine and needs a higher virtual memory map limit than
the Linux default:

```sh
sudo ark-cli fix host-kernel-config
```

You can verify the settings were applied:

```sh
ark-cli check host-kernel-config
```

## Step 3: Create your server instance

```sh
ark-cli create instance --name my-server \
  --server-password "ChangeMe" \
  --admin-password "MyAdminPass"
```

This creates a `docker-compose.yaml` file with your server's configuration.

- `my-server` is the name for this instance. Use something short without spaces.
- `--server-password` is the password players need to join (leave blank to
  make the server public).
- `--admin-password` is for in-game admin commands. Keep this private.

The server will run on the default port `27015/udp`. If that port is already
in use or you want a different one, add `--asa-port 28015` (or any open UDP port).
The host port and container port are always the same value — ARK: Survival
Ascended registers its bound port with Steam, so they must match for clients
to find the server.

## Step 4: Start the server

```sh
ark-cli start instance --instance my-server
```

The first start builds the server image on your machine (about 15-25 minutes:
it downloads the ARK server files from Steam, GE-Proton and other
dependencies). Later starts reuse it. The CLI then pins the instance's compose
file to `ark-server:<build-id>`. The CLI will wait for the server to reach "ready" status, which can take
5–10 minutes on first boot (it generates initial config files). You'll see
a progress indicator while it waits.

## Step 5: Verify it's running

```sh
ark-cli check instance --instance my-server
```

You should see the instance reported as running and ready. To see live logs:

```sh
ark-cli logs instance --instance my-server --tail 50
```

Look for lines like `ARK: Survival Ascended` and `Server listening` to confirm
the server is advertising.

## Step 6: Connect from the game

In ARK: Survival Ascended, open the server browser and search for your
session name (defaults to `my-server`), or use the direct connect option with
`<your-host-ip>:27015`.

If you can't see your server:
- Check your firewall allows UDP traffic on port 27015.
- Use `ark-cli check reachability --instance my-server` to test external reachability.

## Step 7: Stop the server gracefully

When you're done, stop the server gracefully (default 15-minute grace period for save + shutdown):

```sh
ark-cli stop instance --instance my-server
```

To stop immediately with no delay (useful for testing):

```sh
ark-cli stop instance --instance my-server --timeout 0s
```

The server saves player data before stopping.

## What's next

- **Add a second server**: see `docs/user/tutorials/second-instance.md`
- **Schedule backups**: see `docs/user/how-to/schedule-backups.md`
- **Full command reference**: see `docs/user/reference/cli-commands.md`
- **Configuration variables**: see `docs/user/reference/configuration.md`
