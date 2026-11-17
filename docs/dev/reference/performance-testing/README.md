# Performance Testing

This directory documents performance experiments run against the Dockerized ASA server runtime in this repo, including startup timing, idle CPU behavior, save behavior under admin-driven dino load, and burst CPU behavior under multithreaded RCON summon bursts.

## Test Environment (Hardware / Software)

The benchmarks in this directory were run on a single host/workstation/VM environment and should be interpreted relative to that environment (not as universal absolute performance numbers).

### Host Hardware / OS (observed at test time)

- Host OS: Ubuntu 24.04 LTS virtual machine
- CPU (as the VM sees it): a desktop-class x86-64 CPU, 12 logical CPUs (`6` cores / `12` threads), single socket
- Host memory at capture time: ~15 GiB RAM total, ~2 GiB swap configured

### Docker / Runtime Stack

- Docker Engine: `29.1.5` (API `1.52`)
- Docker Compose: `v5.0.2`
- Benchmark target container: `ark-server-1` (`docker-compose.dev.yaml`)
- Compose image target: `ark-server:latest` (`docker-compose.dev.yaml:6`)
- Compose memory limit: `12G` (`docker-compose.dev.yaml:22`)
- CPU quota / cpuset pinning: none configured during the reported benchmarks (no `cpuset`, no `NanoCpus` limit)

### Image / Proton / Wine Configuration

- Base runtime image: `ubuntu:24.04` (`container/dockerfile`)
- Proton version used in image builds: `GE-Proton11-12` (`container/dockerfile`)
- Runtime env defaults currently set in `container/dockerfile`:
  - `PROTON_NO_ESYNC=0`
  - `PROTON_NO_FSYNC=0`
  - `WINEDEBUG=-all`

### ASA Runtime Details Observed in Logs

- ARK/ASA version observed in benchmark runs: `82.8`
- Example observed startup line: `Full Startup: 48.96 seconds`
- Server-reported core count in log: `Number of cores 6`

### Environment Caveats

- Some tests intentionally overrode sync settings at runtime (`disabled` vs `enabled`) without rebuilding the image, while later the `container/dockerfile` default was updated to prefer `fsync/esync`.
- CPU/memory availability on the host may vary over time due to unrelated workloads; alternating A/B runs were used where possible to reduce drift bias.
- Burst CPU numbers are particularly sensitive to sampling method and window length; 1s sampling improved signal, but sub-second cgroup counter sampling would be more precise for future work.

## What Was Tested

### 1. Proton/Wine Sync A/B (Startup + Idle CPU)

Scenario:
- Compare server-side synchronization vs `fsync/esync` using runtime env overrides.
- Measure startup time to the server advertising log marker and 5-minute idle CPU usage.

Modes:
- `disabled`: `PROTON_NO_ESYNC=1`, `PROTON_NO_FSYNC=1`, `WINEESYNC=0`, `WINEFSYNC=0`
- `enabled`: `PROTON_NO_ESYNC=0`, `PROTON_NO_FSYNC=0`, `WINEESYNC=1`, `WINEFSYNC=1`

Methodology:
- 10 alternating startup pairs (20 total starts).
- Readiness marker: `Server has completed startup and is now advertising for join.`
- 5-minute idle CPU window sampled every 5s using `docker stats --no-stream`.

Run / Sample Counts:
- Benchmark script executions: 1
- Startup samples: 10 per mode (20 total)
- Idle CPU windows: 1 per mode (2 total)
- Idle CPU samples: 60 per mode (120 total; 5 min @ 5s)

Key Results:

| Mode | Startup Median (s) | Startup Avg (s) | Idle CPU Avg (%) | Idle CPU Median (%) |
|---|---:|---:|---:|---:|
| disabled | 116.000 | 118.900 | 11.643 | 11.480 |
| enabled | 109.000 | 111.000 | 10.257 | 9.870 |

Interpretation:
- `enabled` (`fsync`) was lower on idle CPU and faster on startup in this rerun, but startup timing remains noisy across runs and should be treated as a secondary signal.

