#!/usr/bin/env bash
# Download the ARK server files with only Docker (or Podman) installed: SteamCMD
# runs in a throwaway container (download.sh) and writes $WORK_DIR/out
# (steam-client, layers/0..4). The caller owns out afterwards; it is removed on failure.
#
# Usage: ./build.sh
# Env:
#   ARK_STEAM_BUILD_ID    build to download; default: Steam's current public build
#   STEAMCMD_DOCKER_OPTS  extra options for the SteamCMD container, e.g.
#                         "--security-opt seccomp=unconfined" where Docker's default
#                         seccomp profile blocks SteamCMD (GitHub-hosted runners)
#   CONTAINER_RUNTIME     docker (default) or podman
#   ARK_DEPOTCACHE_SEED_IMAGE  image to take SteamCMD's depot manifests from when work/cache
#                         has none yet (caches from before they were kept); default
#                         ark-server:latest, skipped when it doesn't exist
#   WORK_DIR              work folder; default ./work. work/cache keeps the SteamCMD
#                         install (about 12 GB) so later builds download only changed
#                         files; work/out is rebuilt each run
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
rt="${CONTAINER_RUNTIME:-docker}"
work="$(realpath -m "${WORK_DIR:-$root/work}")"

build_id="${ARK_STEAM_BUILD_ID:-$(
  curl -fsS --max-time 30 https://api.steamcmd.net/v1/info/2430930 |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["2430930"]["depots"]["branches"]["public"]["buildid"])'
)}"
if [[ ! "$build_id" =~ ^[0-9]+$ ]]; then
  echo "invalid ARK build ID: '$build_id'" >&2
  exit 1
fi

# Download files must end up owned by the caller. Rootless engines map container
# root to the caller; rootful ones need the caller's IDs passed in.
user=(--user "$(id -u):$(id -g)")
if [[ "$rt" == podman ]]; then
  [[ "$(podman info --format '{{.Host.Security.Rootless}}')" == true ]] && user=()
elif docker info --format '{{.SecurityOptions}}' | grep -q rootless; then
  user=()
fi

"$rt" build -q -f "$root/steamcmd.dockerfile" -t arkascended-steamcmd "$root/scripts" >/dev/null
mkdir -p "$work/cache"
# A cache from before depotcache was kept: seed it from the last image, which has the
# installed build's manifest. Without it SteamCMD can only update with the 0x6 fallback.
seed="${ARK_DEPOTCACHE_SEED_IMAGE:-ark-server:latest}"
if [[ ! -d "$work/cache/depotcache" ]] && "$rt" image inspect "$seed" >/dev/null 2>&1; then
  seed_id="$("$rt" create "$seed")"
  if "$rt" cp "$seed_id:/home/ark-user/.local/share/Steam/depotcache" "$work/cache/depotcache"; then
    echo "Seeded the SteamCMD depot cache from $seed."
  else
    echo "Warning: could not copy the depot cache from $seed; this update may need the full validate." >&2
  fi
  "$rt" rm "$seed_id" >/dev/null
fi
# shellcheck disable=SC2086 # STEAMCMD_DOCKER_OPTS holds separate options
if ! "$rt" run --rm ${STEAMCMD_DOCKER_OPTS:-} "${user[@]}" \
  -e ARK_STEAM_BUILD_ID="$build_id" \
  -v "$root:/src:ro" -v "$work:/work" \
  arkascended-steamcmd bash /src/download.sh /work; then
  rm -rf "$work/out"
  exit 1
fi
echo "Downloaded ARK build $build_id to $work/out"
