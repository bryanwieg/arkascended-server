# How Updates Work

ARK images are built on your host from Steam's current public build. Nothing
is pulled from a registry and no ARK images are published anywhere. Each
image is immutable: the server files live inside it, and only
`ShooterGame/Saved` and clusters are volumes. That is what makes rollback a
tag switch.

## Building an image

`ark-cli` builds `ark-server:<steam build id>-<recipe id>` (also tagged
`ark-server:<version>` and `ark-server:latest`). The build recipe is embedded
in the `ark-cli` binary and extracted to `<compose dir>/.ark-manage/build/context`
when needed, so no repo clone is required. The recipe ID is a hash of that
recipe, so a new `ark-cli` whose container scripts changed builds a new image
even when ARK itself hasn't updated.

- The first `start instance` builds the image if it does not exist yet and
  pins compose to `ark-server:<build-id>-<recipe-id>`. If that image already
  exists, no build happens.
- SteamCMD runs in a throwaway container. Its install is cached in
  `<compose dir>/.ark-manage/build/work/cache` (about 12 GB, kept), so later
  updates download only changed files.
- Before a build, free space in `.ark-manage` must be at least
  `ARK_BUILD_REQUIRED_FREE_GB` (default 50) GiB. A lock stops two builds from
  running at once.

Requirements: about 24 GB RAM, 60–70 GB free disk (current and previous image
at about 25 GB each, the SteamCMD cache, and the build peak), and Docker with
BuildKit/buildx. The host must reach Steam (`api.steamcmd.net` and the Steam
CDN) and `ghcr.io`, where the ARK-free base image (packages, Proton) is
published. If the base can't be pulled, it is built locally, which needs GitHub
(GE-Proton), Launchpad (deadsnakes PPA), NuGet (XAudio2) and Ubuntu mirrors. A
build takes about 15–25 minutes.

## Updating

`update instance` compares the instance's `io.ark.server.build_id` image label
with Steam's current public build, and its `io.ark.recipe` label with the
recipe embedded in this `ark-cli`. Either difference is an update. Images built
before recipe labels existed count as a recipe change once.

1. `--check` only prints the comparison table.
2. Without `--rollout` (or its alias `--restart`), it builds the new image and
   leaves compose unchanged. It prints
   `Built ark-server:<id>. Switch with: ark-cli update instance --instance <name> --rollout`.
3. With `--rollout`, for each instance in name order: stop (the stop saves the
   world), create a `pre-update` backup, switch compose to the new tag, then
   start and wait until ready.

A failed build changes nothing.

## Automatic rollback

If the new image fails to start, `ark-cli` switches compose back, restores the
pre-update backup, starts the old image and records the attempt. It prints
`Update of <name> to <new> failed; rolled back to <old>: <reason>`, skips the
remaining instances and exits with code 1.

Pre-update backups are normal tar.gz backups in the usual backup directory,
named with the prefix `pre-update`, and follow the usual retention (20 backups,
7 days, 5 GiB).

## Alerts

The result of the last update attempt is stored per instance in
`<compose dir>/.ark-manage/state/updates/<instance>.json` and shown by
`check instance` as `last_update:` (and `last_update_message:`). It does not
change the exit code.

Set `ARK_UPDATE_WEBHOOK_URL` to be notified: on `build_failed`, `rolled_back`
or `rollback_failed`, `ark-cli` posts a Discord-format JSON message to that
URL. See [Configuration variables](../reference/configuration.md).

## Old images are removed

After `update instance`, and after every `start instance` or `restart instance`,
the CLI keeps each instance's current image and its previous image (the
rollback image). Older `ark-server` images, all temporary `ark-server-files`
images and unused build cache are removed.

- An image a container still uses, running or stopped, keeps all its tags. This
  includes containers of other compose projects on the same host, whose images
  this project's cleanup can't otherwise tell apart from old ones.
- Only ARK images are touched; other services' images in the compose file are
  left alone, as are images pinned by digest.
- Removed images are listed (`Removed old image …`). If removal fails, the
  command prints a warning but still succeeds.

To run an update, see [Update your server](../how-to/update-server.md).
For the build pipeline details, see the developer
[image build reference](../../dev/reference/image-build.md).
