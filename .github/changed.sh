#!/usr/bin/env bash
# Change groups for CI: which checks have work in a change, and a fingerprint
# of each group's files so a job can skip a hash it already passed.
#   changed.sh GROUP [FILE...]  prints true/false (files default to the diff
#                               against the merge base with origin/main)
#   changed.sh hash GROUP       prints a sha256 of the group's tracked files at HEAD
# GROUP: python | go | unit | smoke. Exit 2 on a bad group or a git error.
set -euo pipefail

die() {
	echo "changed.sh: $*" >&2
	exit 2
}

# One path list per group. Docs never count.
in_group() {
	local group=$1 path=$2
	case $path in *.md | docs/*) return 1 ;; esac
	case $group in
	python)
		case $path in
		*.py | ruff.toml | .github/workflows/python.yml | .github/changed.sh) return 0 ;;
		esac
		;;
	go)
		case $path in
		app/* | container/* | go.work | go.work.sum | .golangci.yml | .github/workflows/cli.yml | .github/changed.sh) return 0 ;;
		esac
		;;
	unit)
		case $path in
		container/* | tests/container/unit/* | tests/container/integration/* | tests/app/ark_cli/integration/* | \
			tests/fixtures/lite_image/* | tests/helpers/* | tests/conftest.py | app/ark-cli/* | app/pkg/* | \
			requirements-dev.txt | .github/build-image.sh | .github/workflows/* | .github/changed.sh | \
			.github/release-version.py | tests/test_ci_workflows.py | tests/test_release_version.py)
			return 0
			;;
		esac
		in_group smoke "$path" && return 0
		;;
	smoke)
		case $path in
		container/base.dockerfile | container/dockerfile | container/files/build.sh | container/files/download.sh | \
			container/files/steamcmd.dockerfile | container/files/scripts/installer/* | \
			container/scripts/installer/generate-default-ini.py | .github/build-image.sh | \
			.github/workflows/image.yml | .github/changed.sh)
			return 0
			;;
		container/scripts/runtime/container-entrypoint.py | container/scripts/runtime/container-entrypoint.sh | \
			container/scripts/runtime/ark-server-start.py | container/scripts/runtime/ark-server-stop.py | \
			container/scripts/runtime/ark-stop-reason.py | container/scripts/runtime/rcon_client.py | \
			container/scripts/runtime/runtime_utils.py | container/scripts/runtime/_logging.py | \
			container/scripts/runtime/ini_settings.py | container/scripts/runtime/ini_history.py | \
			container/scripts/runtime/ark_backup/save_markers.py)
			return 0
			;;
		tests/container/integration/slow/test_ark_server_start_script.py | \
			tests/container/integration/slow/test_ark_server_stop_script.py | tests/conftest.py | \
			tests/container/conftest.py | tests/container/integration/conftest.py | tests/.test-order.yaml | \
			tests/helpers/* | pytest.ini | run-tests.py | requirements-dev.txt)
			return 0
			;;
		esac
		;;
	*) die "unknown group: $group" ;;
	esac
	return 1
}

check_group() {
	case $1 in python | go | unit | smoke) ;; *) die "unknown group: $1" ;; esac
}

if [[ ${1:-} == hash ]]; then
	group=${2:-}
	check_group "$group"
	tree=$(git ls-tree -r HEAD) || die "git ls-tree failed"
	# Blob ids and paths of the group's files: same bytes, same hash.
	while IFS=$'\t' read -r meta path; do
		if in_group "$group" "$path"; then printf '%s\t%s\n' "${meta##* }" "$path"; fi
	done <<<"$tree" | sha256sum | cut -d' ' -f1
	exit 0
fi

group=${1:-}
check_group "$group"
shift
if (($# > 0)); then
	files=("$@")
else
	base=$(git merge-base origin/main HEAD) || die "no merge base with origin/main"
	diff=$(git diff --name-only "$base" HEAD) || die "git diff failed"
	mapfile -t files <<<"$diff"
fi
for path in "${files[@]}"; do
	if [[ -n $path ]] && in_group "$group" "$path"; then
		echo true
		exit 0
	fi
done
echo false
