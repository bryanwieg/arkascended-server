#!/usr/bin/env bash
set -euo pipefail

# Install dev requirements once per venv to keep onboarding lightweight.
# The stamp file records the Python version so a venv rebuild (e.g. Python
# upgrade) triggers reinstallation automatically.
venv_python=".venv/bin/python"
stamp_file=".venv/.requirements-dev.installed"

if [[ ! -x "${venv_python}" ]]; then
  exit 0
fi

readonly current_version="$("${venv_python}" --version 2>&1)"

if [[ -f "${stamp_file}" ]] && [[ "$(cat "${stamp_file}")" == "${current_version}" ]]; then
  exit 0
fi

# Ensure pip exists in the venv for minimal Python installs.
if ! "${venv_python}" -m pip --version >/dev/null 2>&1; then
  "${venv_python}" -m ensurepip --upgrade
fi

"${venv_python}" -m pip install -r requirements-dev.txt
echo "${current_version}" > "${stamp_file}"
