# How It Works

A high-level overview of how the ARK Docker server manager is structured,
written for server admins rather than developers.

## The two pieces

There are two parts to this system:

**`ark-cli`** is a command-line tool you run on your host machine. It handles
everything you interact with directly: creating server configs, starting and
stopping containers, creating backups, managing schedules, and checking status.
Think of it as the control panel.

**The Docker container** holds the actual ARK server process, Proton/Wine (the
compatibility layer that lets the Windows ARK binary run on Linux), and a set
of Python scripts that manage the server from inside the container.

## What each part does

```
Your terminal
    ↓
ark-cli (the control panel)
    ↓  sends instructions via the Docker SDK
Docker container
    ↓
ARK server process (via Proton/Wine)
```

When you run `ark-cli start instance --instance my-server`, the CLI tells Docker to start
the container. The container's startup scripts then launch the ARK server
process, wait for it to initialize, and report back when it's ready.

When you run `ark-cli stop instance --instance my-server`, the CLI sends a message to the
container's shutdown script, which tells the ARK server to warn players, save
the world, and exit cleanly — no data loss.

## Why Docker?

Running ARK in Docker means:

- The server is isolated from your host system.
- You can run multiple server instances on the same machine, each on different
  ports with separate save data.
- Updates are atomic: pull a new image and restart, rather than patching files
  in place.
- Backups and restores are simple: the save data lives in a Docker volume that
  can be archived and restored with standard tools.

## First boot

The image includes ARK's default `GameUserSettings.ini`, generated when the
image was built. When a server starts and its save volume has no
`GameUserSettings.ini` yet (a new instance, or a volume or backup without one),
the container copies the default in. The first start needs no extra setup
step, and an existing `GameUserSettings.ini` is never replaced.

## Saves and backups

ARK stores player and world data in a `ShooterGame/Saved/` directory inside
the container's volume. The backup system archives this directory into
compressed archives (`.tar.gz` by default). Restoring replaces that directory
with the contents of the chosen archive.

The CLI always keeps at least two normal backups, but under low disk space
it keeps a minimum of one so a recovery point is always available.

## RCON

The stop and save commands work through a protocol called RCON (Remote Console),
which is a built-in feature of ARK's dedicated server. This lets the system
send commands to the running server — like "save the world now" or "warn
players that shutdown is in 5 minutes" — and wait for confirmation that they
completed before doing anything else.

This is why graceful stops take a few minutes: the system is waiting for the
server to actually save and exit cleanly, not just killing the process.
