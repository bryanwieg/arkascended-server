#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

CONTAINER=${1:-ark-server-1}
BURST_CONCURRENCY=${2:-100}
BURST_REPEATS=${3:-1}
PRE_SECONDS=${4:-15}
POST_SECONDS=${5:-20}
SAMPLE_INTERVAL=${6:-1}
RCON_PASS=${7:-MyPassword}
RCON_HOST=${8:-localhost:27020}
COMPOSE_BASE=${9:-docker-compose.dev.yaml}
RUNS_PER_MODE=${10:-1}
COMPOSE_SERVICE=${11:-asa-server-1}
DINO_CMD=${DINO_CMD:-"cheat Summon Dodo_Character_BP_C"}

stamp=$(date -u +%Y%m%dT%H%M%SZ)
out_dir="codex/benchmarks"
results_csv="$out_dir/dino-burst-cpu-$stamp.csv"
summary_csv="$out_dir/dino-burst-cpu-summary-$stamp.csv"
report_md="$out_dir/dino-burst-cpu-$stamp-report.md"
log_file="$out_dir/dino-burst-cpu-$stamp.log"
mkdir -p "$out_dir"

printf 'timestamp_utc,mode,category,run,metric,value,unit,detail\n' > "$results_csv"
: > "$log_file"

need_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }
need_cmd docker
need_cmd rg
need_cmd python3

ts_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts_utc)] $*" | tee -a "$log_file"; }
append_row() { printf '%s,%s,%s,%s,%s,%s,%s,%s\n' "$1" "$2" "$3" "$4" "$5" "$6" "$7" "${8//,/;}" >> "$results_csv"; }
log "config burst_concurrency=$BURST_CONCURRENCY burst_repeats=$BURST_REPEATS pre_seconds=$PRE_SECONDS post_seconds=$POST_SECONDS sample_interval=$SAMPLE_INTERVAL runs_per_mode=$RUNS_PER_MODE"

OVR_DISABLED=$(mktemp /tmp/compose-sync-disabled.XXXXXX.yaml)
OVR_ENABLED=$(mktemp /tmp/compose-sync-enabled.XXXXXX.yaml)
COMPOSE_UP_LOG=$(mktemp /tmp/dino_burst_compose.XXXXXX.log)
cleanup() {
  rm -f "$OVR_DISABLED" "$OVR_ENABLED" "$COMPOSE_UP_LOG"
}
trap cleanup EXIT

cat > "$OVR_DISABLED" <<'YAML'
services:
  __SERVICE_NAME__:
    environment:
      PROTON_NO_ESYNC: "1"
      PROTON_NO_FSYNC: "1"
      WINEESYNC: "0"
      WINEFSYNC: "0"
YAML
cat > "$OVR_ENABLED" <<'YAML'
services:
  __SERVICE_NAME__:
    environment:
      PROTON_NO_ESYNC: "0"
      PROTON_NO_FSYNC: "0"
      WINEESYNC: "1"
      WINEFSYNC: "1"
YAML
sed -i "s/__SERVICE_NAME__/$COMPOSE_SERVICE/" "$OVR_DISABLED" "$OVR_ENABLED"

mode_override() {
  case "$1" in
    disabled) echo "$OVR_DISABLED" ;;
    enabled) echo "$OVR_ENABLED" ;;
    *) return 1 ;;
  esac
}

compose_recreate_mode() {
  local mode=$1 over
  over=$(mode_override "$mode")
  docker compose -f "$COMPOSE_BASE" -f "$over" up -d --force-recreate "$COMPOSE_SERVICE" >"$COMPOSE_UP_LOG" 2>&1
}

wait_ready() {
  local max_polls=${1:-180}
  local i logs
  for i in $(seq 1 "$max_polls"); do
    logs=$(docker logs --tail 500 "$CONTAINER" 2>&1 || true)
    if echo "$logs" | rg -q 'Server has completed startup and is now advertising for join\.'; then
      local sync_line
      sync_line=$(echo "$logs" | rg -m1 'fsync: up and running\.|wineserver: using server-side synchronization\.' || true)
      printf '%s\n' "$sync_line"
      return 0
    fi
    sleep 5
  done
  return 1
}

rcon() {
  # The password goes through the environment so it stays off every command line.
  SERVER_ADMIN_PASSWORD="$RCON_PASS" docker exec -e SERVER_ADMIN_PASSWORD -e RCON_HOST="${RCON_HOST%:*}" \
    -e RCON_PORT="${RCON_HOST##*:}" "$CONTAINER" /usr/bin/python3.14 /home/ark-user/scripts/runtime/ark-rcon.py "$@"
}

count_save_complete() {
  docker exec "$CONTAINER" sh -lc "grep -c 'World Save Complete\\. Took:' /home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log 2>/dev/null || true" | tail -n1 | tr -d '\r ' 
}
last_save_complete_line() {
  docker exec "$CONTAINER" sh -lc "grep 'World Save Complete\\. Took:' /home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log | tail -n 1" | tr -d '\r'
}
parse_save_took() { sed -n 's/.*World Save Complete\. Took: \([0-9.]*\).*/\1/p'; }

