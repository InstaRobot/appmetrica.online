#!/usr/bin/env bash
# План деплоя для .github/workflows/deploy.yml (T-024): что собирать, какой тег и какие сервисы выкатывать.
# Печатает строки key=value для $GITHUB_OUTPUT. Входы — переменные окружения (не ${{ }} в run: — защита от инъекций):
#   OWNER     — github.repository_owner (приводится к нижнему регистру, как требует GHCR);
#   EVENT     — github.event_name; workflow_run — автодеплой после зелёного CI, иначе берутся REF/TAG/SERVICES;
#   HEAD_SHA  — github.event.workflow_run.head_sha (только для workflow_run);
#   REF       — git ref или sha: собрать образы из него и выкатить (git rev-parse в текущем репозитории);
#   TAG       — существующий тег образов в GHCR (<sha> или <sha>-geo-YYYYMM): выкатить без сборки (откат);
#   SERVICES  — сервисы через запятую из api,ingest,worker,web (по умолчанию все; при workflow_run всегда все).
# Ровно один из REF и TAG. Выход: 0 — план напечатан, 1 — ошибка входа (сообщение в stderr).
set -euo pipefail

ALL="api ingest worker web"

die() { echo "deploy-plan: $*" >&2; exit 1; }

owner=$(printf '%s' "${OWNER:-}" | tr '[:upper:]' '[:lower:]')
[ -n "$owner" ] || die "OWNER (github.repository_owner) не задан"

# services_list <csv> — сервисы в порядке деплоя api → ingest → worker → web, без повторов.
services_list() {
  local raw s want=" " result=""
  raw=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr ',' ' ')
  for s in $raw; do
    case " $ALL " in *" $s "*) want="$want$s " ;; *) die "неизвестный сервис «${s}», допустимы: ${ALL// /, }" ;; esac
  done
  for s in $ALL; do
    case "$want" in *" $s "*) result="${result:+$result,}$s" ;; esac
  done
  [ -n "$result" ] || die "список сервисов пуст"
  printf '%s' "$result"
}

if [ "${EVENT:-}" = workflow_run ]; then
  [[ "${HEAD_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || die "workflow_run без head_sha"
  build=true
  tag=$HEAD_SHA
  sha=$HEAD_SHA
  tag_main=true
  services=$(services_list "${ALL// /,}")
else
  ref=${REF:-} tag=${TAG:-}
  [ -n "$ref" ] && [ -n "$tag" ] && die "ref и tag взаимоисключающие: ref — собрать и выкатить, tag — выкатить готовый образ"
  [ -z "$ref" ] && [ -z "$tag" ] && die "укажите ref (собрать и выкатить) или tag (выкатить готовый образ, откат)"
  services=$(services_list "${SERVICES:-${ALL// /,}}")
  tag_main=false
  if [ -n "$ref" ]; then
    sha=$(git rev-parse --verify --quiet "${ref}^{commit}" || git rev-parse --verify --quiet "origin/${ref}^{commit}") ||
      die "ref «${ref}» не найден в репозитории"
    build=true
    tag=$sha
  else
    [[ "$tag" =~ ^[0-9a-f]{40}(-geo-[0-9]{6})?$ ]] || die "tag должен быть git sha (40 hex) или <sha>-geo-YYYYMM, получено «${tag}»"
    if [[ "$tag" == *-geo-* ]] && [[ ",$services," == *,web,* ]]; then
      die "тег ${tag} есть только у appmetrica-backend (geo-refresh), web с ним выкатить нельзя"
    fi
    build=false
    sha=""
  fi
fi

has() { [[ ",$services," == *",$1,"* ]]; }
build_backend=false build_web=false
if [ "$build" = true ]; then
  if has api || has ingest || has worker; then build_backend=true; fi
  if has web; then build_web=true; fi
fi

# Тег main переезжает только при автодеплое с main: им пользуется первичная настройка Applications в Dokploy.
backend_main_image="" web_main_image=""
if [ "$tag_main" = true ]; then
  backend_main_image=ghcr.io/$owner/appmetrica-backend:main
  web_main_image=ghcr.io/$owner/appmetrica-web:main
fi

cat <<EOF
build=$build
build_backend=$build_backend
build_web=$build_web
sha=$sha
tag=$tag
tag_main=$tag_main
services=$services
backend_image=ghcr.io/$owner/appmetrica-backend:$tag
web_image=ghcr.io/$owner/appmetrica-web:$tag
backend_main_image=$backend_main_image
web_main_image=$web_main_image
EOF
