#!/usr/bin/env bash
# Тесты деплоя T-024: scripts/deploy-plan.sh, scripts/geo-month.sh, scripts/dokploy-deploy.sh и структура
# .github/workflows/{ci,deploy,geo-refresh}.yml. Dokploy, GHCR и DB-IP подменяются заглушкой curl в PATH.
# Запуск из любой директории: bash scripts/test-deploy.sh (нужны bash, git, jq и python3 с PyYAML).
# Совместим с bash 3.2 (macOS) — без ассоциативных массивов.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLAN="$ROOT/scripts/deploy-plan.sh"
GEO="$ROOT/scripts/geo-month.sh"
DEPLOY="$ROOT/scripts/dokploy-deploy.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

failures=0
total=0
pass() { total=$((total + 1)); printf '  ok    %s\n' "$1"; }
fail() { total=$((total + 1)); failures=$((failures + 1)); printf '  FAIL  %s\n' "$1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | sed 's/^/        | /'; }

# run <cmd...> — выполняет команду, сохраняет вывод (stdout + stderr) в $out и код выхода в $code.
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
expect_eq() { # name got want
  if [ "$2" = "$3" ]; then pass "$1"; else fail "$1 (получено «$2», ожидалось «$3»)"; fi
}

SHA=0123456789abcdef0123456789abcdef01234567
PREV=fedcba9876543210fedcba9876543210fedcba98

# ---------------------------------------------------------------------------
echo "deploy-plan.sh"

# plan <env...> — запуск deploy-plan.sh с чистым окружением плана.
plan() { run env -u EVENT -u REF -u TAG -u SERVICES -u HEAD_SHA OWNER=OnlyApps "$@" bash "$PLAN"; }

plan EVENT=workflow_run HEAD_SHA=$SHA
expect_code "workflow_run → exit 0" 0
expect_out "workflow_run: сборка" "build=true"
expect_out "workflow_run: тег = head_sha" "tag=$SHA"
expect_out "workflow_run: все сервисы в порядке api → ingest → worker → web" "services=api,ingest,worker,web"
expect_out "образ backend в GHCR владельца (нижний регистр)" "backend_image=ghcr.io/onlyapps/appmetrica-backend:$SHA"
expect_out "образ web" "web_image=ghcr.io/onlyapps/appmetrica-web:$SHA"
expect_out "workflow_run: двигается тег main" "tag_main=true"
expect_out "workflow_run: тег main у backend" "backend_main_image=ghcr.io/onlyapps/appmetrica-backend:main"
expect_out "workflow_run: тег main у web" "web_main_image=ghcr.io/onlyapps/appmetrica-web:main"
expect_out "workflow_run: собираются оба образа" "build_backend=true"
expect_out "workflow_run: собирается web" "build_web=true"

plan EVENT=workflow_run HEAD_SHA=$SHA SERVICES=web
expect_out "workflow_run игнорирует вход services" "services=api,ingest,worker,web"

plan EVENT=workflow_run HEAD_SHA=main
expect_code "workflow_run без sha → exit 1" 1

# Репозиторий для разбора ref: main и ветка feature с отдельным коммитом.
repo="$TMP/repo"
mkdir -p "$repo"
git -C "$repo" init -q -b main
git -C "$repo" config user.name test
git -C "$repo" config user.email test@example.com
git -C "$repo" config commit.gpgsign false
echo a >"$repo/a" && git -C "$repo" add -A && git -C "$repo" commit -q -m "chore: a"
main_sha=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" switch -q -c feature
echo b >"$repo/b" && git -C "$repo" add -A && git -C "$repo" commit -q -m "chore: b"
feature_sha=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" switch -q main
planr() { run env -u EVENT -u REF -u TAG -u SERVICES -u HEAD_SHA OWNER=onlyapps "$@" bash -c 'cd "$1" && bash "$2"' _ "$repo" "$PLAN"; }

planr EVENT=workflow_dispatch REF="$main_sha"
expect_code "AC-024.1: ref=<sha> → exit 0" 0
expect_out "AC-024.1: ref=<sha> собирает образы" "build=true"
expect_out "AC-024.1: ref=<sha> → тег = sha" "tag=$main_sha"
expect_out "ref: sha для checkout" "sha=$main_sha"
expect_out "ручной деплой не двигает тег main" "tag_main=false"
expect_eq "ручной деплой: без образов :main" "$(grep -cxE '(backend|web)_main_image=' <<<"$out")" "2"

planr EVENT=workflow_dispatch REF="${main_sha:0:7}"
expect_out "короткий sha раскрывается до полного" "tag=$main_sha"

planr EVENT=workflow_dispatch REF=feature
expect_out "ref=ветка раскрывается в sha её HEAD" "tag=$feature_sha"

planr EVENT=workflow_dispatch REF=no-such-branch
expect_code "неизвестный ref → exit 1" 1
expect_out "неизвестный ref назван в ошибке" "no-such-branch"

plan EVENT=workflow_dispatch TAG=$PREV
expect_code "AC-024.2: tag=<prev-sha> → exit 0" 0
expect_out "AC-024.2: откат без сборки" "build=false"
expect_out "AC-024.2: откат на прежний тег backend" "backend_image=ghcr.io/onlyapps/appmetrica-backend:$PREV"
expect_out "AC-024.2: откат на прежний тег web" "web_image=ghcr.io/onlyapps/appmetrica-web:$PREV"
expect_out "откат: без сборки backend" "build_backend=false"

plan EVENT=workflow_dispatch TAG=$PREV SERVICES=" Web , api,api"
expect_code "services с пробелами, регистром и повтором → exit 0" 0
expect_out "services нормализуются и упорядочиваются" "services=api,web"

plan EVENT=workflow_dispatch TAG=$PREV SERVICES=api,cron
expect_code "неизвестный сервис → exit 1" 1
expect_out "неизвестный сервис назван" "cron"

plan EVENT=workflow_dispatch TAG=$PREV SERVICES=" , "
expect_code "пустой список сервисов → exit 1" 1

plan EVENT=workflow_dispatch TAG="$PREV-geo-202611" SERVICES=ingest
expect_code "гео-тег для ingest → exit 0" 0
expect_out "гео-тег сохраняется" "tag=$PREV-geo-202611"

plan EVENT=schedule TAG="$PREV-geo-202611" SERVICES=ingest
expect_code "вызов из geo-refresh по расписанию берёт inputs" 0
expect_out "schedule: services из inputs" "services=ingest"

plan EVENT=workflow_dispatch TAG="$PREV-geo-202611"
expect_code "гео-тег с web → exit 1 (образа web с таким тегом нет)" 1

plan EVENT=workflow_dispatch TAG=latest
expect_code "tag не sha → exit 1" 1
plan EVENT=workflow_dispatch TAG='abc;rm -rf /'
expect_code "tag с метасимволами → exit 1" 1

plan EVENT=workflow_dispatch
expect_code "ни ref, ни tag → exit 1" 1
expect_out "подсказка про ref/tag" "ref"

planr EVENT=workflow_dispatch REF=main TAG=$PREV
expect_code "ref и tag одновременно → exit 1" 1

planr EVENT=workflow_dispatch REF=main SERVICES=web
expect_out "ref + только web: backend не собирается" "build_backend=false"
expect_out "ref + только web: web собирается" "build_web=true"

planr EVENT=workflow_dispatch REF=main SERVICES=ingest
expect_out "ref + только ingest: web не собирается" "build_web=false"

run env -u OWNER EVENT=workflow_run HEAD_SHA=$SHA bash "$PLAN"
expect_code "без OWNER → exit 1" 1

# ---------------------------------------------------------------------------
# Заглушка curl: пишет вызовы в $STUB_DIR/calls (метод URL тело), отвечает по URL.
#   STUB_FAIL_MATCH   — подстрока URL, на которой curl падает с exit 22 (HTTP 4xx/5xx);
#   STUB_STATUS_<app> — последовательность статусов деплоя через запятую (по умолчанию done);
#   STUB_HEAD_FAIL    — подстрока URL, на которой HEAD (-I) падает (нет файла DB-IP).
mkdir -p "$TMP/bin" "$TMP/stub"
cat >"$TMP/bin/curl" <<'STUB'
#!/usr/bin/env bash
method=GET url= body= head=0
while [ $# -gt 0 ]; do
  case "$1" in
    -X) method=$2; shift ;;
    -d|--data|--data-raw|--data-binary) body=$2; shift ;;
    --data-urlencode) url="${url}?$2"; shift ;;
    -H|-o|-w|--retry|--retry-delay|--max-time|--connect-timeout) shift ;;
    -I|--head) head=1; method=HEAD ;;
    --get|-G) method=GET ;;
    http*) url="$1$url" ;;
  esac
  shift
