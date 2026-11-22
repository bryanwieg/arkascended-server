# Your First ARK Server

From nothing to a running ARK: Survival Ascended dedicated server. About
40-60 minutes, mostly the image build and first boot.

## Prerequisites

- Host, Docker and network requirements in the
  [README](../../../README.md#prerequisites)
- `ark-cli` on your `PATH` ([README setup](../../../README.md#setup))
- Your user in the `docker` group, or root

`ark-cli` reads and writes `docker-compose.yaml` in the current directory
(override with `--compose-file` or `COMPOSE_FILE`). Run every command below
from the same directory.

## Step 1: Apply kernel settings

Raises `vm.max_map_count`, `net.core.rmem_max`/`wmem_max` and `fs.file-max`
for Proton/Wine and UDP traffic, and persists them:

```sh
sudo ark-cli fix host-kernel-config
ark-cli check host-kernel-config
```

## Step 2: Create the instance

```sh
ark-cli create instance --name my-server \
  --server-password "ChangeMe" \
  --admin-password "MyAdminPass"
```

Writes the `my-server` service to `docker-compose.yaml`.

- `--server-password`: join password. Use `--no-server-password` for an open
  server.
- `--admin-password`: in-game admin password.
- Game port: first free UDP port from `27015`. Override with `--asa-port`.
  Host and container ports are always equal: ARK registers its bound port
  with Steam, so clients need the same number.
- Map: `TheIsland_WP`. Override with `--map-name`.

## Step 3: Start

```sh
ark-cli start instance --instance my-server
```

The first start builds the server image on the host (15-25 minutes: ARK
server files from Steam, GE-Proton and other dependencies) and pins the
service to `ark-server:<build-id>-<recipe-id>`. Later starts reuse it.

The CLI then waits for ready status (default 5 minutes, `--wait-timeout`
to change). First boot generates config files and can take longer; if the
wait times out, the server keeps starting. Check it in Step 4.

## Step 4: Verify

```sh
ark-cli check instance --instance my-server
ark-cli logs instance --instance my-server --tail 50
```

Ready when the log shows
`Server has completed startup and is now advertising for join.`

## Step 5: Connect

In the game, search the server browser for the session name (defaults to the
instance name, `my-server`), or direct-connect to `<host-ip>:27015`.

Not listed? Open the game port (UDP) in the firewall, then run
`ark-cli check reachability --instance my-server`.

## Step 6: Stop

```sh
ark-cli stop instance --instance my-server
```

Saves the world, then exits. Grace period comes from the compose `stop_grace_period` (8 minutes by default); override with `--timeout`.
`--kill` stops immediately without saving.

## Next

- [Add a second instance](second-instance.md)
- [Schedule backups](../how-to/schedule-backups.md)
- [CLI reference](../reference/cli-commands.md)
- [Configuration variables](../reference/configuration.md)
