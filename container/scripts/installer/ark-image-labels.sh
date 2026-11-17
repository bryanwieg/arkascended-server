#!/usr/bin/env bash

# will retreive the ark server buildid and version from the image, and embed them as files
# will be used to set tags and labels for the image
set -euo pipefail

manifest_path="/home/ark-user/ark-server/steamapps/appmanifest_2430930.acf"
server_exe="/home/ark-user/ark-server/ShooterGame/Binaries/Win64/ArkAscendedServer.exe"

# get ark build id, matches build id used in steacmd api 
build_id="$(awk -F'"' '/"buildid"/ {print $4; exit}' "$manifest_path")"
if [[ -z "$build_id" ]]; then
  echo "build id not found in $manifest_path" >&2
  exit 1
fi
if [[ ! "$build_id" =~ ^[0-9]+$ ]]; then
  echo "build id has invalid format: $build_id" >&2
  exit 1
fi

# get ark server version, matches in game version number
ark_version="$(
  exiftool -ArkVersion "$server_exe" |
    awk -F': ' '/Ark Version/ {gsub(/^[[:space:]]+|[[:space:]]+$/, "", $2); print $2; exit}'
)"
if [[ -z "$ark_version" ]]; then
  echo "ark version not found in $server_exe" >&2
  exit 1
fi
if [[ ! "$ark_version" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
  echo "ark version has invalid format: $ark_version" >&2
  exit 1
fi

# save version and build data to image
printf "%s\n" "$ark_version" > "/home/ark-user/ARK_SERVER_VERSION"
printf "%s\n" "$build_id" > "/home/ark-user/ARK_SERVER_BUILDID"
