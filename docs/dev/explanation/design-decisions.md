# Design Decisions

Settled architectural choices and why. Read before proposing to change one.

## Why Proton/Wine instead of a native Linux binary

ARK: Survival Ascended ships only a Windows dedicated server. GE-Proton (community
Proton fork with extra patches) is the most reliable compatibility layer for it.

The version is pinned via the `PROTON_VERSION` build arg in
`container/base.dockerfile` (currently `11-12`), not tracked to latest, so
upstream Proton changes can't break the image unannounced.

## Why RCON is the primary control plane

Signals and exec can't confirm that player saves were flushed. RCON can: issue
`saveworld` and `DoExit`, wait for confirmation, and only then fall back to a kill
if the server is unresponsive. This removes the most common cause of corrupted
saves.

The container's exit code follows the same rule: RCON confirmations, not the
game's exit code, decide whether a stop was clean. After a confirmed `saveworld`
and `DoExit`, ARK intermittently exits 2 or 5; Proton passes that through, and
the logs look the same as for 0. So when save, shutdown and stop are all
confirmed, `ark-server-start.py` exits 0 and logs the game's code. An
unconfirmed step, or a server killed by a signal, still exits non-zero.

## Why YAML AST editing instead of template replacement

Regenerating `docker-compose.yaml` from a template on every `reconfigure` would
erase user-added services, comments and custom fields. AST-level edits via
`goccy/go-yaml` touch only the changed keys. Tradeoff: the editor must handle
YAML edge cases carefully.

`SaveProject` reloads the file, diffs it against the changed project, and
rewrites only the differing image, environment, label and port lines. Every
other byte stays. Before writing, it loads the edited copy and checks it matches
the requested project. If it doesn't, or the change needs flow style (`{...}`),
a port range, a section inherited through a `<<` merge key, or any other field,
it refuses, leaves the file alone and asks the user to edit by hand. The
approach it replaced (re-marshaling the loaded project) baked `.env` values and
`env_file` secrets into the file, made paths absolute, dropped comments, and
deleted services with an inactive profile.

## Why outcome-driven tests instead of mock-heavy unit tests

Mocking every internal call yields tests that pass while observable behavior is
broken. Outcome-driven tests (assert output, exit code, persisted state) catch
real regressions and survive internal refactors. Seam injection via `AppDeps`
gives mocking's isolation without the fragility.

Patterns: [reference/test-architecture.md](../reference/test-architecture.md).

## Why the two-layer host CLI + container runtime split

Orchestration lives in a static Go binary (the CLI); runtime logic lives in
Python scripts inside the container. Separate seams:

- Host-side changes ship as a CLI release; the image changes only when the
  build recipe embedded in the CLI does.
- Container scripts are tested in isolation, without the CLI.
- Clear boundary: the CLI speaks Docker/Compose SDK calls; the container speaks
  process lifecycle and RCON.

## Why the backup ≥2 floor

Count, age and size retention rules could otherwise delete every backup under
disk pressure. Retention always keeps at least 2 backups (1 when free space is
below the low-space threshold), so a recovery point always exists. Deliberate
tradeoff: that space is reserved regardless of user retention settings.

## Why two output channels with a strict user-text / operational-event split

A single structured channel (consumer demuxes) was rejected. Integration tests
match substrings in stdout/stderr, and operators pipe CLI output through text
tools; wrapping `"Created schedule nightly"` in a JSON envelope would churn many
assertions for no gain and hurt human readability. A separate `*slog.Logger`
stream for operational events (subprocess exec trace, domain lifecycle) keeps
`-v` useful for debugging without touching the channel operators and tests
depend on.

