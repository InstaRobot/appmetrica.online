#!/usr/bin/env bash
# Выкатка образов в Dokploy (T-024, 12-dev-runbook.md §9.2). Вызывается из .github/workflows/deploy.yml
# после входа в tailnet: панель Dokploy доступна только через Tailscale.
#
# Для каждого сервиса из SERVICES в порядке api → ingest → worker → web:
#   1. POST /api/application.saveDockerProvider — новый образ и учётка GHCR;
#   2. POST /api/application.deploy с уникальным заголовком DEPLOY_TITLE;
#   3. опрос GET /api/deployment.all, пока деплой с этим заголовком не станет done (error/cancelled — провал);
#   4. для api, ingest и web — опрос /ready (/api/health у web), пока не ответит 200.
# api идёт первым: он применяет миграции при старте, ingest и worker стартуют уже на новой схеме.
# Первая ошибка останавливает выкатку — следующие сервисы остаются на прежней версии.
#
# Окружение: DOKPLOY_URL, DOKPLOY_API_KEY, GHCR_PULL_USER, GHCR_PULL_TOKEN, SERVICES, BACKEND_IMAGE, WEB_IMAGE,
# DOKPLOY_APP_ID_{API,INGEST,WORKER,WEB} (только для выбранных сервисов), DEPLOY_TITLE (по умолчанию «deploy <тег>»),
# POLL_ATTEMPTS/POLL_INTERVAL (статус деплоя, по умолчанию 120 × 5 с), READY_ATTEMPTS/READY_INTERVAL (36 × 5 с),
# READY_URL_{API,INGEST,WEB}. Ответы Dokploy не печатаются: в них есть пароль реестра.
set -euo pipefail

ALL="api ingest worker web"
POLL_ATTEMPTS=${POLL_ATTEMPTS:-120}
POLL_INTERVAL=${POLL_INTERVAL:-5}
READY_ATTEMPTS=${READY_ATTEMPTS:-36}
READY_INTERVAL=${READY_INTERVAL:-5}
READY_URL_API=${READY_URL_API:-https://api.appmetrica.online/ready}
READY_URL_INGEST=${READY_URL_INGEST:-https://i.appmetrica.online/ready}
READY_URL_WEB=${READY_URL_WEB:-https://appmetrica.online/api/health}

die() { echo "dokploy-deploy: $*" >&2; exit 1; }
need() { [ -n "${!1:-}" ] || die "переменная $1 не задана"; }

upper() { printf '%s' "$1" | tr '[:lower:]' '[:upper:]'; }

# Проверяем всё до первого вызова Dokploy, чтобы не выкатить половину сервисов.
for v in DOKPLOY_URL DOKPLOY_API_KEY GHCR_PULL_USER GHCR_PULL_TOKEN SERVICES; do need "$v"; done
selected=" "
for s in $(printf '%s' "$SERVICES" | tr ',' ' '); do
  case " $ALL " in *" $s "*) selected="$selected$s " ;; *) die "неизвестный сервис «${s}»" ;; esac
done
for s in $ALL; do
  case "$selected" in
    *" $s "*)
      need "DOKPLOY_APP_ID_$(upper "$s")"
      if [ "$s" = web ]; then need WEB_IMAGE; else need BACKEND_IMAGE; fi
      ;;
  esac
done

dokploy_post() { # endpoint json
  curl -fsS --retry 3 --retry-delay 5 -X POST "${DOKPLOY_URL}/api/$1" \
    -H "x-api-key: ${DOKPLOY_API_KEY}" -H "Content-Type: application/json" -d "$2" >/dev/null ||
    die "POST /api/$1 не выполнен"
}

# deployment_status <app_id> <title> — статус последнего деплоя с этим заголовком (пусто, если ещё не появился).
deployment_status() {
  curl -fsS --retry 3 --retry-delay 5 --get "${DOKPLOY_URL}/api/deployment.all" \
    -H "x-api-key: ${DOKPLOY_API_KEY}" --data-urlencode "applicationId=$1" |
    jq -r --arg t "$2" '[.[] | select(.title == $t)] | sort_by(.createdAt) | last | .status // ""'
}

wait_deployment() { # service app_id title
  local i status=""
  for ((i = 1; i <= POLL_ATTEMPTS; i++)); do
    status=$(deployment_status "$2" "$3") || die "GET /api/deployment.all для $1 не выполнен"
    case "$status" in
      done) echo "$1: деплой в Dokploy завершён"; return 0 ;;
      error | cancelled) die "$1: деплой в Dokploy завершился статусом ${status} (логи — Dokploy → Deployments)" ;;
    esac
    sleep "$POLL_INTERVAL"
  done
  die "$1: деплой в Dokploy не завершился за $((POLL_ATTEMPTS * POLL_INTERVAL)) с (последний статус «${status:-нет}»)"
}

wait_ready() { # service url
  local i
  for ((i = 1; i <= READY_ATTEMPTS; i++)); do
    if curl -fsS --max-time 5 -o /dev/null "$2" 2>/dev/null; then echo "$1: готов ($2)"; return 0; fi
    sleep "$READY_INTERVAL"
  done
  die "$1: $2 не ответил 200 за $((READY_ATTEMPTS * READY_INTERVAL)) с"
}

deployed=""
for s in $ALL; do
  case "$selected" in *" $s "*) ;; *) continue ;; esac
  var="DOKPLOY_APP_ID_$(upper "$s")"
  app_id=${!var}
  if [ "$s" = web ]; then image=$WEB_IMAGE; else image=$BACKEND_IMAGE; fi
  title=${DEPLOY_TITLE:-deploy ${image##*:}}

  echo "::group::$s ← $image"
  dokploy_post application.saveDockerProvider "$(jq -cn --arg id "$app_id" --arg img "$image" \
    --arg u "$GHCR_PULL_USER" --arg p "$GHCR_PULL_TOKEN" \
    '{applicationId: $id, dockerImage: $img, username: $u, password: $p, registryUrl: "ghcr.io"}')"
  dokploy_post application.deploy "$(jq -cn --arg id "$app_id" --arg t "$title" \
    '{applicationId: $id, title: $t, description: "GitHub Actions deploy.yml"}')"
  wait_deployment "$s" "$app_id" "$title"
  case "$s" in
    api) wait_ready "$s" "$READY_URL_API" ;;
    ingest) wait_ready "$s" "$READY_URL_INGEST" ;;
    web) wait_ready "$s" "$READY_URL_WEB" ;;
  esac
  echo "::endgroup::"
  deployed="${deployed:+$deployed }$s"
done

echo "выкачено: $deployed"
