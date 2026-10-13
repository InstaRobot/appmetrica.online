# appmetrica.online

Внутренняя аналитика портфеля приложений (iOS/macOS, Android, сайты, боты, бэкенды) — замена Яндекс AppMetrica
без SDK. Клиенты шлют `session_start` / `session_end` по HTTP на `https://i.appmetrica.online/v1/events`,
владелец смотрит портфель, аудиторию, retention, realtime и профили в дашборде `https://appmetrica.online`
(вход по passkey) и читает те же данные через Read API `https://api.appmetrica.online/v1`.

- `backend/` — Go-модуль, один бинарник `appmetrica` (роли `ingest`, `api`, `worker`), ClickHouse 26.8 + PostgreSQL 18.6.
- `web/` — Next.js 16.4: дашборд и документация API за логином.
- `contracts/openapi/` — OpenAPI-контракты, из них генерируется код Go и TS.
- `deploy/` — стек ClickHouse + clickhouse-backup для Dokploy.

## Запуск за 5 минут

Нужны Go 1.27.2, Node 24.21.0, pnpm 12.11.2, Docker.

```bash
make env          # .env из .env.example с рабочими dev-значениями
make install      # Go-модули и pnpm-зависимости
make dev-infra    # ClickHouse и Postgres в Docker, ждёт healthcheck
make dev          # миграции + ingest :8081, api :8080, worker :8082, web :3000
```

Если порт 5432 занят другим Postgres, поменяй `POSTGRES_HOST_PORT` и порт в `POSTGRES_URL` в `.env`.

Все команды — `make help`. Перед коммитом — `make check`. Проверка каркаса — `scripts/check-bootstrap.sh --infra`.

## Документация

- Правила разработки (TDD, запреты, Validation Commands): [AGENTS.md](AGENTS.md)
- Discovery-пакет (PRD, архитектура, модель данных, контракты, UX): [docs/discovery/2026-10-13-appmetrica-online/](docs/discovery/2026-10-13-appmetrica-online/)
- Команды разработки: [12-dev-runbook.md](docs/discovery/2026-10-13-appmetrica-online/12-dev-runbook.md)
- План работ: [docs/plans/20261013-appmetrica-online.md](docs/plans/20261013-appmetrica-online.md)
