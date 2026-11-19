#!/usr/bin/env bash
# Download the ARK server with SteamCMD and lay it out for the dockerfile:
#   <work>/cache/ark-server  persistent SteamCMD install; later runs download only changed files
#   <work>/out/steam-client  SteamCMD's client files (Proton needs them at runtime)
#   <work>/out/layers/0..4   the server, split by ark-split-layers.sh
# out/ holds hardlinks into cache/, so it costs no extra disk. Hardlinks can't cross
# mount points, so cache/ and out/ sit under one mount (<work>).
# build.sh runs this inside the steamcmd.dockerfile container.
# Set ARK_STEAM_BUILD_ID to fail if Steam serves another build.
set -euo pipefail

work="$(realpath -m "${1:-work}")"
scripts="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/scripts/installer"
cache="$work/cache/ark-server"
out="$work/out"

# SteamCMD's client files land in $HOME/.local/share/Steam. steamcmd.dockerfile
# sets HOME to the runtime image's home, so recorded paths match it.
mkdir -p "$cache"
# ark-cli reads the install script's one-line failure reason from here.
export ARK_FAILURE_FILE="$work/failure"
rm -f "$ARK_FAILURE_FILE"
ARK_INSTALL_DIR="$cache" bash "$scripts/ark-server-install.sh"

rm -rf "$out"
mkdir -p "$out"
mv "$HOME/.local/share/Steam" "$out/steam-client"
cp -al "$cache" "$out/ark-server"
# Removing the link in out/ leaves the cached file for the next validate.
rm -f "$out/ark-server/ShooterGame/Binaries/Win64/steamclient64.dll"
bash "$scripts/ark-split-layers.sh" "$out/ark-server" "$out/layers" 4
