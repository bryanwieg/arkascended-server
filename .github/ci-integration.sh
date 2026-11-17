#!/usr/bin/env bash
set -euo pipefail

instance="${INSTANCE_NAME:-ci-ark-${GITHUB_RUN_ID:-local}}"
compose_file="${COMPOSE_FILE:-}"
backup_dir="${ARK_BACKUP_DIR:-}"
image_name="${IMAGE_NAME:-}"
admin_password="${ADMIN_PASSWORD:-AdminPass123}"
server_password="${SERVER_PASSWORD:-ServerPass123}"
skip_pull="${SKIP_PULL:-}"
created_compose_file="false"
created_backup_dir="false"
skip_cleanup="${SKIP_CLEANUP:-}"
cli_path="${CLI_PATH:-./app/ark-cli/ark-cli}"

cleanup() {
  if [[ "$skip_cleanup" == "1" || "$skip_cleanup" == "true" ]]; then
    return
  fi
  if [[ -n "${compose_file:-}" ]]; then
    docker compose -f "$compose_file" down -v >/dev/null 2>&1 || true
  fi
  if [[ "$created_compose_file" == "true" && -n "${compose_file:-}" ]]; then
    rm -f "$compose_file"
  fi
  if [[ "$created_backup_dir" == "true" && -n "${backup_dir:-}" ]]; then
    rm -rf "$backup_dir"
  fi
}
trap cleanup EXIT

if [[ -z "$compose_file" ]]; then
  compose_file="$(mktemp -p . ci-compose-XXXX.yaml)"
  export COMPOSE_FILE="$compose_file"
  created_compose_file="true"
fi

if [[ -z "$backup_dir" ]]; then
  backup_dir="$(mktemp -d)"
  export ARK_BACKUP_DIR="$backup_dir"
  created_backup_dir="true"
fi

if [[ -n "$image_name" && "$skip_pull" != "1" && "$skip_pull" != "true" ]]; then
  docker pull "$image_name"
fi

if [[ ! -x "$cli_path" ]]; then
  ./.github/build-cli.sh
fi

"$cli_path" create instance --name "$instance" \
  --server-password "$server_password" \
  --admin-password "$admin_password"

if [[ -n "$image_name" && "$image_name" != "ark-server:latest" ]]; then
  docker tag "$image_name" ark-server:latest
fi

docker compose -f "$compose_file" up -d --no-build "$instance"

timeout 900 sh -c "until docker compose -f \"$compose_file\" exec -T \"$instance\" test -f /home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log; do sleep 5; done"
timeout 300 sh -c "until docker compose -f \"$compose_file\" exec -T \"$instance\" /usr/bin/python3.14 /home/ark-user/scripts/runtime/ark-rcon.py GetServerStatus >/dev/null 2>&1; do sleep 5; done"

"$cli_path" create backup --instance "$instance"
"$cli_path" stop instance --instance "$instance"
"$cli_path" restore backup --instance "$instance" --archive latest
