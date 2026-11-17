# Update Your Server

How to check for and apply ARK server image updates. For what happens during
an update, see [How updates work](../explanation/updates.md).

## Check for updates without applying

```sh
ark-cli update instance --check
```

Reports whether a newer image is available for each instance without making
any changes.

## Build the new image

```sh
ark-cli update instance --instance my-server
```

This builds the new image only and leaves compose unchanged. It prints
`Built ark-server:<id>. Switch with: ark-cli update instance --instance my-server --rollout`.
The build needs free disk space (see `ARK_BUILD_REQUIRED_FREE_GB`).

## Switch with a safe rollout

```sh
ark-cli update instance --instance my-server --rollout
# or all instances, one after another in name order:
ark-cli update instance --instance all --rollout
```

`--restart` is an alias for `--rollout`. Each instance is stopped (saving the
world), backed up (`pre-update` backup), switched to the new image, started,
and waited on until ready. If the new image fails, the instance is rolled back
to the old image and the backup restored; the command prints
`Update of <name> to <new> failed; rolled back to <old>: <reason>`, skips the
remaining instances and exits with code 1.

## Check the last update result

`ark-cli check instance` shows `last_update:` for each instance. To get a chat
message on failures, set `ARK_UPDATE_WEBHOOK_URL` (see
[Configuration variables](../reference/configuration.md)).

## Verbose output

Use `-v` to see detailed logging of the update process — Steam API requests,
image label inspection, build-ID comparison, and Docker build output:

```sh
ark-cli -v update instance --check
```
