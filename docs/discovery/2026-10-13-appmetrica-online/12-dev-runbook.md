# Dev Runbook: appmetrica.online

**Цель**: пошаговое руководство от пустого репозитория до работающего production на Hetzner + Dokploy.
**Consumer**: разработчик проекта (Go, TS/React, Linux/Docker; ClickHouse — по runbook-ам).

> Каждый шаг — конкретная команда. Значения (пути, версии, имена переменных) совпадают с
> [04-tech-stack.md](./04-tech-stack.md), [05-data-model.md](./05-data-model.md), [06-api-contracts.md](./06-api-contracts.md),
> [07-project-structure.md](./07-project-structure.md).
> Эксплуатация ClickHouse, бэкапы и инциденты — в [21-ops-runbooks.md](./21-ops-runbooks.md).

---

## 0. Prerequisites

### 0.1. Installed tools
```bash
# Проверка версий (требуемые минимумы)
go version             # go1.27.x (go.mod: go 1.27, toolchain go1.27.2 — нужный toolchain скачается автоматически)
node --version         # v24.21.0 (Node 24 LTS «Krypton»; Node 25+ не поддерживается engines)
pnpm --version         # 12.11.2 (через corepack, см. ниже)
docker --version       # 27+ (Docker Desktop / OrbStack / Colima) — локальные ClickHouse и Postgres, testcontainers
git --version          # 2.40+
gh --version           # 2.60+ (GitHub CLI: PR, секреты)
golangci-lint --version  # 2.14.0
lefthook version       # 2.2.2
jq --version           # 1.7+ (скрипты деплоя и отладка API)
tailscale version      # 1.8x+ (доступ к панели Dokploy и SSH сервера)
```

Установка на macOS (Homebrew). Go-toolchain точной версии 1.27.2 скачается автоматически при первом `go build` благодаря
директиве `toolchain` в `backend/go.mod`; Node фиксируется на 24.21.0, pnpm — через corepack (в Node 24 он ещё встроен):
```bash
brew install go node@24 git gh golangci-lint lefthook jq tailscale
brew install --cask docker            # или: brew install orbstack
corepack enable
corepack prepare pnpm@12.11.2 --activate
```

### 0.2. Accounts & access
- [ ] GitHub — репозиторий (приватный), Actions включены, GHCR (GitHub Packages) доступен.
- [ ] Hetzner — аккаунт Robot (выделенный сервер AX102-1, FSN1) и Hetzner Console (Object Storage, локация fsn1).
- [ ] Регистратор домена `appmetrica.online` — доступ к DNS-записям (A/AAAA для `appmetrica.online`, `i.appmetrica.online`, `api.appmetrica.online`).
- [ ] Tailscale — tailnet владельца (бесплатный Personal-план), OAuth-клиент для CI с тегом `tag:ci`.
- [ ] Telegram — бот, созданный через @BotFather (токен), и chat_id владельца (через @userinfobot или `getUpdates`).
- [ ] healthchecks.io — бесплатный план Hobbyist (20 checks), интеграция Telegram.
- [ ] Sentry / PostHog / Clerk / Stripe — **не используются** (ошибки и метрики — slog + Prometheus-эндпоинты + Telegram-алерты worker; auth — собственные passkey; биллинга нет).

### 0.3. Secrets (хранить в менеджере паролей владельца)
- [ ] `POSTGRES_PASSWORD` (production) и итоговый `POSTGRES_URL`
- [ ] `CLICKHOUSE_ADMIN_PASSWORD`, `CLICKHOUSE_RW_PASSWORD`, `CLICKHOUSE_RO_PASSWORD`
- [ ] `KEY_HASH_PEPPER` (base64 от 32 байт: `openssl rand -base64 32`)
- [ ] `TELEGRAM_BOT_TOKEN`, telegram chat_id (вводится в UI настроек, S-12)
- [ ] `HEALTHCHECKS_PING_URL` (`https://hc-ping.com/<uuid>`)
- [ ] `S3_ACCESS_KEY`, `S3_SECRET_KEY` (Hetzner Object Storage, бакеты `appmetrica-ch-backup`, `appmetrica-pg-backup`)
- [ ] `DOKPLOY_API_KEY`, `DOKPLOY_URL`, `DOKPLOY_APP_ID_API`, `DOKPLOY_APP_ID_INGEST`, `DOKPLOY_APP_ID_WORKER`, `DOKPLOY_APP_ID_WEB`
- [ ] `TS_OAUTH_CLIENT_ID`, `TS_OAUTH_SECRET` (Tailscale OAuth для CI)
- [ ] `GHCR_PULL_USER`, `GHCR_PULL_TOKEN` (classic PAT со scope `read:packages` — Dokploy тянет приватные образы)

---

## 1. Scaffold repository

### 1.1. Create empty repo
Репозиторий уже существует — клонируй его:
```bash
git clone https://github.com/InstaRobot/appmetrica.online.git
cd appmetrica.online
```

### 1.2. Copy bootstrap files
> **N/A для этого проекта.** Причина: каркас (Makefile, конфиги, CI, Dockerfile-ы) уже лежит в репозитории.

### 1.3. First commit
Установи git hooks (lefthook): они проверяют формат, линтеры и Conventional Commits перед коммитом.
```bash
lefthook install
```

---

## 2. Install dependencies

```bash
make env                                   # .env из .env.example (рабочие dev-значения)
cd backend && go mod download && cd ..     # модули из go.sum
cd web && pnpm install && cd ..            # создаёт pnpm-lock.yaml — закоммитить; postinstall генерирует .source/
```

Ожидаемое: `pnpm install` ~30 с, `Done ... using pnpm v12.11.2`. Если pnpm ругается на `ERR_PNPM_IGNORED_BUILDS` или
`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION` — см. раздел 13.

Проверить:
```bash
cd backend && go build ./... && go vet ./... && cd ..   # пустой каркас компилируется
cd web && pnpm biome check . && pnpm typecheck && cd .. # конфиги валидны
golangci-lint config verify -c backend/.golangci.yml
```

---

## 3. Environment variables

### 3.1. Create `.env`
```bash
make env        # копирует .env.example → .env, если .env ещё нет
```

