#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$ROOT_DIR"
# Temp files and output go to the project temp dir (gitignored), like the CLI.
tmp_dir=${ARK_TMP_DIR:-.ark-tmp}
mkdir -p "$tmp_dir"

need_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }
need_cmd docker
need_cmd rg

CONTAINER=${1:-ark-server-1}
SPAWN_COUNT=${2:-120}
SAVE_RUNS=${3:-5}
RCON_PASS=${4:-MyPassword}
RCON_HOST=${5:-localhost:27020}
DINO_CMD=${6:-"cheat Summon Dodo_Character_BP_C"}

stamp=$(date -u +%Y%m%dT%H%M%SZ)
out_dir="$tmp_dir/benchmarks"
results_csv="$out_dir/dino-save-poc-$stamp.csv"
log_file="$out_dir/dino-save-poc-$stamp.log"
report_md="$out_dir/dino-save-poc-$stamp-report.md"

mkdir -p "$out_dir"
printf 'timestamp_utc,phase,run,metric,value,unit,detail\n' > "$results_csv"
: > "$log_file"

ts_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts_utc)] $*" | tee -a "$log_file"; }
append_row() {
  printf '%s,%s,%s,%s,%s,%s,%s\n' "$1" "$2" "$3" "$4" "$5" "$6" "${7//,/;}" >> "$results_csv"
}
rcon() {
  # The password goes through the environment so it stays off every command line.
  SERVER_ADMIN_PASSWORD="$RCON_PASS" docker exec -e SERVER_ADMIN_PASSWORD -e RCON_HOST="${RCON_HOST%:*}" \
    -e RCON_PORT="${RCON_HOST##*:}" "$CONTAINER" /usr/bin/python3.14 /home/ark-user/scripts/runtime/ark-rcon.py "$@"
}
log_path() {
  echo "/home/ark-user/ark-server/ShooterGame/Saved/Logs/ShooterGame.log"
}
count_save_complete() {
  docker exec "$CONTAINER" sh -lc "grep -c 'World Save Complete\\. Took:' $(log_path) 2>/dev/null || true" | tail -n1 | tr -d '\r' | tr -d ' '
}
last_save_complete_line() {
  docker exec "$CONTAINER" sh -lc "grep 'World Save Complete\\. Took:' $(log_path) | tail -n 1" | tr -d '\r'
}
last_save_start_line() {
  docker exec "$CONTAINER" sh -lc "grep 'Saving world\.\.\.' $(log_path) | tail -n 1" | tr -d '\r'
}
wait_server_ready() {
  for i in $(seq 1 120); do
    if docker logs --tail 300 "$CONTAINER" 2>&1 | rg -q 'Server has completed startup and is now advertising for join\.'; then
      return 0
    fi
    sleep 5
  done
  return 1
}
parse_save_took() {
  sed -n 's/.*World Save Complete\. Took: \([0-9.]*\).*/\1/p'
}
get_cpu_once() {
  local raw pct
  raw=$(docker stats --no-stream --format '{{.CPUPerc}}' "$CONTAINER" 2>/dev/null | head -n1 || true)
  pct=$(echo "$raw" | tr -d '%' | tr -d ' ')
  if [[ "$pct" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then echo "$pct"; else echo nan; fi
}
cpu_window() {
  local phase=$1 samples=${2:-12} interval=${3:-5}
  local vals=() i pct tmpf count avg median min max
  for i in $(seq 1 "$samples"); do
    pct=$(get_cpu_once)
    vals+=("$pct")
    append_row "$(ts_utc)" "$phase" 0 cpu_sample "$pct" percent "sample=$i/$samples"
    [ "$i" -lt "$samples" ] && sleep "$interval"
  done
  tmpf=$(mktemp -p "$tmp_dir")
  printf '%s\n' "${vals[@]}" | rg '^[0-9]+(\.[0-9]+)?$' > "$tmpf" || true
  count=$(wc -l < "$tmpf" | tr -d ' ')
  if [ "$count" -gt 0 ]; then
    avg=$(awk '{s+=$1} END {printf "%.3f", s/NR}' "$tmpf")
    median=$(sort -n "$tmpf" | awk '{a[NR]=$1} END { if (NR%2) printf "%.3f", a[(NR+1)/2]; else printf "%.3f", (a[NR/2]+a[NR/2+1])/2 }')
    min=$(sort -n "$tmpf" | head -n1)
    max=$(sort -n "$tmpf" | tail -n1)
  else
    avg=nan; median=nan; min=nan; max=nan
  fi
  rm -f "$tmpf"
  append_row "$(ts_utc)" "$phase" 0 cpu_avg "$avg" percent "samples=$samples"
  append_row "$(ts_utc)" "$phase" 0 cpu_median "$median" percent "samples=$samples"
  append_row "$(ts_utc)" "$phase" 0 cpu_min "$min" percent "samples=$samples"
  append_row "$(ts_utc)" "$phase" 0 cpu_max "$max" percent "samples=$samples"
  echo "$avg,$median,$min,$max,$count"
}
run_save_measure() {
  local phase=$1 run_no=$2 timeout_polls=60
  local before after i rcon_start rcon_end rtt save_line save_took start_line
  before=$(count_save_complete)
  rcon_start=$(date +%s%3N)
  local rcon_out
  rcon_out=$(rcon saveworld 2>&1 || true)
  rcon_end=$(date +%s%3N)
  rtt=$((rcon_end-rcon_start))
  append_row "$(ts_utc)" "$phase" "$run_no" rcon_save_rtt_ms "$rtt" ms "$rcon_out"
  for i in $(seq 1 "$timeout_polls"); do
    after=$(count_save_complete)
    if [ "${after:-0}" -gt "${before:-0}" ]; then
      save_line=$(last_save_complete_line)
      start_line=$(last_save_start_line)
      save_took=$(echo "$save_line" | parse_save_took)
      append_row "$(ts_utc)" "$phase" "$run_no" save_complete_took "$save_took" s "$save_line"
      log "save phase=$phase run=$run_no rtt_ms=$rtt took_s=$save_took"
      return 0
    fi
    sleep 1
  done
  log "save phase=$phase run=$run_no timeout waiting for save complete"
  append_row "$(ts_utc)" "$phase" "$run_no" save_complete_took - timeout "no marker"
  return 1
}

spawn_dinos() {
  local count=$1 success=0 fail=0 i out ack
  log "spawning count=$count cmd=$DINO_CMD"
  for i in $(seq 1 "$count"); do
    out=$(rcon "$DINO_CMD" 2>&1 || true)
    if echo "$out" | rg -q 'Server received'; then
      success=$((success+1))
    else
      fail=$((fail+1))
    fi
    if echo "$out" | rg -q 'Server received'; then
      ack=1
    else
      ack=0
    fi
    append_row "$(ts_utc)" spawn "$i" summon_ack "$ack" count "$out"
    if (( i % 20 == 0 )); then
      log "spawn progress $i/$count success=$success fail=$fail"
    fi
    sleep 0.2
  done
  append_row "$(ts_utc)" spawn 0 summon_success "$success" count "requested=$count"
  append_row "$(ts_utc)" spawn 0 summon_fail "$fail" count "requested=$count"
  log "spawn complete success=$success fail=$fail"
}

summary_metric() {
  local phase=$1 metric=$2 agg=$3
  awk -F, -v p="$phase" -v m="$metric" '$2==p && $4==m && $5 != "-" {print $5}' "$results_csv" | \
    case "$agg" in
      min) sort -n | head -n1 ;;
      max) sort -n | tail -n1 ;;
      avg) awk '{s+=$1;n++} END{if(n) printf "%.3f", s/n; else print "nan"}' ;;
      median) sort -n | awk '{a[NR]=$1} END{ if(NR==0) print "nan"; else if(NR%2) printf "%.3f", a[(NR+1)/2]; else printf "%.3f", (a[NR/2]+a[NR/2+1])/2 }' ;;
      count) awk 'END{print NR+0}' ;;
    esac
}

