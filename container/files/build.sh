#!/usr/bin/env bash
# Build the server files image with only Docker (or Podman) installed:
# SteamCMD runs in a throwaway container (download.sh), then the dockerfile
# copies its output in.
#
# Usage: ./build.sh [image] [extra build args, e.g. --push -t image:latest]
#   image defaults to arkascended-server-files; it is tagged <image>:<build ID>.
# Env:
#   ARK_STEAM_BUILD_ID    build to download; default: Steam's current public build
#   STEAMCMD_DOCKER_OPTS  extra options for the SteamCMD container, e.g.
#                         "--security-opt seccomp=unconfined" where Docker's default
#                         seccomp profile blocks SteamCMD (GitHub-hosted runners)
#   CONTAINER_RUNTIME     docker (default) or podman
#   WORK_DIR              work folder; default ./work. work/cache keeps the SteamCMD
#                         install (about 12 GB) so later builds download only changed
#                         files; work/out is rebuilt each run and removed afterwards
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
image="${1:-arkascended-server-files}"
shift $(( $# > 0 ? 1 : 0 ))
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
mkdir -p "$work"
trap 'rm -rf "$work/out"' EXIT
# shellcheck disable=SC2086 # STEAMCMD_DOCKER_OPTS holds separate options
"$rt" run --rm ${STEAMCMD_DOCKER_OPTS:-} "${user[@]}" \
  -e ARK_STEAM_BUILD_ID="$build_id" \
  -v "$root:/src:ro" -v "$work:/work" \
  arkascended-steamcmd bash /src/download.sh /work

"$rt" build -f "$root/dockerfile" --build-arg ARK_STEAM_BUILD_ID="$build_id" \
  -t "${image}:${build_id}" "$@" "$work/out"
echo "Built ${image}:${build_id}"