done
printf '%s %s %s\n' "$method" "$url" "$body" >>"$STUB_DIR/calls"
if [ -n "${STUB_FAIL_MATCH:-}" ] && [[ "$url" == *"$STUB_FAIL_MATCH"* ]]; then
  echo "curl: (22) The requested URL returned error: 500" >&2; exit 22
fi
if [ "$head" = 1 ] && [ -n "${STUB_HEAD_FAIL:-}" ] && [[ "$url" == *"$STUB_HEAD_FAIL"* ]]; then
  echo "curl: (22) The requested URL returned error: 404" >&2; exit 22
fi
case "$url" in
  */api/application.saveDockerProvider)
    # Dokploy возвращает объект приложения — вместе с паролем реестра.
    printf '{"applicationId":"x","password":"%s","username":"%s"}\n' "$GHCR_PULL_TOKEN" "$GHCR_PULL_USER" ;;
  */api/application.deploy)
    app=$(jq -r .applicationId <<<"$body"); jq -r .title <<<"$body" >"$STUB_DIR/title-$app"; echo true ;;
  */api/deployment.all*)
    app=${url##*applicationId=}
    var="STUB_STATUS_$app"; seq="${!var:-done}"
    n=$(cat "$STUB_DIR/polls-$app" 2>/dev/null || echo 0); echo $((n + 1)) >"$STUB_DIR/polls-$app"
    IFS=, read -r -a st <<<"$seq"; i=$n; [ "$i" -ge "${#st[@]}" ] && i=$((${#st[@]} - 1))
    title=$(cat "$STUB_DIR/title-$app" 2>/dev/null || echo none)
    jq -n --arg t "$title" --arg s "${st[$i]}" \
      '[{deploymentId:"new",title:$t,status:$s,createdAt:"2026-10-14T10:00:00Z"},
        {deploymentId:"old",title:"deploy old",status:"done",createdAt:"2026-10-13T10:00:00Z"}]' ;;
  *) echo ok ;;
esac
STUB
chmod +x "$TMP/bin/curl"

# deploy <env...> — dokploy-deploy.sh с заглушкой curl и мгновенным опросом.
deploy() {
  rm -rf "$TMP/stub" && mkdir -p "$TMP/stub"
  run env PATH="$TMP/bin:$PATH" STUB_DIR="$TMP/stub" \
    DOKPLOY_URL=http://appmetrica-prod:3000 DOKPLOY_API_KEY=dk-secret-api-key \
    GHCR_PULL_USER=puller GHCR_PULL_TOKEN=ghp-secret-pull-token \
    DOKPLOY_APP_ID_API=app_api DOKPLOY_APP_ID_INGEST=app_ingest DOKPLOY_APP_ID_WORKER=app_worker DOKPLOY_APP_ID_WEB=app_web \
    BACKEND_IMAGE=ghcr.io/onlyapps/appmetrica-backend:$SHA WEB_IMAGE=ghcr.io/onlyapps/appmetrica-web:$SHA \
    SERVICES=api,ingest,worker,web DEPLOY_TITLE="deploy $SHA run 7" \
    POLL_INTERVAL=0 READY_INTERVAL=0 "$@" bash "$DEPLOY"
  calls="$(cat "$TMP/stub/calls" 2>/dev/null)"
}
# order — последовательность «эндпоинт:приложение» из вызовов заглушки, без повторных опросов.
order() {
  grep -E 'saveDockerProvider|application\.deploy|/ready|/api/health' <<<"$calls" | while read -r m u b; do
    case "$u" in
      *saveDockerProvider) echo "save:$(jq -r .applicationId <<<"$b")" ;;
      *application.deploy) echo "deploy:$(jq -r .applicationId <<<"$b")" ;;
      *) echo "ready:$u" ;;
    esac
  done | paste -sd' ' -
}

