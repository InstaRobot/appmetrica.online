#!/usr/bin/env bash
# Проверка порядка коммитов TDD (T-008, 22-test-strategy.md §8.1).
#
#   bash scripts/check-tdd-order.sh [base]      # base по умолчанию origin/main
#
# Для каждого пакета (каталога), в котором ветка меняет тесты (*_test.go, *.test.ts(x), *.spec.ts(x)),
# первый коммит с тестом должен быть не позже первого коммита, меняющего код этого же пакета (*.go, *.ts, *.tsx).
# Тест и заглушки для компиляции в одном коммите допустимы. Сгенерированный код и testdata/ не учитываются.
# Проверяются только коммиты base..HEAD без merge-коммитов. В CI шаг пропускается при метке PR `tdd-exception`.
#
# Exit: 0 — порядок соблюдён, 1 — есть нарушения (перечислены), 2 — ошибка использования.
set -euo pipefail

base=${1:-origin/main}

if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
  echo "check-tdd-order: базовая ветка «${base}» не найдена (в CI нужен actions/checkout с fetch-depth: 0)" >&2
  exit 2
fi
merge_base=$(git merge-base "$base" HEAD)

# %x01 отмечает начало коммита: «\001<sha>\t<subject>», далее — изменённые файлы по одному в строке.
violations=$(git log --reverse --no-merges --no-renames --format='%x01%h%x09%s' --name-only "${merge_base}..HEAD" |
  awk -F '\t' '
    function dir_of(path,   d) {
      d = path
      if (sub(/\/[^\/]*$/, "", d) == 0) { d = "." }
      return d
    }
    /^\001/ {
      idx++
      sha = substr($1, 2)
      subject = $0; sub(/^[^\t]*\t/, "", subject)
      next
    }
    $0 == "" { next }
    {
      path = $0
      if (path ~ /^(backend\/internal\/gen|backend\/internal\/pgstore\/sqlc|web\/src\/lib\/api\/gen)\//) { next }
      if (path ~ /(^|\/)testdata\//) { next }
      d = dir_of(path)
      if (path ~ /(_test\.go|\.test\.tsx?|\.spec\.tsx?)$/) {
        if (!(d in test_idx)) { test_idx[d] = idx; test_ref[d] = sha " «" subject "»" }
      } else if (path ~ /\.(go|tsx?)$/) {
        if (!(d in src_idx)) { src_idx[d] = idx; src_ref[d] = sha " «" subject "»" }
      }
    }
    END {
      for (d in test_idx) {
        if ((d in src_idx) && src_idx[d] < test_idx[d]) {
          printf "  %s: код изменён в %s раньше теста в %s\n", d, src_ref[d], test_ref[d]
          bad++
        }
      }
      exit (bad > 0)
    }
  ' | sort) && status=0 || status=$?

commits=$(git rev-list --count --no-merges "${merge_base}..HEAD")
if [ "$status" -ne 0 ]; then
  echo "check-tdd-order: нарушен порядок TDD — тест пакета должен быть закоммичен не позже его реализации:"
  printf '%s\n' "$violations"
  echo "Разделите коммиты (сначала test(<scope>), затем feat/fix(<scope>)) или попросите владельца поставить метку tdd-exception."
  exit 1
fi
echo "check-tdd-order: OK, проверено коммитов: $commits (база ${base})"
