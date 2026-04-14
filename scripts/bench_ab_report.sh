#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.benchmarks}"
BENCH_TIME="${BENCH_TIME:-1s}"
BENCH_COUNT="${BENCH_COUNT:-6}"
BENCH_CPU="${BENCH_CPU:-}"

mkdir -p "$OUT_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"

OLD_SHADOW_RAW="$OUT_DIR/ab-shadow-old-${STAMP}.txt"
NEW_SHADOW_RAW="$OUT_DIR/ab-shadow-new-${STAMP}.txt"
OLD_API_RAW="$OUT_DIR/ab-api-old-${STAMP}.txt"
NEW_API_RAW="$OUT_DIR/ab-api-new-${STAMP}.txt"
OLD_NORM="$OUT_DIR/ab-old-normalized-${STAMP}.txt"
NEW_NORM="$OUT_DIR/ab-new-normalized-${STAMP}.txt"
REPORT_FILE="$OUT_DIR/ab-report-${STAMP}.txt"

cd "$ROOT_DIR"

run_bench() {
  local bench_regex="$1"
  local out_file="$2"
  if [[ -n "$BENCH_CPU" ]]; then
    go test ./internal/service \
      -run '^$' \
      -bench "$bench_regex" \
      -benchmem \
      -benchtime "$BENCH_TIME" \
      -count "$BENCH_COUNT" \
      -cpu "$BENCH_CPU" | tee "$out_file"
    return
  fi

  go test ./internal/service \
    -run '^$' \
    -bench "$bench_regex" \
    -benchmem \
    -benchtime "$BENCH_TIME" \
    -count "$BENCH_COUNT" | tee "$out_file"
}

echo "[ab] running old-path approximation benchmarks..."
run_bench 'BenchmarkHotPathEndpointSelector_DirectModel_(NoShadow|WithShadowSyncApprox)$' "$OLD_SHADOW_RAW"

echo "[ab] running new-path benchmarks..."
run_bench 'BenchmarkHotPathEndpointSelector_DirectModel_(NoShadow|WithShadowAsync)$' "$NEW_SHADOW_RAW"

echo "[ab] running API detect benchmark (old=no cache hit)..."
run_bench 'BenchmarkHotPathAPIDetectCache/cache_miss_unique_model$' "$OLD_API_RAW"

echo "[ab] running API detect benchmark (new=cache hit)..."
run_bench 'BenchmarkHotPathAPIDetectCache/cache_hit$' "$NEW_API_RAW"

{
  grep -E '^(goos:|goarch:|pkg:|cpu:)' "$OLD_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathEndpointSelector_DirectModel_WithShadowSyncApprox(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathEndpointSelector_DirectModel_Shadow\1\2/p' "$OLD_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathEndpointSelector_DirectModel_NoShadow(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathEndpointSelector_DirectModel_NoShadow\1\2/p' "$OLD_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathAPIDetectCache\/cache_miss_unique_model(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathAPIDetectCache\/lookup\1\2/p' "$OLD_API_RAW"
} > "$OLD_NORM"

{
  grep -E '^(goos:|goarch:|pkg:|cpu:)' "$NEW_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathEndpointSelector_DirectModel_WithShadowAsync(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathEndpointSelector_DirectModel_Shadow\1\2/p' "$NEW_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathEndpointSelector_DirectModel_NoShadow(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathEndpointSelector_DirectModel_NoShadow\1\2/p' "$NEW_SHADOW_RAW"
  sed -nE 's/^BenchmarkHotPathAPIDetectCache\/cache_hit(-[0-9]+)?([[:space:]]+)/BenchmarkHotPathAPIDetectCache\/lookup\1\2/p' "$NEW_API_RAW"
} > "$NEW_NORM"

echo "[ab] generating benchstat report..."
{
  echo "=== A/B Inputs ==="
  echo "OLD (approx): $OLD_NORM"
  echo "NEW (current): $NEW_NORM"
  echo
  echo "=== benchstat ==="
  go run golang.org/x/perf/cmd/benchstat@latest "$OLD_NORM" "$NEW_NORM"
} | tee "$REPORT_FILE"

echo
echo "[ab] report saved to: $REPORT_FILE"
