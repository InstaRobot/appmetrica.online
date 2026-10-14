# appmetrica.online

Внутренняя аналитика портфеля приложений (iOS/macOS, Android, сайты, боты, бэкенды) — замена Яндекс AppMetrica
без SDK. Клиенты шлют `session_start` / `session_end` по HTTP на `https://i.appmetrica.online/v1/events`,
владелец смотрит портфель, аудиторию, retention, realtime и профили в дашборде `https://appmetrica.online`
(вход по passkey) и читает те же данные через Read API `https://api.appmetrica.online/v1`.

- `backend/` — Go-модуль, один бинарник `appmetrica` (роли `ingest`, `api`, `worker` и служебные `migrate`, `admin`, `healthcheck`), ClickHouse 26.8 + PostgreSQL 18.6.
- `web/` — Next.js 16.4: дашборд и документация API за логином.
- `contracts/openapi/` — OpenAPI-контракты, из них генерируется код Go и TS.
- `deploy/` — стек ClickHouse + clickhouse-backup для Dokploy.
- `scripts/` — скрипты CI и деплоя, `scripts/reconcile` — сверка цифр с AppMetrica.

## Запуск за 5 минут

Нужны Go 1.27.2, Node 24.21.0, pnpm 12.11.2, Docker; для `make check` — golangci-lint 2.14, для перф-тестов и деплой-скриптов — `jq`.

```bash
make env          # .env из .env.example с рабочими dev-значениями
make install      # Go-модули и pnpm-зависимости
make dev-infra    # ClickHouse и Postgres в Docker, ждёт healthcheck
make dev          # миграции + ingest :8081, api :8080, worker :8082, web :3000
```

Если порт 5432 занят другим Postgres, поменяй `POSTGRES_HOST_PORT` и порт в `POSTGRES_URL` в `.env`.
Web требует именно Node 24 (`engines`): под Node 22 `pnpm typecheck` падает.

После `make dev`: `make seed` — демо-данные (3 приложения, 35 дней событий), `make passkey-link` — ссылка регистрации
passkey для первого входа, `make send-test-batch` — тестовый батч в локальный ingest.

Все команды — `make help`. Перед коммитом — `make check`. Проверка каркаса — `scripts/check-bootstrap.sh --infra`.

## Проверки

```bash
make check              # gen-check + линтеры + unit-тесты Go, web и скриптов
make test-integration   # интеграционные тесты Go (testcontainers, нужен Docker)
make coverage           # покрытие Go с гейтом ≥ 80 % для ключевых пакетов
make e2e                # Playwright; сначала make dev-infra migrate-up geo-db seed, затем run-ingest, run-api-e2e (APP_ENV=e2e) и run-worker
make perf-ingest        # k6: 500 rps × 10 мин (NFR-001); сначала make seed и make run-ingest-perf (лимит 20 000 событий/с), baseline — backend/perf/reports/baseline.json
make perf-reports       # отчёты на 100k DAU × 90 дней (NFR-003, ~16 ГБ RAM)
cd web && pnpm build && pnpm check:bundle   # бюджет initial JS 250 KB gzip (NFR-006)
```

## Сверка с AppMetrica

14 дней параллельной работы с AppMetrica до удаления SDK (NFR-010): DAU и новые ≤ 5 %, D1 ≤ 3 п.п.

```bash
AM_SK=sk_… make reconcile ARGS="-csv appmetrica-audience.csv -api https://api.appmetrica.online/v1 -app <app_id>"
```

Процедура и формат экспорта — [docs/reconciliation/README.md](docs/reconciliation/README.md).

## Документация

- Правила разработки (TDD, запреты, Validation Commands): [AGENTS.md](AGENTS.md)
- Discovery-пакет (PRD, архитектура, модель данных, контракты, UX): [docs/discovery/2026-10-13-appmetrica-online/](docs/discovery/2026-10-13-appmetrica-online/)
- Структура репозитория и «куда положить файл»: [07-project-structure.md](docs/discovery/2026-10-13-appmetrica-online/07-project-structure.md)
- Команды разработки: [12-dev-runbook.md](docs/discovery/2026-10-13-appmetrica-online/12-dev-runbook.md)
- Эксплуатация (деплой, бэкапы, алерты, инциденты): [21-ops-runbooks.md](docs/discovery/2026-10-13-appmetrica-online/21-ops-runbooks.md),
  ручные шаги Dokploy — [deploy/dokploy/README.md](deploy/dokploy/README.md)
- Документация API для интеграций: `https://appmetrica.online/docs` (за логином, исходники — `web/content/docs/`)
- План работ: [docs/plans/20261013-appmetrica-online.md](docs/plans/20261013-appmetrica-online.md)