### 2. Dino Save PoC (120 summons)

Scenario:
- Test whether a moderate dino spawn burst changes `saveworld` duration.
- Use `cheat Summon Dodo_Character_BP_C` via RCON.

Methodology:
- Baseline: 1-minute CPU window + 5 `saveworld` runs (using `World Save Complete. Took:` from server log).
- Spawn: 120 summon commands (RCON-acknowledged).
- Post-spawn: repeat 1-minute CPU window + 5 `saveworld` runs.

Run / Sample Counts:
- Benchmark script executions: 1
- Summon commands: 120 total
- Save measurements: 10 total (5 baseline + 5 post-spawn)
- CPU windows: 2 total (baseline + post-spawn)
- CPU samples: 24 total (12 + 12; 1 min @ 5s each phase)

Key Results:

| Phase | Save Median (s) | Save Avg (s) | CPU Avg (%) | CPU Median (%) |
|---|---:|---:|---:|---:|
| Baseline | 0.383 | 0.398 | 9.825 | 9.600 |
| Post-spawn (120 dodos) | 0.381 | 0.389 | 9.837 | 9.855 |

Interpretation:
- No meaningful save-time penalty was observed from 120 dodo summons in this PoC, and short idle CPU after the spawn burst remained similar to baseline.

### 3. Dino Save PoC (5000 summons, completed)

Scenario:
- Escalate the same PoC to 5000 summon commands to test stronger transient load.

Methodology / Outcome:
- 5000/5000 summon commands were acknowledged (`success=5000`, `fail=0`).
- All post-spawn CPU and save windows completed.

Run / Sample Counts:
- Benchmark script executions: 1
- Summon commands: 5000 total
- Completed save measurements: 5 baseline, 5 post-spawn
- Completed CPU windows: 12 baseline, 12 post-spawn

Data Available:
- Baseline CPU summary present (12 samples)
- Baseline save samples present (5 runs)
- Post-spawn CPU samples present (12 samples)
- Post-spawn save samples present (5 runs)

Interpretation:
- No significant save-time penalty observed from 5000 dino summons. Post-spawn CPU and save metrics remained similar to baseline.

### 4. Burst CPU Benchmark (1 run per mode, upgraded process split)

Scenario:
- 100 concurrent dino summons in one burst.
- 1-second CPU sampling around the burst.
- Track CPU separately for container total, `ArkAscendedServer.exe`, `wineserver`, and their sum.

Methodology:
- Per mode: pre-burst CPU window (15s), burst, immediate post-burst `saveworld`, post-burst CPU window (20s).
- CPU samples:
  - container: `docker stats --no-stream`
  - processes: `/proc` CPU tick deltas (`ArkAscendedServer.exe`, `wineserver`)

Run / Sample Counts:
- Benchmark script executions: 1
- Burst passes: 1 per mode (2 total)
- Summon commands: 100 per pass (200 total)
- Post-burst save measurements: 1 per mode (2 total)
- CPU sampling cadence: 1s (pre + burst + post windows; exact samples vary slightly by timing)

Key Results:
- `enabled` (`fsync`) showed lower post-burst medians across tracked CPU series and better immediate post-burst save timing in the sampled run.
- In this rerun, `enabled` also showed lower container burst/post CPU and better immediate post-burst save timing, but single-run burst results remain secondary to the 5-run aggregate.

### 5. Burst CPU Benchmark (5 runs per mode, upgraded process split)

Scenario:
- Same burst benchmark as above, but 5 alternating runs per mode (10 total passes).

Methodology:
- Alternating order (`disabled`, `enabled`) to reduce drift bias.
- Aggregate values taken from the patched script's multi-run summary/report output (`scope=aggregate` rows in the summary CSV).

Run / Sample Counts:
- Benchmark script executions: 1
- Burst passes: 5 per mode (10 total)
- Summon commands: 100 per pass (1000 total)
- Post-burst save measurements: 10 total (1 per pass)
- CPU sampling cadence: 1s per pass (pre + burst + post windows)

