#!/usr/bin/env bash
# Сравнение двух отчётов k6 (T-015, NFR-001): bash scripts/perf-compare.sh old.json new.json
# Печатает p95/p99 http_req_duration и разницу в процентах. Рост p99 больше PERF_MAX_P99_GROWTH
# (по умолчанию 20 %) — регрессия, exit 1. Ошибка использования или нечитаемый отчёт — exit 2.
# Читает оба формата: `k6 run --summary-export` (p(99) прямо в метрике) и handleSummary (metrics.*.values).
set -euo pipefail

max_growth="${PERF_MAX_P99_GROWTH:-20}"

if [ "$#" -ne 2 ]; then
  echo "Использование: bash scripts/perf-compare.sh old.json new.json" >&2
  exit 2
fi
command -v jq >/dev/null || { echo "perf-compare: нужен jq" >&2; exit 2; }

# stat <file> <p(95)|p(99)> — значение в миллисекундах; пусто, если метрики нет.
stat() {
  jq -r --arg s "$2" '.metrics.http_req_duration | (.values // .) | .[$s] // empty' "$1" 2>/dev/null
}

read_report() { # file → p95 p99
  local f=$1 p95 p99
  if [ ! -r "$f" ]; then echo "perf-compare: нет файла $f" >&2; exit 2; fi
  if ! jq -e . "$f" >/dev/null 2>&1; then echo "perf-compare: $f — не JSON" >&2; exit 2; fi
  p95=$(stat "$f" 'p(95)')
  p99=$(stat "$f" 'p(99)')
  if [ -z "$p95" ] || [ -z "$p99" ]; then
    echo "perf-compare: в $f нет p(95)/p(99) http_req_duration (k6 запускать с --summary-trend-stats)" >&2
    exit 2
  fi
  echo "$p95 $p99"
}

# Подстановка $(...) — подоболочка: её exit 2 пробрасываем явно.
old=$(read_report "$1") || exit $?
new=$(read_report "$2") || exit $?
read -r old95 old99 <<<"$old"
read -r new95 new99 <<<"$new"

awk -v o95="$old95" -v n95="$new95" -v o99="$old99" -v n99="$new99" -v max="$max_growth" '
function line(name, o, n) {
  printf "%s: %.2f ms → %.2f ms (%+.1f%%)\n", name, o, n, (n - o) / o * 100
}
BEGIN {
  if (o95 <= 0 || o99 <= 0) { print "perf-compare: нулевой p95/p99 в старом отчёте" > "/dev/stderr"; exit 2 }
  line("p95", o95, n95)
  line("p99", o99, n99)
  growth = (n99 - o99) / o99 * 100
  # Сравнение с округлением до 0,1 п.п.: ровно +20 % при плавающей арифметике не должно стать регрессией.
  if (sprintf("%.1f", growth) + 0 > max + 0) {
    printf "регрессия: p99 вырос на %.1f%% (> %s%%)\n", growth, max
    exit 1
  }
  printf "ok: рост p99 не больше %s%%\n", max
}'
