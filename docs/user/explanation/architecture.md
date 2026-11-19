# How It Works

Admin-level overview. Code layers and container internals:
[developer architecture](../../dev/explanation/architecture.md).

## The two pieces

| Piece | Runs | Does |
|---|---|---|
| `ark-cli` | On the host | Edits `docker-compose.yaml`, builds images, starts/stops containers, backups, schedules, status. Talks to Docker or rootless Podman through their API; never touches the ARK process directly. |
| Container | One per instance, as `ark-user` | ARK server under GE-Proton (Wine), plus Python scripts for startup, save, shutdown, backup and restore. |

```
ark-cli ──Docker/Podman API──▶ container ──▶ ARK server (GE-Proton)
                                   ▲
                                   └── RCON (save, shutdown)
```

## Start and stop

`start instance` starts the container and waits until the server log shows
it is advertising for join (default 5 minutes).

`stop instance` stops the container with a 15-minute grace period. The
container's stop script runs on SIGTERM: an optional player countdown while
players are online (`SHUTDOWN_WARNING_MINUTES`, default 5), save the world over RCON, exit, and
force-kill only if the server does not exit in time. `--kill` skips the save.
Flags: [CLI reference](../reference/cli-commands.md).

## Images and volumes

Server files live in the image; ARK images are built on the host from Steam's
current build, not pulled. Per-instance data lives in volumes:

| Volume | Mounted at | Holds |
|---|---|---|
| `<instance>-saved` | `ShooterGame/Saved` | World, player and tribe saves, INI files, logs |
| `ark-cluster-1` | `ShooterGame/Saved/clusters` | Cluster transfer data, shared by all instances |

So an update is an image tag switch, and rollback switches back. See
[How Updates Work](updates.md).

## First boot

The image ships ARK's default `GameUserSettings.ini`, generated at image build.
If the save volume has none, the container copies it in; an existing file is
never replaced. RCON and password settings from the compose environment are
written into it on every start.

## Saves and backups

A backup archives the instance's `Saved/` volume, excluding logs and the
shared cluster directory. Restore takes a pre-restore backup first, then
replaces the save data with the archive. Retention never deletes the last two
backups (one when disk is low). Details: [Understanding Backups](backups.md).

## RCON

Saves and graceful stops use RCON, ARK's built-in remote console, so the
scripts can wait for the server to confirm a save before continuing. The RCON
host port is bound to `127.0.0.1` unless you run `ark-cli enable remote-rcon`.
