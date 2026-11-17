# Image Build Pipeline

The ARK image is built on the user's host and never published. Only its
ARK-free base is published (see [base image](#base-image)). The build recipe is
the `container/` tree (`base.dockerfile`, `dockerfile`, `dockerfile.labels`,
`scripts/`, and the server-files scripts in `container/files/`). It is
embedded in the `ark-cli` binary (Go module `container/embed.go`, package
`arkcontainer`) and extracted to `<compose dir>/.ark-manage/build/context` at
build time. No repo clone is needed.

Three images take part:

- Base image (`container/base.dockerfile`): packages, locale, `ark-user`, GE-Proton
  and the XAudio redist. Identical for every ARK build and host; see
  [base image](#base-image).
- Runtime build (`container/dockerfile`): starts `FROM ${BASE_IMAGE}` and adds the
  server files, the XAudio copy into the server, and the scripts. It runs the server as `ark-user` for least privilege. The server
  files come from the temporary [server files image](#server-files-image): the
  build copies in its SteamCMD client files (`~/.local/share/Steam`, which
  `STEAM_COMPAT_CLIENT_INSTALL_PATH` points at) and its five server
  directories, each as a separate layer. The ~11 GB of pak files are spread
  over four of them to keep each layer small.
- Labeled build (`container/dockerfile.labels`): adds version/build-id labels and tags.

## CLI-native build

The CLI builds images natively through the Docker API with BuildKit
(`version=2`) — no external scripts required. The API defaults to the legacy
builder, which rejects `COPY --chmod` and `RUN --mount`, so the CLI requests
BuildKit explicitly. It applies `container/.dockerignore` itself, because it
tars the context client-side. Build errors arrive inside an HTTP 200 response
stream, so the CLI decodes the stream and reports them. This is the path used
by `ark-cli start instance` (when `ark-server:<build-id>` is missing) and
`ark-cli update instance` (when Steam has a newer build).

Before building, the CLI checks that free space in `.ark-manage` is at least
`ARK_BUILD_REQUIRED_FREE_GB` (default 50) GiB and takes a build lock so two
builds cannot run at once. If `ark-server:<id>` already exists, nothing is
built.

The build process:

0. **Base image:** `arkcontainer.BaseImageRef()`; a local copy, else pulled from
   GHCR, else built here from `base.dockerfile` (see [base image](#base-image)).
1. **Server files:** `container/files/build.sh` downloads the build from Steam
   in a throwaway SteamCMD container and builds the temporary local image
   `ark-server-files:<id>`.
2. **Stage 1:** a BuildKit build of `container/dockerfile` with
   `BASE_IMAGE` and `ARK_FILES_IMAGE` produces `ark-server:base` with the full
   server installation.
3. **Extract labels:** the CLI runs throwaway containers against the base image
   to read `/home/ark-user/ARK_SERVER_VERSION` and
   `/home/ark-user/ARK_SERVER_BUILDID`.
4. **Stage 2 (labeled):** a build of `container/dockerfile.labels`
   with `--build-arg BASE_IMAGE`, `ARK_SERVER_VERSION`, and `ARK_BUILD_ID`
   produces the final tagged images:
   - `ark-server:latest`
   - `ark-server:<version>` (e.g. `84.19`)
   - `ark-server:<build-id>` (e.g. `22359937`)
5. **Cleanup:** `ark-server-files:<id>` is removed. Cleanup after start/update
   keeps each instance's current and previous image; older `ark-server` images,
   all `ark-server-files` images and base tags other than the current one are
   removed.

### Base image

`ghcr.io/bryanwieg/ark-server-base:<tag>`, where `<tag>` is the first 12 hex
characters of the sha256 of `container/base.dockerfile`. `ark-cli` computes it from
its embedded recipe and `.github/build-image.sh` from the file, so each recipe maps
to exactly one base and no version is kept by hand. Because the file alone defines
the tag, `base.dockerfile` never copies from the build context (a test enforces it).

Both get the base the same way: a local copy, else a pull from GHCR, else a local
build of `base.dockerfile` tagged with the same name. A host therefore needs GHCR
only to skip that local build, which reaches GitHub, Launchpad, NuGet and the
Ubuntu mirrors.

CI publishes the base from `main` after the smoke tests pass, and only when GHCR
doesn't have the tag. The `# refresh: YYYY-MM` line is bumped monthly by
`.github/workflows/base-refresh.yml` (it opens a PR); the new tag makes CI rebuild
the base with current Ubuntu packages. See [ci.md](ci.md).

### Server files image

SteamCMD doesn't run inside the BuildKit build. `container/files/build.sh` and
`download.sh` run it in a throwaway container with the work folder
`<compose dir>/.ark-manage/build/work`:

- `work/cache` keeps the SteamCMD install (~12 GB), so later updates download
  only changed files.
- `work/out` holds hardlinks into the cache (`cp -al`, so no extra disk) and is
  deleted after each build.
- The result is `ark-server-files:<Steam build ID>`, which the runtime build
  copies from (`--build-arg ARK_FILES_IMAGE=ark-server-files:<id>`).

A new ARK build changes the tag, so `update instance` picks up the new server.
Every other change reuses the cached `COPY` layers.

Why SteamCMD runs outside BuildKit: its download used to live in a build stage
with BuildKit cache mounts, but BuildKit evicted those mounts (default limits),
so updates fell back to full downloads, and on GitHub-hosted runners Docker's
default seccomp profile blocks SteamCMD inside containers (CI passes
`STEAMCMD_DOCKER_OPTS="--security-opt seccomp=unconfined"`).

### Layer and cache layout

Each section reruns only when its own inputs change, to limit downloads and
rebuilt layers:

- **Build-only stages pin their base by digest** (`BUILD_BASE_IMAGE`). A new
  Ubuntu release then can't invalidate the cached Proton or XAudio
  downloads. Bump the digest deliberately.
  - The runtime stage uses the `ubuntu:24.04` tag so it keeps getting
    security updates.
  - Its rebuild re-runs the package install; the server files layers are
    copied again from the files image.
- **Proton and XAudio each have their own stage**, so a Proton bump
  downloads only Proton.
- **The image is ordered from rarely to often changing:**
  1. packages, locale and user (base)
  2. Proton and the staged XAudio redist (base)
  3. Steam client files
  4. the ARK layers
  5. XAudio, copied into the server so it wins over any copy ARK ships
  6. `ark-image-labels.sh` and the runtime scripts
  7. the default `GameUserSettings.ini` (from the `ini_defaults` stage)
  8. the version files and labels

  `ARG PROTON_VERSION` is declared just before the Proton layer, so a Proton
  bump doesn't re-run the package install. A script change rebuilds only the small layers
  at the end.
- **The `ini_defaults` stage generates the default `GameUserSettings.ini`.** It
  starts from the runtime stage and runs
  `scripts/installer/generate-default-ini.py`, which starts ARK once, stops it
  with SIGTERM and keeps the complete file ARK writes on shutdown (the first
  file ARK writes is partial). The final image copies only that INI to
  `/home/ark-user/config-defaults/GameUserSettings.ini`: the run leaves thousands
  of Proton prefix files and ARK logs that must not ship. The step takes a few
  seconds and fails the build if ARK doesn't write the complete file. It reruns
  whenever the runtime stage changes, scripts included.

**CI:** `image.yml` builds the image from Steam on a hosted runner and runs
the smoke tests; on `main` it publishes only the base image (see [ci.md](ci.md)).

### Build environment variables

- `ARK_BUILD_NO_CACHE` — when set to any non-empty value, passes `--no-cache`
  to the stage-1 buildx command. Used by integration tests to force a real
  build instead of hitting the Docker layer cache. Not needed to pick up an
  ARK update; the files image tag handles that.
- `ARK_BUILD_REQUIRED_FREE_GB` (default `50`) — minimum free GiB in
  `.ark-manage` before a build starts.

## CI/manual build script

The `.github/build-image.sh` script performs the same build (files image,
then the two image stages) and is used by CI and for manual rebuilds outside
the CLI. The CLI never calls this script.

Useful build knobs (all optional):

- `IMAGE_NAME` (default: `ark-server`)
- `CONTEXT_PATH` (default: `container/`) — build context directory
- `DOCKERFILE_PATH` (default: `container/dockerfile`)
- `LABELS_DOCKERFILE_PATH` (default: `container/dockerfile.labels`)
- `BASE_TAG` (default: `ark-server:base`)
- `FINAL_TAG` (default: `ark-server:latest`)

## Update check

The CLI checks for updates by comparing the current image's
`io.ark.server.build_id` label against the latest build ID from the Steam API
(`https://api.steamcmd.net/v1/info/2430930`). If the build IDs differ, an
update is available and the CLI builds the new image. With `--rollout` it then
runs the safe switch: stop, `pre-update` backup, compose to the new tag, start
and wait; on failure it switches compose back, restores the backup and starts
the old image. Per-instance state is in
`<compose dir>/.ark-manage/state/updates/<instance>.json` (`previous_image`
and `last_attempt` with `result` of `succeeded`, `rolled_back`,
`rollback_failed` or `build_failed`). `ARK_UPDATE_WEBHOOK_URL` is called on
failures.

The `.github/update-check.sh` script performs the same check for CI pipelines.
The CLI never calls this script — the check logic is self-contained in Go.

### Update environment variables

- `ARK_STEAM_API_URL` — override the Steam API endpoint. Used by integration
  tests to point at a fake HTTP server. Defaults to the real steamcmd.net URL.
- `ARK_UPDATE_WEBHOOK_URL` — optional webhook for failed updates.
