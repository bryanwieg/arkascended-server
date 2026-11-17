# Server files build

Scripts that download the ARK: Survival Ascended dedicated server (Steam app
2430930) with SteamCMD and package it as a local, files-only image that the
runtime `dockerfile` copies from. Formerly the `arkascended-server-files` repo.

- `build.sh [image] [extra build args]`: runs SteamCMD in a throwaway container
  (`steamcmd.dockerfile`, `download.sh`), then builds `dockerfile` from the
  download. Needs only Docker or Podman.
- `download.sh`: installs the server and lays it out as `steam-client/` and
  `layers/0..4` (split by `scripts/installer/ark-split-layers.sh`, so no single
  layer gets too large).

`WORK_DIR` (default `./work`) holds `cache/`, the SteamCMD install (~12 GB), kept
between builds so a new ARK release downloads only changed files, and `out/`,
hardlinks into the cache laid out for the image, removed after each build.