echo "dokploy-deploy.sh"

deploy
expect_code "AC-024.1: деплой всех сервисов → exit 0" 0
expect_eq "AC-024.1: порядок api → ingest → worker → web, ожидание /ready после api, ingest и web" "$(order)" \
  "save:app_api deploy:app_api ready:https://api.appmetrica.online/ready save:app_ingest deploy:app_ingest ready:https://i.appmetrica.online/ready save:app_worker deploy:app_worker save:app_web deploy:app_web ready:https://appmetrica.online/api/health"
save_api=$(grep -m1 'saveDockerProvider' <<<"$calls" | cut -d' ' -f3-)
expect_eq "saveDockerProvider: новый образ" "$(jq -r .dockerImage <<<"$save_api")" "ghcr.io/onlyapps/appmetrica-backend:$SHA"
expect_eq "saveDockerProvider: реестр ghcr.io" "$(jq -r .registryUrl <<<"$save_api")" "ghcr.io"
expect_eq "saveDockerProvider: логин реестра" "$(jq -r .username <<<"$save_api")" "puller"
save_web=$(grep 'saveDockerProvider' <<<"$calls" | tail -n1 | cut -d' ' -f3-)
expect_eq "web получает образ appmetrica-web" "$(jq -r .dockerImage <<<"$save_web")" "ghcr.io/onlyapps/appmetrica-web:$SHA"
dep_api=$(grep -m1 'application.deploy' <<<"$calls" | cut -d' ' -f3-)
expect_eq "deploy: заголовок деплоя для поиска в deployment.all" "$(jq -r .title <<<"$dep_api")" "deploy $SHA run 7"
expect_eq "деплой каждого приложения опрашивается в deployment.all" "$(grep -c 'deployment.all' <<<"$calls")" "4"
expect_no_out "DoD: ключ Dokploy не печатается" "dk-secret-api-key"
expect_no_out "DoD: пароль GHCR из ответа Dokploy не печатается" "ghp-secret-pull-token"
expect_out "итог перечисляет выкаченные сервисы" "api ingest worker web"