saveworld_measure() {
  local mode=$1 run_no=$2 phase=$3 before after i out start end rtt line took
  before=$(count_save_complete)
  start=$(date +%s%3N)
  out=$(rcon saveworld 2>&1 || true)
  end=$(date +%s%3N)
  rtt=$((end-start))
  append_row "$(ts_utc)" "$mode" event "$run_no" ${phase}_rcon_save_rtt_ms "$rtt" ms "$out"
  for i in $(seq 1 30); do
    after=$(count_save_complete)
    if [ "${after:-0}" -gt "${before:-0}" ]; then
      line=$(last_save_complete_line)
      took=$(echo "$line" | parse_save_took)
      append_row "$(ts_utc)" "$mode" event "$run_no" ${phase}_save_complete_took "$took" s "$line"
      echo "$took"
      return 0
    fi
    sleep 1
  done
  append_row "$(ts_utc)" "$mode" event "$run_no" ${phase}_save_complete_took - timeout "no marker"
  echo "-"
  return 1
}

cpu_monitor_loop() {
  local mode=$1 run_no=$2 start_ms=$3 interval=$4 stop_flag=$5
  local idx=0 now_ms raw pct rel_ms
  local clk_tck prev_epoch_ms prev_ark_ticks prev_wine_ticks
  local ark_ticks wine_ticks ark_pids wine_pids
  local ark_pct wine_pct tracked_sum_pct delta_ms delta_sec
  local snapshot
  clk_tck=$(docker exec "$CONTAINER" sh -lc 'getconf CLK_TCK 2>/dev/null || echo 100' 2>/dev/null | tail -n1 | tr -d '\r ' || true)
  if ! echo "${clk_tck:-}" | rg -q '^[0-9]+$'; then clk_tck=100; fi
  prev_epoch_ms=
  prev_ark_ticks=
  prev_wine_ticks=
  while [ ! -f "$stop_flag" ]; do
    now_ms=$(date +%s%3N)
    raw=$(docker stats --no-stream --format '{{.CPUPerc}}' "$CONTAINER" 2>/dev/null | head -n1 || true)
    pct=$(echo "$raw" | tr -d '%' | tr -d ' ')
    if ! echo "$pct" | rg -q '^[0-9]+(\.[0-9]+)?$'; then pct=nan; fi
    snapshot=$(
      SERVER_ADMIN_PASSWORD="$RCON_PASS" docker exec -e SERVER_ADMIN_PASSWORD -e RCON_HOST="${RCON_HOST%:*}" \
    -e RCON_PORT="${RCON_HOST##*:}" "$CONTAINER" sh -lc '
ark=0; wine=0; ark_n=0; wine_n=0
for d in /proc/[0-9]*; do
  [ -r "$d/stat" ] || continue
  ticks=$(awk "{print \$14+\$15}" "$d/stat" 2>/dev/null || echo 0)
  comm=$(cat "$d/comm" 2>/dev/null || true)
  case "$comm" in
    wineserver)
      wine=$((wine + ticks)); wine_n=$((wine_n + 1))
      ;;
  esac
  if tr "\000" " " < "$d/cmdline" 2>/dev/null | grep -q "ArkAscendedServer.exe"; then
    ark=$((ark + ticks)); ark_n=$((ark_n + 1))
  fi
done
printf "%s %s %s %s\n" "$ark" "$wine" "$ark_n" "$wine_n"
' 2>/dev/null | tail -n1
    )
    ark_ticks=$(echo "$snapshot" | awk '{print $1}')
    wine_ticks=$(echo "$snapshot" | awk '{print $2}')
    ark_pids=$(echo "$snapshot" | awk '{print $3}')
    wine_pids=$(echo "$snapshot" | awk '{print $4}')
    if ! echo "${ark_ticks:-}" | rg -q '^[0-9]+$'; then ark_ticks=0; fi
    if ! echo "${wine_ticks:-}" | rg -q '^[0-9]+$'; then wine_ticks=0; fi
    if ! echo "${ark_pids:-}" | rg -q '^[0-9]+$'; then ark_pids=0; fi
    if ! echo "${wine_pids:-}" | rg -q '^[0-9]+$'; then wine_pids=0; fi

    ark_pct=nan
    wine_pct=nan
    tracked_sum_pct=nan
    if [ -n "${prev_epoch_ms:-}" ] && [ -n "${prev_ark_ticks:-}" ] && [ -n "${prev_wine_ticks:-}" ]; then
      delta_ms=$((now_ms - prev_epoch_ms))
      if [ "$delta_ms" -gt 0 ]; then
        delta_sec=$(awk -v ms="$delta_ms" 'BEGIN { printf "%.6f", ms/1000.0 }')
        ark_pct=$(awk -v cur="$ark_ticks" -v prev="$prev_ark_ticks" -v hz="$clk_tck" -v dt="$delta_sec" 'BEGIN { if (dt<=0 || hz<=0) print "nan"; else printf "%.3f", ((cur-prev)/hz)/dt*100.0 }')
        wine_pct=$(awk -v cur="$wine_ticks" -v prev="$prev_wine_ticks" -v hz="$clk_tck" -v dt="$delta_sec" 'BEGIN { if (dt<=0 || hz<=0) print "nan"; else printf "%.3f", ((cur-prev)/hz)/dt*100.0 }')
        tracked_sum_pct=$(awk -v a="$ark_pct" -v w="$wine_pct" 'BEGIN { if (a=="nan" || w=="nan") print "nan"; else printf "%.3f", a+w }')
      fi
    fi
    prev_epoch_ms=$now_ms
    prev_ark_ticks=$ark_ticks
    prev_wine_ticks=$wine_ticks
    rel_ms=$((now_ms-start_ms))
    idx=$((idx+1))
    append_row "$(ts_utc)" "$mode" cpu_sample "$run_no" cpu_percent "$pct" percent "sample=$idx;rel_ms=$rel_ms;epoch_ms=$now_ms;series=container"
    append_row "$(ts_utc)" "$mode" cpu_sample "$run_no" arkserver_cpu_percent "$ark_pct" percent "sample=$idx;rel_ms=$rel_ms;epoch_ms=$now_ms;ark_pids=$ark_pids"
    append_row "$(ts_utc)" "$mode" cpu_sample "$run_no" wineserver_cpu_percent "$wine_pct" percent "sample=$idx;rel_ms=$rel_ms;epoch_ms=$now_ms;wineserver_pids=$wine_pids"
    append_row "$(ts_utc)" "$mode" cpu_sample "$run_no" tracked_sum_cpu_percent "$tracked_sum_pct" percent "sample=$idx;rel_ms=$rel_ms;epoch_ms=$now_ms"
    sleep "$interval"
  done
}

