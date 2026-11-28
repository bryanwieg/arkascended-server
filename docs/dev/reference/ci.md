# CI and Releases

GitHub Actions: checks on every pull request, one release per merged PR that
states a version, base image publishing on `main`, and update workflows whose
PRs merge themselves once tested (see [automated updates](#automated-updates)).

## Checks

Six check contexts report on **every** PR (opened, synchronized, reopened) and on
every `workflow_dispatch` run of a branch. The `main` ruleset requires all six on
an up-to-date branch. *Why every PR:* a required check that never reports blocks
the PR, so no check workflow has `paths` filters; a job without work is skipped,
and GitHub counts a skipped check as passed.

| Check | Workflow | Group | Does |
|---|---|---|---|
| `ruff` | `python.yml` | `python` | `ruff check` + `ruff format --check` (`ruff.toml`) |
| `test` | `cli.yml` | `go` | `go vet` + `go test` for `app/ark-cli`, `app/pkg/ark`, `app/ark-web`, `container` |
| `lint` | `cli.yml` | `go` | `golangci-lint` (`.golangci.yml`), new findings only |
| `unit` | `image.yml` | `unit` | `pytest -m unit` for the container scripts and the CI scripts, then every `lite` test on the [lite image](test-architecture.md#lite-image-no-ark-server) (container and CLI, including a real OOM kill); the Go build cache the tests use (`.go-cache`) is cached between runs |
| `smoke` | `image.yml` | `smoke` | builds the image from Steam's current ARK build, then a healthy start and clean stop; see [image smoke tests](#image-smoke-tests) |
| `version` | `version.yml` | none | the PR's `Release:` line matches its title and the latest `cli-v*` tag; see [releases](#releases). Also re-runs when the description is edited |

**Change groups** (`.github/changed.sh`, the only place the paths live):
`changed.sh GROUP` prints `true` when the branch's diff against its merge base
with `origin/main` touches the group, and `changed.sh hash GROUP` prints a
sha256 of the group's tracked files at `HEAD`. Docs (`*.md`, `docs/`) never
count. Each group includes its own workflow file and `changed.sh`.

| Group | Paths (summary) |
|---|---|
| `python` | `*.py`, `ruff.toml` |
| `go` | `app/**`, `container/**` (embedded in `ark-cli`), `go.work*`, `.golangci.yml` |
| `unit` | `container/**`, `app/ark-cli/**`, `app/pkg/**`, container and CLI tests, the lite image, the CI scripts and their tests, `release.yml` |
| `smoke` | what the image build and a start/stop use: `container/base.dockerfile`, `container/dockerfile`, the build and install files, the start/stop runtime scripts and their slow tests, the test framework, `run-tests.py`, `requirements-dev.txt`, `.github/build-image.sh`. A smoke path is always a unit path too |

**Pass markers:** a job that passes saves an Actions cache entry
`passed-<job>-<group hash>` (one prefix per job). The `changes` job of each
workflow looks the markers up (`lookup-only`); a job skips when its group has no
work or its own marker exists, so re-running unchanged content (a rebase, a
docs commit, a bot re-dispatch) costs nothing. *Why per job:* `test` passing must
never skip `lint` on the same hash.

**A failed `changes` job fails the checks:** each required job runs under
`!cancelled()` when `changes` did not succeed and its first step exits 1.
*Why:* a job skipped because a need failed counts as passed.

`smoke` runs on a PR once `unit` passed or was skipped. Its concurrency group is
per ref (`image-smoke-<ref>`), cancelling an older run of the same branch, so
runs of different PRs never cancel each other.

**Pushes run no checks.** PR runs come from `pull_request`; a push to `main`
runs only `release.yml` and, when `container/base.dockerfile` changed,
`image.yml`'s `publish-base`.

| Workflow | Triggers | Does |
|---|---|---|
| `python.yml`, `version.yml` | PR; manual (bot branches) | the checks above |
| `cli.yml` | PR; manual (bot branches); called by `release.yml` with a tag | the checks above; with a tag only: builds static `ark-cli-<version>-linux-amd64` with the version stamped, checks `--version`, uploads it plus `.sha256` to the GitHub release |
| `image.yml` | PR; manual; daily 06:17 UTC; push to `main` touching `container/base.dockerfile` | `unit` and `smoke` on PRs and dispatched branches; daily `smoke` on `main` (job `alert` opens an issue on failure); `publish-base` on `main` (below) |
| `release.yml` | push to `main`; manual (started by `auto-merge.yml`) | see [releases](#releases) |
| `base-refresh.yml` | 07:00 UTC on the 1st of each month; manual | See [base image refresh](#base-image-refresh). |
| `proton-update.yml` | Mondays 08:00 UTC; manual | PR for a newer GE-Proton release (`ARG PROTON_VERSION` in `container/base.dockerfile`) |
| `go-update.yml` | Tuesdays 08:00 UTC; manual | PR for a newer stable Go in `go.work` |
| `auto-merge.yml` | hourly; manual | See [automated updates](#automated-updates). |

All under `.github/workflows/`.

## Base image publishing

Job `publish-base` (`image.yml`) runs on `main` only: on a push touching
`container/base.dockerfile`, on a manual run on `main` (which `auto-merge.yml`
starts after a merge) and daily. It needs neither `unit` nor `smoke`. If GHCR
lacks `ghcr.io/bryanwieg/ark-server-base:<tag>` (the tag is the first 12 hex of
the sha256 of `container/base.dockerfile`, like `build-image.sh` and `ark-cli`
compute it), it builds the base, logs in with `GITHUB_TOKEN` (`packages:
write`) and pushes it. Other branches build the base locally when GHCR lacks it
and publish nothing.

No ARK image is published: `ark-cli` builds `ark-server:<steam-build-id>` on each
host from its embedded recipe (see [image build](image-build.md)).

## Releases

One release, `ark-cli`, tagged `cli-v<semver>`, covers the whole repository
except `app/ark-web` (scope `web`). The image recipe in `container/` is embedded
in `ark-cli`, so recipe changes reach hosts only through a CLI release. The
version is stamped with `-ldflags "-X main.version=..."`; `ark-cli --version`
prints it (`dev` for local builds).

**A PR releases when its description states `Release: X.Y.Z`.**
`.github/release-version.py` computes the expected line from the PR title (a
Conventional Commit) and the latest `cli-v*` tag; the `version` check fails
unless the description states exactly that, or no line when none is expected:

- pre-1.0, a breaking change (`!`) or `feat` bumps the minor version; from 1.0 a
  breaking change bumps the major;
- `fix`, `perf`, `revert`, `deps`, `build` (image recipe changes, e.g. a
  GE-Proton update) bump the patch;
- other types and scope `web` release nothing: no `Release:` line.

`release.yml` (push to `main`, or `gh workflow run release.yml --ref main`)
first finishes every `cli-v*` tag missing its GitHub release or binary. Then,
for each first-parent commit on `main` after the latest tag, oldest first, whose
merged PR states `Release: X.Y.Z`, it tags `cli-vX.Y.Z`, creates the GitHub
release (PR title and link: the GitHub releases are the changelog) and calls
`cli.yml` with the tag to attach the binary. Runs take turns (`concurrency:
release`).

**Recovery:** a merged PR whose `Release:` line is wrong stops releases at that commit; later PRs wait. Fix that PR's description,
then start `release.yml` (or wait for `auto-merge.yml`'s hourly start).

**Merges:** squash is the default (PR title as the subject, blank body); merge
commits are allowed when a PR's commits should stay; rebase merges are blocked.
The `Release:` line is read from the PR, not the commit message, so either merge
kind releases.

**History:** the repository was recreated as one commit when it went public, so
older tags are not here. Earlier releases were cut by release-please;
`app/ark-cli/CHANGELOG.md` and `container/CHANGELOG.md` (the former separate
container release, up to 0.2.0) are that history.

## Image smoke tests

The `smoke` job checks that the image still builds and starts with Steam's
current ARK build. It runs on PRs that touch the `smoke` group; the daily run on `main` is an early
warning that a new ARK build breaks the image before hosts update to it.

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
A run takes 15-35 minutes. On PRs and dispatched branches it saves its pass
marker; the daily run saves none.

## Base image refresh

`base-refresh.yml` sets the `# refresh: YYYY-MM` line in
`container/base.dockerfile` to the current month (no-op if already set), pushes
`chore/base-refresh-YYYY-MM`, opens
`build(container): refresh the base image (YYYY-MM)` with its `Release:` line and
starts the check workflows on the branch. Merging it changes the base tag, so `main` rebuilds the base with current
Ubuntu packages and publishes it.

## Repository settings

No secrets or variables needed; workflows use `GITHUB_TOKEN`. The GHCR package
`ark-server-base` is linked to this repository (the base image's
`org.opencontainers.image.source` label) and public, so hosts pull it
anonymously.

- **Merges:** squash default (PR title, blank body), merge commits allowed,
  rebase merges blocked; branches deleted after merge.
- **Ruleset `main`:** changes only through pull requests; requires `ruff`,
  `lint`, `test`, `unit`, `smoke` and `version` on an up-to-date branch (no
  required approvals); no force pushes or deletion.
- **Actions:** workflow token read-only by default and may create pull requests
  (the update workflows need it); workflows from external
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

| Update | Opened by | Branch, title |
|---|---|---|
| GE-Proton | `proton-update.yml`, Mondays | `chore/proton-update-*`, `build(container):` |
| Base refresh | `base-refresh.yml`, monthly | `chore/base-refresh-*`, `build(container):` |
| Go | `go-update.yml`, Tuesdays | `chore/go-update-*`, `build:` |
| Dependabot | Dependabot, weekly | `dependabot/*` |

The update workflows write the PR's `Release:` line (`release-version.py next`).
PRs opened with `GITHUB_TOKEN` don't trigger workflows, but `workflow_dispatch`
does, so they start `python.yml`, `cli.yml`, `image.yml` and `version.yml` on
their branches; the runs report on the branch's head commit, and skip without
work like on any PR.

`auto-merge.yml`, hourly, per bot PR (head in this repository; a `dependabot/*`
branch only when authored by Dependabot; others, such as forks with bot branch
names, are named in the summary and left alone):

- rewrites its `Release:` line when it no longer matches the title and the
  latest tag, then re-runs the PR's `pull_request` `version` run (a Dependabot
  PR), or dispatches `version.yml` when there is none. *Why re-run:* GitHub
  keeps judging a PR by its `pull_request` run of a check, so a second,
  dispatched run on the same commit doesn't replace a failed one; `version.yml`
  reads the PR through the API, so the re-run sees the new line;
- updates a PR behind `main` through the update-branch API and dispatches all
  four check workflows on it. A `chore/*` PR whose branch can't be updated is
  closed and opened again from `main` by its update workflow;
- all six checks' newest runs passed or skipped, none failed: squash-merge
  (subject `<PR title> (#<n>)`, blank body), **at most one per run**, so each
  merge releases on its own;
- any failed: open an issue labelled `automated-update` (one per PR), leave the
  PR open;
- no checks after a day, or passing but unmergeable for 3 days: open an issue.

Then it closes `automated-update` issues whose PR is no longer open. Merges made
with `GITHUB_TOKEN` don't trigger push workflows, so it starts `image.yml` on
`main` after a merge (publishes a changed base) and `release.yml` on every run,
which retries a missed or failed release each hour.

A failing smoke run on `main` opens the issue "Image smoke test failing on main"
(or comments on the open one). Hosts keep their current image meanwhile:
`ark-cli` only moves to a new build after it starts healthy, and rolls back
otherwise.
