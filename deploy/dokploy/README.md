# Dokploy: ручная настройка и деплой из GitHub Actions

> Простой путь — один Compose-сервис из [`docker-compose.yml`](docker-compose.yml) рядом с этим файлом: Dokploy сам
> собирает стек из Git. Так работает appmetrica.online; пошагово — в [README.ru.md](../../README.ru.md#развёртывание-в-dokploy).
> Ниже описан альтернативный конвейер: образы собирает GitHub Actions, а выкатывает их API Dokploy.

Как настроить production один раз и как потом выкатывать новые версии, откатывать их и обновлять гео-базу (T-024).
Сервер, firewall и установку Dokploy описывают `docs/discovery/2026-10-13-appmetrica-online/21-ops-runbooks.md` §5.5
и `12-dev-runbook.md` §9.5. Этот файл — про то, что нужно GitHub Actions: Applications, ключи и секреты.

## Как устроен деплой

```
push в main → CI (ci.yml) зелёный → Deploy (deploy.yml)
  plan    scripts/deploy-plan.sh: ref/tag → образы ghcr.io/<owner>/appmetrica-{backend,web}:<sha>, список сервисов
  build   сборка и push в GHCR (backend с DB-IP City Lite месяца GEO_MONTH внутри образа); при откате пропускается
  deploy  проверка образов в GHCR → tailscale/github-action (tag:ci) → scripts/dokploy-deploy.sh → smoke /health
  prune   в GHCR остаются 10 последних версий каждого образа
```

`scripts/dokploy-deploy.sh` выкатывает сервисы строго по порядку **api → ingest → worker → web**. Для каждого сервиса:

1. `POST $DOKPLOY_URL/api/application.saveDockerProvider` — новый образ и учётка GHCR.
2. `POST $DOKPLOY_URL/api/application.deploy`.
3. Ждёт, пока в `GET /api/deployment.all` этот деплой не станет `done`.
4. Ждёт ответа 200 от `/ready` (у web — от `/api/health`).

api идёт первым, потому что при старте применяет миграции. При первой же ошибке выкатка останавливается, и следующие сервисы остаются на прежней версии.

Geo DB refresh (`geo-refresh.yml`) запускается 2-го числа каждого месяца в 06:00 UTC. Он собирает `appmetrica-backend:<sha main>-geo-YYYYMM` с базой текущего месяца и через `deploy.yml` выкатывает только ingest.

## Один раз: Dokploy (через Tailscale, `http://appmetrica-prod:3000`)

1. **Registry**: Settings → Registry → Add.
   - Name `ghcr`, Registry URL `ghcr.io`.
   - Username — GitHub-логин владельца, Password — classic PAT со scope `read:packages` (тот же, что `GHCR_PULL_TOKEN`).
   - Нажать Test.
2. **Project `appmetrica`**: Postgres `postgres` и compose `clickhouse-stack` — по `21-ops-runbooks.md` §5.5, пункты 8–9.
3. **Applications** в проекте `appmetrica`. App Name задаётся ровно так, без суффикса: это DNS-имена внутри `dokploy-network`.

   | App Name | Provider → Docker image | Advanced → Run Command → Args | Порт / домен |
   |---|---|---|---|
   | `appmetrica-api` | `ghcr.io/<owner>/appmetrica-backend:main` | `api` | 8080 → `api.appmetrica.online` |
   | `appmetrica-ingest` | `ghcr.io/<owner>/appmetrica-backend:main` | `ingest` | 8081 → `i.appmetrica.online` |
   | `appmetrica-worker` | `ghcr.io/<owner>/appmetrica-backend:main` | `worker` | — (без домена) |
   | `appmetrica-web` | `ghcr.io/<owner>/appmetrica-web:main` | — | 3000 → `appmetrica.online` |

   - Роль задаётся одним аргументом в **Args**, поле **Command** остаётся пустым. Dokploy передаёт Command в Swarm как
     `ContainerSpec.Command`, а оно заменяет ENTRYPOINT образа (`/app/appmetrica`): с `Command: api` контейнер ищет
     несуществующий бинарник `api` и не стартует.
   - Тег `:main` нужен только для первого старта. Первый же запуск `deploy.yml` переключит Applications на `<sha>`.
   - Environment — матрица `12-dev-runbook.md` §3.3, значения из `.env.example`.
   - Domains: HTTPS, Let's Encrypt.
4. **Advanced → Swarm Settings** у каждого Go-сервиса.
   - Health Check (порт: api 8080, ingest 8081, worker 8082):
     ```json
     {"Test":["CMD","/app/appmetrica","healthcheck","--url","http://127.0.0.1:8080/health"],"Interval":10000000000,"Timeout":3000000000,"StartPeriod":20000000000,"Retries":3}
     ```
   - Update Config (у всех четырёх Applications): если новый контейнер не прошёл healthcheck, Swarm сам откатит сервис.
     ```json
     {"Parallelism":1,"Delay":10000000000,"FailureAction":"rollback","Order":"start-first"}
     ```
5. **Notifications → Telegram**: события App Deploy, App Build Error, Database Backup, Docker Cleanup, Dokploy Restart.
6. **API key**: Settings → Profile → API/CLI → Generate. Это `DOKPLOY_API_KEY`.
7. **ID приложений** — значения `DOKPLOY_APP_ID_*`. Команду выполнять из tailnet:
   ```bash
   curl -s -H "x-api-key: $DOKPLOY_API_KEY" http://appmetrica-prod:3000/api/project.all \
     | jq '..|objects|select(.applicationId? and .appName?)|{appName,applicationId}'
   ```

## Один раз: Tailscale

1. Access controls: тег `tag:ci` (владелец — `autogroup:admin`) и правило, по которому `tag:ci` видит только `appmetrica-prod:3000`.
   ```json
   "tagOwners": {"tag:ci": ["autogroup:admin"]},
   "grants": [{"src": ["tag:ci"], "dst": ["appmetrica-prod"], "ip": ["tcp:3000"]}]
   ```
2. Settings → OAuth clients → Generate: scopes `auth_keys` и `devices:core` (write) с тегом `tag:ci`. Client ID и secret — это `TS_OAUTH_CLIENT_ID` и `TS_OAUTH_SECRET`.
   Узлы CI эфемерные и удаляются после job.

## Один раз: GitHub

1. Settings → Environments → `production`.
   - Deployment branches: только `main`.
   - Required reviewers не нужны: деплой автоматический после зелёного CI.
2. Секреты окружения `production`. Значения не печатаются в логах: GitHub маскирует секреты, а скрипты не выводят ответы Dokploy.

   | Секрет | Значение |
   |---|---|
   | `DOKPLOY_URL` | `http://appmetrica-prod:3000` (адрес в tailnet, без `/` в конце) |
   | `DOKPLOY_API_KEY` | ключ из Dokploy → Settings → Profile → API/CLI |
   | `DOKPLOY_APP_ID_API` | `applicationId` приложения `appmetrica-api` |
   | `DOKPLOY_APP_ID_INGEST` | `applicationId` приложения `appmetrica-ingest` |
   | `DOKPLOY_APP_ID_WORKER` | `applicationId` приложения `appmetrica-worker` |
   | `DOKPLOY_APP_ID_WEB` | `applicationId` приложения `appmetrica-web` |
   | `TS_OAUTH_CLIENT_ID` | Tailscale OAuth client ID (тег `tag:ci`) |
   | `TS_OAUTH_SECRET` | Tailscale OAuth client secret |
   | `GHCR_PULL_USER` | GitHub-логин, которым Dokploy тянет образы |
   | `GHCR_PULL_TOKEN` | classic PAT со scope `read:packages` |

   ```bash
   for s in DOKPLOY_URL DOKPLOY_API_KEY DOKPLOY_APP_ID_API DOKPLOY_APP_ID_INGEST DOKPLOY_APP_ID_WORKER DOKPLOY_APP_ID_WEB \
            TS_OAUTH_CLIENT_ID TS_OAUTH_SECRET GHCR_PULL_USER GHCR_PULL_TOKEN; do
     gh secret set "$s" --env production   # значение вводится интерактивно и не попадает в историю shell
   done
   ```
3. **Пакеты GHCR** `appmetrica-backend` и `appmetrica-web` появятся после первой сборки.
   - Package settings → Manage Actions access → этот репозиторий с ролью **Admin**: без неё job `prune` не может удалять старые версии.
   - Видимость — Private.

## Деплой, откат, гео-база

```bash
# Собрать образы из ref (sha или ветки) и выкатить все сервисы (AC-024.1)
gh workflow run deploy.yml -f ref=$(git rev-parse HEAD)
gh run watch

# Откат на готовые образы прежнего sha без сборки (AC-024.2); образ должен быть среди 10 последних версий в GHCR
gh workflow run deploy.yml -f tag=<previous-sha>
gh run watch

# Только часть сервисов
gh workflow run deploy.yml -f tag=<sha> -f services=api,ingest

# Пересобрать backend с DB-IP текущего месяца и выкатить ingest (AC-024.3)
gh workflow run geo-refresh.yml
gh run view --log | grep dbip-city-lite-   # в логе: dbip-city-lite-YYYY-MM.mmdb.gz текущего месяца

# После любого деплоя
curl -fsS https://i.appmetrica.online/health && curl -fsS https://api.appmetrica.online/health
```

Ограничения входов (проверяет `scripts/deploy-plan.sh`, тесты — `scripts/test-deploy.sh`):

- нужно указать ровно один из `ref` и `tag`;
- `tag` — это 40-символьный sha или `<sha>-geo-YYYYMM`;
- тег `-geo-` есть только у backend, поэтому вместе с `web` его выкатить нельзя.

Быстрый откат без GitHub: Dokploy → Application → Deployments → Rollback.

## Если деплой упал

| Шаг | Причина | Что делать |
|---|---|---|
| plan | неверный `ref`/`tag`/`services` | текст ошибки в логе шага; исправить вход и перезапустить |
| Check images exist in GHCR | образа с таким тегом нет (удалён ретеншном или не собирался) | выбрать sha из 10 последних или запустить с `-f ref=<sha>` |
| Connect to tailnet | OAuth client отозван, нет тега `tag:ci` | перевыпустить client, обновить `TS_OAUTH_*` |
| Deploy services: `POST /api/... не выполнен` | неверный `DOKPLOY_API_KEY` или `DOKPLOY_APP_ID_*`, Dokploy недоступен | проверить ключ и ID командой `project.all` из tailnet |
| Deploy services: статус `error` | Dokploy не смог стянуть образ или запустить сервис | Dokploy → Application → Deployments → лог; проверить `GHCR_PULL_TOKEN` |
| Deploy services: `/ready не ответил 200` | новый контейнер не готов: ClickHouse, Postgres, миграции | логи сервиса в Dokploy; Swarm откатит контейнер, который не прошёл healthcheck |
| Smoke | сервис отвечает не 200 после выкатки | `21-ops-runbooks.md` §5.4: откат по `-f tag=<previous-sha>` |
