# CI and Releases

GitHub Actions: one test pipeline per shipped artifact (the `ark-cli` binary and
the ARK-free base image), Python lint, a [release-please](https://github.com/googleapis/release-please)
release workflow for `ark-cli`, and update workflows whose PRs merge themselves
once tested (see [automated updates](#automated-updates)).

Path-filtered workflows also trigger on changes to their own workflow file.

| Workflow | Triggers | Does |
|---|---|---|
| `cli.yml` | PR / push to `main` touching `app/**`, `container/**` (embedded in `ark-cli`), `go.work*`, `.golangci.yml`; called by `release.yml`; manual (started by `go-update.yml`) | `go vet` + `go test` for `app/ark-cli`, `app/pkg/ark`, `app/ark-web`, `container`. `golangci-lint` (`.golangci.yml`) reporting new findings only (manual runs: new since `origin/main`); skipped on releases. On a release: builds static `ark-cli-<version>-linux-amd64` with the version stamped, checks `--version`, uploads it plus `.sha256` to the GitHub release. |
| `image.yml` | PR / push to `main` touching `container/**`, `app/ark-cli/**`, `app/pkg/**`, `tests/container/unit/**`, the container and CLI integration tests, `tests/helpers/**`, `tests/fixtures/lite_image/**`, `requirements-dev.txt`, `.github/build-image.sh`; daily 06:17 UTC; manual | Job `unit`: `pytest -m unit tests/container/unit`, then every `lite` test on the [lite image](test-architecture.md#lite-image-no-ark-server) (container and CLI, including a real OOM kill); the Go build cache the tests use (`.go-cache`) is cached between runs. Job `smoke` (not on PRs, except Dependabot `dependabot/docker/*`): builds the image, runs the smoke tests, on `main` publishes the base image. Job `alert`: issue on a failed `main` run. See [image smoke tests](#image-smoke-tests). |
| `python.yml` | PR / push to `main` touching `**.py`, `ruff.toml` | `ruff check` + `ruff format --check` (`ruff.toml`). |
| `base-refresh.yml` | 07:00 UTC on the 1st of each month; manual | See [base image refresh](#base-image-refresh). |
| `proton-update.yml` | Mondays 08:00 UTC; manual | PR for a newer GE-Proton release (`ARG PROTON_VERSION` in `container/base.dockerfile`); starts `image.yml` on it. |
| `go-update.yml` | Tuesdays 08:00 UTC; manual | PR for a newer stable Go in `go.work`; starts `cli.yml` on it. |
| `auto-merge.yml` | daily 10:00 UTC; manual | Merges passing update PRs and the release PR; opens issues for failed ones. |
| `release.yml` | push to `main`; manual (started by `auto-merge.yml`) | Maintains the release PR. When it merges: tags `cli-v<semver>`, creates the GitHub release, calls `cli.yml` to attach the binary. |

All under `.github/workflows/`.

## Versioning

One release, `ark-cli` (component `cli`), covers the whole repository except
`app/ark-web` (package `.` in `release-please-config.json`, `exclude-paths`).
The image recipe in `container/` is embedded in `ark-cli`, so recipe changes reach
hosts only through a CLI release.

| Release | Tag | Changelog | Version source |
|---|---|---|---|
| `ark-cli` | `cli-v<semver>` | `app/ark-cli/CHANGELOG.md` | git tag, stamped with `-ldflags "-X main.version=..."`; `ark-cli --version` prints it (`dev` for local builds) |

Pre-1.0 (`bump-minor-pre-major`): breaking changes and `feat` bump the minor
version, other releasable types the patch. Releasable types (the
`changelog-sections`): `feat`, `fix`, `perf`, `revert`, `deps`, `build` (image
recipe changes, e.g. a GE-Proton update). Current version:
`.release-please-manifest.json`. `container/CHANGELOG.md` is the history of the
former separate container release (up to 0.2.0).

**PRs are squash-merged only**, PR title as the commit subject, blank body
(repository settings: merge commits and rebase merges off,
`squash_merge_commit_title=PR_TITLE`, `squash_merge_commit_message=BLANK`), so the
PR title must be a Conventional Commit. *Why:* release-please also reads
Conventional Commit lines in a commit's body. A merge commit's body is the PR
title, and a squash body of commit messages repeats them, so either lists every
change twice in the changelog.

**History:** the repository was recreated as one commit when it went public, so
older tags are not here.

No ARK image is published: `ark-cli` builds `ark-server:<steam-build-id>` on each
host from its embedded recipe (see [image build](image-build.md)). Only the
ARK-free base image is published (below).

## Image smoke tests

The `smoke` job checks that the image still builds and starts with Steam's
current ARK build. It gates nothing except Dependabot Docker PRs (via
`auto-merge.yml`); the daily run is an early warning that a new ARK build breaks
the image before hosts update to it.

1. Free disk: `docker image prune -af` and `docker builder prune -af`, before the
   build so a crashed run can't leave the next one short of space. On
   GitHub-hosted runners also deletes preinstalled toolchains and switches Docker
   to the classic image store (`containerd-snapshotter: false`): the containerd
   store keeps each layer compressed and unpacked, and the image then doesn't fit
   the disk.
2. `.github/build-image.sh` downloads ARK with SteamCMD (`container/files/build.sh`)
   and builds `ark-server:latest`. `STEAMCMD_DOCKER_OPTS=--security-opt
   seccomp=unconfined` (Docker's default seccomp profile blocks SteamCMD on
   hosted runners). `ARK_BUILD_DELETE_CACHE=1` drops the SteamCMD install once
   the server files are in the build context, to leave room for the runtime image.
3. Hosted runners only: sets the Proton kernel settings with `sysctl`, clears the
   build cache, adds a 16 GB swap file (one server outgrows the runner's RAM and
   is OOM-killed without it).
4. `run-tests.py --suite smoke`: a healthy server start and a clean stop. The full
   slow suite stays manual. The suite log goes to the run summary; each failed
   test gets a collapsible log group with its pytest report and compose, docker
   and framework logs (last 300 lines each). On failure `.tests/` is uploaded as
   artifact `test-artifacts` (14 days).
5. On `main` only, after the smoke tests pass: if GHCR lacks
   `ghcr.io/bryanwieg/ark-server-base:<tag>` (`build-image.sh` prints
   `BASE_IMAGE=<ref>` last; the tag is a hash of `container/base.dockerfile`), it
   logs in with `GITHUB_TOKEN` (`packages: write`) and pushes it. Other branches
   build the base locally when GHCR lacks it and publish nothing.

One smoke run at a time (`concurrency: image-smoke`, no cancel). A run takes
15-35 minutes.

## Base image refresh

`base-refresh.yml` sets the `# refresh: YYYY-MM` line in
`container/base.dockerfile` to the current month (no-op if already set), pushes
`chore/base-refresh-YYYY-MM`, opens
`build(container): refresh the base image (YYYY-MM)` and starts `image.yml` on the
branch. Merging it changes the base tag, so `main` rebuilds the base with current
Ubuntu packages and publishes it.

## Repository settings

No secrets or variables needed; workflows use `GITHUB_TOKEN`. The GHCR package
`ark-server-base` is linked to this repository (the base image's
`org.opencontainers.image.source` label) and public, so hosts pull it
anonymously.

- **Merges:** squash only (PR title, blank body); branches deleted after merge.
- **Ruleset `main`:** changes only through pull requests, squash only (no required
  approvals or checks: workflows are path-filtered, so a required check would
  block docs-only PRs); no force pushes or deletion.
- **Actions:** workflow token read-only by default and may create pull requests
  (release-please and the update workflows need it); workflows from external
  contributors need approval.
- **Security:** secret scanning with push protection; Dependabot alerts and
  security updates.
- **Discussions** on (commercial-license requests, questions).

## Dependabot

`.github/dependabot.yml`: one grouped PR per ecosystem, weekly.

| Ecosystem | Directories | PR title | Releases |
|---|---|---|---|
| Go modules | `app/ark-cli`, `app/ark-web`, `app/pkg/ark` | `deps:` | yes (security updates too) |
| GitHub Actions | `/` | `ci:` | no |
| pip (test tooling) | `/` | `chore(deps):` | no |
| Docker | `app/ark-web/Dockerfile`, `container/files/steamcmd.dockerfile` | `build(deps):` | yes |

Docker `ubuntu` stays on the current LTS (major/minor updates ignored; only the
digest moves). The pinned build image in `container/base.dockerfile` is an `ARG`,
which Dependabot can't update; bump it deliberately. `auto-merge.yml` merges
Dependabot PRs once their checks pass.

## Automated updates

Updates merge and release themselves; the only thing to act on is an issue.

| Update | Opened by | Tested by |
|---|---|---|
| GE-Proton (`chore/proton-update-*`, `build(container):`) | `proton-update.yml`, Mondays | `image.yml` started on the branch: base + image build and smoke tests |
| Base refresh (`chore/base-refresh-*`, `build(container):`) | `base-refresh.yml`, monthly | same |
| Go (`chore/go-update-*`, `build:`) | `go-update.yml`, Tuesdays | `cli.yml` started on the branch: Go tests and lint |
| Dependabot Docker (`dependabot/docker/*`) | Dependabot, weekly | `image.yml` on the PR, smoke tests included |
| Dependabot Go, Actions, pip (`dependabot/*`) | Dependabot, weekly | the PR's normal path-filtered checks |

PRs opened with `GITHUB_TOKEN` don't trigger workflows, but `workflow_dispatch`
does, so the update workflows start `image.yml` / `cli.yml` on their branches;
the runs report on the branch's head commit.

`auto-merge.yml`, per update PR, reads the check runs on the head commit:

- all completed, none failed (success/skipped/neutral), at least one: squash-merge
  (subject `<PR title> (#<n>)`, blank body);
- any failed: open an issue labelled `automated-update` (one per PR), leave the PR
  open;
- checks still running: wait;
- no checks after a day, or passing but unmergeable for 3 days: open an issue.

Then it merges the release PR (`release-please--*`) so updates ship as an
`ark-cli` release, and closes `automated-update` issues whose PR is no longer
open. Merges made with `GITHUB_TOKEN` don't trigger push workflows, so it starts
`image.yml` on `main` (if it merged anything, to publish a changed base) and
always `release.yml`.

A failing smoke run on `main` opens the issue "Image smoke test failing on main"
(or comments on the open one). Hosts keep their current image meanwhile:
`ark-cli` only moves to a new build after it starts healthy, and rolls back
otherwise.

Release PRs are opened with `GITHUB_TOKEN`, so no workflows run on them; they
change only the changelog and version files.