deploy SERVICES=ingest BACKEND_IMAGE="ghcr.io/onlyapps/appmetrica-backend:$PREV-geo-202611"
expect_code "geo-refresh: только ingest → exit 0" 0
expect_eq "geo-refresh: трогается только ingest" "$(order)" "save:app_ingest deploy:app_ingest ready:https://i.appmetrica.online/ready"

deploy SERVICES=worker,api
expect_eq "порядок не зависит от порядка во входе" "$(order)" \
  "save:app_api deploy:app_api ready:https://api.appmetrica.online/ready save:app_worker deploy:app_worker"

deploy STUB_STATUS_app_api=running,running,done
expect_code "деплой в процессе → ждём done" 0
expect_eq "опрос до статуса done" "$(cat "$TMP/stub/polls-app_api")" "3"

deploy STUB_STATUS_app_ingest=running,error
expect_code "Dokploy: статус error → exit 1" 1
expect_out "ошибка называет сервис и статус" "ingest"
expect_out "ошибка называет статус error" "error"
expect_eq "после ошибки ingest worker и web не трогаются" "$(order)" \
  "save:app_api deploy:app_api ready:https://api.appmetrica.online/ready save:app_ingest deploy:app_ingest"

deploy STUB_STATUS_app_api=running POLL_ATTEMPTS=3
expect_code "деплой не завершился за POLL_ATTEMPTS → exit 1" 1
expect_eq "после таймаута api остальные не трогаются" "$(order)" "save:app_api deploy:app_api"