`.env` подключается в `Makefile` (`include .env` + `export`), поэтому каждая цель `make run-*` получает переменные.
Формат строго `KEY=value` без кавычек и пробелов.

### 3.2. Что значит каждая группа (полный список — `.env.example`)
- **Общие Go**: `APP_ENV`, `LOG_LEVEL`, `METRICS_LISTEN_ADDR`, `POSTGRES_URL`, `CLICKHOUSE_ADDR`, `CLICKHOUSE_DATABASE`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_MIGRATE_USER`, `CLICKHOUSE_MIGRATE_PASSWORD`.
- **ingest**: `INGEST_LISTEN_ADDR`, `GEOIP_DB_PATH`, `INGEST_MAX_BODY_BYTES`, `INGEST_MAX_EVENTS`, `INGEST_LATE_WINDOW`, `INGEST_FUTURE_TOLERANCE`, `RATE_APP_EVENTS_PER_SEC`, `RATE_APP_BURST`, `RATE_INSTALL_PER_MIN`, `RATE_INSTALL_BURST`, `KEY_CACHE_REFRESH`, `INGEST_ACCEPT_DEVELOPMENT` (kill switch для `environment=development`).
- **api**: `API_LISTEN_ADDR`, `PUBLIC_BASE_URL`, `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_ORIGINS`, `WEBAUTHN_RP_NAME`, `SESSION_TTL`, `SESSION_IDLE_TTL`, `KEY_HASH_PEPPER`, `MIGRATE_ON_START`, `API_PUBLIC_BEARER_ENABLED` (kill switch Bearer `sk_`), `TELEGRAM_BOT_TOKEN` (кнопка «Отправить тест» на S-12), `BACKUP_API_URL` (блок бэкапа в статусе системы S-12).
- **worker**: `WORKER_LISTEN_ADDR`, `WORKER_JOBS_ENABLED`, `PUBLIC_BASE_URL` (ссылка в дайджесте), `TELEGRAM_BOT_TOKEN`, `HEALTHCHECKS_PING_URL`, `BACKUP_API_URL`, `DISK_FREE_ALERT_PCT`, `INGEST_HEALTH_URL`, `API_HEALTH_URL`, `INGEST_METRICS_URL`; только для E2E-стенда — `MERGE_KEYS_WAIT` (ожидание шага `wait_keys` слияния приложений вместо `deletion.AppPurgeDelay`; допустима лишь при `APP_ENV=e2e`, иначе процесс не стартует; `make run-worker-e2e` задаёт `0`).
- **web**: `API_INTERNAL_URL`, `NEXT_PUBLIC_INGEST_URL`, `NEXT_PUBLIC_API_URL`.
- **Инфраструктура**: `CLICKHOUSE_ADMIN_PASSWORD`, `CLICKHOUSE_RW_PASSWORD`, `CLICKHOUSE_RO_PASSWORD`, `POSTGRES_PASSWORD`; clickhouse-backup: `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `CLICKHOUSE_USERNAME`, `BACKUPS_TO_KEEP_REMOTE`, `WATCH_INTERVAL`, `FULL_INTERVAL`.
- **Только локально**: `DEV_WRITE_KEY` (write-ключ, который создаёт `make seed`).

Пустые `TELEGRAM_BOT_TOKEN`, `HEALTHCHECKS_PING_URL`, `BACKUP_API_URL` локально — норма: worker пишет алерты и дайджест в лог и пропускает проверку бэкапа.

### 3.3. Production
В Dokploy переменные задаются во вкладке **Environment** каждого Application и compose-стека. Матрица «переменная → сервис»:

| Переменные | api | ingest | worker | web | clickhouse-stack |
|---|---|---|---|---|---|
| Общие Go (`APP_ENV=production`, `LOG_LEVEL=info`, `METRICS_LISTEN_ADDR=:9090`, `POSTGRES_URL`, `CLICKHOUSE_ADDR=clickhouse:9000`, `CLICKHOUSE_DATABASE`) | ✓ | ✓ | ✓ | — | — |
| `CLICKHOUSE_USER`/`CLICKHOUSE_PASSWORD` | `appmetrica_ro` | `appmetrica_rw` | `appmetrica_rw` | — | — |
| `CLICKHOUSE_MIGRATE_USER`/`CLICKHOUSE_MIGRATE_PASSWORD`, `MIGRATE_ON_START=true` | ✓ | — | — | — | — |
| Группа ingest | — | ✓ | — | — | — |
| Группа api (`PUBLIC_BASE_URL=https://appmetrica.online`, `WEBAUTHN_RP_ID=appmetrica.online`, `WEBAUTHN_RP_ORIGINS=https://appmetrica.online`, `TELEGRAM_BOT_TOKEN`, `BACKUP_API_URL=http://clickhouse-backup:7171`) | ✓ | — | — | — | — |
| Группа worker (`PUBLIC_BASE_URL=https://appmetrica.online`, `INGEST_HEALTH_URL=http://appmetrica-ingest:8081/health`, `API_HEALTH_URL=http://appmetrica-api:8080/health`, `INGEST_METRICS_URL=http://appmetrica-ingest:9090/metrics`, `BACKUP_API_URL=http://clickhouse-backup:7171`) | — | — | ✓ | — | — |
| `API_INTERNAL_URL` (build-arg; `web/Dockerfile` задаёт его и как ENV для `next start` — переопределять в Environment не нужно) | — | — | — | ✓ | — |
| Пароли ClickHouse, S3, `CLICKHOUSE_USERNAME`, `BACKUPS_TO_KEEP_REMOTE`, `WATCH_INTERVAL`, `FULL_INTERVAL` | — | — | — | — | ✓ |

---

## 4. Database setup

### 4.1. Provider-specific setup

**Локально (Docker)** — ClickHouse 26.8.25.14 и Postgres 18.6 из `docker-compose.dev.yml`:
```bash
make dev-infra       # docker compose up -d --wait (порты только на 127.0.0.1: 9000, 8123, 5432)
docker compose -f docker-compose.dev.yml ps
```
Пользователь `appmetrica_admin` (DDL, миграции) создаётся образом ClickHouse; `appmetrica_rw` и `appmetrica_ro` — из
`deploy/clickhouse/users.d/appmetrica.xml` (пароли из env через `from_env`). База `appmetrica` создаётся переменной `CLICKHOUSE_DB`.

