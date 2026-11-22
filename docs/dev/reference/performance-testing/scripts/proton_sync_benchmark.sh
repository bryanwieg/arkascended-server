#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$ROOT_DIR"
# Temp files and output go to the project temp dir (gitignored), like the CLI.
tmp_dir=${ARK_TMP_DIR:-.ark-tmp}
mkdir -p "$tmp_dir"

pairs=${1:-10}
cpu_minutes=${2:-5}
container_name=${3:-ark-server-1}
compose_base=${4:-docker-compose.dev.yaml}
compose_service=${5:-asa-server-1}

stamp=$(date -u +%Y%m%dT%H%M%SZ)
out_dir="$tmp_dir/benchmarks"
results_csv="$out_dir/proton-sync-benchmark-$stamp.csv"
summary_csv="$out_dir/proton-sync-benchmark-summary-$stamp.csv"
log_txt="$out_dir/proton-sync-benchmark-$stamp.log"

mkdir -p "$out_dir"
: > "$log_txt"

need_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }
need_cmd docker
need_cmd rg

over_disabled=$(mktemp "$tmp_dir"/compose-sync-disabled.XXXXXX.yaml)
over_enabled=$(mktemp "$tmp_dir"/compose-sync-enabled.XXXXXX.yaml)
compose_up_log=$(mktemp "$tmp_dir"/proton_sync_compose_up.XXXXXX.log)
cleanup() {
  rm -f "$over_disabled" "$over_enabled" "$compose_up_log"
}
trap cleanup EXIT

cat > "$over_disabled" <<'YAML'
services:
  __SERVICE_NAME__:
    environment:
      PROTON_NO_ESYNC: "1"
      PROTON_NO_FSYNC: "1"
      WINEESYNC: "0"
      WINEFSYNC: "0"
YAML
cat > "$over_enabled" <<'YAML'
services:
  __SERVICE_NAME__:
    environment:
      PROTON_NO_ESYNC: "0"
      PROTON_NO_FSYNC: "0"
      WINEESYNC: "1"
      WINEFSYNC: "1"
YAML
sed -i "s/__SERVICE_NAME__/$compose_service/" "$over_disabled" "$over_enabled"

printf 'timestamp_utc,category,mode,run,metric,value,unit,detail\n' > "$results_csv"

ts_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts_utc)] $*" | tee -a "$log_txt"; }
append_row() {
  local t=$1 c=$2 m=$3 r=$4 metric=$5 value=$6 unit=$7 detail=$8
  printf '%s,%s,%s,%s,%s,%s,%s,%s\n' "$t" "$c" "$m" "$r" "$metric" "$value" "$unit" "${detail//,/;}" >> "$results_csv"
}

compose_up_mode() {
  local mode=$1
  local over=$2
  docker compose -f "$compose_base" -f "$over" up -d --force-recreate "$compose_service" >"$compose_up_log" 2>&1
}

wait_for_advertising() {
  local mode=$1 run_no=$2 max_polls=${3:-180}
  local i logs end marker sync_line
  for i in $(seq 1 "$max_polls"); do
    logs=$(docker logs --tail 900 "$container_name" 2>&1 || true)
    if echo "$logs" | rg -q 'Server has completed startup and is now advertising for join\.'; then
      end=$(date +%s)
      marker=$(echo "$logs" | rg -n 'Server has completed startup and is now advertising for join\.' | tail -n 1)
      sync_line=$(echo "$logs" | rg -m1 'fsync: up and running\.|esync|wineserver: using server-side synchronization\.' || true)
      printf '%s\n%s\n%s\n%s\n' "$end" "$i" "$marker" "$sync_line"
      return 0
    fi
    sleep 5
  done
  return 1
}

run_startup_case() {
  local mode=$1 over=$2 run_no=$3
  local start end polls elapsed marker sync_line
  start=$(date +%s)
  log "START startup mode=$mode run=$run_no"
  compose_up_mode "$mode" "$over"
  mapfile -t ready < <(wait_for_advertising "$mode" "$run_no") || {
    log "FAIL startup mode=$mode run=$run_no timeout"
    append_row "$(ts_utc)" startup "$mode" "$run_no" status - failure timeout
    docker logs --tail 200 "$container_name" 2>&1 | tail -n 80 | sed 's/^/LOG: /' | tee -a "$log_txt" >/dev/null || true
    return 1
  }
  end=${ready[0]}
  polls=${ready[1]}
  marker=${ready[2]}
  sync_line=${ready[3]:-}
  elapsed=$((end - start))
  append_row "$(ts_utc)" startup "$mode" "$run_no" time_to_advertise "$elapsed" s "$marker"
  append_row "$(ts_utc)" startup "$mode" "$run_no" polls "$polls" count "$marker"
  append_row "$(ts_utc)" startup "$mode" "$run_no" sync_mode 0 text "${sync_line:-none}"
  log "DONE startup mode=$mode run=$run_no elapsed=${elapsed}s polls=$polls sync='${sync_line:-none}'"
}

