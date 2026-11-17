# User Documentation

For ARK server admins. No prior Docker expertise required.

## Tutorials — guided, step-by-step

- [Your first ARK server](tutorials/first-server.md) — zero to a running server
- [Adding a second instance](tutorials/second-instance.md) — multiple servers on one machine

## How-to guides — task recipes

- [Schedule automatic backups](how-to/schedule-backups.md)
- [Restore a backup](how-to/restore-backup.md)
- [Update your server](how-to/update-server.md)
- [Edit server INI files](how-to/edit-ini.md)
- [Reconfigure an instance](how-to/reconfigure-instance.md)
- [Check port reachability](how-to/check-reachability.md)
- [Rootless Docker setup](how-to/rootless-setup.md)

## Reference — complete technical information

- [CLI commands](reference/cli-commands.md) — all `ark-cli` commands with flags and examples
- [Configuration variables](reference/configuration.md) — all container environment variables

## Explanation — background and concepts

- [How it works](explanation/architecture.md) — the two-layer host + container system
- [Understanding backups](explanation/backups.md) — retention rules and safety guarantees
- [How updates work](explanation/updates.md) — host-built images, safe rollout and rollback
- [Rootful vs. rootless Docker](explanation/rootful-vs-rootless.md) — choosing the right setup