if ! docker ps --format '{{.Names}}' | rg -qx "$CONTAINER"; then
  log "container $CONTAINER not running"
  exit 1
fi

if ! wait_server_ready; then
  log "server not ready"
  exit 1
fi
log "server ready; starting dino/save PoC"
append_row "$(ts_utc)" setup 0 container 0 text "$CONTAINER"
append_row "$(ts_utc)" setup 0 spawn_count "$SPAWN_COUNT" count "$DINO_CMD"
append_row "$(ts_utc)" setup 0 save_runs "$SAVE_RUNS" count ""

# Baseline CPU (1 minute) and saves
cpu_window baseline_cpu 12 5 >/dev/null
for run_no in $(seq 1 "$SAVE_RUNS"); do
  run_save_measure baseline_save "$run_no"
  sleep 2
done

# Spawn load, let server settle briefly, then post-load CPU + saves
spawn_dinos "$SPAWN_COUNT"
log "settling 20s after spawn load"
sleep 20
cpu_window post_spawn_cpu 12 5 >/dev/null
for run_no in $(seq 1 "$SAVE_RUNS"); do
  run_save_measure post_spawn_save "$run_no"
  sleep 2
done

# Optional cleanup command (best effort)
cleanup_out=$(rcon 'cheat DestroyWildDinos' 2>&1 || true)
append_row "$(ts_utc)" cleanup 0 destroy_wild_dinos 0 text "$cleanup_out"
log "cleanup issued: $cleanup_out"

