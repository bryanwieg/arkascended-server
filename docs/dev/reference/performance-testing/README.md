# Performance Testing

Proton/Wine sync benchmarks against the dev container: startup time, idle CPU, save time under
dino summon load, and CPU under concurrent RCON summon bursts.

## Test Environment (Hardware / Software)

Single-host results. Read them relative to this environment, not as absolute numbers.

### Host Hardware / OS (observed at test time)

- Ubuntu 24.04 LTS VM
- Desktop-class x86-64 CPU, 6 cores / 12 threads visible to the VM, single socket
- ~15 GiB RAM, ~2 GiB swap

### Docker / Runtime Stack

- Docker Engine `29.1.5` (API `1.52`), Docker Compose `v5.0.2`
- Container `ark-server-1`, image `ark-server:latest`, both from
  [`docker-compose.dev.yaml`](../../../../docker-compose.dev.yaml)
- Memory: 12G (the dev compose file now sets `mem_reservation: 12G`, a soft reservation, not a hard limit)
- No CPU quota or cpuset pinning

### Image / Proton / Wine Configuration

Set in [`container/base.dockerfile`](../../../../container/base.dockerfile):

- Base image `ubuntu:24.04`
- `GE-Proton11-12` (`PROTON_VERSION=11-12`)
- Env defaults: `PROTON_NO_ESYNC=0`, `PROTON_NO_FSYNC=0`, `WINEDEBUG=-all`

### ASA Runtime Details Observed in Logs

- ASA version `82.8`
- Example startup line: `Full Startup: 48.96 seconds`
- Server-reported cores: `Number of cores 6`

### Environment Caveats

- Sync modes were switched with runtime env overrides, not image rebuilds. The image default is now
  fsync/esync enabled.
- Host load varied over time; A/B runs alternate to reduce drift bias.
- Burst numbers depend heavily on sampling method and window. 1s sampling helped; sub-second cgroup
  counter sampling would be more precise.

## What Was Tested

### 1. Proton/Wine Sync A/B (Startup + Idle CPU)

Modes (runtime env overrides):

| Mode | `PROTON_NO_ESYNC` | `PROTON_NO_FSYNC` | `WINEESYNC` | `WINEFSYNC` |
|---|---|---|---|---|
| `disabled` (server-side sync) | 1 | 1 | 0 | 0 |
| `enabled` (fsync/esync) | 0 | 0 | 1 | 1 |

Method:
- 10 alternating start pairs (10 starts per mode, 20 total), 1 script run.
- Startup = time to `Server has completed startup and is now advertising for join.`
- Idle CPU: one 5-minute window per mode, `docker stats --no-stream` every 5s (60 samples per mode).

| Mode | Startup Median (s) | Startup Avg (s) | Idle CPU Avg (%) | Idle CPU Median (%) |
|---|---:|---:|---:|---:|
| disabled | 116.000 | 118.900 | 11.643 | 11.480 |
| enabled | 109.000 | 111.000 | 10.257 | 9.870 |

`enabled` had lower idle CPU and faster startup in this run. Startup timing is noisy across runs;
treat it as a secondary signal.

### 2. Dino Save PoC (120 summons)

Does a moderate dino spawn burst change `saveworld` duration? Summons via RCON:
`cheat Summon Dodo_Character_BP_C`.

Method (1 script run):
- Baseline: 1-minute CPU window (12 samples @ 5s) + 5 `saveworld` runs, timed from the server log
  line `World Save Complete. Took:`.
- Spawn: 120 sequential summons, all RCON-acknowledged.
- Post-spawn: same CPU window + 5 saves.

| Phase | Save Median (s) | Save Avg (s) | CPU Avg (%) | CPU Median (%) |
|---|---:|---:|---:|---:|
| Baseline | 0.383 | 0.398 | 9.825 | 9.600 |
| Post-spawn (120 dodos) | 0.381 | 0.389 | 9.837 | 9.855 |

No meaningful save-time or idle CPU change.

### 3. Dino Save PoC (5000 summons)

Same PoC with 5000 summons (`success=5000`, `fail=0`), 1 script run. All windows completed:
12 CPU samples and 5 saves each for baseline and post-spawn.

No significant save-time penalty; post-spawn CPU and save time stayed near baseline.

### 4. Burst CPU Benchmark (1 run per mode)

Method:
- 100 concurrent summons in one burst.
- Per mode: 15s pre-burst CPU window, burst, immediate `saveworld`, 20s post-burst window.
- 1s sampling of: container total (`docker stats --no-stream`), `ArkAscendedServer.exe` and
  `wineserver` (`/proc` CPU tick deltas inside the container), and the sum of those two.
- 1 script run, 1 pass per mode (200 summons, 2 post-burst saves). Sample counts vary slightly with timing.

`enabled` had lower post-burst medians on all tracked series, lower container burst CPU, and faster
post-burst save. Single run: secondary to section 5.