concurrent_burst_once() {
  local concurrency=$1
  docker exec "$CONTAINER" sh -lc '
TD=$(mktemp -d)
for i in $(seq 1 '"$concurrency"'); do
  (
    /usr/bin/python3.14 /home/ark-user/scripts/runtime/ark-rcon.py "'"$DINO_CMD"'" >"$TD/$i.out" 2>&1 || true
  ) &
done
wait
success=0; fail=0
for i in $(seq 1 '"$concurrency"'); do
  out=$(cat "$TD/$i.out" 2>/dev/null || true)
  if echo "$out" | grep -q "Server received"; then success=$((success+1)); else fail=$((fail+1)); fi
done
printf "%s %s\n" "$success" "$fail"
rm -rf "$TD"'
}

run_burst_mode() {
  local mode=$1 run_no=$2
  local sync_line start_ready_ms bench_start_ms burst_start_ms burst_end_ms stop_flag monitor_pid t_pre_done
  local pre_sleep_ms post_sleep_ms total_success=0 total_fail=0 rep out succ fail save_took
  pre_sleep_ms=$((PRE_SECONDS * 1000))
  post_sleep_ms=$((POST_SECONDS * 1000))

  log "START mode=$mode run=$run_no"
  compose_recreate_mode "$mode"
  sync_line=$(wait_ready) || { log "FAIL mode=$mode run=$run_no readiness timeout"; return 1; }
  append_row "$(ts_utc)" "$mode" event "$run_no" sync_mode 0 text "$sync_line"
  log "READY mode=$mode run=$run_no sync='$sync_line'"

  # Best-effort cleanup and baseline save to start from a cleaner runtime state.
  rcon 'cheat DestroyWildDinos' >/tmp/dino_burst_cleanup.out 2>&1 || true
  append_row "$(ts_utc)" "$mode" event "$run_no" cleanup_destroy_wild_dinos 0 text "$(cat /tmp/dino_burst_cleanup.out 2>/dev/null || true)"
  save_took=$(saveworld_measure "$mode" "$run_no" preclean || true)
  [ -n "${save_took:-}" ] || true

  bench_start_ms=$(date +%s%3N)
  append_row "$(ts_utc)" "$mode" event "$run_no" benchmark_start_ms "$bench_start_ms" ms ""
  stop_flag=$(mktemp)
  rm -f "$stop_flag"
  cpu_monitor_loop "$mode" "$run_no" "$bench_start_ms" "$SAMPLE_INTERVAL" "$stop_flag" &
  monitor_pid=$!

  sleep "$PRE_SECONDS"
  burst_start_ms=$(date +%s%3N)
  append_row "$(ts_utc)" "$mode" event "$run_no" burst_start_ms "$burst_start_ms" ms "concurrency=$BURST_CONCURRENCY;repeats=$BURST_REPEATS"
  log "BURST start mode=$mode run=$run_no"
  for rep in $(seq 1 "$BURST_REPEATS"); do
    out=$(concurrent_burst_once "$BURST_CONCURRENCY")
    succ=$(echo "$out" | awk '{print $1}')
    fail=$(echo "$out" | awk '{print $2}')
    total_success=$((total_success + succ))
    total_fail=$((total_fail + fail))
    append_row "$(ts_utc)" "$mode" burst "$run_no" burst_batch_success "$succ" count "rep=$rep"
    append_row "$(ts_utc)" "$mode" burst "$run_no" burst_batch_fail "$fail" count "rep=$rep"
  done
  burst_end_ms=$(date +%s%3N)
  append_row "$(ts_utc)" "$mode" event "$run_no" burst_end_ms "$burst_end_ms" ms ""
  append_row "$(ts_utc)" "$mode" event "$run_no" burst_elapsed_ms "$((burst_end_ms-burst_start_ms))" ms ""
  append_row "$(ts_utc)" "$mode" event "$run_no" burst_success_total "$total_success" count ""
  append_row "$(ts_utc)" "$mode" event "$run_no" burst_fail_total "$total_fail" count ""
  log "BURST end mode=$mode run=$run_no success=$total_success fail=$total_fail elapsed_ms=$((burst_end_ms-burst_start_ms))"

  saveworld_measure "$mode" "$run_no" postburst >/dev/null || true
  sleep "$POST_SECONDS"

  touch "$stop_flag"
  wait "$monitor_pid" || true
  rm -f "$stop_flag"
  log "DONE mode=$mode run=$run_no"
}