deploy STUB_FAIL_MATCH=saveDockerProvider
expect_code "saveDockerProvider 500 → exit 1" 1
out="$calls"
expect_no_out "после ошибки saveDockerProvider deploy не вызывается" "application.deploy"

deploy STUB_FAIL_MATCH=//i.appmetrica.online/ready READY_ATTEMPTS=2
expect_code "ingest не стал ready → exit 1" 1
expect_out "ошибка готовности называет URL" "https://i.appmetrica.online/ready"
expect_eq "ready опрашивается READY_ATTEMPTS раз" "$(grep -c '//i.appmetrica.online/ready' <<<"$calls")" "2"

deploy DOKPLOY_APP_ID_WEB=
expect_code "нет DOKPLOY_APP_ID_WEB при деплое web → exit 1" 1
expect_out "ошибка называет переменную" "DOKPLOY_APP_ID_WEB"
expect_eq "проверка переменных до первого вызова Dokploy" "${calls:-}" ""

deploy DOKPLOY_APP_ID_WEB= SERVICES=api
expect_code "без DOKPLOY_APP_ID_WEB, но без web → exit 0" 0

deploy DOKPLOY_API_KEY=
expect_code "нет DOKPLOY_API_KEY → exit 1" 1

deploy SERVICES=api,cron
expect_code "неизвестный сервис → exit 1" 1

# ---------------------------------------------------------------------------
echo "geo-month.sh"

geo() { rm -rf "$TMP/stub" && mkdir -p "$TMP/stub"; run env PATH="$TMP/bin:$PATH" STUB_DIR="$TMP/stub" "$@" bash "$GEO"; }

geo GEO_NOW=2026-11
expect_code "текущий месяц опубликован → exit 0" 0
expect_out "печатает geo_month" "geo_month=2026-11"
expect_out "AC-024.3: в логе имя файла DB-IP" "dbip-city-lite-2026-11.mmdb.gz"

geo GEO_NOW=2026-11 STUB_HEAD_FAIL=2026-11
expect_code "текущего ещё нет → предыдущий месяц, exit 0" 0
expect_out "fallback на предыдущий месяц" "geo_month=2026-10"

geo GEO_NOW=2027-01 STUB_HEAD_FAIL=2027-01
expect_out "fallback через границу года" "geo_month=2026-12"

geo GEO_NOW=2026-11 STUB_HEAD_FAIL=2026-11 GEO_STRICT=1
expect_code "geo-refresh: текущего месяца нет → exit 1" 1
expect_no_out "strict: без fallback" "geo_month=2026-10"

geo GEO_NOW=2026-11 STUB_HEAD_FAIL=dbip
expect_code "нет ни текущего, ни предыдущего → exit 1" 1

geo GEO_NOW=2026-13
expect_code "некорректный GEO_NOW → exit 1" 1

# ---------------------------------------------------------------------------
echo "workflows"

# wf <file> — workflow в JSON (PyYAML разбирает ключ on как True — возвращаем ему имя on).
wf() {
  python3 - "$ROOT/.github/workflows/$1" <<'PY'
import json, sys, yaml
def fix(o):
    if isinstance(o, dict):
        return {("on" if k is True else str(k)): fix(v) for k, v in o.items()}
    if isinstance(o, list):
        return [fix(v) for v in o]
    return o
with open(sys.argv[1]) as f:
    print(json.dumps(fix(yaml.safe_load(f))))
PY
}