### 5. Burst CPU Benchmark (5 runs per mode)

Same as section 4, 5 alternating passes per mode (10 passes, 1000 summons, 10 post-burst saves),
1 script run. Values are the medians from the `scope=aggregate` rows of the summary CSV.

| Mode | Burst Elapsed Median (ms) | Post-burst Save Took Median (s) | Post-burst RCON RTT Median (ms) |
|---|---:|---:|---:|
| disabled | 2527.0 | 0.495 | 679.0 |
| enabled | 2036.0 | 0.550 | 727.0 |

| Mode | Series | Pre CPU Median (%) | Peak CPU Median (%) | Post CPU Median (%) |
|---|---|---:|---:|---:|
| disabled | container | 18.210 | 413.730 | 61.510 |
| disabled | arkserver | 13.711 | 180.438 | 34.932 |
| disabled | wineserver | 2.284 | 26.794 | 22.129 |
| disabled | tracked_sum | 15.996 | 198.566 | 57.616 |
| enabled | container | 17.690 | 99.120 | 53.340 |
| enabled | arkserver | 13.230 | 211.890 | 30.736 |
| enabled | wineserver | 1.792 | 20.501 | 19.204 |
| enabled | tracked_sum | 14.872 | 231.848 | 50.566 |

- `enabled`: faster burst completion, lower post-burst CPU medians on every series.
- Peak medians mixed: `enabled` lower for container and `wineserver`, higher for `arkserver` and
  `tracked_sum`. More burst work likely lands in the game process, which still recovers faster.
- Post-burst save time and RCON RTT were worse with `enabled`, so the CPU and save signals diverged.
- Peaks come from 1s windows and are more timing-sensitive than post-burst medians.

## Common Methodology Notes

- Readiness marker: `Server has completed startup and is now advertising for join.`
- Save timing, where measured:
  - RCON round-trip of `saveworld` (in-container `ark-rcon.py`; older runs used `rcon-cli`)
  - Server log `World Save Complete. Took:` (game-side)
- CPU sources: `docker stats --no-stream` (container); `/proc` tick deltas for
  `ArkAscendedServer.exe` and `wineserver` (burst benchmark only).
- 1s burst sampling suits transient spikes, not long-term steady-state trends.

## Canonical Result Basis (Aggregate vs PoC)

| Conclusion | Basis |
|---|---|
| Startup / idle CPU | Section 1: 10 startups + one 5-minute idle window (60 samples) per mode |
| Burst CPU | Section 5: 5 passes per mode, summary CSV `aggregate` rows |
| Save under dino load | Section 2: 5 + 5 saves around 120 summons |

Section 4 is a process-split sanity check only.

## Data

Raw datasets are not kept in the repo. Regenerate with the scripts in [`scripts/`](scripts/). All
args are positional and optional; read the defaults at the top of each script.

| Script | Sections | Args (in order) |
|---|---|---|
| [`proton_sync_benchmark.sh`](scripts/proton_sync_benchmark.sh) | 1 | pairs (10), idle CPU minutes (5), container, compose file, compose service |
| [`dino_save_poc.sh`](scripts/dino_save_poc.sh) | 2, 3 | container, spawn count (120), save runs (5), RCON password, RCON `host:port`, summon command |
| [`dino_burst_cpu_benchmark.sh`](scripts/dino_burst_cpu_benchmark.sh) | 4, 5 | container, burst size (100), burst repeats, pre/post seconds (15/20), sample interval (1), RCON password, RCON `host:port`, compose file, runs per mode (1), compose service |

- Host needs `docker`, `rg`, and `python3` (burst script only). RCON runs inside the container via
  `ark-rcon.py`.
- The sync and burst scripts recreate the compose service with `docker compose up --force-recreate`,
  once per mode switch.
- Scripts `cd` two levels above `scripts/`, which is `docs/dev/reference/`, not the repo root. Pass an
  absolute compose file path. Output (CSV, log, Markdown report) goes to
  `docs/dev/reference/codex/benchmarks/`.

## Conclusions

- fsync/esync (`WINEFSYNC=1`, `WINEESYNC=1`) stays the default: lower idle CPU and faster burst CPU recovery.
- Keep watching save time under burst load. Lower burst CPU did not always mean faster post-burst saves.
- Startup timing: no large regression from fsync/esync. Direction varied across runs (the latest
  10-pair run favored fsync), so it does not predict runtime CPU.
- Idle CPU: low-noise baseline, favors fsync/esync, but does not exercise spike paths.
- Sequential-summon save PoC: validated save timing via `World Save Complete. Took:`. 120 summons
  had no effect on saves. Weak CPU trend generator.
- Concurrent burst (100 summons): clearest CPU signal. Short, repeatable load; peak and post-burst
  CPU can be compared per process. Use burst-style process-split tests for future tuning.
