# Update Your Server

Background: [How updates work](../explanation/updates.md).

`--instance` defaults to `all`.

```sh
ark-cli update instance --check                          # compare only, change nothing
ark-cli update instance --instance my-server             # build the new image; compose unchanged
ark-cli update instance --instance my-server --rollout   # build if needed, then switch
```

`--rollout` and `--restart` are the same flag. Per instance, in name order:
stop (saves), `pre-update` backup, switch image, start, wait for ready. On
failure the instance is rolled back and restored, the remaining instances are
skipped, and the exit code is 1.

The build needs `ARK_BUILD_REQUIRED_FREE_GB` (default 50) GiB free.

## After an update

- `ark-cli check instance --instance my-server` shows `last_update:`.
- Set `ARK_UPDATE_WEBHOOK_URL` for failure alerts
  ([Configuration variables](../reference/configuration.md)).
- `ark-cli -v update instance ...` logs Steam API calls, label comparison and
  build output.
