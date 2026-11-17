# Rootful vs. Rootless Docker

Which Docker mode to use and why.

## The short answer

For most home or small-community server setups: **use rootful Docker with a
dedicated `ark` user**. It's simpler, has fewer edge cases, and is secure
enough for typical use.

For environments where you need stronger isolation (shared hosting,
production infrastructure, security-sensitive setups): consider **rootless
Docker**.

## What rootful Docker means

The Docker daemon runs as root. Users in the `docker` group can issue commands
to the daemon. Effectively, membership in the `docker` group is equivalent to
having root access on the host — the daemon can mount any path, modify any
file.

The ARK server container itself does not run as root. The image is built to
run the server process as the unprivileged `ark-user`. So the container
is isolated, even if the daemon is privileged.

The risk is the daemon access itself: anyone with Docker socket access can
trivially escalate to root.

## What rootless Docker means

The Docker daemon runs as an unprivileged user. Containers are started within
that user's namespace. A container escape cannot escalate beyond the user
account that owns the daemon.

The tradeoffs:
- Slightly more complex initial setup.
- Some kernel features (e.g. binding to ports below 1024) require additional
  configuration.
- Volume performance can differ from rootful in some configurations.
- Some Docker features (e.g. `--privileged`) behave differently.

For ARK on port 27015+, the port restriction is not an issue.

## Recommendation by use case

| Situation | Recommendation |
|---|---|
| Home server, you own the machine | Rootful + dedicated `ark` user |
| Shared server with multiple users | Rootless (limits blast radius) |
| VPS or cloud host, single purpose | Either; rootful is simpler |
| Security-sensitive or production | Rootless |
| Not sure | Rootful + dedicated `ark` user |

## Setup guides

- Rootful with dedicated user: `docs/user/how-to/rootless-setup.md` (Option A)
- Rootless Docker: `docs/user/how-to/rootless-setup.md` (Option B)
