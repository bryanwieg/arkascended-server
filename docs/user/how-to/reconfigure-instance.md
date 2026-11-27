# Reconfigure an Instance

Change session name, passwords, mods, cluster, ports, timezone or CPU
priority of an existing instance.

## Apply one or more setting changes

```sh
ark-cli reconfigure instance --instance my-server \
  --session-name "My Center Server" \
  --tz Europe/Berlin
```

Any combination in one command:

| Flag | Changes |
|---|---|
| `--session-name` | `SESSION_NAME`: server name shown in the browser |
| `--admin-password` | `SERVER_ADMIN_PASSWORD`: in-game admin and RCON password |
| `--server-password` | `SERVER_PASSWORD`: password players need to join |
| `--mods` | `MOD_IDS`: CurseForge mod IDs (e.g. `940975,1386057`) |
| `--cluster-id` | `CLUSTER_ID`: cluster for character, item and dino transfers |
| `--asa-port` | Game UDP port (host and container port kept in sync) |
| `--rcon-port` | Published RCON host port (maps to `27020/tcp`) used by stop/save/backup |
| `--cpu-shares` | `cpu_shares`: CPU weight when the host's CPU is busy (`create instance` writes 4096; Docker's default is 1024) |
| `--tz` | `TZ` of the container (e.g. `America/New_York`) |
| `--map-name` | `MAP_NAME`; only if not set yet, see [Change the map](#change-the-map) |

Passwords: letters and digits only. An empty value means "not given", so
`reconfigure` cannot clear a setting (e.g. make a server public or remove all
mods); edit `docker-compose.yaml` for that.

Changes are written to `docker-compose.yaml` and take effect on the next
restart. To publish RCON beyond `127.0.0.1`, use `enable remote-rcon`; see
[cli-commands.md](../reference/cli-commands.md).

## Reconfigure while the server is running

Refused by default. `--force` updates compose without restarting:

```sh
ark-cli reconfigure instance --instance my-server --session-name "New Name" --force
```

## Apply changes and restart immediately

```sh
ark-cli reconfigure instance --instance my-server --session-name "New Name" --force --restart
```

`--restart` stops (with save) and starts the instance, waiting for it to be
ready. It only restarts an instance that was running; a stopped instance stays
stopped.

## Change the map

`reconfigure` refuses to change `MAP_NAME` once set, and `create instance`
always sets it. Create a new instance for a different map:

```sh
ark-cli create instance --name my-scorched-server --map-name ScorchedEarth_WP \
  --admin-password <pw> --no-server-password
```

Official maps include: `TheIsland_WP`, `TheCenter_WP`, `ScorchedEarth_WP`,
`Aberration_WP`, `Extinction_WP`, `Genesis_WP`, `Genesis2_WP`, `CrystalIsles_WP`,
`Valguero_WP`, `LostIsland_WP`, `Fjordur_WP`. Names without `_WP` produce a
warning; the server may fail to start.

## Load mods

Comma-separated CurseForge project IDs:

```sh
ark-cli reconfigure instance --instance my-server --mods 940975,1386057 --force --restart
```

The server downloads and updates mods on each start. They live in the
container, not the save volume, so they are downloaded again after the
container is recreated (e.g. by `update instance`).

Keep the same mods when restoring a save that used them: ARK deletes mod
items, structures and creatures it can't load on its next save.

## Join servers into a cluster

Same cluster ID on each server; players can then transfer characters, items
and dinos between them:

```sh
ark-cli reconfigure instance --instance island --cluster-id my-cluster --force --restart
ark-cli reconfigure instance --instance aberration --cluster-id my-cluster --force --restart
```

Letters, digits, `_` and `-` only. Transfer data lives in the `ark-cluster-1`
volume (shared by all instances in the compose project, mounted at
`Saved/clusters`); servers without a cluster ID don't use it.

## Change ports

```sh
ark-cli reconfigure instance --instance my-server \
  --asa-port 28015 \
  --rcon-port 28020 \
  --force --restart
```

Fails if another instance in the compose file already publishes the port.
Update firewall/router rules for the new game port.

## Verify the changes

```sh
ark-cli show instance --instance my-server
```