**Production** — Postgres: managed DB Dokploy (App Name `postgres`, образ `postgres:18.6-trixie`, БД и пользователь
`appmetrica`); ClickHouse: compose-стек `clickhouse-stack` (раздел 9.5).

### 4.2. Run migrations (goose, Postgres + ClickHouse)
```bash
make migrate-up       # go run ./cmd/appmetrica migrate up — оба диалекта, миграции встроены в бинарник (embed.FS)
make migrate-status   # таблицы goose_db_version в Postgres и ClickHouse
```
Создать новую миграцию:
```bash
# NNNNN — следующий номер после последнего файла в каталоге (сейчас Postgres 00002, ClickHouse 00004)
# Postgres
printf -- '-- +goose Up\n\n-- +goose Down\n' > backend/migrations/postgres/NNNNN_<change>.sql
# ClickHouse (каждый DDL — отдельный блок; без транзакций)
printf -- '-- +goose NO TRANSACTION\n-- +goose Up\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n' > backend/migrations/clickhouse/NNNNN_<change>.sql
make gen-sqlc         # если менялась схема Postgres
```
**Правило**: миграции только expand/contract — новая колонка добавляется с DEFAULT, старая удаляется отдельной миграцией
после того, как код перестал её читать. Старые миграции не редактируются. `ALTER ... MODIFY COLUMN` в ClickHouse запускает
мутацию всей таблицы — такие миграции выполняются отдельным релизом по [21-ops-runbooks.md](./21-ops-runbooks.md).

### 4.3. Seed initial data
```bash
make seed
```
`backend/cmd/seed` создаёт в Postgres 4 приложения («Seed iOS» ios, «Seed Android» android, «Seed Site» web и
«Seed Notes» ios+android+web, TZ `Europe/Moscow`), write-ключ `DEV_WRITE_KEY` для «Seed iOS» и пишет в ClickHouse напрямую (минуя ingest и окно 7 дней)
35 дней событий: ~400 новых установок в день, сессии по кривой удержания D1≈35 %, D7≈15 %, 20 % установок с `user_id`,
5 % установок со вторым устройством того же `user_id` на той же платформе; у «Seed Notes» около 5 % людей с одним `user_id` на iOS и Web. Повторный запуск идемпотентен (детерминированный seed RNG; флаг `--seed <uint64>`, по умолчанию 1). Остальные флаги — `--apps`, `--days`, `--daily-installs`, `--write-key`.

---

## 5. Run local dev

### 5.1. Start dev server
Решение: **без live-reload (air)** для Go. Разработка идёт через TDD — цикл «тест → код» не требует запущенного
сервера, а одна зависимость меньше; после правки Go-кода сервис перезапускается `Ctrl+C` + `make dev` (сборка ~3 с).
```bash
make dev
```
`make dev` = `dev-infra` → `migrate-up` → `geo-db` → параллельно `run-ingest`, `run-api`, `run-worker`, `run-web`.
Ожидаемый вывод (в `APP_ENV=development` логи текстовые, в production — JSON; у каждого процесса ещё одна строка `listening` для порта метрик):
```
time=… level=INFO msg=listening process=ingest version=dev addr=[::]:8081
time=… level=INFO msg=listening process=api version=dev addr=[::]:8080
time=… level=INFO msg="jobs enabled" process=worker version=dev jobs=system_check,digest,close_day,deletion,session_cleanup,app_merge
time=… level=INFO msg=listening process=worker version=dev addr=[::]:8082
▲ Next.js 16.4.0
- Local:    http://localhost:3000
✓ Ready in 2.1s
```
Отдельные процессы (в разных терминалах): `make run-ingest`, `make run-api`, `make run-worker`, `make run-web`.
Порты: ingest 8081 (метрики 9091), api 8080 (9092), worker 8082 (9093), web 3000.

### 5.2. Проверить health
- [ ] `curl -s localhost:8081/health` → `{"status":"ok"}`; `curl -s localhost:8081/ready` → 200
- [ ] `curl -s localhost:8080/health` → `{"status":"ok"}`; `curl -s localhost:8080/ready` → 200
- [ ] `curl -s localhost:3000/api/health` → `{"status":"ok"}`
- [ ] `http://localhost:3000` → редирект на `/login`
- [ ] Логи процессов без `ERROR`; в логах ingest нет IP-адресов (`grep -E '([0-9]{1,3}\.){3}[0-9]{1,3}'` по выводу — пусто, кроме адресов слушателей)

### 5.3. Run parallel services (если нужно)
Отправить тестовый батч в ingest (после `make seed`, ключ из `DEV_WRITE_KEY`):
```bash
make send-test-batch
# ожидаемо: {"accepted":2,"rejected":[],"warnings":[],"server_time":"2026-..."}
```
Ручной запрос с gzip и проверкой частичного отказа:
```bash
cat > /tmp/batch.json <<'EOF'
{"sent_at":"2026-10-13T12:00:00.000Z",
 "context":{"platform":"android","os_version":"16","app_version":"2.3.0","environment":"development"},
 "events":[
  {"insert_id":"7b0c3a7e-6f39-4c41-9a51-0a4c7f1d2b11","name":"session_start","timestamp":"2026-10-13T11:59:58.000Z",
   "install_id":"5d3f6f0e-2c7b-4d1e-8a9f-3b2c1d0e9f8a","session_id":"0f8e7d6c-5b4a-4392-8170-6f5e4d3c2b1a"},
  {"insert_id":"8c1d4b8f-7a40-4d52-8b62-1b5d8e2e3c22","name":"session_start","timestamp":"2026-10-13T11:59:59.000Z",
   "install_id":"5d3f6f0e-2c7b-4d1e-8a9f-3b2c1d0e9f8a","session_id":"1a2b3c4d-5e6f-4789-8abc-def012345678","user_id":"ivan@example.com"}
 ]}
EOF
gzip -c /tmp/batch.json > /tmp/batch.json.gz
curl -sS -X POST localhost:8081/v1/events -H "X-App-Key: $(grep DEV_WRITE_KEY .env | cut -d= -f2)" \
  -H "Content-Type: application/json" -H "Content-Encoding: gzip" --data-binary @/tmp/batch.json.gz | jq
# ожидаемо: accepted=1, rejected[0].code="user_id_looks_like_pii", rejected[0].index=1, retryable=false
```

