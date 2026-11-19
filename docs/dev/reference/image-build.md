# Image Build Pipeline

The ARK image is built on the user's host and never published; only its ARK-free
base is (see [base image](#base-image)). The recipe is the `container/` tree
(`base.dockerfile`, `dockerfile`, `dockerfile.labels`, `.dockerignore`, `scripts/`,
`files/`), embedded in `ark-cli` (`container/embed.go`, package `arkcontainer`) and
extracted to `<compose dir>/.ark-manage/build/context` at build time. No repo clone
needed.

Images involved:

| Image | Source | Contents |
| --- | --- | --- |
| Base `ghcr.io/bryanwieg/ark-server-base:<tag>` | `container/base.dockerfile` | packages, locale, `ark-user`, GE-Proton, staged XAudio redist. Same for every ARK build and host. |
| Server files `ark-server-files:<build-id>` (temporary) | `container/files/` | SteamCMD client files and the server split into `layers/0..4`. See [server files image](#server-files-image). |
| `ark-server:base` | `container/dockerfile` (`FROM ${BASE_IMAGE}`) | Steam client files to `~/.local/share/Steam` (`STEAM_COMPAT_CLIENT_INSTALL_PATH`), five server layers (layer 0: everything but paks; 1-4: the ~11 GB of pak files), XAudio copied into the server, scripts, default INI, version files. Runs as `ark-user`. |
| `ark-server:<build-id>-<recipe-id>`, `:<version>`, `:latest` | `container/dockerfile.labels` | `ark-server:base` plus labels. |

## CLI-native build

`ark-cli` builds through the Docker API (`app/pkg/ark/sdkbackend/engine.go`), no
external scripts:

- Requests BuildKit explicitly (`build.BuilderBuildKit`); the API default is the
  legacy builder, which rejects `COPY --chmod` and `RUN --mount`.
- Tars the context client-side, so it applies `container/.dockerignore` itself.
- Build errors arrive inside an HTTP 200 stream; the CLI decodes the stream and
  reports them with the tail of the build output.

Triggers:

- `ark-cli start instance`: compose image is `ark-server`/`ark-server:latest` and no
  `:latest` exists, or the pinned `ark-server:<tag>` is missing and equals Steam's
  current build with this recipe. Any other missing tag asks for
  `update instance --rollout`.
- `ark-cli update instance`: Steam has a newer build, or the image's `io.ark.recipe`
  differs from this binary's recipe.

`BuildLocalImage` (`app/pkg/ark/update/build.go`) returns at once if
`ark-server:<build-id>-<recipe-id>` exists. Otherwise it takes the `image-build`
lock in `.ark-manage/locks` (rechecks the image inside it), then requires
`ARK_BUILD_REQUIRED_FREE_GB` (default 50) GiB free in `.ark-manage`.

Recipe ID (`arkcontainer.RecipeID()`): first 12 hex chars of a SHA-256 over every
embedded file, path and contents (`__pycache__` skipped). *Why:* without it an
`ark-cli` with changed scripts would reuse the image for the same ARK build. A
separate tag, not a rebuilt `ark-server:<build-id>`, keeps the old image for
rollback.

Steps:

0. **Base image:** `arkcontainer.BaseImageRef()`; local copy, else pull, else build
   from `base.dockerfile` (see [base image](#base-image)).
1. **Server files:** `container/files/build.sh` downloads the Steam build in a
   throwaway SteamCMD container and builds `ark-server-files:<build-id>`.
2. **Stage 1:** BuildKit build of `container/dockerfile` with `BASE_IMAGE` and
   `ARK_FILES_IMAGE` → `ark-server:base`.
3. **Read version:** the CLI creates (does not start) a container from
   `ark-server:base` and copies out `/home/ark-user/ARK_SERVER_VERSION` and
   `/home/ark-user/ARK_SERVER_BUILDID` (written by
   `scripts/installer/ark-image-labels.sh`). Fails if the build ID differs from
   what Steam reported.
4. **Stage 2:** build of `container/dockerfile.labels` with build args `BASE_IMAGE`,
   `ARK_SERVER_VERSION`, `ARK_BUILD_ID`, `ARK_RECIPE_ID`. Labels:
   `org.opencontainers.image.version`, `org.opencontainers.image.description`
   (`Build ID: <id>`), `io.ark.server.version`, `io.ark.server.build_id`,
   `io.ark.recipe`. Tags:
   - `ark-server:<build-id>-<recipe-id>` (e.g. `22359937-a3db839e5516`)
   - `ark-server:<version>` (e.g. `84.19`)
   - `ark-server:latest`
5. **Cleanup:** `ark-server-files:<build-id>` is removed.

Image cleanup after start/update (`RemoveOldImages`, `images.go`) keeps each
instance's compose image and its `previous_image`; removes other `ark-server`
tags, all `ark-server-files` tags, base tags other than the current one, then
dangling images and unused build cache. Never removes a tag of an image any
container uses (`ImagesInUse`, any project, running or stopped).

### Base image

`ghcr.io/bryanwieg/ark-server-base:<tag>`, `<tag>` = first 12 hex chars of
sha256(`container/base.dockerfile`). `ark-cli` hashes its embedded copy,
`.github/build-image.sh` the file, so each recipe maps to one base and no version
is kept by hand. Since the file alone defines the tag, `base.dockerfile` never
copies from the build context (`container/embed_test.go` enforces it).

Both resolve it the same way: local copy, else pull from GHCR, else local build
tagged with the same name. GHCR only saves that build, which reaches GitHub,
Launchpad, NuGet and the Ubuntu mirrors.

CI publishes the base from `main` after the smoke tests pass, only if GHCR lacks
the tag. `.github/workflows/base-refresh.yml` bumps the `# refresh: YYYY-MM` line
monthly via PR; the new hash rebuilds the base with current Ubuntu packages. See
[ci.md](ci.md).

### Server files image

SteamCMD runs outside BuildKit: `container/files/build.sh` runs `download.sh` in a
throwaway container with work folder `<compose dir>/.ark-manage/build/work`:

- `work/cache`: persistent SteamCMD install (~12 GB); later updates download only
  changed files.
- `work/cache/depotcache`: SteamCMD's depot manifests, copied into the throwaway
  container's Steam home before the download and back after. An update needs the
  installed build's manifest; Steam refuses superseded manifests to anonymous
  logins, which ends in `state is 0x6`. A cache without it is seeded once from
  `ARK_DEPOTCACHE_SEED_IMAGE` (default `ark-server:latest`), whose Steam client
  files include it.
- `work/out`: hardlinks into the cache (`cp -al`, no extra disk), split into
  `layers/0..4` by `ark-split-layers.sh`; deleted after each build.
- Result: `ark-server-files:<build-id>`, the runtime build's `ARK_FILES_IMAGE`.

A new ARK build changes the tag, so `update instance` picks up the new server;
anything else reuses the cached `COPY` layers.

Why outside BuildKit: the download used to be a build stage with cache mounts,
which BuildKit evicted (default limits), so updates fell back to full downloads.
Also, on GitHub-hosted runners Docker's default seccomp profile blocks SteamCMD;
CI passes `STEAMCMD_DOCKER_OPTS="--security-opt seccomp=unconfined"`.

### Layer and cache layout

Each part reruns only when its own inputs change:

- **`base.dockerfile` build-only stages pin by digest** (`BUILD_BASE_IMAGE`), so a
  new Ubuntu release can't invalidate cached Proton/XAudio downloads. Bump the
  digest deliberately. The base's final stage uses the `ubuntu:24.04` tag to get
  security updates on each rebuild. A new base tag rebuilds every runtime layer.
- **Proton and XAudio have separate fetch stages**, so a Proton bump downloads only
  Proton.
- **Layer order, rarely to often changing:**
  1. packages, locale, user (base)
  2. Proton, staged XAudio redist (base)
  3. Steam client files
  4. ARK server layers
  5. XAudio copied into the server, so it wins over any copy ARK ships
  6. `ark-image-labels.sh` and runtime scripts
  7. default `GameUserSettings.ini` (from `ini_defaults`)
  8. version files and labels

  `ARG PROTON_VERSION` is declared just before the Proton layer, so a Proton bump
  doesn't rerun the package install. A script change rebuilds only the small layers
  at the end.
- **`ini_defaults` stage** (from the runtime stage) runs
  `scripts/installer/generate-default-ini.py`: starts ARK, sends SIGTERM once the
  partial INI appears, keeps the complete file ARK writes on shutdown, strips the
  random `SessionName`. Only that file is copied to
  `/home/ark-user/config-defaults/GameUserSettings.ini`; the Proton prefix and logs
  from the run don't ship. Fails the build if no complete file appears. Reruns
  whenever the runtime stage changes, scripts included.

**CI:** `image.yml` builds from Steam on a hosted runner and runs the smoke tests;
on `main` it publishes only the base (see [ci.md](ci.md)).

### Build environment variables

- `ARK_BUILD_NO_CACHE`: any non-empty value disables the cache for the stage-1
  (`container/dockerfile`) build. Used by integration tests to force a real build.
  Not needed for ARK updates; the files image tag handles that.
- `ARK_BUILD_REQUIRED_FREE_GB` (default `50`): minimum free GiB in `.ark-manage`.
  Must be a positive integer.

## CI/manual build script

`.github/build-image.sh` does the same build (base, files image, both stages) with
`docker buildx build --load` or `podman build`. Used by CI and manual rebuilds; the
CLI never calls it. It tags and labels like the CLI (`:<build-id>-<recipe-id>`,
`:<version>`, `:latest`, `io.ark.recipe`), taking the recipe ID from
`go run ./container/cmd/recipe-id`, so it needs Go. The last output line is
`BASE_IMAGE=<ref>` for CI to publish.

Knobs (all optional):

| Variable | Default |
| --- | --- |
| `CONTAINER_RUNTIME` | `docker` (or `podman`) |
| `IMAGE_NAME` | `ark-server` |
| `CONTEXT_PATH` | `container/` |
| `DOCKERFILE_PATH` | `container/dockerfile` |
| `LABELS_DOCKERFILE_PATH` | `container/dockerfile.labels` |
| `BASE_TAG` | `<IMAGE_NAME>:base` |
| `FINAL_TAG` | `<IMAGE_NAME>:latest` |
| `ARK_BUILD_WORK_DIR` | `.ark-tmp/build-work` (SteamCMD work folder) |
| `ARK_BUILD_DELETE_CACHE` | unset; `1` deletes the work folder once the files image exists |
| `BUILD_REQUIRED_FREE_GB` | `50`; podman only, exits 77 if less is free |
| `STEAMCMD_DOCKER_OPTS` | unset; passed to the SteamCMD container |

## Update check

`update instance` compares the image's `io.ark.server.build_id` label with Steam's
public build ID (`https://api.steamcmd.net/v1/info/2430930`) and its
`io.ark.recipe` with this binary's recipe ID. Either differing means an update:
the CLI builds the new image. With `--rollout` it then stops, takes a `pre-update`
backup, switches compose to the new tag, starts and waits; on failure it switches
compose back, restores the backup and starts the old image.

State per instance: `<compose dir>/.ark-manage/state/updates/<instance>.json`
(`previous_image`; `last_attempt` with `result` = `succeeded`, `rolled_back`,
`rollback_failed` or `build_failed`). Failures are posted to
`ARK_UPDATE_WEBHOOK_URL` if set.

`.github/update-check.sh` is a standalone host/CI helper for the same check (env
`IMAGE`, `STEAM_API_URL`; exit 10 = update available). No workflow or CLI code
calls it.

### Update environment variables

- `ARK_STEAM_API_URL`: Steam API endpoint override (integration tests point it at
  a fake server). Default: the steamcmd.net URL above.
- `ARK_UPDATE_WEBHOOK_URL`: webhook for failed updates; must be http(s), else alerts
  are disabled.