cpu_samples_for_mode() {
  local mode=$1 over=$2 run_no=$3 minutes=$4
  local start end polls elapsed marker sync_line sample_count interval_sec samples target_samples
  interval_sec=5
  target_samples=$(( (minutes * 60) / interval_sec ))
  if [ "$target_samples" -lt 1 ]; then target_samples=1; fi

  log "START cpu mode=$mode run=$run_no minutes=$minutes"
  start=$(date +%s)
  compose_up_mode "$mode" "$over"
  mapfile -t ready < <(wait_for_advertising "$mode" "$run_no") || {
    log "FAIL cpu mode=$mode run=$run_no startup_timeout"
    append_row "$(ts_utc)" cpu "$mode" "$run_no" status - failure startup_timeout
    return 1
  }
  end=${ready[0]}
  polls=${ready[1]}
  marker=${ready[2]}
  sync_line=${ready[3]:-}
  elapsed=$((end - start))
  append_row "$(ts_utc)" cpu "$mode" "$run_no" time_to_advertise "$elapsed" s "$marker"
  append_row "$(ts_utc)" cpu "$mode" "$run_no" sync_mode 0 text "${sync_line:-none}"
  log "CPU benchmark ready mode=$mode run=$run_no elapsed=${elapsed}s sync='${sync_line:-none}'"

  samples=()
  for sample_count in $(seq 1 "$target_samples"); do
    local raw pct
    raw=$(docker stats --no-stream --format '{{.CPUPerc}}' "$container_name" 2>/dev/null | head -n 1 || true)
    pct=$(echo "$raw" | tr -d '%' | tr -d ' ')
    if [[ -z "$pct" || ! "$pct" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
      pct="nan"
    fi
    append_row "$(ts_utc)" cpu_sample "$mode" "$run_no" cpu_percent "$pct" percent "sample=$sample_count"
    samples+=("$pct")
    if (( sample_count % 12 == 0 )); then
      log "CPU progress mode=$mode run=$run_no samples=$sample_count/$target_samples"
    fi
    if [ "$sample_count" -lt "$target_samples" ]; then
      sleep "$interval_sec"
    fi
  done

  # summarize numeric samples
  local tmpf
  tmpf=$(mktemp -p "$tmp_dir")
  printf '%s\n' "${samples[@]}" | rg '^[0-9]+(\.[0-9]+)?$' > "$tmpf" || true
  local count min max avg median
  count=$(wc -l < "$tmpf" | tr -d ' ')
  if [ "$count" -gt 0 ]; then
    min=$(sort -n "$tmpf" | head -n 1)
    max=$(sort -n "$tmpf" | tail -n 1)
    avg=$(awk '{s+=$1} END {if (NR) printf "%.3f", s/NR; else print "nan"}' "$tmpf")
    median=$(sort -n "$tmpf" | awk '{a[NR]=$1} END { if (NR==0) {print "nan"} else if (NR%2==1) {printf "%.3f", a[(NR+1)/2]} else {printf "%.3f", (a[NR/2]+a[NR/2+1])/2} }')
  else
    min=nan; max=nan; avg=nan; median=nan
  fi
  rm -f "$tmpf"

  append_row "$(ts_utc)" cpu_summary "$mode" "$run_no" sample_count "$count" count "${minutes}m@${interval_sec}s"
  append_row "$(ts_utc)" cpu_summary "$mode" "$run_no" cpu_percent_avg "$avg" percent "${minutes}m"
  append_row "$(ts_utc)" cpu_summary "$mode" "$run_no" cpu_percent_median "$median" percent "${minutes}m"
  append_row "$(ts_utc)" cpu_summary "$mode" "$run_no" cpu_percent_min "$min" percent "${minutes}m"
  append_row "$(ts_utc)" cpu_summary "$mode" "$run_no" cpu_percent_max "$max" percent "${minutes}m"
  log "DONE cpu mode=$mode run=$run_no avg=$avg median=$median min=$min max=$max count=$count"
}

# 1) Larger startup sample (alternating runs)
for run_no in $(seq 1 "$pairs"); do
  run_startup_case disabled "$over_disabled" "$run_no"
  run_startup_case enabled "$over_enabled" "$run_no"
done

# 2) 5-minute CPU sampling per mode (runtime overrides only)
cpu_samples_for_mode disabled "$over_disabled" 1 "$cpu_minutes"
cpu_samples_for_mode enabled "$over_enabled" 1 "$cpu_minutes"

# 3) Build compact summary CSV from accumulated results.
{
  echo 'section,mode,metric,value'
  for mode in disabled enabled; do
    vals=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {print $6}' "$results_csv" | tr '\n' ' ')
    if [ -n "$vals" ]; then
      count=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {n++} END{print n+0}' "$results_csv")
      min=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {print $6}' "$results_csv" | sort -n | head -n1)
      max=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {print $6}' "$results_csv" | sort -n | tail -n1)
      avg=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {s+=$6;n++} END{if(n) printf "%.3f", s/n; else print "nan"}' "$results_csv")
      median=$(awk -F, -v m="$mode" '$2=="startup" && $3==m && $5=="time_to_advertise" {print $6}' "$results_csv" | sort -n | awk '{a[NR]=$1} END { if (NR==0) print "nan"; else if (NR%2) printf "%.3f", a[(NR+1)/2]; else printf "%.3f", (a[NR/2]+a[NR/2+1])/2 }')
      echo "startup,$mode,count,$count"
      echo "startup,$mode,min_s,$min"
      echo "startup,$mode,median_s,$median"
      echo "startup,$mode,avg_s,$avg"
      echo "startup,$mode,max_s,$max"
    fi
    for metric in cpu_percent_avg cpu_percent_median cpu_percent_min cpu_percent_max sample_count time_to_advertise; do
      val=$(awk -F, -v m="$mode" -v metric="$metric" '$2=="cpu_summary" && $3==m && $5==metric {print $6; exit}' "$results_csv")
      [ -n "$val" ] && echo "cpu,$mode,$metric,$val"
    done
  done
} > "$summary_csv"

log "COMPLETE results_csv=$results_csv summary_csv=$summary_csv"
echo "RESULTS_CSV=$results_csv"
echo "SUMMARY_CSV=$summary_csv"
