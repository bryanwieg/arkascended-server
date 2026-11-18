# Check Port Reachability

Full flag list: [cli-commands.md](../reference/cli-commands.md#port-reachability).

## Check an instance's ports

Instance must be running (unless `--listen`, below).

```sh
ark-cli check reachability --instance my-server
ark-cli check reachability --instance my-server --ports asa    # game port only
ark-cli check reachability --instance my-server --ports rcon   # RCON only
```

Host ports come from the compose file (`--compose-file` or `COMPOSE_FILE` to override):

| Label | Protocol | Port |
|---|---|---|
| `ASA` | UDP | game port players connect to |
| `RCON` | TCP | host port published for container port 27020 |

RCON is published on `127.0.0.1` by default, so `CLOSED` from the internet or another host is
expected; the CLI prints a note. `ark-cli enable remote-rcon --instance <name>` publishes it on all
interfaces.

## Check a port by number

No compose file or running server needed. `--protocol` is `tcp`, `udp` or `both` (default).

```sh
ark-cli check reachability --port 27015 --protocol udp
ark-cli check reachability --port 27020 --protocol tcp
```

## Choose where to probe from

`--from` (default `both`):

| Value | Probes |
|---|---|
| `remote` | From the internet, against this host's public IP |
| `local` | `127.0.0.1` from this host |
| `both` | `remote` + `local` |
| `<addr>` | The given address, from this host |

Output is a table with one column per vantage point. Statuses:

| Status | Meaning |
|---|---|
| `OPEN` | Probe got a response (TCP connect, or UDP reply to a UE5 handshake packet) |
| `CLOSED` | Nothing listening / connection refused |
| `LISTENING` | UDP port is bound locally but did not answer the probe |
| `UNKNOWN` | External checker got no conclusive answer (common for UDP), or a remote `--from <addr>` UDP port gave no reply |
| `ERROR` | External checker request failed |

The public IP comes from `portchecker.io/api/me`, falling back to `api.ipify.org`. Internet checks
use `starrupture-utilities.com` (TCP and UDP), falling back to `portchecker.io` for TCP.

## Test forwarding without a running server

`--listen` opens temporary TCP/UDP listeners on the port(s) for the duration of the probe. The UDP
listener echoes packets, so a UDP probe gets a definitive `OPEN`.

```sh
ark-cli check reachability --port 27015 --protocol udp --listen
ark-cli check reachability --instance my-server --listen
```

- Fails with "address already in use" if the server (or anything else) holds the port. Stop it first.
- Not allowed with `--from remote`.

## Keep listeners open for an external test

`listen port` opens listeners (default `--protocol both`) and blocks until Ctrl+C, for probing
from another machine with your own tools.

```sh
ark-cli listen port --port 27015 --protocol udp
```

## Troubleshooting

If the game port is not `OPEN` from the internet:

1. Firewalls: host, cloud security group, router. Game port is UDP.
2. Forwarded port must match the compose host port (`ark-cli show instance --instance my-server`
   shows the game port).
3. Behind NAT: forward the port on the router to this host.
4. `UNKNOWN` on UDP with the server running: re-run with the server stopped and `--listen`.