Full design: [explanation/logging-architecture.md](logging-architecture.md).
Channel rules: [reference/conventions.md](../reference/conventions.md#logging-and-cli-output).

## Why the SDK backend

One backend (`app/pkg/ark/sdkbackend/`) implements both `ContainerEngine` and
`ComposeService` through the Docker Go SDK: no subprocess overhead and full API
coverage. Podman works
through its Docker-compatible API socket (`DOCKER_HOST`), so one code path
serves both runtimes; conformance tests in `app/pkg/ark/conformance/` run
against both.

Interface contract and conformance details: [explanation/backends.md](backends.md).

## Why schedules run in a compose scheduler service

Schedules and the health monitor run in `ark-cli run scheduler`, the
`ark-scheduler` service in the instances' own `docker-compose.yaml`
(`enable scheduler` writes it). Earlier releases installed a generated
crontab with `apply schedules` instead. That was dropped because:

1. **Hosts without cron**: Fedora CoreOS has no `crontab` binary and no active
   `crond`, so scheduled backups could not run there at all.
2. **Missed runs**: cron skips runs silently while the host is off. The
   scheduler catches up once at start.
3. **Crontab clobbering**: `apply schedules` replaced the user's whole crontab,
   losing other entries. `enable scheduler` only removes ark-cli's own block.
4. **Visibility**: cron called `create backup`, so `check schedule` never saw
   cron runs. The scheduler shares the run, state and log code with
   `run schedule`.

Why a compose service: Komodo and `docker compose ps` show it with the
instances, it restarts with the engine (`restart: unless-stopped`), and its
health is a Docker healthcheck. It has no `ark.port.asa` label, which keeps it
out of every instance path (`ComposeProject.ServiceNames()`).

Why instances are identified by the `ark.port.asa` label: every
`create instance` service has it, so other services in the same compose file
(the scheduler, a web UI, a database) are never taken for ARK servers. The
older rule, every service but the scheduler, made `--instance all` and the
health monitor act on any sidecar.

Why a local `FROM scratch` image of the running binary
(`ark-scheduler:<sha256 prefix>`): the scheduler is always the same build as
the host CLI, nothing is published, and the tag shows when an upgrade needs
`enable scheduler` again. The binary must be static; `time/tzdata` is
embedded so `TZ` works without OS files.

Schedule definitions live in the compose file under `x-ark-schedules:`, read
with `LoadProject` and written with `SaveProject`; the scheduler rereads them
every minute. Why there and not in a file of their own: the compose file is
already the one file a user backs up, moves and hands to Komodo, and every
other ark-cli edit goes through it, so schedules travel with the instances they
target and need no second path rule (the old file lived under the current
directory, which had to be the compose directory). Compose keeps `x-` keys
untouched; `$` is written as `$$` so interpolation leaves commands alone. The
old schedule file under `.ark-manage/` is not migrated: `enable scheduler` warns
while it exists and the user recreates the schedules.

Run state stays in files under the compose file's `.ark-manage/` (readable, no
database):

| File | Purpose |
|---|---|
| `locks/schedule-<name>.lock` | Prevents concurrent runs of the same schedule |
| `state/schedules/<name>.last.json` | Last-run time, status and error for `check schedule`, catch-up and health |
| `state/scheduler/health.json` | Last health report, read by `check scheduler` |

## Why kernel file locks

Instance and schedule locks are Linux open-file-description locks
(`F_OFD_SETLK`, `app/pkg/ark/filelock/`). The earlier lock files held a PID and
were removed when that PID was not running. That breaks across PID namespaces:
the host CLI and the scheduler container cannot see each other's PIDs, so each
would delete the other's live locks. The kernel releases an OFD lock when its
holder exits, also after a crash, and the lock works across containers on a
shared bind mount, so no stale-lock cleanup is needed.

OFD rather than `flock(2)`: `F_OFD_GETLK` tests a lock without taking it, so
the monitor's "is this instance busy?" probe can never make a real acquisition
fail. The locks conflict within one process too and are not re-entrant, so a
command takes each lock once at its entry point; an update rollout's own stop,
backup and restore run inside it (`LockHeld`).

## Relative Temp Directory

The CLI uses `./.ark-tmp/` (override: `ARK_TMP_DIR`) instead of `/tmp`: temp
files stay in the project directory for cleanup, and `/tmp` is often too small.

## Why GameUserSettings.ini is baked into the image

ARK has no documented default `GameUserSettings.ini`, and its contents change
between versions, so the image lets ARK write it. The build's `ini_defaults`
stage (`container/scripts/installer/generate-default-ini.py`) starts the server
once, sends SIGTERM, and keeps the file ARK writes as
`/home/ark-user/config-defaults/GameUserSettings.ini`. On every start the
entrypoint installs it into the save volume if the volume has none, then
re-applies the RCON and password settings (below).

*Why at build time, not first boot:* first boot used to start a temporary
server, grab the INI and kill it, gated by `.ark-config-init.done` / `.not_done`
marker files. That was brittle:

