# Reconfigure an Instance

How to change server settings (map, session name, ports, passwords, timezone)
for an instance.

## Apply one or more setting changes

```sh
ark-cli reconfigure instance --instance my-server \
  --map-name TheCenter \
  --session-name "My Center Server"
```

You can change any combination of these in one command:

| Flag | What it changes |
|---|---|
| `--map-name` | The map (e.g. `TheIsland_WP`, `TheCenter`, `Aberration`) |
| `--session-name` | In-game server name shown in the browser |
| `--admin-password` | Admin password for in-game commands |
| `--server-password` | Password players need to join (blank = public) |
| `--mods` | CurseForge mod IDs the server loads (e.g. `940975,1386057`) |
| `--asa-port` | UDP port players connect to (host and container port kept in sync) |
| `--rcon-port` | Published RCON host port used by stop/save/backup |
| `--tz` | Timezone for the container (e.g. `America/New_York`) |

Changes are written to `docker-compose.yaml`. They take effect on the next
restart.

## Reconfigure while the server is running

If the server is currently running, the command refuses by default. Pass
`--force` to allow reconfiguration while running:

```sh
ark-cli reconfigure instance --instance my-server --map-name TheCenter --force
```

This updates compose without restarting. Restart the server manually when
ready for changes to take effect.

## Apply changes and restart immediately

Add `--force --restart` to reconfigure a running instance and restart in one
step:

```sh
ark-cli reconfigure instance --instance my-server --map-name Aberration --force --restart
```

If the instance is not currently running when you use `--restart`, the command
applies the changes and exits without starting the instance.

Without `--restart`, the command updates compose and exits with a reminder
that a restart is needed.

## Change the map

> **Note:** `MAP_NAME` cannot be changed on an existing instance. The
> `reconfigure` command blocks map changes for instances that already have a
> `MAP_NAME` set (all instances created via `create instance` have this set).
> To use a different map, create a new instance with `--map-name`.

```sh
ark-cli create instance --name my-scorched-server --map-name ScorchedEarth
```

Available official maps include: `TheIsland_WP`, `TheCenter_WP`, `ScorchedEarth_WP`,
`Aberration_WP`, `Extinction_WP`, `Genesis_WP`, `Genesis2_WP`, `CrystalIsles_WP`,
`Valguero_WP`, `LostIsland_WP`, `Fjordur_WP`.

Map names for ARK: Survival Ascended must end in `_WP`. If you provide a map
name without this suffix, a warning will be displayed.

## Load mods

Pass the CurseForge project IDs of the mods, comma-separated:

```sh
ark-cli reconfigure instance --instance my-server --mods 940975,1386057 --force --restart
```

The server downloads and updates the mods itself on each start. They are
stored in the container, not in the save volume, so they are downloaded again
after the container is recreated (for example by `update instance`).

Mods add content to the save. Keep the same mods when you restore a save that
used them; ARK deletes mod items, structures and creatures it can't load the
next time it saves.

## Change ports

If you need to move your server to different ports:

```sh
ark-cli reconfigure instance --instance my-server \
  --asa-port 28015 \
  --rcon-port 28020 \
  --restart
```

Update your firewall/router rules to match the new ASA port.

## Verify the changes

```sh
ark-cli show instance --instance my-server
```
