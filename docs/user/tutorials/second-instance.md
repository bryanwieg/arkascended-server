# Adding a Second Server Instance

This tutorial shows how to add a second ARK server to your existing setup.
Each instance runs in its own container with its own map, ports, and save data.

## Prerequisite

You have completed `docs/user/tutorials/first-server.md` and have a working
first server.

## Step 1: Create the second instance

Each instance needs its own unique ports. Use different port numbers than
your first server:

```sh
ark-cli create instance --name my-server-2 \
  --server-password "AnotherPassword" \
  --admin-password "MyAdminPass2" \
  --asa-port 27016 \
  --rcon-port 27021 \
  --map-name TheCenter
```

This adds a new service to your existing `docker-compose.yaml`. The first
server is unchanged.

Common options:
- `--map-name` — the map to run (e.g. `TheIsland_WP`, `TheCenter`, `Aberration`,
  `ScorchedEarth`). Defaults to `TheIsland_WP`.
- `--asa-port` — the UDP game port players connect to. Must be unique per instance.
  The host and container port are always the same value (1:1 binding).
- `--rcon-port` — the TCP RCON port used internally. Must be unique per instance.
- `--session-name` — the in-game server name shown in the server browser.
  Defaults to the instance name.

## Step 2: Start the second instance

```sh
ark-cli start instance --instance my-server-2
```

Or start all instances at once:

```sh
ark-cli start instance --instance all
```

## Step 3: Verify both are running

```sh
ark-cli list instances
```

Check a specific instance:

```sh
ark-cli check instance --instance my-server-2
```

## Managing multiple instances

Most commands accept `all` as the instance name to apply to every instance:

```sh
ark-cli stop instance --instance all           # graceful stop all servers
ark-cli restart instance --instance all        # restart all servers
ark-cli create backup --instance all           # back up all instances
```

To target one specific instance, use its name:

```sh
ark-cli logs instance --instance my-server-2 --tail 100
ark-cli save instance --instance my-server-2
```

## Port planning

Each instance needs two unique ports:

| Port | Protocol | Purpose |
|---|---|---|
| `ASA_PORT` (e.g. 27015) | UDP | Game clients connect here |
| `RCON_PORT` (e.g. 27020) | TCP | Internal management (stop, save) |

Both must be open in your firewall/router if you want:
- Players to connect (ASA port)
- External RCON access (RCON port — optional, can be internal-only)

## What's next

- **Schedule backups for all instances**: `docs/user/how-to/schedule-backups.md`
- **Reconfigure an instance**: `docs/user/how-to/reconfigure-instance.md`
