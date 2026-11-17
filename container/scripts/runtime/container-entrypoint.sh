#!/usr/bin/env bash
set -euo pipefail

ark_user="ark-user"
launcher="/home/ark-user/scripts/runtime/container-entrypoint.py"

if [[ "$(id -u)" -eq 0 ]]; then
  echo "Entrypoint: running as root; launching runtime as ${ark_user}." >&2
  # setpriv replaces this process; runuser would stay as parent and SIGKILL the shutdown manager 2s after SIGTERM.
  export USER="$ark_user" LOGNAME="$ark_user" HOME="/home/$ark_user"
  exec setpriv --reuid="$ark_user" --regid="$ark_user" --init-groups -- /usr/bin/python3.14 "$launcher" "$@"
fi

echo "Entrypoint: running as non-root user $(id -un); no user switch needed." >&2
exec /usr/bin/python3.14 "$launcher" "$@"
