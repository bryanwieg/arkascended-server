#!/usr/bin/env bash

# Check local ARK image metadata against Steam app 2430930 and optionally rebuild.
# Host/CI helper only; this script is intentionally not bundled into the image.
# Exit codes:
#   0  -> up-to-date (or rebuild succeeded)
#   10 -> update available (check-only mode)
#   1  -> error

set -euo pipefail

image="${IMAGE:-ark-server:latest}"
check_only=1
rebuild=0
steam_api_url="${STEAM_API_URL:-https://api.steamcmd.net/v1/info/2430930}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      image="${2:-}"
      shift 2
      ;;
    --check-only)
      check_only=1
      shift
      ;;
    --rebuild)
      rebuild=1
      check_only=0
      shift
      ;;
    --steam-api-url)
      steam_api_url="${2:-}"
      shift 2
      ;;
    *)
      echo "MESSAGE=unknown argument: $1"
      exit 1
      ;;
  esac
done

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "MESSAGE=required command missing: $1"
    exit 1
  fi
}

require_cmd docker
require_cmd curl

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

image_build_label() {
  local raw
  raw="$(docker image inspect "$image" --format '{{index .Config.Labels "io.ark.server.build_id"}}' 2>/dev/null || true)"
  raw="$(trim "$raw")"
  if [[ -n "$raw" && "$raw" != "<no value>" ]]; then
    printf '%s' "$raw"
    return 0
  fi

  local desc
  desc="$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.description"}}' 2>/dev/null || true)"
  desc="$(trim "$desc")"
  if [[ "$desc" =~ [Bb]uild[[:space:]]*[Ii][Dd][[:space:]]*:[[:space:]]*([0-9]+) ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
    return 0
  fi
  printf ''
}

image_version_label() {
  local raw
  raw="$(docker image inspect "$image" --format '{{index .Config.Labels "io.ark.server.version"}}' 2>/dev/null || true)"
  raw="$(trim "$raw")"
  if [[ -n "$raw" && "$raw" != "<no value>" ]]; then
    printf '%s' "$raw"
    return 0
  fi

  raw="$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}' 2>/dev/null || true)"
  raw="$(trim "$raw")"
  if [[ "$raw" == "<no value>" ]]; then
    raw=""
  fi
  printf '%s' "$raw"
}

extract_latest_build_id() {
  local payload="$1"
  local compact
  compact="$(printf '%s' "$payload" | tr -d '\n\r\t ')"
  printf '%s' "$compact" | sed -n 's/.*"branches":{"public":{"buildid":"\([0-9][0-9]*\)".*/\1/p'
}

if ! docker image inspect "$image" >/dev/null 2>&1; then
  echo "IMAGE=$image"
  echo "MESSAGE=image not found locally"
  exit 1
fi

api_response="$(curl -fsSL "$steam_api_url" 2>/dev/null || true)"
if [[ -z "$api_response" ]]; then
  echo "IMAGE=$image"
  echo "MESSAGE=failed to query steam api"
  exit 1
fi

latest_build_id="$(extract_latest_build_id "$api_response")"
if [[ -z "$latest_build_id" ]]; then
  echo "IMAGE=$image"
  echo "MESSAGE=failed to parse latest steam build id"
  exit 1
fi

current_build_id="$(image_build_label)"
current_version="$(image_version_label)"

if [[ -z "$current_build_id" ]]; then
  echo "IMAGE=$image"
  echo "CURRENT_VERSION=$current_version"
  echo "LATEST_BUILD_ID=$latest_build_id"
  echo "UPDATE_AVAILABLE=0"
  echo "MESSAGE=local image build id label missing"
  exit 1
fi

update_available=0
if [[ "$current_build_id" != "$latest_build_id" ]]; then
  update_available=1
fi

echo "IMAGE=$image"
echo "CURRENT_VERSION=$current_version"
echo "LATEST_VERSION="
echo "CURRENT_BUILD_ID=$current_build_id"
echo "LATEST_BUILD_ID=$latest_build_id"
echo "UPDATE_AVAILABLE=$update_available"

if [[ "$rebuild" -eq 1 && "$update_available" -eq 1 ]]; then
  image_repo="$image"
  if [[ "$image_repo" == *"@"* ]]; then
    image_repo="${image_repo%%@*}"
  fi
  if [[ "$image_repo" == *":"* ]]; then
    image_repo="${image_repo%:*}"
  fi

  if [[ ! -x "$repo_root/.github/build-image.sh" ]]; then
    echo "MESSAGE=build script missing: $repo_root/.github/build-image.sh"
    exit 1
  fi

  (
    cd "$repo_root"
    IMAGE_NAME="$image_repo" "$repo_root/.github/build-image.sh"
  )

  current_build_id="$(image_build_label)"
  current_version="$(image_version_label)"

  echo "POST_REBUILD_CURRENT_VERSION=$current_version"
  echo "POST_REBUILD_CURRENT_BUILD_ID=$current_build_id"
  if [[ "$current_build_id" == "$latest_build_id" ]]; then
    echo "MESSAGE=rebuild completed and image is now current"
  else
    echo "MESSAGE=rebuild completed but build id still differs"
  fi
  exit 0
fi

if [[ "$update_available" -eq 1 ]]; then
  echo "MESSAGE=update available"
  if [[ "$check_only" -eq 1 ]]; then
    exit 10
  fi
fi

echo "MESSAGE=image is up to date"
exit 0
