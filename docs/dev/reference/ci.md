# CI and Releases

GitHub Actions runs two independent pipelines, one per shipped artifact, a
monthly base refresh, and a release workflow that versions both with
[release-please](https://github.com/googleapis/release-please).

| Workflow | Triggers | Does |
|---|---|---|
| `.github/workflows/cli.yml` | PR / push to `main` touching `app/**`, `container/**` (embedded in `ark-cli`), `go.work*`, `.golangci.yml`; called by `release.yml` | `go vet` + `go test` for `app/ark-cli`, `app/pkg/ark`, `app/ark-web` and `container`; `golangci-lint` (`.golangci.yml`) failing on new findings only. On a release: builds `ark-cli-<version>-linux-amd64` (static, version stamped) and attaches it plus a `.sha256` to the GitHub release. |
| `.github/workflows/image.yml` | PR / push to `main` touching `container/**`, container unit tests, `build-image.sh`; daily at 06:17 UTC; manual | Python unit tests (`tests/container/unit`) and `test_server_oom_report.py`, which triggers a real OOM kill in a `python:3.14-slim` container. Pushes to `main`, scheduled and manual runs then build the image and run the smoke tests, and on `main` publish the ARK-free base image (see below). |
| `.github/workflows/python.yml` | PR / push to `main` touching `**.py`, `ruff.toml` | `ruff check` + `ruff format --check` (config: `ruff.toml`). |
| `.github/workflows/base-refresh.yml` | 1st of each month; manual | Opens a PR bumping the base image's refresh line (see below). |
| `.github/workflows/release.yml` | push to `main` | Maintains one release PR per component. Merging one tags it and creates the GitHub release; a CLI release also calls `cli.yml` to attach the binary. |

## Versioning

| Component | Path | Tag | Changelog | Version source |
|---|---|---|---|---|
| CLI | `app/` (excluding `app/ark-web`) | `cli-v<semver>` | `app/ark-cli/CHANGELOG.md` | git tag, stamped with `-ldflags "-X main.version=..."`; `ark-cli --version` prints it (`dev` for local builds) |
| Container packaging | `container/` | `container-v<semver>` | `container/CHANGELOG.md` | `container/version.txt` |

Both are pre-1.0: breaking changes and `feat` bump the minor version, `fix` the
patch. Version bumps come from Conventional Commit prefixes on the commits
that touch each path. For the CLI only `feat`, `fix`, `perf`, `revert` and
`deps` are releasable. The container also counts `build:` commits, because
Dockerfile changes reach users.

**PRs are squash-merged only**, with the PR title as the commit subject and a
blank body (repository settings: merge commits and rebase merges are off,
`squash_merge_commit_title=PR_TITLE`, `squash_merge_commit_message=BLANK`), so
the PR title must be a Conventional Commit. *Why:* release-please also reads
Conventional Commit lines in a commit's body as changes. A merge commit's body
is the PR title, and a squash body of commit messages repeats them, so either
one lists every change twice in the changelog.

**First release:** `bootstrap-sha` keeps pre-CI history out of the first
changelogs. Both components have released (`cli-v0.1.0`, `container-v0.1.0`)
and now version from their commits.

No ARK image is published: `ark-cli` builds `ark-server:<steam-build-id>` on
each host from the recipe embedded in it (see [image build](image-build.md)).
Only the ARK-free base image is published (below). A container release is a tag
and a changelog.

## Image smoke tests

The `smoke` job checks that the image still builds and starts with Steam's
current ARK build. It gates nothing; the daily run is an early warning that a
new ARK build breaks the image before hosts update to it.

1. It frees disk: it deletes all images and all BuildKit cache
   (`docker builder prune -af`). Pruning before the build, not after, means a
   crashed run can't leave the next one short of space. On GitHub-hosted
   runners it also deletes preinstalled toolchains and switches Docker to the
   classic image store (`containerd-snapshotter: false`): the containerd store
   keeps each layer compressed and unpacked, and the image then overflows the
   72 GB disk.
2. It runs `.github/build-image.sh`, which downloads ARK with SteamCMD
   (`container/files/build.sh`, with `STEAMCMD_DOCKER_OPTS=--security-opt
   seccomp=unconfined`, which GitHub-hosted runners need) and builds
   `ark-server:latest`. `ARK_BUILD_DELETE_CACHE=1` deletes the SteamCMD
   install once the server files image exists, to leave room for the runtime
   image.
3. Before the smoke tests it sets the Proton kernel settings with `sysctl`,
   clears the build cache and adds a 16 GB swap file. One server outgrows the
   runner's RAM and is OOM-killed without it.
4. It runs the smoke suite (`run-tests.py --suite smoke`) against the new
   image: a healthy server start and a clean stop. The full slow suite stays
   manual. The batch table goes to the run summary. For each failed test, the
   job log gets a collapsible group with its pytest report and compose, docker
   and framework logs (last 300 lines each), and `.tests/` is uploaded as the
   `test-artifacts` artifact.

5. On `main` only, after the smoke tests pass: if GHCR has no
   `ghcr.io/bryanwieg/ark-server-base:<tag>` for the base the build used
   (`build-image.sh` prints `BASE_IMAGE=<ref>` last), it logs in with
   `GITHUB_TOKEN` (`packages: write`) and pushes it. Runs on other branches build
   the base locally when GHCR lacks it and publish nothing.

Only one smoke run happens at a time (`concurrency: image-smoke`). A run takes
about 25 minutes; on a public repository GitHub-hosted runners are free.

## Base image refresh

`.github/workflows/base-refresh.yml` runs at 07:00 UTC on the 1st of each month
(or manually). It sets the `# refresh: YYYY-MM` line in `container/base.dockerfile`
to the current month, pushes a branch and opens
`build(container): refresh the base image (YYYY-MM)`. Merging it changes the base
tag, so the next `image.yml` run on `main` rebuilds the base with current Ubuntu
packages and publishes it. The PR is opened with `GITHUB_TOKEN`, so no checks run
on it; it changes one comment line.

## Repository settings

No secrets or variables are needed. The first publish creates the GHCR package
`ark-server-base` linked to this repository (the base image's
`org.opencontainers.image.source` label). Set its visibility to **Public** once
(Packages → `ark-server-base` → Package settings): until then anonymous pulls
fail and hosts build the base themselves.

Also enable **Settings → Actions → General → Allow GitHub Actions to create and
approve pull requests**, or release-please can't open its PRs.

Release PRs are opened with `GITHUB_TOKEN`, so the CLI and image workflows
don't run on them. Their only changes are the changelog and version files.