Отладка ClickHouse (`make ch` открывает clickhouse-client от `appmetrica_admin`):
```sql
-- события за сегодня по приложениям и типам
SELECT app_id, name, environment, count() FROM appmetrica.events WHERE event_date = today() GROUP BY ALL ORDER BY ALL;
-- последние отказы ingest с причинами
SELECT received_at, code, field, message, left(payload_sample, 200) FROM appmetrica.rejected_events ORDER BY received_at DESC LIMIT 20;
-- связки install → первый владелец
SELECT install_id, argMinMerge(first_user) AS user, min(first_linked_at) FROM appmetrica.identity_links GROUP BY install_id LIMIT 20;
-- асинхронные вставки ingest (ошибки и задержки)
SELECT event_time, status, exception, rows FROM system.asynchronous_insert_log WHERE database = 'appmetrica' ORDER BY event_time DESC LIMIT 20;
-- медленные запросы api за час
SELECT query_duration_ms, read_rows, formatReadableSize(memory_usage), left(query, 160) FROM system.query_log
WHERE type = 'QueryFinish' AND event_time > now() - INTERVAL 1 HOUR AND user = 'appmetrica_ro' ORDER BY query_duration_ms DESC LIMIT 10;
-- размер таблиц и сжатие
SELECT table, formatReadableSize(sum(data_compressed_bytes)) c, formatReadableSize(sum(data_uncompressed_bytes)) u, sum(rows)
FROM system.parts WHERE database = 'appmetrica' AND active GROUP BY table;
```
Postgres: `make psql`, затем `\dt`, `SELECT id, name, platform, status FROM apps;`.

---

## 6. First feature — smoke test

Проверь, что окружение разработки работает:

1. `cd backend && go test -race -count=1 ./... && golangci-lint run ./...` — без ошибок.
2. `make gen-check` — без диффа.
3. `cd web && pnpm biome check . && pnpm typecheck && pnpm test` — зелёные.
4. `docker build --build-arg GEO_MONTH=$(date -u +%Y-%m) -t appmetrica-backend:dev backend` — образ собирается; `docker run --rm appmetrica-backend:dev healthcheck --url http://127.0.0.1:1` завершается с кодом 1.
5. `make dev`, затем `make seed` и `make passkey-link` — дашборд на `http://localhost:3000` открывается с демо-данными.

Изменения дальше вносятся по [CONTRIBUTING.md](../../../CONTRIBUTING.md): сначала падающий тест, затем реализация.

---

## 7. Testing setup

### 7.1. Unit / integration
```bash
make test-go            # Go unit: go test -race -count=1 ./... (без Docker, < 30 с)
make test-integration   # Go integration: -tags=integration, testcontainers ClickHouse 26.8.25.14 + Postgres 18.6
make coverage           # покрытие ≥ 80 % для internal/{ingest,reports,auth,identity,deletion} — падает ниже порога
make test-web           # Vitest 5 (jsdom, MSW 3)
cd web && pnpm test:coverage   # пороги web: lines/statements/functions 70 %, branches 60 %
```
Интеграционные тесты отчётов загружают NDJSON-фикстуры из `backend/testdata/fixtures/` и сравнивают ответ с
`*.golden.json`. Обновить golden-файл после осознанного изменения определения метрики:
`cd backend && go test -tags=integration ./internal/reports/ -run TestRetention -update` (флаг `-update` реализуется в `internal/testutil`).

### 7.2. E2E (Playwright)
```bash
cd web && pnpm exec playwright install chromium   # один раз
make dev-infra migrate-up geo-db                  # инфраструктура
make seed                                         # «Seed iOS», «Seed Notes» и др.: данные для audience, retention, realtime и multi-platform
make run-ingest & make run-api-e2e & make run-worker-e2e &  # backend в фоне; api и worker с APP_ENV=e2e, MERGE_KEYS_WAIT=0 для слияния
make e2e                                          # pnpm build && pnpm e2e (webServer поднимает next start)
cd web && pnpm e2e:ui                             # интерактивный режим
```
`e2e/global-setup.ts` сбрасывает auth-базу (`POST /internal/e2e/reset-auth`), получает ссылку из
`POST /internal/e2e/passkey-link` (оба маршрута есть только при `APP_ENV=e2e`), регистрирует passkey через
виртуальный аутентификатор Chrome DevTools Protocol (`WebAuthn.addVirtualAuthenticator`, `ctap2`, `internal`,
`hasResidentKey`, `isUserVerified`) и сохраняет `e2e/.auth/owner.json`. Сброс удаляет владельца и passkey в базе
`POSTGRES_URL` api: чтобы не терять локального владельца, запускай E2E на отдельной базе. Если порты 3000/8080 заняты,
укажи `E2E_PORT` (порт web; тот же origin — в `PUBLIC_BASE_URL` и `WEBAUTHN_RP_ORIGINS` api), `E2E_API_URL` и
`API_INTERNAL_URL` (адрес api для сборки и для `next start`).
`realtime.spec.ts` отправляет `session_start` в ingest (`E2E_INGEST_URL`, по умолчанию `http://localhost:8081`)
write-ключом «Seed iOS» (`E2E_WRITE_KEY`, по умолчанию `DEV_WRITE_KEY` из `.env.example`) и ждёт событие в ленте
S-07: ingest должен писать в те же базы Postgres и ClickHouse, что читает api. Без `make geo-db` ingest можно
запустить с синтетической базой `GEOIP_DB_PATH=backend/testdata/geo/test-city.mmdb`.
`app-onboarding.spec.ts` создаёт приложение, копирует curl-сниппет S-10 и выполняет его в bash (нужны `curl` и
`uuidgen`), поэтому web собирается с `NEXT_PUBLIC_INGEST_URL`, равным `E2E_INGEST_URL` (адрес сниппета вшивается
при `pnpm build`). Новый write-ключ ingest видит после перезагрузки кэша (`KEY_CACHE_REFRESH`, 15 с): спека
повторяет сниппет до принятого события.

### 7.3. Type check
```bash
cd web && pnpm typecheck     # next typegen && tsc --noEmit (TS 6.0.3 strict)
cd backend && go vet ./...
```

