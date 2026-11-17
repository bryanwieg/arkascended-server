#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$SCRIPT_DIR"

DRY_RUN=0
VERBOSE=0

usage() {
  cat <<'EOF'
Clean generated temp data, logs, and caches inside this repo.

Usage:
  clean-repo.sh [--dry-run] [--verbose]

Options:
  --dry-run   Show what would be removed without deleting anything
  --verbose   Print skipped paths and safety checks
  -h, --help  Show this help

Notes:
  - Only removes known generated paths/patterns.
  - Never removes tracked git files.
  - Does not touch Docker resources (containers/volumes/images).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      ;;
    --verbose)
      VERBOSE=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if ! command -v git >/dev/null 2>&1; then
  echo "ERROR: git is required for tracked-file safety checks." >&2
  exit 2
fi

if [[ ! -d "$REPO_ROOT/.git" ]]; then
  echo "ERROR: repo root not detected at $REPO_ROOT" >&2
  exit 2
fi

relpath() {
  local abs="$1"
  abs="${abs#"$REPO_ROOT"/}"
  printf '%s\n' "$abs"
}

is_tracked_path() {
  local abs="$1"
  local rel
  rel="$(relpath "$abs")"
  git -C "$REPO_ROOT" ls-files --error-unmatch -- "$rel" >/dev/null 2>&1
}

dir_contains_tracked_files() {
  local abs="$1"
  local rel
  rel="$(relpath "$abs")"
  git -C "$REPO_ROOT" ls-files -- "$rel" | grep -q .
}

safe_repo_path() {
  local abs="$1"
  [[ "$abs" == "$REPO_ROOT" ]] && return 1
  [[ "$abs" == "$REPO_ROOT/"* ]] && return 0
  return 1
}

declare -a candidates=()
declare -A seen=()

add_candidate() {
  local abs="$1"
  [[ -e "$abs" ]] || return 0
  safe_repo_path "$abs" || return 0
  if [[ -z "${seen["$abs"]+x}" ]]; then
    candidates+=("$abs")
    seen["$abs"]=1
  fi
}

# Common repo-local test artifacts and caches.
add_candidate "$REPO_ROOT/.tests"
add_candidate "$REPO_ROOT/.pytest_cache"
add_candidate "$REPO_ROOT/.coverage"
add_candidate "$REPO_ROOT/htmlcov"
add_candidate "$REPO_ROOT/.mypy_cache"
add_candidate "$REPO_ROOT/.ruff_cache"
add_candidate "$REPO_ROOT/.hypothesis"
add_candidate "$REPO_ROOT/.tox"
add_candidate "$REPO_ROOT/ark-cli/ark-cli"
add_candidate "$REPO_ROOT/backups"


# Python bytecode caches anywhere in the repo.
while IFS= read -r path; do
  add_candidate "$path"
done < <(
  find "$REPO_ROOT" \
    \( -path "$REPO_ROOT/.git" -o -path "$REPO_ROOT/.venv" \) -prune -o \
    -type d -name '__pycache__' -print 2>/dev/null
)

# Root-level generated compose files and logs used by local test/debug runs.
while IFS= read -r path; do
  add_candidate "$path"
done < <(find "$REPO_ROOT" -maxdepth 1 -type f \( \
  -name 'pytest-compose-*.yaml' -o \
  -name 'debug-compose-*.yaml' -o \
  -name '*.cli.log' -o \
  -name '*.docker.log' \
  \) 2>/dev/null)

removed_count=0
skipped_count=0

echo "Repo temp cleanup root: $REPO_ROOT"
[[ "$DRY_RUN" -eq 1 ]] && echo "Mode: dry-run"

for abs in "${candidates[@]}"; do
  if [[ ! -e "$abs" ]]; then
    continue
  fi

  if [[ -f "$abs" ]] && is_tracked_path "$abs"; then
    skipped_count=$((skipped_count + 1))
    [[ "$VERBOSE" -eq 1 ]] && echo "SKIP tracked file: $(relpath "$abs")"
    continue
  fi

  if [[ -d "$abs" ]] && dir_contains_tracked_files "$abs"; then
    skipped_count=$((skipped_count + 1))
    [[ "$VERBOSE" -eq 1 ]] && echo "SKIP dir with tracked files: $(relpath "$abs")"
    continue
  fi

  echo "REMOVE $(relpath "$abs")"
  if [[ "$DRY_RUN" -eq 0 ]]; then
    rm -rf -- "$abs"
  fi
  removed_count=$((removed_count + 1))
done

echo "Cleanup complete: $removed_count path(s) removed, $skipped_count skipped."
