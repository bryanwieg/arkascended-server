# Server files build

Downloads the ARK: Survival Ascended dedicated server (Steam app 2430930) with
SteamCMD into `$WORK_DIR/out`; it builds no image. The caller moves `out` into
the build context as `ark-files/`, which the runtime [`dockerfile`](../dockerfile)
copies from. Callers: [`.github/build-image.sh`](../../.github/build-image.sh)
runs `build.sh`; `update instance` (`app/pkg/ark/update/build.go`) runs
`download.sh` the same way from Go. Cache and design: [image-build.md](../../docs/dev/reference/image-build.md#server-files-image).

| File | Role |
|------|------|
| `build.sh` | Builds `steamcmd.dockerfile` and runs `download.sh` in it, leaving `$WORK_DIR/out` for the caller (removed on failure). Host needs Docker or Podman, plus `curl` and `python3` when `ARK_STEAM_BUILD_ID` is unset. |
| `steamcmd.dockerfile` | Throwaway SteamCMD container (pinned Ubuntu 24.04, `HOME=/home/ark-user`). |
| `download.sh <work>` | Installs into `cache/ark-server`, then builds `out/`: `steam-client/` plus the server hardlinked (`cp -al`) and split into `layers/0..4`. Removes `steamclient64.dll` from `out/`. |
| `scripts/installer/ark-server-install.sh` | SteamCMD `app_update` with retry on "Missing configuration". |
| `scripts/installer/ark-split-layers.sh` | Spreads the pak files over `layers/1..4` so no layer hits GHCR's 10 GB limit. |

`build.sh` environment:

| Var | Default | Purpose |
|-----|---------|---------|
| `ARK_STEAM_BUILD_ID` | current public build (api.steamcmd.net) | Build to download; `download.sh` fails if Steam serves another. |
| `CONTAINER_RUNTIME` | `docker` | `docker` or `podman`. |
| `WORK_DIR` | `container/files/work` | `cache/` (~12 GB SteamCMD install, kept) and `out/` (rebuilt each run; the caller moves or removes it). |
| `STEAMCMD_DOCKER_OPTS` | none | Extra `run` options for the SteamCMD container, e.g. `--security-opt seccomp=unconfined`. |
