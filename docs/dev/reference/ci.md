# CI and Releases

GitHub Actions runs two independent pipelines, one per shipped artifact (the
`ark-cli` binary and the ARK-free base image), a release workflow that versions
`ark-cli` with [release-please](https://github.com/googleapis/release-please),
and automated updates that merge themselves once their tests pass (see
[automated updates](#automated-updates)).

| Workflow | Triggers | Does |
|---|---|---|
| `.github/workflows/cli.yml` | PR / push to `main` touching `app/**`, `container/**` (embedded in `ark-cli`), `go.work*`, `.golangci.yml`; called by `release.yml`; manual (started by `go-update.yml`) | `go vet` + `go test` for `app/ark-cli`, `app/pkg/ark`, `app/ark-web` and `container`; `golangci-lint` (`.golangci.yml`) failing on new findings only. On a release: builds `ark-cli-<version>-linux-amd64` (static, version stamped) and attaches it plus a `.sha256` to the GitHub release. |
| `.github/workflows/image.yml` | PR / push to `main` touching `container/**`, container unit tests, `build-image.sh`; daily at 06:17 UTC; manual | Python unit tests (`tests/container/unit`) and `test_server_oom_report.py`, which triggers a real OOM kill in a `python:3.14-slim` container. Pushes to `main`, scheduled and manual runs, and Dependabot's Docker PRs then build the image and run the smoke tests; on `main` it publishes the ARK-free base image and opens an issue if the smoke tests fail (see below). |
| `.github/workflows/python.yml` | PR / push to `main` touching `**.py`, `ruff.toml` | `ruff check` + `ruff format --check` (config: `ruff.toml`). |
| `.github/workflows/base-refresh.yml` | 1st of each month; manual | Opens a PR bumping the base image's refresh line and starts the smoke tests on it. |
| `.github/workflows/proton-update.yml` | Mondays; manual | Opens a PR for a newer GE-Proton release and starts the smoke tests on it. |
| `.github/workflows/go-update.yml` | Tuesdays; manual | Opens a PR for a newer Go release in `go.work` and starts the CLI tests and lint on it. |
| `.github/workflows/auto-merge.yml` | daily at 10:00 UTC; manual | Merges automated update PRs whose checks passed and the release PR; opens an issue for a failed one. |
| `.github/workflows/release.yml` | push to `main`; manual (started by `auto-merge.yml`) | Maintains the release PR. Merging it tags `cli-v<semver>`, creates the GitHub release and calls `cli.yml` to attach the binary. |

## Versioning

One release, `ark-cli`, covers the whole repository except `app/ark-web` (package
`.` in `release-please-config.json`): the image recipe in `container/` is embedded
in `ark-cli`, so recipe changes reach hosts only through a CLI release.

| Release | Tag | Changelog | Version source |
|---|---|---|---|
| `ark-cli` | `cli-v<semver>` | `app/ark-cli/CHANGELOG.md` | git tag, stamped with `-ldflags "-X main.version=..."`; `ark-cli --version` prints it (`dev` for local builds) |

It is pre-1.0: breaking changes and `feat` bump the minor version, `fix` the
patch. Version bumps come from Conventional Commit prefixes. Releasable types:
`feat`, `fix`, `perf`, `revert`, `deps` and `build` (image recipe changes, such as
a GE-Proton update). `container/CHANGELOG.md` is the history of the former
separate container release (up to `container-v0.2.0`).

**PRs are squash-merged only**, with the PR title as the commit subject and a
blank body (repository settings: merge commits and rebase merges are off,
`squash_merge_commit_title=PR_TITLE`, `squash_merge_commit_message=BLANK`), so
the PR title must be a Conventional Commit. *Why:* release-please also reads
Conventional Commit lines in a commit's body as changes. A merge commit's body
is the PR title, and a squash body of commit messages repeats them, so either
one lists every change twice in the changelog.

**History:** the repository was recreated as one commit when it went public, so
there are no earlier tags here; release-please starts from the manifest version
(`0.3.0`).

No ARK image is published: `ark-cli` builds `ark-server:<steam-build-id>` on
each host from the recipe embedded in it (see [image build](image-build.md)).
Only the ARK-free base image is published (below).

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
to the current month, pushes a branch, opens
`build(container): refresh the base image (YYYY-MM)` and starts `image.yml` on the
branch (see [automated updates](#automated-updates)). Merging it changes the base
tag, so `main` rebuilds the base with current Ubuntu packages and publishes it.

## Repository settings

No secrets or variables are needed. The GHCR package `ark-server-base` is linked
to this repository (the base image's `org.opencontainers.image.source` label) and
public, so hosts pull it anonymously.

Settings in use:

- **Merges:** squash only, with the PR title as the commit subject and a blank
  body; branches are deleted after merge.
- **Ruleset `main`:** changes only through pull requests (no required approvals or
  checks: workflows are path-filtered, so a required check could block docs-only
  PRs); no force pushes or deletion.
- **Actions:** the workflow token is read-only by default and may create pull
  requests (release-please and `base-refresh.yml` need it); workflows from outside
  contributors need approval before they run.
- **Security:** secret scanning with push protection; Dependabot alerts and
  security updates.
- **Discussions** on (commercial-license requests, questions).

## Dependabot

`.github/dependabot.yml` opens one grouped PR per ecosystem each week: Go modules
(`app/ark-cli`, `app/ark-web`, `app/pkg/ark`) titled `deps:`, which release-please
releases as a CLI change; GitHub Actions titled `ci:`; Python test tooling titled
`chore(deps):`; and the Docker base images in `app/ark-web/Dockerfile` and
`container/files/steamcmd.dockerfile` titled `build(deps):`, which counts as a
release. The pinned build image in `container/base.dockerfile` is an `ARG`, which
Dependabot can't update; bump it deliberately. Dependabot's PRs are merged by
`auto-merge.yml` once their checks pass.

## Automated updates

Updates merge and release themselves; the only thing to act on is an issue.

| Update | Opened by | Tested by |
|---|---|---|
| GE-Proton (`chore/proton-update-*`, `build(container):`) | `proton-update.yml`, Mondays | `image.yml` started on the branch: base + image build and smoke tests |
| Base refresh (`chore/base-refresh-*`, `build(container):`) | `base-refresh.yml`, monthly | same |
| Go (`chore/go-update-*`, `build:`) | `go-update.yml`, Tuesdays | `cli.yml` started on the branch: Go tests and lint |
| Dependabot Docker (`dependabot/docker/*`) | Dependabot, weekly | `image.yml` on the PR, smoke tests included |
| Dependabot Go, Actions, pip (`dependabot/*`) | Dependabot, weekly | the PR's normal checks (`cli.yml`, `image.yml` unit tests, `python.yml`) |

PRs opened with `GITHUB_TOKEN` don't trigger workflows, but `workflow_dispatch`
does, so the update workflows start `image.yml` on their branches themselves; the
runs report on the branch's head commit.

`auto-merge.yml` runs daily. For each of those PRs it reads the check runs on the
head commit: if all completed without failing (and at least one ran) it
squash-merges the PR; if one failed it opens an issue labelled `automated-update`
(once per PR) and leaves the PR open; while checks run it waits. A PR whose
update workflow never got checks running is reported after a day. It then merges
the release PR (`release-please--*`), so updates ship as an `ark-cli` release.
Merges made with `GITHUB_TOKEN` don't trigger push workflows, so it starts
`image.yml` (to publish a changed base) and `release.yml` on `main` itself.

A failing smoke run on `main` opens the issue "Image smoke test failing on main"
(or comments on it). Hosts keep their current image meanwhile: `ark-cli` only
moves to a new build after it starts healthy, and rolls back otherwise.

Release PRs are opened with `GITHUB_TOKEN`, so the CLI and image workflows
don't run on them. Their only changes are the changelog and version files.