Key Results (medians across 5 runs):

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

Interpretation:
- In this rerun, `enabled` (`fsync`) improved burst completion time and lowered post-burst CPU medians across container/process-tracked series.
- Peak CPU medians were mixed: `enabled` reduced container and `wineserver` peak medians, but increased `arkserver` and `tracked_sum` peak medians, suggesting more burst work may be concentrated into the game process while still recovering faster afterward.
- Post-burst save timing and RCON RTT were worse in `enabled` mode in this sample, so the burst CPU signal and burst save signal diverged.
- Peak metrics are derived from 1-second burst/post sampling windows and are more timing-sensitive than post-burst medians, so they should be treated as a noisier transient signal.
- This reinforces the value of tracking both CPU recovery metrics and save-path metrics rather than assuming they move together.

## Common Methodology Notes

- Startup readiness marker used throughout: `Server has completed startup and is now advertising for join.`
- RCON save timing was measured in two ways when applicable:
  - RCON round-trip time (`ark-rcon.py` command RTT; older runs used `rcon-cli`)
  - server log `World Save Complete. Took:` (game-side save completion)
- CPU sampling sources:
  - `docker stats --no-stream` for container percent
  - `/proc` CPU tick deltas for `ArkAscendedServer.exe` and `wineserver` (upgraded burst benchmark)
- Burst benchmarks use 1s sampling and are suitable for transient spikes, but not ideal for long-term steady-state trend analysis.

## Canonical Result Basis (Aggregate vs PoC)

The conclusions in this README are based on the following scenario-level result sets:

- Startup / idle CPU A/B (`Section 1`): aggregate within one benchmark execution using `10` startup samples per mode plus one `5-minute` idle CPU window per mode (`60` samples/mode).
- Burst CPU A/B (`Section 5`): aggregate across `5` burst passes per mode (10 total passes), using the multi-run summary CSV `aggregate` rows.
- Dino save PoC (`Section 2`): PoC-style scenario with repeated measurements inside one run (`5` saves baseline + `5` saves post-spawn, `120` summons total).

The single-run burst benchmark in `Section 4` is retained as a process-split example and sanity check, but the primary burst conclusions are drawn from the `5`-run aggregate in `Section 5`.

## Data

The raw datasets behind these results are not kept in the repository. To
reproduce or extend them, run the scripts in [`scripts/`](scripts/).

## Recommendation Summary

Based on the tests collected here, `fsync/esync` (`WINEFSYNC=1`, `WINEESYNC=1`) remains the preferred default for CPU-oriented behavior (idle CPU and burst CPU recovery/trends), but save-path behavior under burst load should continue to be monitored because burst CPU improvements did not consistently translate to better post-burst save timing in every rerun.

## Overall Conclusions Across Test Approaches

The different test methods produced different levels of signal:

- Startup-time benchmarking was useful for validating that `fsync/esync` does not introduce a large startup regression; across reruns it showed small shifts in either direction (the latest 10-pair sample favored `fsync`, while earlier reruns showed mixed/slower startup), so startup timing alone is not a strong proxy for runtime CPU behavior.
- Idle CPU benchmarking was useful for establishing a low-noise baseline and also favored `fsync/esync`, but it does not stress the runtime paths that matter during in-game spikes.
- The dino save PoC with sequential summons was useful to validate save measurement methodology (`World Save Complete. Took:`) and showed that small/moderate summon counts (e.g. 120 dodos) do not meaningfully impact save duration, but it was not a strong CPU trend generator.
- The multithreaded burst benchmark (100 concurrent summons) produced the most meaningful CPU trend data because it creates a short, repeatable transient load and allows direct comparison of peak and post-burst CPU behavior, especially when split by `ArkAscendedServer`, `wineserver`, and their sum.

Overall, the burst multithreaded benchmark provided the clearest CPU-related signal, while startup and idle tests provided supporting evidence and regression checks. Taken together, the combined results support using `fsync/esync` as the default and using burst-style process-split CPU tests as the primary method for future tuning comparisons.