### 7.4. Lint & format
```bash
make lint      # golangci-lint run ./... + pnpm biome check . + pnpm typecheck
make fmt       # golangci-lint fmt (gofumpt + goimports) + pnpm format
```

### 7.5. All checks at once (Validation Commands — перед каждым коммитом)
```bash
make gen-check
cd backend && go test -race -count=1 ./... && golangci-lint run ./... && cd ..
cd web && pnpm biome check . && pnpm typecheck && pnpm test && cd ..
```
Дополнительно, если тикет затрагивает SQL (ClickHouse/Postgres): `cd backend && go test -race -count=1 -tags=integration ./... && cd ..`.
Дополнительно, если тикет затрагивает UI-флоу: `make e2e`. Короткая форма всего базового набора — `make check`.

---

## 8. Git workflow

### 8.1. Branching model
- `main` — production (защищена: merge только через PR с зелёным CI; deploy запускается после CI на `main`).
- `feat/T-NNN-<slug>` — фича по тикету; `fix/T-NNN-<slug>` — исправление; `chore/<slug>` — инфраструктура и зависимости.
- Ветки живут < 48 часов (trunk-based). Незавершённая функциональность в `main` скрывается переменной окружения
  `FEATURE_<NAME>=false` в `internal/config` (feature flags на уровне env: один владелец, отдельный сервис флагов не нужен — # Deviation от baseline «feature flags service»).

### 8.2. Commit convention
Conventional Commits `<type>(<scope>): <subject>` (проверяет хук `commit-msg` из `lefthook.yml`):
- `feat(ingest): reject events older than late window`
- `fix(reports): count retention cohorts in app timezone`
- `test(identity): add first-owner golden fixture`
- `chore(deps): bump clickhouse-go to v2.49.0`
- `ci(deploy): wait for api readiness before ingest`
В теле коммита — ссылка на тикет: `Refs: T-012`.

### 8.3. PR workflow
```bash
git switch -c feat/T-012-ingest-time-correction
# TDD: тест → реализация → рефакторинг
make check
git add -A
git commit -m "feat(ingest): correct client clock using sent_at"
git push -u origin feat/T-012-ingest-time-correction
gh pr create --fill --body "Refs: T-012"
gh pr checks --watch
```

### 8.4. Merge
- Squash merge в `main` (`gh pr merge --squash --delete-branch`).
- После merge CI собирает образы `ghcr.io/<owner>/appmetrica-backend:<sha>` и `appmetrica-web:<sha>`, затем workflow Deploy выкатывает их.
- Теги релизов не обязательны: версия = git sha (виден в логах старта процессов).

---

## 9. Deploy pipeline

### 9.1. Preview deployments
> **N/A для этого проекта.** Причина: один владелец и один production-стенд; Dokploy не поддерживает preview для compose,
> а preview Applications потребовали бы второго ClickHouse. Замена — E2E в CI на полном стеке (job `e2e`) и `development`
> environment событий. (# Deviation от baseline «preview deployments на каждый PR».)

### 9.2. Production deploy
**Trigger**: push в `main` → workflow `CI` (gen-check, backend, web, e2e) → по успеху workflow `Deploy` (сборка образов `<sha>` в GHCR, затем деплой; ручные шаги — `deploy/dokploy/README.md`):
1. Подключение к tailnet (`tailscale/github-action@v4.2.0`, тег `tag:ci`).
2. Для каждого сервиса: `POST $DOKPLOY_URL/api/application.saveDockerProvider` (новый образ) → `POST $DOKPLOY_URL/api/application.deploy`.
3. Порядок: `api` (выполняет миграции при старте, ожидание `https://api.appmetrica.online/ready`) → `ingest`
   (ожидание `https://i.appmetrica.online/ready`) → `worker` → `web` (ожидание `https://appmetrica.online/api/health`).
4. Swarm rolling update `start-first` с healthcheck; при провале healthcheck Swarm сам откатывает сервис (`FailureAction: rollback`).

**Manual deploy** конкретного тега:
```bash
gh workflow run deploy.yml -f ref=<sha>                              # собрать образы из ref и выкатить
gh workflow run deploy.yml -f tag=<sha> -f services=api,ingest      # выкатить готовые образы без сборки
gh run watch
```

### 9.3. Database migrations on deploy
Миграции Postgres и ClickHouse выполняет **api при старте** (`MIGRATE_ON_START=true`, пользователь ClickHouse
`CLICKHOUSE_MIGRATE_USER=appmetrica_admin`) до открытия порта. Поэтому api деплоится первым, а ingest и worker
стартуют уже на новой схеме. **Правило**: миграции обратно совместимы (expand → deploy → contract), иначе откат кода сломает старую версию.

### 9.4. Rollback
**Option 1 (Dokploy UI, быстрый)**: Tailscale → `http://<tailscale-имя-сервера>:3000` → Project `appmetrica` → Application → Deployments → Rollback на предыдущий образ.

**Option 2 (GitHub Actions по тегу)**:
```bash
git log --oneline -5                       # найти предыдущий рабочий sha
gh workflow run deploy.yml -f tag=<prev_sha>   # образ должен быть среди 10 последних версий в GHCR
```

**Option 3 (git revert)**: `git revert <sha> && git push` → обычный CI/Deploy.

**Option 4 (база данных)**: откат миграций в production не выполняется (`goose down` запрещён) — вместо него новая
миграция, возвращающая схему. Восстановление данных — из бэкапа по [21-ops-runbooks.md](./21-ops-runbooks.md).

### 9.5. Первичная настройка сервера и Dokploy (один раз)
1. **Сервер**: Hetzner Robot → заказать AX102-1 (FSN1). Rescue → `installimage`: Ubuntu 24.04, software RAID 1 на двух NVMe, hostname `appmetrica-prod`.
2. **Firewall** (Robot → Firewall, stateless): in TCP 80, in TCP 443, in UDP 41641 (Tailscale), in TCP с флагом ACK на порты 32768–65535, in ICMP; всё остальное — drop. Включить фильтрацию IPv6. Порт 22 и 3000 снаружи закрыты.
3. **Tailscale на сервере** (пока SSH ещё доступен через rescue/консоль): `curl -fsSL https://tailscale.com/install.sh | sh && tailscale up --ssh --hostname appmetrica-prod`. Дальше доступ только `ssh root@appmetrica-prod` через tailnet. Создать пользователя `deploy` (не root) для повседневной работы.
4. **Dokploy**: `curl -sSL https://dokploy.com/install.sh | sh`; открыть `http://appmetrica-prod:3000` через tailnet, создать администратора, включить 2FA (TOTP), Settings → отключить автообновление; убедиться, что версия ≥ v0.30.8.
5. **DNS** у регистратора: `A` и `AAAA` для `appmetrica.online`, `i.appmetrica.online`, `api.appmetrica.online` → IPv4/IPv6 сервера, TTL 300.
6. **Hetzner Object Storage** (fsn1): бакеты `appmetrica-ch-backup` и `appmetrica-pg-backup` (versioning включён), ключи доступа S3.
7. **Dokploy → Registry**: GHCR (`ghcr.io`, `GHCR_PULL_USER`, `GHCR_PULL_TOKEN`) → Test.
8. **Dokploy → Project `appmetrica` → Database → Postgres**: App Name `postgres`, образ `postgres:18.6-trixie`, database/user `appmetrica`; Backups → S3 Destination `appmetrica-pg-backup`, ежедневно 03:00, хранить 30.
9. **Compose `clickhouse-stack`**: источник Git (этот репозиторий, ветка `main`), Compose Path `deploy/clickhouse/compose.yaml`; Environment — пароли ClickHouse и переменные S3 из раздела 3.3 → Deploy. Проверить: `docker exec <clickhouse-контейнер> clickhouse-client --user appmetrica_admin --password ... --query 'SELECT version()'` → `26.8.25.14`.
10. **Applications** (`appmetrica-api`, `appmetrica-ingest`, `appmetrica-worker`, `appmetrica-web`): источник Docker, образ `ghcr.io/<owner>/appmetrica-backend:main` (web — `appmetrica-web:main`), App Name ровно как указано (это DNS-имя внутри `dokploy-network`), команда-аргумент роли `api` / `ingest` / `worker`; Environment по матрице раздела 3.3; Advanced → Swarm Settings:
    - Health Check (пример для api, для ingest порт 8081, для worker 8082):
      `{"Test":["CMD","/app/appmetrica","healthcheck","--url","http://127.0.0.1:8080/health"],"Interval":10000000000,"Timeout":3000000000,"StartPeriod":20000000000,"Retries":3}`
    - Update Config: `{"Parallelism":1,"Delay":10000000000,"FailureAction":"rollback","Order":"start-first"}`
    - Domains: api → `api.appmetrica.online` порт 8080; ingest → `i.appmetrica.online` порт 8081; web → `appmetrica.online` порт 3000; HTTPS Let's Encrypt (HTTP-01). worker — без домена.
11. **Dokploy → Notifications → Telegram**: события App Deploy, App Build Error, Database Backup, Docker Cleanup, Dokploy Restart.
12. **Dokploy API key**: Settings → Profile → API/CLI → Generate; `applicationId` каждого приложения: `curl -s -H "x-api-key: $KEY" http://appmetrica-prod:3000/api/project.all | jq '..|objects|select(.applicationId?)|{name,applicationId}'`.
13. **Tailscale ACL**: `tag:ci` разрешён доступ только к `appmetrica-prod:3000`; OAuth client с scope `devices:core` (write) и тегом `tag:ci`.
14. **GitHub → Settings → Environments → production → Secrets**: все секреты CI из раздела 0.3.
15. **healthchecks.io**: check `appmetrica-worker`, period 5 мин, grace 10 мин, интеграция Telegram; URL → `HEALTHCHECKS_PING_URL` в worker.

### 9.6. Обновление Dokploy и ротация секретов
**Обновление Dokploy** (вручную, автообновление выключено): раз в 2 недели или в течение 48 часов после security-релиза
(GitHub Security Advisories `Dokploy/dokploy`). Порядок: прочитать release notes → Dokploy → Settings → Backups (dokployBackup в S3)
→ Update → проверить, что порт 3000 по-прежнему закрыт снаружи (`nc -zv <публичный IP> 3000` с внешней машины → timeout;
известная проблема Dokploy issue #2661 — firewall Hetzner защищает независимо от неё).

**Ротация секретов**:
| Секрет | Как ротировать | Последствие |
|---|---|---|
| `KEY_HASH_PEPPER` | новое значение в api → redeploy | все `sk_`-ключи перестают работать: перевыпустить в S-12 |
| `CLICKHOUSE_RW_PASSWORD` / `CLICKHOUSE_RO_PASSWORD` | compose Environment → redeploy стека → обновить env ingest/worker/api → redeploy | минута ошибок 503 у ingest (клиенты ретраят) |
| `CLICKHOUSE_ADMIN_PASSWORD` | `ALTER USER appmetrica_admin IDENTIFIED BY '...'` → compose env + api `CLICKHOUSE_MIGRATE_PASSWORD` + backup env | — |
| `POSTGRES_PASSWORD` | Dokploy → postgres → смена пароля → `POSTGRES_URL` в api/ingest/worker → redeploy | — |
| `S3_*` | новый ключ в Hetzner → env clickhouse-stack и Dokploy S3 Destination → удалить старый ключ | — |
| `TELEGRAM_BOT_TOKEN` | @BotFather → /revoke → env worker → redeploy | — |
| `DOKPLOY_API_KEY`, `GHCR_PULL_TOKEN`, `TS_OAUTH_*` | перевыпуск → `gh secret set <NAME> --env production` | — |
| Write-ключ `pk_` приложения | S-11: создать новый → выпустить версии приложений с новым ключом → отозвать старый, когда доля старых версий < 1 % | старые версии приложений получают 401 после отзыва |

---

## 10. Observability — setup

### 10.1. Errors (вместо Sentry)
> # Deviation: внешний error tracker и OpenTelemetry-коллектор в MVP не используются (один сервис-владелец, бюджет внимания).
> Ошибки — структурированные логи `slog` JSON с `request_id`, уровнем `ERROR` и кодом ошибки из конверта `Error`;
> контрольные точки — Telegram-алерты worker (`insert_errors`, `ingest_down`, `api_down`). Подключение OTel — open question в
> [10-open-questions.md](./10-open-questions.md).

Проверка после деплоя: `curl -s -X POST https://i.appmetrica.online/v1/events -d '{' -H 'X-App-Key: pk_invalid'` → `401`
с `request_id`; в Dokploy → appmetrica-ingest → Logs видна строка с тем же `request_id`, без IP.

### 10.2. Метрики
Каждый Go-процесс отдаёт Prometheus-метрики на `:9090/metrics` (внутри сети Dokploy, наружу не публикуется). Ключевые:
`ingest_requests_total{status}`, `ingest_events_accepted_total{app_id}`, `ingest_events_rejected_total{code}`, `ingest_warnings_total{code}`,
`ingest_batch_duration_seconds`, `ingest_clickhouse_insert_duration_seconds`, `ingest_clickhouse_insert_errors_total`, `ingest_rate_limited_total{scope}`,
`ingest_platform_missing_total` (события мультиплатформенного приложения без платформы, записанные как `other`),
`ingest_key_cache_size`, `ingest_key_cache_last_refresh_timestamp_seconds`, `api_request_duration_seconds{operation}`, `api_clickhouse_query_duration_seconds`,
`api_auth_failures_total{reason}`, `worker_job_duration_seconds{job}`, `worker_job_failures_total{job}`, `worker_alerts_firing{key}`,
`worker_telegram_send_errors_total`, `merge_copied_events_total` и `merge_step_duration_seconds{step}` (job `app_merge`). Worker читает метрики ingest для алерта `insert_errors`.
Локально: `curl -s localhost:9091/metrics | grep ingest_`.

### 10.3. Logs
Production: Dokploy → Application → Logs (stdout контейнеров, JSON). С сервера: `docker service logs appmetrica-ingest --since 30m | jq -c 'select(.level=="ERROR")'`.
Логи ClickHouse — `docker logs <clickhouse-контейнер>`, уровень `warning` (`config.d/appmetrica.xml`).

### 10.4. Uptime monitoring
- healthchecks.io dead-man: worker пингует `HEALTHCHECKS_PING_URL` после каждого успешного `system_check` (каждые 5 мин); тишина > 10 мин → Telegram. Ловит «упал весь сервер».
- worker `system_check` → Telegram: `disk_free` (< 20 %), `insert_errors` (> 1 % за 5 мин), `backup_stale` (> 26 ч), `ingest_down`, `api_down`, `clickhouse_down`.
- Dokploy notifications → Telegram: деплои, ошибки сборки, бэкапы Postgres.

---

## 11. Billing setup (если используем Stripe)

> **N/A для этого проекта.** Причина: внутренний инструмент одного владельца, платных тарифов и оплаты нет (PRD, out of scope).

### 11.1. Products & prices
> N/A. Причина: биллинга нет — продукт используется только владельцем, тарифов, цен и Stripe-продуктов не существует.

### 11.2. Webhook endpoint
> N/A. Причина: входящих вебхуков платёжных провайдеров нет; единственные внешние вызовы — Telegram Bot API и healthchecks.io (исходящие).

### 11.3. Test flow
> N/A. Причина: биллинга нет.

---

## 12. Auth setup (passkey, без внешнего провайдера)

### 12.1. Первый вход локально
```bash
make dev                      # или make run-api + make run-web
make passkey-link             # печатает http://localhost:3000/setup/<token> (действует 15 минут, одноразовая)
```
Открыть ссылку в Chrome → «Название устройства» (например «MacBook Touch ID») → Touch ID / системный диалог → редирект на `/`.
Без биометрии: Chrome DevTools → More tools → WebAuthn → Enable virtual authenticator environment → Add (ctap2, internal, resident key, user verification).
Важно: открывать именно `http://localhost:3000`, не `127.0.0.1` — RP ID `localhost` должен совпадать с хостом.

### 12.2. Production: первый passkey и восстановление
```bash
# через Tailscale SSH на сервере
docker exec -it $(docker ps -q -f name=appmetrica-api | head -1) /app/appmetrica admin passkey-link
# → https://appmetrica.online/setup/<token>
```
Та же команда используется для восстановления, если все passkey потеряны. Рекомендуется сразу зарегистрировать минимум два
passkey (телефон + ноутбук) в S-12 → Passkeys; удалить последний passkey нельзя (`409 last_passkey`).

### 12.3. Сессии и защита
- Cookie `am_session`: HttpOnly, Secure (кроме `APP_ENV=development`), SameSite=Lax, host-only, 30 дней абсолютно / 7 дней простоя.
- Next.js `src/proxy.ts` редиректит на `/login`, если cookie нет; валидность проверяет api.
- Изменяющие запросы `/admin/v1/*` требуют заголовок `X-Requested-With: appmetrica-web` (`403 csrf_check_failed` без него).

### 12.4. Test flow
1. `make passkey-link` → регистрация → портфель (пустой).
2. Выйти (меню → «Выйти») → `/login` → «Войти с passkey» → портфель.
3. Повторно открыть старую setup-ссылку → `410 setup_token_expired` («Ссылка недействительна»).
4. `curl -s localhost:8080/v1/apps` без авторизации → `401 unauthenticated`.

---

## 13. Common issues / troubleshooting

| Issue | Fix |
|---|---|
| `make dev` / `make migrate-up`: `POSTGRES_URL is required` | `.env` создаётся целью `env`, но читается Makefile только при следующем запуске: выполни `make env`, затем снова `make dev` |
| `docker compose up` падает: `port 9000 already allocated` | Порт занят (другой ClickHouse, php-fpm). Остановить процесс (`lsof -i :9000`) или поменять публикацию в `docker-compose.dev.yml` и `CLICKHOUSE_ADDR` |
| ingest отвечает `401 invalid_app_key` сразу после `make seed` | Кэш ключей перезагружается раз в `KEY_CACHE_REFRESH=15s` — подожди 15 с или перезапусти `make run-ingest` |
| Все события `rejected` с `event_too_old` | В `timestamp` нет смещения часового пояса или неверный `sent_at`: формат строго RFC 3339 с `Z`/`+03:00`; окно — 7 дней от времени приёма |
| Passkey на localhost: `SecurityError` / «RP ID is not a registrable domain suffix» | `WEBAUTHN_RP_ID=localhost`, `WEBAUTHN_RP_ORIGINS=http://localhost:3000`, открывать `localhost`, а не `127.0.0.1` |
| Safari не сохраняет сессию на localhost | Safari не принимает `Secure` cookie по http: держи `APP_ENV=development` или используй Chrome |
| `go: unknown command "oapi-codegen"` / `go tool` не находит | `cd backend && go mod download`; требуется Go ≥ 1.24 (tool-директивы), проект — go1.27.2 |
| `make gen-sqlc`: `permission denied` на `internal/pgstore/sqlc` | Контейнер sqlc запускается с `-u $(id -u):$(id -g)`; удали каталог, созданный ранее от root: `sudo rm -rf backend/internal/pgstore/sqlc` |
| `pnpm install`: `ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION` | pnpm 12 не ставит версии моложе 24 часов. Подожди сутки после релиза или выбери предыдущую версию; политику не отключать |
| `pnpm install`: `ERR_PNPM_IGNORED_BUILDS` | Новая зависимость со скриптом сборки: проверь пакет и добавь `<name>: true` в `allowBuilds` (`web/pnpm-workspace.yaml`) |
| В production дашборд ходит на `localhost:8080` | `API_INTERNAL_URL` нужен и при `next build` (`rewrites`), и при `next start` (server components, `/docs`): образ web собран без `--build-arg API_INTERNAL_URL=http://appmetrica-api:8080` — пересобрать (CI передаёт build-arg, `web/Dockerfile` прокидывает его в ENV стадии run) |
| `make test-integration`: `Cannot connect to the Docker daemon` | Запусти Docker Desktop/OrbStack; для Colima: `export DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock` |
| В событиях пустые `country`/`city` локально | Нет гео-базы: `make geo-db`; `GEOIP_DB_PATH=.geo/dbip-city-lite.mmdb` — путь относительно `backend/`. Приватные IP (`10.*`, `192.168.*`) страну не дают — передай `context.ip` с публичным адресом |
| Отчёт падает `Memory limit exceeded` (`appmetrica_ro`, 8 ГБ) | Сузить период (≤ 366 дней) или фильтр; проверить, что запрос фильтрует по `app_id` и `event_date` (первые ключи сортировки). Подробно — [21-ops-runbooks.md](./21-ops-runbooks.md) |
| Typecheck web падает после изменения контракта | `make gen-web` (orval) и закоммить `web/src/lib/api/gen` |
| Deploy: `curl: (7) Failed to connect to <DOKPLOY_URL>` | Tailscale-шаг не подключился: проверь `TS_OAUTH_*`, ACL `tag:ci → appmetrica-prod:3000`, что `DOKPLOY_URL` = `http://appmetrica-prod:3000` (MagicDNS) |
| Deploy: `401` от Dokploy API | Перевыпусти `DOKPLOY_API_KEY` (Settings → Profile → API/CLI) и обнови секрет `gh secret set DOKPLOY_API_KEY --env production` |
| Новый контейнер api постоянно откатывается Swarm-ом | Healthcheck не проходит: смотри логи (обычно ошибка миграции или недоступен ClickHouse). Миграцию чинить новой миграцией, не правкой старой |

---

## 14. First deploy checklist

- [ ] CI зелёный на `main` (gen-check, backend, web, e2e), Deploy собрал образы `appmetrica-backend:<sha>` и `appmetrica-web:<sha>` в GHCR
- [ ] Сервер AX102-1: RAID 1 (`cat /proc/mdstat` → `[UU]`), Tailscale подключён, порт 22 и 3000 снаружи закрыты (`nc -zv <IP> 22` → timeout)
- [ ] Dokploy ≥ v0.30.8, 2FA включена, автообновление выключено, Telegram-уведомления настроены
- [ ] Postgres (managed) работает, бэкап в `appmetrica-pg-backup` выполнен вручную один раз (Backups → Run)
- [ ] `clickhouse-stack` работает: ClickHouse `26.8.25.14`, `clickhouse-backup` отвечает `curl -s http://clickhouse-backup:7171/backup/list/remote` изнутри сети, первый полный бэкап появился в S3
- [ ] api стартовал, миграции применены (`appmetrica migrate status` через `docker exec`)
- [ ] Env во всех Applications по матрице раздела 3.3 (`APP_ENV=production`, верные ClickHouse-пользователи)
- [ ] DNS: `dig +short i.appmetrica.online api.appmetrica.online appmetrica.online` → IP сервера; HTTPS-сертификаты выданы
- [ ] `curl -s https://i.appmetrica.online/health`, `https://api.appmetrica.online/health`, `https://appmetrica.online/api/health` → 200
- [ ] Первый passkey зарегистрирован через `admin passkey-link`, второй passkey добавлен в S-12
- [ ] Telegram: в S-12 указан chat_id, «Отправить тестовое сообщение» доходит; дайджест включён (09:00)
- [ ] healthchecks.io: check `appmetrica-worker` в статусе up
- [ ] Создано первое приложение (S-04), сниппет из S-10 отправлен curl-ом → статус «Получено первое событие»
- [ ] Smoke на production: события видны в Realtime ≤ 10 с, портфель показывает приложение, Read API с `sk_`-ключом отвечает `GET /v1/apps`
- [ ] Запущена 14-дневная сверка с AppMetrica (NFR-010) по [08-roadmap.md](./08-roadmap.md)

---

## 15. Next steps — handoff

После того как runbook выполнен и smoke-тест прошёл:
→ правила разработки (TDD, проверки перед коммитом, запреты) — [CONTRIBUTING.md](../../../CONTRIBUTING.md);
→ установка на сервер через Docker Compose — [README.ru.md](../../../README.ru.md);
→ эксплуатация после запуска (бэкапы, восстановление, диск, инциденты) — [21-ops-runbooks.md](./21-ops-runbooks.md).

---

## Changelog

| Date | Version | Changes |
|---|---|---|
| 2026-10-13 | 1.0 | Первая версия runbook после discovery |
| 2026-10-15 | 1.1 | Мультиплатформенные приложения и слияние: переменная worker `MERGE_KEYS_WAIT` (только `APP_ENV=e2e`, §3), `app_merge` в логе `jobs enabled` (§5), метрики `ingest_platform_missing_total`, `merge_copied_events_total`, `merge_step_duration_seconds{step}` (§10.2) |