for f in ci.yml deploy.yml geo-refresh.yml; do
  if json=$(wf "$f" 2>&1) && jq -e . >/dev/null <<<"$json"; then pass "$f: YAML разбирается"; else fail "$f: YAML разбирается" "$json"; fi
done
deploy_wf=$(wf deploy.yml 2>/dev/null || echo '{}')
geo_wf=$(wf geo-refresh.yml 2>/dev/null || echo '{}')
ci_wf=$(wf ci.yml 2>/dev/null || echo '{}')

jqt() { # name json filter — фильтр должен вернуть true
  if jq -e "$3" >/dev/null 2>&1 <<<"$2"; then pass "$1"; else fail "$1" "jq: $3"; fi
}

jqt "deploy: вход ref в workflow_dispatch" "$deploy_wf" '.on.workflow_dispatch.inputs | has("ref") and has("tag") and has("services")'
jqt "deploy: те же входы в workflow_call" "$deploy_wf" '.on.workflow_call.inputs | has("ref") and has("tag") and has("services")'
jqt "deploy: запуск после зелёного CI на main" "$deploy_wf" '.on.workflow_run.workflows == ["CI"] and .on.workflow_run.branches == ["main"]'
jqt "deploy: автодеплой только по push в main с зелёным CI" "$deploy_wf" '.jobs.plan.if | test("conclusion == .success.") and test("workflow_run.event == .push.")'
jqt "deploy: jobs plan → build → deploy → prune" "$deploy_wf" '.jobs | has("plan") and has("build") and has("deploy") and has("prune")'
jqt "deploy: build пишет в GHCR" "$deploy_wf" '.jobs.build.permissions.packages == "write"'
jqt "deploy: build передаёт GEO_MONTH" "$deploy_wf" '[.jobs.build.steps[] | .with["build-args"]? // "" | test("GEO_MONTH=")] | any'
jqt "deploy: теги образов из плана (ghcr.io/<owner>/appmetrica-{backend,web}:<sha>)" "$deploy_wf" '[.jobs.build.steps[] | select(.uses? // "" | startswith("docker/build-push-action")) | .with.tags | test("needs.plan.outputs.(backend|web)_image")] | length == 2 and all(.)'
jqt "deploy: provenance выключен (одна версия пакета на образ — ретеншн 10 тегов)" "$deploy_wf" '[.jobs.build.steps[] | select(.uses? // "" | startswith("docker/build-push-action")) | .with.provenance == false] | length >= 2 and all(.)'
jqt "deploy: tailscale/github-action v4.2.0 с tag:ci" "$deploy_wf" '[.jobs.deploy.steps[] | select(.uses? == "tailscale/github-action@v4.2.0") | .with.tags == "tag:ci"] | any'
jqt "deploy: окружение production" "$deploy_wf" '.jobs.deploy.environment == "production"'
jqt "deploy: деплой через scripts/dokploy-deploy.sh" "$deploy_wf" '[.jobs.deploy.steps[] | .run? // "" | test("scripts/dokploy-deploy.sh")] | any'
jqt "deploy: проверка образов в GHCR до Dokploy" "$deploy_wf" '[.jobs.deploy.steps[] | .run? // "" | test("imagetools inspect")] | any'
jqt "deploy: smoke /health ingest и api" "$deploy_wf" '[.jobs.deploy.steps[] | .run? // ""] | join("\n") | test("i.appmetrica.online/health") and test("api.appmetrica.online/health")'
jqt "DoD: GHCR хранит 10 последних версий каждого образа" "$deploy_wf" '[.jobs.prune.steps[] | select(.uses? // "" | startswith("actions/delete-package-versions@")) | .with["min-versions-to-keep"] == 10] | length == 2 and all(.)'
jqt "deploy: один деплой за раз" "$deploy_wf" '.concurrency["cancel-in-progress"] == false'
jqt "deploy: входы не подставляются в run (защита от инъекций)" "$deploy_wf" '[.. | objects | .run? // empty | test("\\$\\{\\{ *(inputs|github\\.event)\\.")] | any | not'
jqt "deploy: секреты не подставляются в run" "$deploy_wf" '[.. | objects | .run? // empty | test("\\$\\{\\{ *secrets\\.")] | any | not'

