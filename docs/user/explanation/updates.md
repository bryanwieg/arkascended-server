# How Updates Work

ARK images are built on the host from Steam's current public build. No ARK
image is published or pulled; only the ARK-free base image comes from
`ghcr.io`. Server files live inside the immutable image; only
`ShooterGame/Saved` and clusters are volumes, so rollback is a tag switch.

## Building an image

`ark-cli` builds `ark-server:<steam build id>-<recipe id>` (also tagged
`ark-server:<version>` and `ark-server:latest`). The build recipe is embedded
in `ark-cli` and extracted to `<compose dir>/.ark-manage/build/context`. The
recipe ID is its hash, so an `ark-cli` with changed container scripts builds a
new image even when ARK hasn't updated.

- The first `start instance` builds the image if it does not exist yet and
  pins compose to `ark-server:<build-id>-<recipe-id>`. If that image already
  exists, no build happens.
- SteamCMD runs in a throwaway container. Its install is cached in
  `<compose dir>/.ark-manage/build/work/cache` (about 12 GB, kept), with
  SteamCMD's depot manifests, so later updates download only changed files.
- Before a build, free space in `.ark-manage` must be at least
  `ARK_BUILD_REQUIRED_FREE_GB` (default 50) GiB. A lock stops two builds from
  running at once.

A build takes 15-25 minutes. Host, disk and network requirements:
[README](../../../README.md#prerequisites).

## Updating

`update instance` compares the instance's `io.ark.server.build_id` image label
with Steam's current public build, and its `io.ark.recipe` label with the
recipe embedded in this `ark-cli`. Either difference is an update. Images built
before recipe labels existed count as a recipe change once.

1. `--check` only prints the comparison table. Its `target` column shows
   Steam's build (`build <id>`): the ARK version is only known from the built
   image's `io.ark.server.version` label.
2. Without `--rollout` (same as `--restart`), it builds the new image, prints
   the table with the new version and one line per instance,
   `<instance>: <current version> -> <new version> (build <id>)`, and
   leaves compose unchanged. It prints
   `Built ark-server:<id>. Switch with: ark-cli update instance --instance <name> --rollout`.
3. With `--rollout`, for each instance in name order: stop (the stop saves the
   world), create a `pre-update` backup, switch compose to the new tag, then
   start and wait until ready. Players online get a countdown
   (`SHUTDOWN_WARNING_MINUTES`, default 5) naming the update (`updating to 98.9`); see
   [Configuration](../reference/configuration.md). Each step prints one line
   (`asa1: stopping …`, `asa1: backup saved to …`, `asa1: ready`), without `-v`.

The image is built before any instance is touched, so a failed build changes
nothing. Its error names the cause in one line and ends with
`no instances were changed`, e.g.
`Error: server files build failed: SteamCMD download failed: state is 0x6; no instances were changed`.

## Automatic rollback

If the new image fails to start, `ark-cli` switches compose back, restores the
pre-update backup, starts the old image and records the attempt. It prints
`Update of <name> to <new> failed; rolled back to <old>: <reason>`, skips the
remaining instances and exits with code 1.

The pre-update backup is `<instance>-pre-update-<YYYYMMDD-HHMMSS>.tar.gz` in the instance's
backup directory.

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

Steps: [Update your server](../how-to/update-server.md). Build pipeline:
[image build reference](../../dev/reference/image-build.md).
