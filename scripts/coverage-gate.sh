#!/usr/bin/env bash
# Гейт покрытия Go (T-008, NFR-009): каждый ключевой пакет internal/<pkg> (вместе с подпакетами)
# должен иметь покрытие statements не ниже COVER_MIN процентов.
#
#   bash scripts/coverage-gate.sh <coverprofile>
#
# Профиль — результат `go test -coverpkg=./internal/... -coverprofile=...` (формат mode: set|count|atomic).
# Один блок может встречаться в профиле несколько раз (по разу на каждый тест-бинарник): блок считается
# покрытым, если хотя бы одна запись имеет count > 0.
# Пакет, которого ещё нет в профиле (тикет не реализован), пропускается.
#
# Переменные: COVER_MIN (по умолчанию 80), COVER_PKGS (по умолчанию "ingest reports auth identity deletion merge"),
# COVER_MODULE (по умолчанию appmetrica.online/backend).
# Exit: 0 — все пакеты проходят, 1 — есть пакет ниже порога, 2 — ошибка использования.
set -euo pipefail

profile=${1:-}
min=${COVER_MIN:-80}
pkgs=${COVER_PKGS:-ingest reports auth identity deletion merge}
module=${COVER_MODULE:-appmetrica.online/backend}

if [ -z "$profile" ]; then
  echo "usage: $0 <coverprofile>" >&2
  exit 2
fi
if [ ! -f "$profile" ]; then
  echo "coverage-gate: файл профиля не найден: $profile" >&2
  exit 2
fi
if ! head -n1 "$profile" | grep -qE '^mode: (set|count|atomic)$'; then
  echo "coverage-gate: $profile не похож на Go coverprofile (нет строки mode:)" >&2
  exit 2
fi

awk -v min="$min" -v pkgs="$pkgs" -v prefix="$module/internal/" '
  NR == 1 { next }
  NF != 3 { next }
  {
    # $1 = <import path>/<file>.go:<start>,<end>; $2 = statements; $3 = count.
    block = $1
    if (!(block in stmts)) { stmts[block] = $2; order[++n] = block }
    if ($3 > 0) { covered[block] = 1 }
  }
  END {
    np = split(pkgs, list, " ")
    failed = 0
    for (i = 1; i <= np; i++) {
      p = list[i]
      want = prefix p "/"
      total = 0; hit = 0
      for (j = 1; j <= n; j++) {
        b = order[j]
        if (substr(b, 1, length(want)) != want) { continue }
        total += stmts[b]
        if (b in covered) { hit += stmts[b] }
      }
      if (total == 0) {
        printf "internal/%s: нет в профиле, пропуск\n", p
        continue
      }
      # Обрезаем до 0,1 % вниз, чтобы 79,96 % не печаталось как 80,0 %.
      shown = int(hit * 1000 / total) / 10
      if (hit * 100 < min * total) {
        printf "internal/%s %.1f%% < %s%%\n", p, shown, min
        failed = 1
      } else {
        printf "internal/%s %.1f%% >= %s%%\n", p, shown, min
      }
    }
    exit failed
  }
' "$profile"