jqt "geo-refresh: cron 0 6 2 * *" "$geo_wf" '.on.schedule == [{"cron": "0 6 2 * *"}]'
jqt "geo-refresh: ручной запуск" "$geo_wf" '.on | has("workflow_dispatch")'
jqt "geo-refresh: строгий месяц DB-IP" "$geo_wf" '[.jobs.build.steps[] | .run? // ""] | join("\n") | test("GEO_STRICT=1") and test("scripts/geo-month.sh")'
jqt "geo-refresh: деплой через deploy.yml только ingest по тегу" "$geo_wf" '.jobs.deploy.uses == "./.github/workflows/deploy.yml" and .jobs.deploy.with.services == "ingest" and (.jobs.deploy.with.tag | test("needs.build.outputs"))'
jqt "geo-refresh: секреты наследуются" "$geo_wf" '.jobs.deploy.secrets == "inherit"'
jqt "geo-refresh: вызов deploy.yml получает права на GHCR" "$geo_wf" '.jobs.deploy.permissions.packages == "write"'

jqt "ci: образы собирает deploy.yml, не ci.yml" "$ci_wf" '.jobs | has("images") | not'
jqt "ci: тесты деплоя в CI" "$ci_wf" '[.jobs[].steps[]? | .run? // "" | test("scripts/test-deploy.sh")] | any'
jqt "ci: перф отчётов еженедельно по cron (понедельник 03:00 UTC) и вручную" "$ci_wf" '.on.schedule == [{"cron": "0 3 * * 1"}] and (.on | has("workflow_dispatch"))'
jqt "ci: perf-reports — только cron, ручной запуск и метка perf на PR" "$ci_wf" '.jobs["perf-reports"].if | test("schedule") and test("workflow_dispatch") and test("labels.*perf")'
jqt "ci: perf-reports запускает make perf-reports и сохраняет reports-latest.json" "$ci_wf" '([.jobs["perf-reports"].steps[] | .run? // ""] | join("\n") | test("make perf-reports")) and ([.jobs["perf-reports"].steps[] | .with.path? // ""] | join("\n") | test("reports-latest.json"))'
jqt "ci: cron не отменяется push в main (event в группе concurrency)" "$ci_wf" '.concurrency.group | test("github.event_name")'

readme="$ROOT/deploy/dokploy/README.md"
if [ -f "$readme" ]; then pass "deploy/dokploy/README.md есть"; else fail "deploy/dokploy/README.md есть"; fi
for s in DOKPLOY_URL DOKPLOY_API_KEY DOKPLOY_APP_ID_API DOKPLOY_APP_ID_INGEST DOKPLOY_APP_ID_WORKER DOKPLOY_APP_ID_WEB \
  TS_OAUTH_CLIENT_ID TS_OAUTH_SECRET GHCR_PULL_USER GHCR_PULL_TOKEN; do
  if grep -qF "$s" "$readme" 2>/dev/null; then pass "README: секрет $s"; else fail "README: секрет $s"; fi
done
for s in "deploy.yml -f ref=" "deploy.yml -f tag=" "geo-refresh.yml"; do
  if grep -qF -- "$s" "$readme" 2>/dev/null; then pass "README: команда «${s}»"; else fail "README: команда «${s}»"; fi
done

echo
if [ "$failures" -gt 0 ]; then
  echo "test-deploy: $failures из $total проверок упали"
  exit 1
fi
echo "test-deploy: все $total проверок прошли"
