#!/usr/bin/env bash

# Steamcmd, when downloading the ark server files, will sometimes encounter the error "(Missing configuration)".
# A simply retry usually addresses the problem. Therefore, this install script will retry on this error.

set -euo pipefail

# Installing over a previous install lets steamcmd download only changed files.
install_dir="${ARK_INSTALL_DIR:-/home/ark-user/ark-server}"

# fail prints a one-line reason and, when ARK_FAILURE_FILE is set, writes it there
# for ark-cli, which reports it instead of SteamCMD's raw output.
fail() {
  echo "$1" >&2
  [[ -n "${ARK_FAILURE_FILE:-}" ]] && printf '%s\n' "$1" > "$ARK_FAILURE_FILE"
  exit 1
}

max_attempts=10
sleep_seconds=5
retry_error="ERROR! Failed to install app '2430930' (Missing configuration)"

for ((attempt=1; attempt<=max_attempts; attempt++)); do
  tmp_output="$(mktemp)"
  set +e
  steamcmd +force_install_dir "$install_dir" \
    +login anonymous \
    +app_update 2430930 validate \
    +quit 2>&1 | tee "$tmp_output"
  status=${PIPESTATUS[0]}
  set -e
  output="$(cat "$tmp_output")"
  rm -f "$tmp_output"

  if [[ "$status" -eq 0 ]]; then
    # The build ID is the download's cache key; a mismatch (an ARK release landing
    # mid-build) would cache these files under the wrong build.
    expected="${ARK_STEAM_BUILD_ID:-}"
    if [[ "$expected" =~ ^[0-9]+$ ]]; then
      actual="$(awk -F'"' '/"buildid"/ {print $4; exit}' "$install_dir/steamapps/appmanifest_2430930.acf")"
      if [[ "$actual" != "$expected" ]]; then
        fail "Downloaded ARK build $actual, but the build was keyed to $expected; rebuild to pick up the new key."
      fi
    fi
    exit 0
  fi

  if ! grep -Fq "$retry_error" <<< "$output"; then
    # SteamCMD's output is already on screen (tee); name the cause in one line.
    state="$(grep -o 'state is 0x[0-9a-fA-F]*' <<< "$output" | tail -n1 || true)"
    fail "SteamCMD download failed: ${state:-exit code $status}"
  fi
  if [[ "$attempt" -lt "$max_attempts" ]]; then
    sleep "$sleep_seconds"
  fi
done

fail "SteamCMD download failed: (Missing configuration) after ${max_attempts} attempts"
