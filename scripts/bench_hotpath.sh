#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.benchmarks}"
BENCH_TIME="${BENCH_TIME:-2s}"
BENCH_COUNT="${BENCH_COUNT:-3}"
BENCH_REGEX="${BENCH_REGEX:-BenchmarkHotPath|BenchmarkRoutingClassifier_Classify}"

mkdir -p "$OUT_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_FILE="$OUT_DIR/hotpath-${STAMP}.txt"

cd "$ROOT_DIR"

echo "[bench] running hotpath benchmarks..."
echo "[bench] regex: $BENCH_REGEX"
echo "[bench] benchtime: $BENCH_TIME, count: $BENCH_COUNT"
echo "[bench] output: $OUT_FILE"

go test ./internal/service \
  -run '^$' \
  -bench "$BENCH_REGEX" \
  -benchmem \
  -benchtime "$BENCH_TIME" \
  -count "$BENCH_COUNT" | tee "$OUT_FILE"

echo
echo "[bench] summary (hotpath lines):"
grep -E '^Benchmark(HotPath|RoutingClassifier_Classify)' "$OUT_FILE" || true
echo "[bench] done"
