# Adding a Second Server Instance

Each instance is its own compose service and container, with its own map,
ports and `<name>-saved` volume. Assumes a working
[first server](first-server.md).

## Step 1: Create

```sh
ark-cli create instance --name my-server-2 \
  --server-password "AnotherPassword" \
  --admin-password "MyAdminPass2" \
  --map-name TheCenter_WP
```

Adds `my-server-2` to the existing `docker-compose.yaml`; other services are
untouched. Ports are picked automatically, the next free one from each base:

| Port | Base | Binding |
|---|---|---|
| Game (`ASA_PORT`) | 27015/udp | All interfaces, host port = container port |
| RCON | 27020/tcp | `127.0.0.1` only |

Pick the game port with `--asa-port`. Change ports, session name and other
settings later with
[`reconfigure instance`](../how-to/reconfigure-instance.md).

Map names need the `_WP` suffix; `create` warns without it.

## Step 2: Start

```sh
ark-cli start instance --instance my-server-2
```

## Step 3: Verify

```sh
ark-cli list instances
ark-cli check instance --instance my-server-2
```

## Cluster transfers (optional)

Instances with the same `CLUSTER_ID` share character, item and dino
transfers (`--force` because the instances are running):

```sh
ark-cli reconfigure instance --instance my-server --cluster-id main --force --restart
ark-cli reconfigure instance --instance my-server-2 --cluster-id main --force --restart
```

See [Configuration variables](../reference/configuration.md).

## Multiple instances

`--instance all` targets every instance in start, stop, restart, save and
`create backup`:

```sh
ark-cli stop instance --instance all
ark-cli create backup --instance all
```

## Next

- [Schedule backups](../how-to/schedule-backups.md)
- [Reconfigure an instance](../how-to/reconfigure-instance.md)