# Compute summary from CSV using Python for simpler time-window math.
compute_summary() {
python3 - "$results_csv" "$summary_csv" "$report_md" "$BURST_CONCURRENCY" "$BURST_REPEATS" "$SAMPLE_INTERVAL" "$PRE_SECONDS" "$POST_SECONDS" <<'PY'
import csv, sys, statistics, math
from pathlib import Path

results = Path(sys.argv[1])
summary = Path(sys.argv[2])
report = Path(sys.argv[3])
burst_concurrency = sys.argv[4]
burst_repeats = sys.argv[5]
sample_interval = sys.argv[6]
pre_seconds = sys.argv[7]
post_seconds = sys.argv[8]
rows = list(csv.DictReader(results.open()))

# parse rows into per-mode structures
modes = ['disabled','enabled']
out_rows = []

def parse_detail(detail):
    d = {}
    for part in (detail or '').split(';'):
        if '=' in part:
            k,v = part.split('=',1)
            d[k]=v
    return d

SERIES = [
    ('cpu_percent', 'container'),
    ('arkserver_cpu_percent', 'arkserver'),
    ('wineserver_cpu_percent', 'wineserver'),
    ('tracked_sum_cpu_percent', 'tracked_sum'),
]

for mode in modes:
    mrows = [r for r in rows if r['mode'] == mode]
    events = {r['metric']: r for r in mrows if r['category']=='event'}
    series_samples = {metric: [] for metric, _ in SERIES}
    for r in mrows:
        if r['category'] != 'cpu_sample':
            continue
        if r['metric'] not in series_samples:
            continue
        try:
            val = float(r['value'])
        except ValueError:
            continue
        det = parse_detail(r['detail'])
        rel_ms = int(det.get('rel_ms','0'))
        epoch_ms = int(det.get('epoch_ms','0'))
        series_samples[r['metric']].append((epoch_ms, rel_ms, val))
    for metric in list(series_samples):
        series_samples[metric].sort()
    def event_ms(name):
        try:
            return int(events[name]['value'])
        except Exception:
            return None
    bench_start = event_ms('benchmark_start_ms')
    burst_start = event_ms('burst_start_ms')
    burst_end = event_ms('burst_end_ms')
    if not bench_start or not burst_start or not burst_end:
        continue
    def ev_value(metric):
        r = events.get(metric)
        return r['value'] if r else ''

    for metric, alias in SERIES:
        cpu_samples = series_samples.get(metric, [])
        pre_vals = [v for epoch, rel, v in cpu_samples if epoch < burst_start]
        post_vals = [v for epoch, rel, v in cpu_samples if epoch >= burst_end]
        burst_to_post_vals = [v for epoch, rel, v in cpu_samples if epoch >= burst_start]
        pre_med = statistics.median(pre_vals) if pre_vals else math.nan
        pre_avg = statistics.mean(pre_vals) if pre_vals else math.nan
        peak = max(burst_to_post_vals) if burst_to_post_vals else math.nan
        post_peak = max(post_vals) if post_vals else math.nan
        post_avg = statistics.mean(post_vals) if post_vals else math.nan
        post_med = statistics.median(post_vals) if post_vals else math.nan
        threshold = pre_med * 1.10 if pre_vals else math.nan

        decay_sec = None
        if post_vals and not math.isnan(threshold):
            seq = [(epoch, v) for epoch, rel, v in cpu_samples if epoch >= burst_end]
            consec = 0
            first_epoch = None
            for epoch, v in seq:
                if v <= threshold:
                    consec += 1
                    if first_epoch is None:
                        first_epoch = epoch
                    if consec >= 3:
                        decay_sec = max(0.0, (first_epoch - burst_end) / 1000.0)
                        break
                else:
                    consec = 0
                    first_epoch = None

        out_rows.extend([
            (mode, f'{alias}_pre_cpu_sample_count', len(pre_vals)),
            (mode, f'{alias}_post_cpu_sample_count', len(post_vals)),
            (mode, f'{alias}_pre_cpu_avg_pct', f'{pre_avg:.3f}' if not math.isnan(pre_avg) else 'nan'),
            (mode, f'{alias}_pre_cpu_median_pct', f'{pre_med:.3f}' if not math.isnan(pre_med) else 'nan'),
            (mode, f'{alias}_burst_to_post_cpu_peak_pct', f'{peak:.3f}' if not math.isnan(peak) else 'nan'),
            (mode, f'{alias}_post_cpu_peak_pct', f'{post_peak:.3f}' if not math.isnan(post_peak) else 'nan'),
            (mode, f'{alias}_post_cpu_avg_pct', f'{post_avg:.3f}' if not math.isnan(post_avg) else 'nan'),
            (mode, f'{alias}_post_cpu_median_pct', f'{post_med:.3f}' if not math.isnan(post_med) else 'nan'),
            (mode, f'{alias}_decay_threshold_pct', f'{threshold:.3f}' if not math.isnan(threshold) else 'nan'),
            (mode, f'{alias}_decay_to_110pct_pre_median_s', f'{decay_sec:.3f}' if decay_sec is not None else 'not_reached'),
        ])

    out_rows.extend([
        (mode,'burst_elapsed_ms',ev_value('burst_elapsed_ms')),
        (mode,'burst_success_total',ev_value('burst_success_total')),
        (mode,'burst_fail_total',ev_value('burst_fail_total')),
        (mode,'postburst_save_complete_took_s',ev_value('postburst_save_complete_took')),
        (mode,'postburst_rcon_save_rtt_ms',ev_value('postburst_rcon_save_rtt_ms')),
        (mode,'sync_mode', (events.get('sync_mode') or {}).get('detail','')),
    ])

with summary.open('w', newline='') as f:
    w = csv.writer(f)
    w.writerow(['mode','metric','value'])
    w.writerows(out_rows)

# Build markdown report.
by_mode = {m:{} for m in modes}
for m, k, v in out_rows:
    by_mode.setdefault(m,{})[k]=v

report_text = f'''# Dino Burst CPU Benchmark ({results.stem.split("dino-burst-cpu-")[-1]})

## Scenario

- Multithreaded dino spawn burst via RCON
- Burst pattern: {burst_repeats} x {burst_concurrency} concurrent summon commands
- CPU sampling interval: {sample_interval}s
- CPU trends tracked separately for container total, Ark server process, wineserver process, and Ark+wineserver sum
- Metrics include peak CPU and decay time after burst

## Files

- Raw CSV: `{results}`
- Summary CSV: `{summary}`

## Burst / Save Summary

| Mode | Sync | Burst Success | Burst Fail | Burst Elapsed (ms) | Post-burst Save Took (s) | Post-burst Save RCON RTT (ms) |
|---|---|---:|---:|---:|---:|---:|
| disabled | {by_mode.get('disabled',{}).get('sync_mode','')} | {by_mode.get('disabled',{}).get('burst_success_total','')} | {by_mode.get('disabled',{}).get('burst_fail_total','')} | {by_mode.get('disabled',{}).get('burst_elapsed_ms','')} | {by_mode.get('disabled',{}).get('postburst_save_complete_took_s','')} | {by_mode.get('disabled',{}).get('postburst_rcon_save_rtt_ms','')} |
| enabled | {by_mode.get('enabled',{}).get('sync_mode','')} | {by_mode.get('enabled',{}).get('burst_success_total','')} | {by_mode.get('enabled',{}).get('burst_fail_total','')} | {by_mode.get('enabled',{}).get('burst_elapsed_ms','')} | {by_mode.get('enabled',{}).get('postburst_save_complete_took_s','')} | {by_mode.get('enabled',{}).get('postburst_rcon_save_rtt_ms','')} |

## CPU Trend Summary (Container / Ark / wineserver / Sum)

| Mode | Series | Pre Median (%) | Peak (burst+post) (%) | Post Median (%) | Decay Threshold (110% of pre median) (%) | Decay to Threshold (s) |
|---|---|---:|---:|---:|---:|---|
| disabled | container | {by_mode.get('disabled',{}).get('container_pre_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('container_burst_to_post_cpu_peak_pct','')} | {by_mode.get('disabled',{}).get('container_post_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('container_decay_threshold_pct','')} | {by_mode.get('disabled',{}).get('container_decay_to_110pct_pre_median_s','')} |
| disabled | arkserver | {by_mode.get('disabled',{}).get('arkserver_pre_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('arkserver_burst_to_post_cpu_peak_pct','')} | {by_mode.get('disabled',{}).get('arkserver_post_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('arkserver_decay_threshold_pct','')} | {by_mode.get('disabled',{}).get('arkserver_decay_to_110pct_pre_median_s','')} |
| disabled | wineserver | {by_mode.get('disabled',{}).get('wineserver_pre_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('wineserver_burst_to_post_cpu_peak_pct','')} | {by_mode.get('disabled',{}).get('wineserver_post_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('wineserver_decay_threshold_pct','')} | {by_mode.get('disabled',{}).get('wineserver_decay_to_110pct_pre_median_s','')} |
| disabled | tracked_sum | {by_mode.get('disabled',{}).get('tracked_sum_pre_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('tracked_sum_burst_to_post_cpu_peak_pct','')} | {by_mode.get('disabled',{}).get('tracked_sum_post_cpu_median_pct','')} | {by_mode.get('disabled',{}).get('tracked_sum_decay_threshold_pct','')} | {by_mode.get('disabled',{}).get('tracked_sum_decay_to_110pct_pre_median_s','')} |
| enabled | container | {by_mode.get('enabled',{}).get('container_pre_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('container_burst_to_post_cpu_peak_pct','')} | {by_mode.get('enabled',{}).get('container_post_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('container_decay_threshold_pct','')} | {by_mode.get('enabled',{}).get('container_decay_to_110pct_pre_median_s','')} |
| enabled | arkserver | {by_mode.get('enabled',{}).get('arkserver_pre_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('arkserver_burst_to_post_cpu_peak_pct','')} | {by_mode.get('enabled',{}).get('arkserver_post_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('arkserver_decay_threshold_pct','')} | {by_mode.get('enabled',{}).get('arkserver_decay_to_110pct_pre_median_s','')} |
| enabled | wineserver | {by_mode.get('enabled',{}).get('wineserver_pre_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('wineserver_burst_to_post_cpu_peak_pct','')} | {by_mode.get('enabled',{}).get('wineserver_post_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('wineserver_decay_threshold_pct','')} | {by_mode.get('enabled',{}).get('wineserver_decay_to_110pct_pre_median_s','')} |
| enabled | tracked_sum | {by_mode.get('enabled',{}).get('tracked_sum_pre_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('tracked_sum_burst_to_post_cpu_peak_pct','')} | {by_mode.get('enabled',{}).get('tracked_sum_post_cpu_median_pct','')} | {by_mode.get('enabled',{}).get('tracked_sum_decay_threshold_pct','')} | {by_mode.get('enabled',{}).get('tracked_sum_decay_to_110pct_pre_median_s','')} |

## Decay Rule

`decay_to_110pct_pre_median_s` is the first time after burst end where CPU is at or below `1.10 * pre-burst median` for 3 consecutive samples.
'''
report.write_text(report_text)
print(summary)
print(report)
PY
}

