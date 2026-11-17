# Check Port Reachability

How to verify that your server's ports are reachable from the internet.

## Check an instance's ports

With your server running:

```sh
ark-cli check reachability --instance my-server
```

This reads the ports from your compose file and checks both:
- The ARK game port (UDP) — what players connect to
- The RCON port (TCP) — used internally for server management

Pass/fail is reported for each port.

## Check only one port type

```sh
ark-cli check reachability --instance my-server --ports asa    # game port only
ark-cli check reachability --instance my-server --ports rcon   # RCON port only
ark-cli check reachability --instance my-server --ports all   # both (default)
```

## Check a specific port manually

If you want to test a port without reading your compose file:

```sh
ark-cli check reachability --port 27015
ark-cli check reachability --port 27015 --protocol udp
ark-cli check reachability --port 27020 --protocol tcp
ark-cli check reachability --port 27015 --protocol both
```

## Choose where to probe from

By default, both local and internet-side checks are performed. Use `--from` to
select a specific vantage point:

```sh
ark-cli check reachability --port 27015 --from local      # localhost only
ark-cli check reachability --port 27015 --from remote     # internet only
ark-cli check reachability --port 27015 --from 10.0.0.1  # custom host
ark-cli check reachability --port 27015 --from both       # both (default)
```

## Test port forwarding without a running server

Use `--listen` to have the CLI open a temporary listener during the probe.
This lets you verify port forwarding is working even when the server is stopped:

```sh
ark-cli check reachability --port 27015 --protocol udp --listen
ark-cli check reachability --port 27020 --protocol tcp --listen
```

Note: `--listen` will fail for an instance check if the server is already
running and bound to the same ports. Stop the server first or use the manual
port form.

## Open listeners without probing

Use `listen port` to open listeners and block until Ctrl+C. This is useful for
manually verifying firewall rules from another machine:

```sh
ark-cli listen port --port 27015
ark-cli listen port --port 27015 --protocol udp
ark-cli listen port --port 27015 --protocol both
```

## Troubleshooting

If ports show as unreachable:

1. Check that your firewall (local and cloud/router) allows inbound traffic on
   the game port (UDP) and optionally the RCON port (TCP).
2. Make sure the ports in your compose file match the ones you've forwarded.
   Check with `ark-cli show instance --instance my-server`.
3. If your host is behind NAT, the external check goes out to the internet and
   back in — make sure port forwarding is set up on your router.

External checks use `starrupture-utilities.com` as the primary probe, with
`portchecker.io` as a TCP fallback.
