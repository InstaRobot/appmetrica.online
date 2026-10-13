#!/usr/bin/env bash
# Тесты CI-скриптов: scripts/coverage-gate.sh и scripts/check-tdd-order.sh (T-008), scripts/perf-compare.sh (T-015).
# Запуск из любой директории: bash scripts/test-ci-gates.sh (нужны только bash, awk, git и jq).
# Совместим с bash 3.2 (macOS) — без ассоциативных массивов.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GATE="$ROOT/scripts/coverage-gate.sh"
TDD="$ROOT/scripts/check-tdd-order.sh"
PERF="$ROOT/scripts/perf-compare.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

failures=0
total=0
pass() { total=$((total + 1)); printf '  ok    %s\n' "$1"; }
fail() { total=$((total + 1)); failures=$((failures + 1)); printf '  FAIL  %s\n' "$1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/        | /'; }

# run <cmd...> — выполняет команду, сохраняет вывод в $out и код выхода в $code.
run() { out="$("$@" 2>&1)"; code=$?; }

expect_code() { # name want
  if [ "$code" -eq "$2" ]; then pass "$1"; else fail "$1 (exit $code, ожидался $2)" "$out"; fi
}
expect_out() { # name fixed-string
  if grep -qF -- "$2" <<<"$out"; then pass "$1"; else fail "$1 (нет строки «$2»)" "$out"; fi
}
expect_no_out() { # name fixed-string
  if grep -qF -- "$2" <<<"$out"; then fail "$1 (лишняя строка «$2»)" "$out"; else pass "$1"; fi
}

M=appmetrica.online/backend/internal

echo "coverage-gate.sh"

run bash "$GATE" "$ROOT/scripts/testdata/coverage-79.out"
expect_code "AC-008.1: internal/ingest 79 % → exit 1" 1
expect_out "AC-008.1: строка «internal/ingest 79.0% < 80%»" "internal/ingest 79.0% < 80%"
expect_out "остальные пакеты проходят" "internal/reports 85.0% >= 80%"

run bash "$GATE" "$ROOT/scripts/testdata/coverage-85.out"
expect_code "AC-008.1: coverage-85.out → exit 0" 0
for p in ingest reports auth identity deletion; do
  expect_out "coverage-85.out: internal/$p 85.0%" "internal/$p 85.0% >= 80%"
done

run bash "$GATE" "$TMP/missing.out"
expect_code "нет файла профиля → exit 2" 2

run bash "$GATE"
expect_code "без аргумента → exit 2" 2

printf 'garbage\n' >"$TMP/bad.out"
run bash "$GATE" "$TMP/bad.out"
expect_code "файл без строки mode: → exit 2" 2

# Один и тот же блок встречается в профиле несколько раз (так пишет -coverpkg для каждого тест-бинарника):
# блок покрыт, если хотя бы одна запись имеет count > 0; statements не суммируются повторно.
cat >"$TMP/dup.out" <<EOF
mode: atomic
$M/ingest/handler.go:10.2,12.3 8 0
$M/ingest/handler.go:10.2,12.3 8 4
$M/ingest/handler.go:14.2,16.3 2 0
$M/ingest/handler.go:14.2,16.3 2 0
EOF
run env COVER_PKGS=ingest bash "$GATE" "$TMP/dup.out"
expect_code "дубликаты блоков: 8 из 10 → exit 0" 0
expect_out "дубликаты блоков дедуплицируются" "internal/ingest 80.0% >= 80%"

# Подпакеты агрегируются в пакет верхнего уровня; internal/ingestx — не подпакет internal/ingest.
cat >"$TMP/sub.out" <<EOF
mode: set
$M/ingest/handler.go:1.1,2.2 5 1
$M/ingest/decode/gzip.go:1.1,2.2 5 0
$M/ingestx/other.go:1.1,2.2 90 1
EOF
run env COVER_PKGS=ingest bash "$GATE" "$TMP/sub.out"
expect_code "подпакеты: 5 из 10 → exit 1" 1
expect_out "подпакеты учитываются, соседний префикс — нет" "internal/ingest 50.0% < 80%"

# Пакета ещё нет в профиле (тикет не реализован) — пропуск, не провал.
run env COVER_PKGS="ingest deletion" bash "$GATE" "$TMP/sub.out"
expect_out "отсутствующий пакет пропускается" "internal/deletion: нет в профиле, пропуск"

run env COVER_PKGS=ingest COVER_MIN=50 bash "$GATE" "$TMP/sub.out"
expect_code "COVER_MIN=50: 50 % → exit 0" 0
expect_out "COVER_MIN подставляется в вывод" "internal/ingest 50.0% >= 50%"

# 79.96 % не округляется до 80 % и не проходит.
cat >"$TMP/edge.out" <<EOF
mode: set
$M/auth/a.go:1.1,2.2 7996 1
$M/auth/a.go:3.1,4.2 2004 0
EOF
run env COVER_PKGS=auth bash "$GATE" "$TMP/edge.out"
expect_code "79.96 % → exit 1" 1
expect_out "79.96 % печатается без округления вверх" "internal/auth 79.9% < 80%"

echo "check-tdd-order.sh"

# new_repo — пустой репозиторий с коммитом на main; дальше работаем в ветке feature.
new_repo() {
  repo="$TMP/repo-$1"
  mkdir -p "$repo"
  git -C "$repo" init -q -b main
  git -C "$repo" config user.name test
  git -C "$repo" config user.email test@example.com
  git -C "$repo" config commit.gpgsign false
  echo base >"$repo/README.md"
  git -C "$repo" add -A && git -C "$repo" commit -q -m "chore: init"
  git -C "$repo" switch -q -c feature
}
# commit <msg> <file>... — создаёт/меняет файлы и коммитит их.
commit() {
  local msg=$1; shift
  for f in "$@"; do mkdir -p "$repo/$(dirname "$f")"; echo "$RANDOM $msg" >>"$repo/$f"; done
  git -C "$repo" add -A && git -C "$repo" commit -q -m "$msg"
}
tdd() { run bash -c 'cd "$1" && bash "$2" "$3"' _ "$repo" "$TDD" "$1"; }

new_repo ok
commit "test(ingest): add failing handler tests" backend/internal/ingest/handler_test.go
commit "feat(ingest): implement handler" backend/internal/ingest/handler.go
commit "test(web): add fetcher tests" web/src/lib/api/fetcher.test.ts
commit "feat(web): add fetcher" web/src/lib/api/fetcher.ts
tdd main
expect_code "тест раньше реализации → exit 0" 0

new_repo bad
commit "feat(ingest): implement handler" backend/internal/ingest/handler.go
commit "test(ingest): add handler tests" backend/internal/ingest/handler_test.go
commit "feat(web): add plural" web/src/lib/plural.ts
commit "test(web): add plural tests" web/src/lib/plural.test.ts
tdd main
expect_code "AC-008.2: feat(ingest) раньше test(ingest) → exit 1" 1
expect_out "AC-008.2: нарушение указывает пакет Go" "backend/internal/ingest:"
expect_out "AC-008.2: нарушение указывает коммит реализации" "feat(ingest): implement handler"
expect_out "AC-008.2: нарушение указывает коммит теста" "test(ingest): add handler tests"
expect_out "нарушение в web-пакете тоже найдено" "web/src/lib:"

new_repo same
commit "test(httpx): add tests with compile stubs" backend/internal/httpx/cors_test.go backend/internal/httpx/cors.go
commit "feat(httpx): implement cors" backend/internal/httpx/cors.go
tdd main
expect_code "тест и заглушки в одном коммите → exit 0" 0

new_repo other
commit "feat(geo): implement lookup" backend/internal/geo/lookup.go
commit "test(ingest): add tests" backend/internal/ingest/enrich_test.go
commit "feat(ingest): implement enrich" backend/internal/ingest/enrich.go
tdd main
expect_code "код без тестов в другом пакете не нарушение порядка" 0

new_repo gen
commit "chore(api): regenerate" backend/internal/gen/ingestapi/models.go web/src/lib/api/gen/client.ts backend/internal/pgstore/sqlc/q.go
commit "test(gen): touch tests" backend/internal/gen/ingestapi/models_test.go web/src/lib/api/gen/client.test.ts backend/internal/pgstore/sqlc/q_test.go
tdd main
expect_code "сгенерированный код игнорируется" 0

new_repo testdata
commit "feat(reports): implement audience" backend/internal/reports/audience.go
commit "test(reports): golden fixture" backend/internal/reports/testdata/fixture_test.go
tdd main
expect_code "testdata не считается тестом пакета" 0

new_repo tsx
commit "feat(reports): add heatmap" web/src/components/reports/retention-heatmap.tsx
commit "test(reports): add heatmap test" web/src/components/reports/retention-heatmap.test.tsx
tdd main
expect_code "*.test.tsx после компонента → exit 1" 1
expect_out "*.test.tsx: нарушение в web/src/components/reports" "web/src/components/reports:"

new_repo docs
commit "docs: notes" docs/notes.md
commit "test(e2e): add spec" web/e2e/portfolio.spec.ts
tdd main
expect_code "e2e-спека без кода в каталоге → exit 0" 0

new_repo merged
commit "feat(ingest): implement handler" backend/internal/ingest/handler.go
git -C "$repo" switch -q main
git -C "$repo" merge -q --no-ff feature -m "Merge feature"
git -C "$repo" switch -q -c next
commit "test(ingest): add handler tests" backend/internal/ingest/handler_test.go
tdd main
expect_code "коммиты, уже влитые в базу, не проверяются" 0

new_repo empty
tdd main
expect_code "пустой диапазон → exit 0" 0

new_repo nobase
tdd origin/main
expect_code "базовая ветка не найдена → exit 2" 2

echo "perf-compare.sh"

# k6_report <file> <p95> <p99> — отчёт `k6 run --summary-export` (метрики трендов лежат прямо в объекте метрики).
k6_report() {
  cat >"$1" <<EOF
{"metrics":{"http_req_duration":{"avg":40.1,"min":3.2,"med":35,"max":900,"p(90)":60,"p(95)":$2,"p(99)":$3},
 "http_reqs":{"count":300000,"rate":500}}}
EOF
}
k6_report "$TMP/old.json" 80 200
k6_report "$TMP/faster.json" 70 150
k6_report "$TMP/plus10.json" 90 220
k6_report "$TMP/plus20.json" 100 240
k6_report "$TMP/plus25.json" 85 250

run bash "$PERF" "$TMP/old.json" "$TMP/plus10.json"
expect_code "AC-015.2: p99 +10 % → exit 0" 0
expect_out "AC-015.2: печатает разницу p95" "p95: 80.00 ms → 90.00 ms (+12.5%)"
expect_out "AC-015.2: печатает разницу p99" "p99: 200.00 ms → 220.00 ms (+10.0%)"

run bash "$PERF" "$TMP/old.json" "$TMP/plus25.json"
expect_code "AC-015.2: p99 +25 % → exit 1" 1
expect_out "AC-015.2: регрессия названа" "регрессия: p99 вырос на 25.0% (> 20%)"

run bash "$PERF" "$TMP/old.json" "$TMP/plus20.json"
expect_code "p99 ровно +20 % — ещё не регрессия" 0
expect_no_out "p99 +20 %: без строки регрессии" "регрессия"

run bash "$PERF" "$TMP/old.json" "$TMP/faster.json"
expect_code "p99 уменьшился → exit 0" 0
expect_out "уменьшение со знаком минус" "p99: 200.00 ms → 150.00 ms (-25.0%)"

run env PERF_MAX_P99_GROWTH=5 bash "$PERF" "$TMP/old.json" "$TMP/plus10.json"
expect_code "PERF_MAX_P99_GROWTH=5: p99 +10 % → exit 1" 1
expect_out "порог подставляется в вывод" "(> 5%)"

# Формат handleSummary / --new-machine-readable-summary: значения во вложенном объекте values.
cat >"$TMP/values.json" <<EOF
{"metrics":{"http_req_duration":{"type":"trend","contains":"time","values":{"p(95)":88,"p(99)":230}}}}
EOF
run bash "$PERF" "$TMP/old.json" "$TMP/values.json"
expect_code "формат с values читается" 0
expect_out "формат с values: p99" "p99: 200.00 ms → 230.00 ms (+15.0%)"

# Без p(99) (прогон без --summary-trend-stats) сравнивать нечего — ошибка использования, а не «зелёный».
cat >"$TMP/nop99.json" <<EOF
{"metrics":{"http_req_duration":{"avg":40,"p(90)":60,"p(95)":80}}}
EOF
run bash "$PERF" "$TMP/old.json" "$TMP/nop99.json"
expect_code "нет p(99) → exit 2" 2
expect_out "нет p(99): указан файл" "nop99.json"

printf 'not json\n' >"$TMP/bad.json"
run bash "$PERF" "$TMP/old.json" "$TMP/bad.json"
expect_code "не JSON → exit 2" 2

k6_report "$TMP/zero.json" 0 0
run bash "$PERF" "$TMP/zero.json" "$TMP/old.json"
expect_code "нулевой p99 в старом отчёте → exit 2" 2

run bash "$PERF" "$TMP/old.json" "$TMP/missing.json"
expect_code "нет файла → exit 2" 2

run bash "$PERF" "$TMP/old.json"
expect_code "один аргумент → exit 2" 2

echo
if [ "$failures" -gt 0 ]; then
  echo "test-ci-gates: $failures из $total проверок упали"
  exit 1
fi
echo "test-ci-gates: все $total проверок прошли"