compute_multi_run_summary() {
python3 - "$results_csv" "$summary_csv" "$report_md" "$BURST_CONCURRENCY" "$BURST_REPEATS" "$SAMPLE_INTERVAL" "$PRE_SECONDS" "$POST_SECONDS" <<'PY'
import csv, sys, math, statistics
from pathlib import Path
from collections import defaultdict

results = Path(sys.argv[1])
summary = Path(sys.argv[2])
report = Path(sys.argv[3])
burst_concurrency = sys.argv[4]
burst_repeats = sys.argv[5]
sample_interval = sys.argv[6]

rows = list(csv.DictReader(results.open()))
SERIES = [
    ('cpu_percent', 'container'),
    ('arkserver_cpu_percent', 'arkserver'),
    ('wineserver_cpu_percent', 'wineserver'),
    ('tracked_sum_cpu_percent', 'tracked_sum'),
]

def parse_detail(detail):
    out = {}
    for part in (detail or '').split(';'):
        if '=' in part:
            k, v = part.split('=', 1)
            out[k] = v
    return out

def fnum(value):
    try:
        return float(value)
    except Exception:
        return None

def inum(value):
    try:
        return int(float(value))
    except Exception:
        return None

def median_or_nan(vals):
    vals = [v for v in vals if isinstance(v, (int, float)) and not math.isnan(v)]
    return statistics.median(vals) if vals else math.nan

grouped = defaultdict(list)
for r in rows:
    grouped[(r['mode'], r['run'])].append(r)

per_run = []
for (mode, run), rset in sorted(grouped.items(), key=lambda x: (x[0][0], int(x[0][1]))):
    events = {r['metric']: r for r in rset if r['category'] == 'event'}
    samples = {metric: [] for metric, _ in SERIES}
    for r in rset:
        if r['category'] != 'cpu_sample' or r['metric'] not in samples:
            continue
        val = fnum(r['value'])
        if val is None:
            continue
        det = parse_detail(r.get('detail', ''))
        epoch_ms = inum(det.get('epoch_ms'))
        if epoch_ms is None:
            continue
        samples[r['metric']].append((epoch_ms, val))
    for metric in samples:
        samples[metric].sort()

    def ev(name):
        return events.get(name, {}).get('value', '')
    def ev_i(name):
        return inum(ev(name))

    bench_start = ev_i('benchmark_start_ms')
    burst_start = ev_i('burst_start_ms')
    burst_end = ev_i('burst_end_ms')
    sync_detail = (events.get('sync_mode') or {}).get('detail', '')

    run_metrics = {
        'mode': mode,
        'run': run,
        'sync_mode': sync_detail,
        'burst_elapsed_ms': ev('burst_elapsed_ms'),
        'burst_success_total': ev('burst_success_total'),
        'burst_fail_total': ev('burst_fail_total'),
        'postburst_save_complete_took_s': ev('postburst_save_complete_took'),
        'postburst_rcon_save_rtt_ms': ev('postburst_rcon_save_rtt_ms'),
    }

    if bench_start and burst_start and burst_end:
        for metric, alias in SERIES:
            seq = samples.get(metric, [])
            pre = [v for epoch, v in seq if epoch < burst_start]
            post = [v for epoch, v in seq if epoch >= burst_end]
            burst_post = [v for epoch, v in seq if epoch >= burst_start]
            pre_med = statistics.median(pre) if pre else math.nan
            peak = max(burst_post) if burst_post else math.nan
            post_med = statistics.median(post) if post else math.nan
            threshold = pre_med * 1.10 if pre else math.nan
            decay_sec = None
            if post and not math.isnan(threshold):
                consec = 0
                first_epoch = None
                for epoch, v in [(e, v) for e, v in seq if e >= burst_end]:
                    if v <= threshold:
                        consec += 1
                        if first_epoch is None:
                            first_epoch = epoch
                        if consec >= 3:
                            decay_sec = max(0.0, (first_epoch - burst_end) / 1000.0)
                            break
                    else:
                        consec = 0
                        first_epoch = None
            run_metrics[f'{alias}_pre_cpu_median_pct'] = pre_med
            run_metrics[f'{alias}_burst_to_post_cpu_peak_pct'] = peak
            run_metrics[f'{alias}_post_cpu_median_pct'] = post_med
            run_metrics[f'{alias}_decay_to_110pct_pre_median_s'] = decay_sec if decay_sec is not None else 'not_reached'
    per_run.append(run_metrics)

summary_rows = []
for rec in per_run:
    mode = rec['mode']
    run = rec['run']
    for k, v in rec.items():
        if k in ('mode', 'run'):
            continue
        summary_rows.append(['per_run', mode, run, k, v])

agg = {}
for mode in sorted({r['mode'] for r in per_run}):
    m_runs = [r for r in per_run if r['mode'] == mode]
    if not m_runs:
        continue
    metrics = set()
    for r in m_runs:
      metrics.update(k for k in r.keys() if k not in ('mode', 'run'))
    for metric in sorted(metrics):
        vals = [r.get(metric, '') for r in m_runs]
        nums = []
        for v in vals:
            if isinstance(v, (int, float)):
                if not (isinstance(v, float) and math.isnan(v)):
                    nums.append(float(v))
            else:
                fv = fnum(v)
                if fv is not None and not math.isnan(fv):
                    nums.append(fv)
        if nums:
            agg_val = f'{statistics.median(nums):.3f}'
        else:
            if metric.endswith('_decay_to_110pct_pre_median_s'):
                not_reached = sum(1 for v in vals if str(v) == 'not_reached')
                agg_val = 'not_reached' if not_reached == len(vals) else f'mixed_not_reached_{not_reached}_of_{len(vals)}'
            elif metric == 'sync_mode':
                uniq = sorted(set(str(v) for v in vals if str(v)))
                agg_val = uniq[0] if len(uniq) == 1 else ' / '.join(uniq)
            else:
                agg_val = str(vals[0]) if vals else ''
        summary_rows.append(['aggregate', mode, '', metric, agg_val])
        agg[(mode, metric)] = agg_val

with summary.open('w', newline='') as f:
    w = csv.writer(f)
    w.writerow(['scope', 'mode', 'run', 'metric', 'value'])
    w.writerows(summary_rows)

def get(mode, metric):
    return agg.get((mode, metric), '')

def run_table_rows(mode):
    lines = []
    for r in sorted([r for r in per_run if r['mode'] == mode], key=lambda x: int(x['run'])):
        lines.append(
            f"| {mode} | {r['run']} | {r.get('burst_elapsed_ms','')} | {r.get('postburst_save_complete_took_s','')} | {r.get('postburst_rcon_save_rtt_ms','')} |"
        )
    return "\n".join(lines)

report.write_text(f"""# Dino Burst CPU Benchmark ({results.stem.split('dino-burst-cpu-')[-1]})

## Scenario

- Multirun multithreaded dino spawn burst via RCON
- Burst pattern: {burst_repeats} x {burst_concurrency} concurrent summon commands
- CPU sampling interval: {sample_interval}s
- Per-process CPU tracked for `ArkAscendedServer.exe` and `wineserver`, plus container total and tracked sum

## Files

- Raw CSV: `{results}`
- Summary CSV: `{summary}`

## Per-Run Burst / Save

| Mode | Run | Burst Elapsed (ms) | Post-burst Save Took (s) | Post-burst Save RCON RTT (ms) |
|---|---:|---:|---:|---:|
{run_table_rows('disabled')}
{run_table_rows('enabled')}

## Aggregate (Median Across Runs)

| Mode | Burst Elapsed (ms) | Post-burst Save Took (s) | Post-burst Save RCON RTT (ms) |
|---|---:|---:|---:|
| disabled | {get('disabled','burst_elapsed_ms')} | {get('disabled','postburst_save_complete_took_s')} | {get('disabled','postburst_rcon_save_rtt_ms')} |
| enabled | {get('enabled','burst_elapsed_ms')} | {get('enabled','postburst_save_complete_took_s')} | {get('enabled','postburst_rcon_save_rtt_ms')} |

## CPU Trend Metrics (Median Across Runs)

| Mode | Series | Pre CPU Median (%) | Peak CPU (burst+post) (%) | Post CPU Median (%) | Decay to 110% Pre Median |
|---|---|---:|---:|---:|---|
| disabled | container | {get('disabled','container_pre_cpu_median_pct')} | {get('disabled','container_burst_to_post_cpu_peak_pct')} | {get('disabled','container_post_cpu_median_pct')} | {get('disabled','container_decay_to_110pct_pre_median_s')} |
| disabled | arkserver | {get('disabled','arkserver_pre_cpu_median_pct')} | {get('disabled','arkserver_burst_to_post_cpu_peak_pct')} | {get('disabled','arkserver_post_cpu_median_pct')} | {get('disabled','arkserver_decay_to_110pct_pre_median_s')} |
| disabled | wineserver | {get('disabled','wineserver_pre_cpu_median_pct')} | {get('disabled','wineserver_burst_to_post_cpu_peak_pct')} | {get('disabled','wineserver_post_cpu_median_pct')} | {get('disabled','wineserver_decay_to_110pct_pre_median_s')} |
| disabled | tracked_sum | {get('disabled','tracked_sum_pre_cpu_median_pct')} | {get('disabled','tracked_sum_burst_to_post_cpu_peak_pct')} | {get('disabled','tracked_sum_post_cpu_median_pct')} | {get('disabled','tracked_sum_decay_to_110pct_pre_median_s')} |
| enabled | container | {get('enabled','container_pre_cpu_median_pct')} | {get('enabled','container_burst_to_post_cpu_peak_pct')} | {get('enabled','container_post_cpu_median_pct')} | {get('enabled','container_decay_to_110pct_pre_median_s')} |
| enabled | arkserver | {get('enabled','arkserver_pre_cpu_median_pct')} | {get('enabled','arkserver_burst_to_post_cpu_peak_pct')} | {get('enabled','arkserver_post_cpu_median_pct')} | {get('enabled','arkserver_decay_to_110pct_pre_median_s')} |
| enabled | wineserver | {get('enabled','wineserver_pre_cpu_median_pct')} | {get('enabled','wineserver_burst_to_post_cpu_peak_pct')} | {get('enabled','wineserver_post_cpu_median_pct')} | {get('enabled','wineserver_decay_to_110pct_pre_median_s')} |
| enabled | tracked_sum | {get('enabled','tracked_sum_pre_cpu_median_pct')} | {get('enabled','tracked_sum_burst_to_post_cpu_peak_pct')} | {get('enabled','tracked_sum_post_cpu_median_pct')} | {get('enabled','tracked_sum_decay_to_110pct_pre_median_s')} |
""")
print(summary)
print(report)
PY
}

# Alternate runs to reduce drift effects.
for run_no in $(seq 1 "$RUNS_PER_MODE"); do
  run_burst_mode disabled "$run_no"
  run_burst_mode enabled "$run_no"
done

if [ "$RUNS_PER_MODE" -eq 1 ]; then
  compute_summary
else
  compute_multi_run_summary
fi

log "COMPLETE results_csv=$results_csv summary_csv=$summary_csv report_md=$report_md"
echo "RESULTS_CSV=$results_csv"
echo "SUMMARY_CSV=$summary_csv"
echo "REPORT_MD=$report_md"
