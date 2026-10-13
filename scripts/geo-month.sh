#!/usr/bin/env bash
# Месяц базы DB-IP City Lite для сборки backend (build-arg GEO_MONTH, T-024): печатает geo_month=YYYY-MM.
# DB-IP публикует файл месяца в первые дни месяца; по умолчанию, если файла текущего месяца ещё нет,
# берётся предыдущий. GEO_STRICT=1 (geo-refresh.yml) — только текущий месяц, иначе ошибка.
# GEO_NOW=YYYY-MM подменяет текущий месяц (тесты). Выход: 0 — месяц напечатан, 1 — файла нет.
set -euo pipefail

die() { echo "geo-month: $*" >&2; exit 1; }

now=${GEO_NOW:-$(date -u +%Y-%m)}
[[ "$now" =~ ^[0-9]{4}-(0[1-9]|1[0-2])$ ]] || die "некорректный месяц «${now}», нужен YYYY-MM"

url() { echo "https://download.db-ip.com/free/dbip-city-lite-$1.mmdb.gz"; }

# published <YYYY-MM> — файл месяца доступен (HEAD 200); имя файла попадает в лог шага сборки.
published() {
  echo "DB-IP: проверяю $(url "$1")" >&2
  curl -fsS --retry 3 --retry-delay 5 --head -o /dev/null "$(url "$1")"
}

if published "$now"; then
  month=$now
elif [ "${GEO_STRICT:-0}" = 1 ]; then
  die "файл dbip-city-lite-${now}.mmdb.gz ещё не опубликован"
else
  y=$((10#${now%-*})) m=$((10#${now#*-}))
  if [ "$m" -eq 1 ]; then y=$((y - 1)) m=12; else m=$((m - 1)); fi
  month=$(printf '%04d-%02d' "$y" "$m")
  published "$month" || die "нет ни dbip-city-lite-${now}.mmdb.gz, ни dbip-city-lite-${month}.mmdb.gz"
fi

echo "DB-IP: GEO_MONTH=${month}, файл dbip-city-lite-${month}.mmdb.gz" >&2
echo "geo_month=${month}"