# Generate summary and report
b_count=$(summary_metric baseline_save save_complete_took count)
b_min=$(summary_metric baseline_save save_complete_took min)
b_med=$(summary_metric baseline_save save_complete_took median)
b_avg=$(summary_metric baseline_save save_complete_took avg)
b_max=$(summary_metric baseline_save save_complete_took max)
p_count=$(summary_metric post_spawn_save save_complete_took count)
p_min=$(summary_metric post_spawn_save save_complete_took min)
p_med=$(summary_metric post_spawn_save save_complete_took median)
p_avg=$(summary_metric post_spawn_save save_complete_took avg)
p_max=$(summary_metric post_spawn_save save_complete_took max)

b_cpu_avg=$(awk -F, '$2=="baseline_cpu" && $4=="cpu_avg" {print $5; exit}' "$results_csv")
b_cpu_med=$(awk -F, '$2=="baseline_cpu" && $4=="cpu_median" {print $5; exit}' "$results_csv")
b_cpu_max=$(awk -F, '$2=="baseline_cpu" && $4=="cpu_max" {print $5; exit}' "$results_csv")
p_cpu_avg=$(awk -F, '$2=="post_spawn_cpu" && $4=="cpu_avg" {print $5; exit}' "$results_csv")
p_cpu_med=$(awk -F, '$2=="post_spawn_cpu" && $4=="cpu_median" {print $5; exit}' "$results_csv")
p_cpu_max=$(awk -F, '$2=="post_spawn_cpu" && $4=="cpu_max" {print $5; exit}' "$results_csv")
spawn_success=$(awk -F, '$2=="spawn" && $4=="summon_success" {print $5; exit}' "$results_csv")
spawn_fail=$(awk -F, '$2=="spawn" && $4=="summon_fail" {print $5; exit}' "$results_csv")

cat > "$report_md" <<REPORT
# Dino Save PoC Benchmark ($stamp)

## Goal

Test whether a controlled burst of RCON dino spawn commands causes a meaningful change in:
- save duration (\`World Save Complete. Took:\` from server log)
- short idle runtime CPU usage (1-minute window before and after spawn burst)

## Configuration

- Container: \`$CONTAINER\`
- Spawn command: \`$DINO_CMD\`
- Requested spawn count: $SPAWN_COUNT
- Save runs per phase: $SAVE_RUNS
- RCON target: $RCON_HOST
- Spawn acknowledgements: success=$spawn_success fail=$spawn_fail

## Results Summary

| Phase | Metric | Min | Median | Avg | Max | Count |
|---|---|---:|---:|---:|---:|---:|
| Baseline | Save complete took (s) | $b_min | $b_med | $b_avg | $b_max | $b_count |
| Post-spawn | Save complete took (s) | $p_min | $p_med | $p_avg | $p_max | $p_count |

| Phase | CPU Avg (%) | CPU Median (%) | CPU Max (%) | Samples |
|---|---:|---:|---:|---:|
| Baseline (1 min) | $b_cpu_avg | $b_cpu_med | $b_cpu_max | 12 |
| Post-spawn (1 min) | $p_cpu_avg | $p_cpu_med | $p_cpu_max | 12 |

## Raw Files

- CSV: \`$results_csv\`
- Log: \`$log_file\`

## Notes

- Spawn success here means the RCON response indicated the server accepted the command (e.g., \`Server received, But no response!!\`).
- This PoC does not directly count spawned dinos; it infers impact from save duration and CPU changes.
- \`cheat DestroyWildDinos\` was issued at the end as best-effort cleanup.
REPORT

log "COMPLETE csv=$results_csv report=$report_md"
echo "RESULTS_CSV=$results_csv"
echo "REPORT_MD=$report_md"
