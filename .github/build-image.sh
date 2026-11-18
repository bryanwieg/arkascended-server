#!/usr/bin/env bash

# Build the ARK server container image for the selected runtime.
# Runtime is controlled by CONTAINER_RUNTIME env var (docker [default] | podman).
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

image_name="${IMAGE_NAME:-ark-server}"
dockerfile_path="${DOCKERFILE_PATH:-$repo_root/container/dockerfile}"
context_path="${CONTEXT_PATH:-$repo_root/container}"

base_tag="${BASE_TAG:-${image_name}:base}"
final_tag="${FINAL_TAG:-${image_name}:latest}"
labels_dockerfile_path="${LABELS_DOCKERFILE_PATH:-$repo_root/container/dockerfile.labels}"

# Runtime selection: "docker" (default) or "podman"
container_runtime="${CONTAINER_RUNTIME:-docker}"

# Podman's rootless overlay backend can consume ~40 GB during the multi-stage
# build. Check for sufficient free space before starting; running out mid-build
# produces opaque errors that are hard to diagnose.
# Docker buildx uses the host's buildkit cache and does not have this issue.
if [ "$container_runtime" = "podman" ]; then
    required_free_gb="${BUILD_REQUIRED_FREE_GB:-50}"
    available_kb="$(df --output=avail "$repo_root" | tail -1)"
    available_gb="$((available_kb / 1024 / 1024))"
    if [ "$available_gb" -lt "$required_free_gb" ]; then
        echo "ERROR: Podman build requires at least ${required_free_gb} GB of free disk space." >&2
        echo "  Rootless podman uses an overlay mount (~40 GB) for the multi-stage image build." >&2
        echo "  Available: ${available_gb} GB on $(df --output=target "$repo_root" | tail -1)" >&2
        echo "  Set BUILD_REQUIRED_FREE_GB to override (not recommended)." >&2
        exit 77
    fi
fi

# Build command differs by runtime.
# Docker buildx needs --load so the image is available for docker run below.
if [ "$container_runtime" = "podman" ]; then
    build_cmd=(podman build)
else
    build_cmd=(docker buildx build --load)
fi

# Recipe ID: the hash ark-cli embeds for this checkout's container/ files. Tagging and
# labelling with it lets `update instance` reuse this image instead of rebuilding.
recipe_id="$(cd "$repo_root" && go run ./container/cmd/recipe-id)"

# Base image (container/base.dockerfile): everything but the ARK files, published to GHCR
# and tagged by the file's hash, as ark-cli does. Use a local copy, else pull it, else
# build it here. Done first, so a failure costs no ARK download.
base_ref="ghcr.io/bryanwieg/ark-server-base:$(sha256sum "$repo_root/container/base.dockerfile" | cut -c1-12)"
if ! "$container_runtime" image inspect "$base_ref" >/dev/null 2>&1 &&
  ! "$container_runtime" pull "$base_ref" >/dev/null 2>&1; then
  echo "Base image $base_ref not available; building it locally." >&2
  "${build_cmd[@]}" -f "$repo_root/container/base.dockerfile" -t "$base_ref" "$context_path"
fi

# Server files: download Steam's current ARK build with SteamCMD (container/files)
# into a local files image. WORK_DIR keeps the SteamCMD install so later builds
# download only changed files; ARK_BUILD_DELETE_CACHE=1 drops it once the files
# image exists (CI, to save disk).
files_repo="ark-server-files"
work_dir="${ARK_BUILD_WORK_DIR:-$repo_root/.ark-tmp/build-work}"
steam_build_id="$(
  curl -fsS --max-time 30 https://api.steamcmd.net/v1/info/2430930 |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["2430930"]["depots"]["branches"]["public"]["buildid"])'
)"
if [[ ! "$steam_build_id" =~ ^[0-9]+$ ]]; then
  echo "ERROR: could not read ARK's current build ID from Steam (got '$steam_build_id')." >&2
  exit 1
fi
files_image="${files_repo}:${steam_build_id}"
ARK_STEAM_BUILD_ID="$steam_build_id" CONTAINER_RUNTIME="$container_runtime" WORK_DIR="$work_dir" \
  "$repo_root/container/files/build.sh" "$files_repo"
if [[ "${ARK_BUILD_DELETE_CACHE:-}" == 1 ]]; then
  rm -rf "$work_dir"
fi

"${build_cmd[@]}" -f "$dockerfile_path" -t "$base_tag" \
  --build-arg BASE_IMAGE="$base_ref" \
  --build-arg ARK_FILES_IMAGE="$files_image" \
  "$context_path"

ark_version="$(
  "$container_runtime" run --rm "$base_tag" cat /home/ark-user/ARK_SERVER_VERSION
)"

ark_build_id="$(
  "$container_runtime" run --rm "$base_tag" cat /home/ark-user/ARK_SERVER_BUILDID
)"

"${build_cmd[@]}" -f "$labels_dockerfile_path" \
  -t "$final_tag" \
  -t "${image_name}:${ark_version}" \
  -t "${image_name}:${ark_build_id}-${recipe_id}" \
  --build-arg BASE_IMAGE="$base_tag" \
  --build-arg ARK_SERVER_VERSION="$ark_version" \
  --build-arg ARK_BUILD_ID="$ark_build_id" \
  --build-arg ARK_RECIPE_ID="$recipe_id" \
  "$context_path"

"$container_runtime" image rm "$files_image" >/dev/null  # only needed during the build
echo "BASE_IMAGE=$base_ref"  # last line: CI publishes this tag
