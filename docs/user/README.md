# User Documentation

For ARK server admins.

## Tutorials

- [Your first ARK server](tutorials/first-server.md): install to running server
- [Adding a second instance](tutorials/second-instance.md): several servers on one host

## How-to guides

- [Schedule automatic backups](how-to/schedule-backups.md)
- [Restore a backup](how-to/restore-backup.md)
- [Update your server](how-to/update-server.md)
- [Edit server INI files](how-to/edit-ini.md)
- [Reconfigure an instance](how-to/reconfigure-instance.md)
- [Check port reachability](how-to/check-reachability.md)
- [Run as a non-root user](how-to/rootless-setup.md): dedicated `ark` user (rootful) or rootless Docker

## Reference

- [CLI commands](reference/cli-commands.md): every `ark-cli` command and flag
- [Configuration variables](reference/configuration.md): container and CLI environment variables
- [Server INI settings](reference/server-ini-settings.md): every `GameUserSettings.ini` and `Game.ini` server setting, with defaults

## Explanation

- [How it works](explanation/architecture.md): host CLI + container
- [Understanding backups](explanation/backups.md): contents, retention, save confirmation
- [How updates work](explanation/updates.md): host-built images, rollout, automatic rollback
- [Rootful vs. rootless Docker](explanation/rootful-vs-rootless.md): which to use
