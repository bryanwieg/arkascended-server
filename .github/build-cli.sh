#!/usr/bin/env bash
set -euo pipefail

# Build the ark-cli CLI binary in-place for CI or local use.

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cli_dir="${repo_root}/app/ark-cli"
output="${repo_root}/app/ark-cli"

cd "$cli_dir"
go build -o "$output" .
echo "Built ${output}"
