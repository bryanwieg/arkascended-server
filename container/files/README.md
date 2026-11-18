# Server files build

Downloads the ARK: Survival Ascended dedicated server (Steam app 2430930) with
SteamCMD and packages it as a files-only image, `<image>:<Steam build ID>`. The
runtime [`dockerfile`](../dockerfile) copies from it (`ARK_FILES_IMAGE`).
Callers: `update instance` (`app/pkg/ark/update/build.go`) and
[`.github/build-image.sh`](../../.github/build-image.sh); both pass image
`ark-server-files`. Cache and design: [image-build.md](../../docs/dev/reference/image-build.md#server-files-image).

| File | Role |
|------|------|
| `build.sh [image] [build args]` | Builds `steamcmd.dockerfile`, runs `download.sh` in it, then builds `dockerfile` from `$WORK_DIR/out`. Host needs Docker or Podman, plus `curl` and `python3` when `ARK_STEAM_BUILD_ID` is unset. |
| `steamcmd.dockerfile` | Throwaway SteamCMD container (pinned Ubuntu 24.04, `HOME=/home/ark-user`). |
| `download.sh <work>` | Installs into `cache/ark-server`, then builds `out/`: `steam-client/` plus the server hardlinked (`cp -al`) and split into `layers/0..4`. Removes `steamclient64.dll` from `out/`. |
| `dockerfile` | `FROM scratch`; copies `steam-client` and `layers/0..4` to `/home/ark-user/`, owned 3000:3000. |
| `scripts/installer/ark-server-install.sh` | SteamCMD `app_update` with retry on "Missing configuration". |
| `scripts/installer/ark-split-layers.sh` | Spreads the pak files over `layers/1..4` so no layer hits GHCR's 10 GB limit. |

`build.sh` environment:

| Var | Default | Purpose |
|-----|---------|---------|
| `ARK_STEAM_BUILD_ID` | current public build (api.steamcmd.net) | Build to download; `download.sh` fails if Steam serves another. |
| `CONTAINER_RUNTIME` | `docker` | `docker` or `podman`. |
| `WORK_DIR` | `container/files/work` | `cache/` (~12 GB SteamCMD install, kept) and `out/` (removed after each run). |
| `STEAMCMD_DOCKER_OPTS` | none | Extra `run` options for the SteamCMD container, e.g. `--security-opt seccomp=unconfined`. |