- ARK writes a partial INI shortly after launch and the complete one (all
  `[ServerSettings]` defaults) only after SIGTERM. Runtime staging kept the
  partial file. The generator waits for the complete one (`[ServerSettings]`
  present, size settled); ARK doesn't reliably exit after writing it, so the
  file, not the process, is the signal.
- `.not_done` only reached the volume when Docker pre-filled an empty named
  volume. Bind mounts, Kubernetes volumes, pre-filled volumes and some restores
  lacked it, and config init then exited 1 on every start: an endless restart
  loop.
- The markers guarded deleting the temporary server's world in `SavedArks/`,
  but the server was stopped before it created one.

Now one rule works for any volume: no `GameUserSettings.ini` → install the
default. An existing file is never touched. The default is copied to a temp file
and renamed into place, so an interrupted start can't leave a partial INI.
Marker files on older volumes are ignored. The build fails if ARK doesn't write
the complete file, rather than ship a partial default. Generation runs in its
own stage because Proton writes thousands of prefix files and ARK writes logs
that must not ship.

**SessionName is stripped.** ARK writes a random `[SessionSettings]
SessionName=ARK #NNNNNN` during the build run. The generator removes it so no
server inherits it; the name comes from `SESSION_NAME` on the command line.

**RCON and password settings are re-applied on every start.** `RCONEnabled`,
`RCONPort`, `ServerAdminPassword` and `ServerPassword` are written from the
environment each start (`ini_settings.update_config`, skipped when unchanged).
*Why:* stop and backup authenticate with the env password. If the INI kept an
old value, changing `SERVER_ADMIN_PASSWORD` would break every RCON call, turning
every stop into a forced kill without a save.

## Why images are built on the host

`ark-cli` builds `ark-server:<build-id>-<recipe-id>` locally from Steam; no ARK
image is published anywhere. *Why:* an image holding Studio Wildcard's server
binaries is redistribution and risks a DMCA takedown (e.g. Apple against
Docker-OSX on Docker Hub). Building on the host means this project distributes
no ARK files. The recipe is embedded in the `ark-cli` binary, so no repo clone
is needed.

Server files live in immutable images, never a volume; only `ShooterGame/Saved`
and clusters are volumes. *Why:* tools that kept server files on a volume and
updated them in place broke on ARK or Proton updates, with no rollback and no
notice.

Immutable images make rollback a tag switch: a failed update switches compose
back to the previous tag and restores the pre-update backup. Image cleanup keeps
each instance's current and previous image.

SteamCMD runs in its own throwaway container (`container/files/build.sh`), not
in a BuildKit stage. BuildKit evicted its cache mounts (Docker's default
build-cache limits dropped the SteamCMD install), so delta updates silently
became full 12 GB downloads, and raising the limits needs host config on every
machine. The install is cached in `.ark-manage/build/work/cache`, outside
BuildKit's control. Don't move SteamCMD back into the runtime build.

## Why the INI files have a git history

ARK reads some keys as lists (one line per engram or item), and duplicates also
come from hand edits and restored backups. `set ini` must not collapse lines
ARK wrote on purpose, so it needs to know who wrote each line, not just the
file's current content. A per-instance git repository next to the files
(`Saved/Config/WindowsServer/.git`) answers that per line with `git blame`, and
gives a full log and diff for free (`list ini-history`, `restore ini`).

- **Authors, not heuristics.** Each writer commits under its own author name
  (`ini_history.py`). The start script commits whatever changed during a run as
  ARK's: the entrypoint committed just before the server started, and ark-cli
  refuses to write while it runs. A first attempt kept one JSON snapshot of
  ARK's last write and matched values against it; it couldn't tell repeated
  identical values apart and lost everything older than the last write.
- **In the save volume.** Backups archive the history with the files, and a
  restore brings back the matching history, so origins stay true after a
  restore.
- **Container-side only.** Commits are made by the runtime scripts, never by
  ark-cli on the host ([container script boundary](architecture.md#container-script-boundary)).
- **Never blocks.** A git failure warns and the server starts anyway; the
  history is informational.
- **Packed, never capped.** ARK rewrites its INI files on every stop, so the
  history grows with every run. Unpacked, a revision costs about 18 KB (whole
  file copies); `commit` packs the repository once enough objects are loose
  and keeps no reflog or hooks, which brings it to about 0.3 KB per revision.
  At that size a cap on revisions or bytes would save nothing worth having,
  and dropping revisions would rewrite history: `git blame` origins would lose
  their dates and `restore ini` its targets. So no revision is ever dropped.
