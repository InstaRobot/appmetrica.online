# AGENTS.md — правила для AI-агентов и разработчиков appmetrica.online

> `CLAUDE.md` — симлинк на этот файл (`ln -s AGENTS.md CLAUDE.md`). Правь только `AGENTS.md`.

## 1. Что это за проект

appmetrica.online — внутренняя система аналитики приложений одного владельца (замена Яндекс AppMetrica без SDK).
Клиенты (iOS, Android, веб, боты) шлют системные события `session_start` / `session_end` по HTTP на
`https://i.appmetrica.online/v1/events`; владелец смотрит портфель, аудиторию, retention, realtime и профили
в дашборде `https://appmetrica.online` и читает те же данные через Read API `https://api.appmetrica.online/v1`.

Источник правды по требованиям — discovery-пакет `docs/discovery/2026-10-13-appmetrica-online/`:

| Вопрос | Документ |
|---|---|
| Что строим и зачем, FR-/NFR-ID | `01-prd.md`, `02-tech-spec.md` |
| Компоненты, потоки данных, sequence-диаграммы | `03-architecture.md` |
| Версии всех зависимостей | `04-tech-stack.md` |
| Схемы Postgres и ClickHouse, SQL отчётов | `05-data-model.md` |
| HTTP-контракты (OpenAPI) | `06-api-contracts.md`, `contracts/openapi/*.yaml` |
| Куда класть файлы | `07-project-structure.md` |
| Экраны, токены, состояния | `14-ux-ui-spec.md` |
| Тикеты и порядок работ | `13-tickets.md`, `docs/plans/20261013-appmetrica-online.md` |
| Команды разработки и деплоя | `12-dev-runbook.md`, `21-ops-runbooks.md` |
| Стратегия тестирования | `22-test-strategy.md` |

## 2. Раскладка репозитория (кратко)

- `contracts/openapi/ingest.yaml`, `contracts/openapi/api.yaml` — OpenAPI 3.0.3, единственный источник HTTP-контрактов.
- `backend/` — Go-модуль `appmetrica.online/backend`, один бинарник `appmetrica` (подкоманды `ingest`, `api`, `worker`, `migrate`, `admin`, `healthcheck`).
- `web/` — Next.js 16.4 (App Router), дашборд и документация (Fumadocs).
- `deploy/` — compose-стек ClickHouse + clickhouse-backup для Dokploy и конфиги ClickHouse.
- `scripts/` — shell-скрипты CI и деплоя (с тестами `scripts/test-*.sh`) и отдельный Go-модуль `scripts/reconcile` (сверка с AppMetrica, процедура — `docs/reconciliation/README.md`).
- Сгенерированный код: `backend/internal/gen/**`, `backend/internal/pgstore/sqlc/**`, `web/src/lib/api/gen/**`. Руками не редактируется, перегенерируется `make gen` и коммитится.

## 3. Обязательный процесс работы (TDD)

1. Возьми тикет из `13-tickets.md` (или задачу из ralphex-плана). Прочитай его acceptance criteria и указанные секции discovery-доков.
2. **Сначала тест.** Напиши падающий тест по `test_path` из тикета (Go: `*_test.go` рядом с кодом; интеграционные — с build-тегом `integration`; web: `*.test.ts(x)` рядом с компонентом; E2E — `web/e2e/*.spec.ts`). Запусти и убедись, что он падает по ожидаемой причине.
3. Минимальная реализация до зелёного теста. Затем рефакторинг при зелёных тестах.
4. Если меняется HTTP-контракт: сначала правишь `contracts/openapi/*.yaml`, затем `make gen`, затем код и тесты.
5. Если меняется схема БД: новая миграция goose (`backend/migrations/{postgres,clickhouse}/NNNNN_snake_case.sql`), старые миграции не редактируются никогда.
6. Перед коммитом выполни Validation Commands (раздел 4). Все должны пройти.
7. Коммит по Conventional Commits: `<type>(<scope>): <subject>` на английском, scope — имя пакета или экрана (`feat(ingest): reject events older than late window`). Типы: feat, fix, refactor, test, docs, chore, ci, build, perf, revert. В теле — ID тикета (`Refs: T-012`). Тест и реализация — разными коммитами: сначала `test(<scope>): …`, потом `feat`/`fix`. CI-job `tdd-order` (`scripts/check-tdd-order.sh`) падает, если первый коммит с кодом пакета раньше первого коммита с его тестом; локально — `make tdd-order BASE=origin/main`. Метку `tdd-exception` (hotfix) ставит только владелец.
8. Один тикет — одна ветка `feat/T-012-short-slug` — один PR не больше ~500 строк диффа (без сгенерированного кода).

## 4. Validation Commands (выполнять перед каждым коммитом)

```bash
make gen-check            # сгенерированный код соответствует contracts/ и queries/
cd backend && go test -race -count=1 ./... && golangci-lint run ./... && cd ..
cd web && pnpm biome check . && pnpm typecheck && pnpm test && cd ..
```

Короткая форма всего набора — `make check` (gen-check + линтеры, включая `scripts/reconcile`, + unit-тесты Go, web и скриптов). Если тикет трогает `scripts/**` — обязательно `make test-scripts` (bash-тесты CI-скриптов и деплоя, `go test` в `scripts/reconcile`); если трогает PII-guard ingest — `make fuzz` (CI гоняет 30 с).

Если тикет затрагивает SQL ClickHouse или Postgres — дополнительно интеграционные тесты (нужен Docker):

```bash
cd backend && go test -race -count=1 -tags=integration ./... && cd ..
```

Если тикет затрагивает UI-флоу — дополнительно E2E: `make e2e`. Цель собирает web и запускает Playwright, а сервисы должны быть уже запущены: `make dev-infra migrate-up geo-db seed` (демо-данные нужны `audience.spec.ts`, `retention.spec.ts`, `realtime.spec.ts`), затем `make run-ingest`, `make run-api-e2e` (`APP_ENV=e2e`, сбрасывает владельца и passkey в базе!) и `make run-worker` (см. `12-dev-runbook.md`).

Если тикет добавляет клиентский код или зависимость в web — дополнительно бюджет initial JS: `cd web && pnpm build && pnpm check:bundle` (≤ 250 KB gzip на страницу дашборда).

## 5. Жёсткие запреты

- **Не сохранять IP-адрес клиента нигде**: ни в Postgres, ни в ClickHouse, ни в логах, ни в метриках, ни в текстах ошибок. IP используется только в памяти для гео-поиска в `internal/geo` и сразу выбрасывается. `http.Request.RemoteAddr` читается только в `backend/internal/httpx/clientip.go` (линтер forbidigo это проверяет).
- **Не логировать** тела запросов ingest целиком, значения `user_id`, ключи `pk_`/`sk_` (только `key_prefix`), cookie `am_session`.
- **Не редактировать руками** `backend/internal/gen/**`, `backend/internal/pgstore/sqlc/**`, `web/src/lib/api/gen/**`.
- **Не добавлять зависимости** (go.mod, package.json), которых нет в `04-tech-stack.md`. Если без новой зависимости не обойтись — сначала добавь строку в `04-tech-stack.md` с версией, причиной и альтернативами, затем зависимость, и отметь это в описании PR.
- **Не использовать** `'use cache'` / Cache Components на страницах с данными аутентифицированного владельца (весь `(dashboard)`), только для статического контента документации.
- **Не принимать** ключ `pk_`/`sk_` в query string. Только заголовок `X-App-Key` / поле тела `app_key` (ingest) и `Authorization: Bearer` (Read API).
- **Не менять** определения метрик (active, new, sessions, retention day-N classic, active_now) без изменения `05-data-model.md` и golden-фикстур в `backend/testdata/fixtures/`.
- **Не использовать** anti-pattern стек: Jest, ESLint, axios, moment.js, lodash целиком, styled-components, redux-saga, class-компоненты React, `node:latest`.
- **Не коммитить** `.env`, дампы БД, `*.mmdb` (исключение — синтетическая `backend/testdata/geo/test-city.mmdb` из `go run ./testdata/gen geo`), сгенерированные отчёты покрытия.

## 6. Стиль кода

Go:
- Go 1.27, `gofumpt` + `goimports` (local prefix `appmetrica.online/backend`), golangci-lint v2 (`backend/.golangci.yml`).
- Ошибки оборачиваются через `fmt.Errorf("context: %w", err)`, проверяются `errors.Is/As`. Никаких `panic` в обработчиках запросов.
- Логирование только `log/slog` (JSON в production). Поля: `request_id`, `app_id`, `key_prefix`, `code`. Никаких `fmt.Print*`.
- Время берётся только из `internal/clock.Clock` (в тестах — фиксированные часы).
- Контекст `context.Context` — первый аргумент всех функций, делающих I/O.
- Тесты: стандартный `testing` + `testify/require`; table-driven; интеграционные тесты с testcontainers-go под тегом `//go:build integration`.

TypeScript:
- TS 6.0.3 strict + `noUncheckedIndexedAccess` + `exactOptionalPropertyTypes`. `any` запрещён (Biome `noExplicitAny: error`).
- Biome 2.5 — единственный линтер и форматтер. Импорты через алиас `@/` (`web/src/*`).
- Server Components по умолчанию; `'use client'` только для интерактивных компонентов (графики, фильтры, формы, polling).
- Данные с бэкенда — только через сгенерированный orval-клиент из `@/lib/api/gen`. Ручные `fetch` к API запрещены (исключение — `src/lib/api/fetcher.ts`).
- Состояние фильтров и периода — в URL через `nuqs`.
- Тексты интерфейса — на русском, прямо в коде (i18n не используется).

## 7. Где спрашивать, если неясно

1. `10-open-questions.md` — возможно, вопрос уже записан и у него есть дефолт.
2. Если решения нет — не выдумывай: выбери вариант, который не ломает контракты и схему, запиши его в описание PR в раздел «Допущения» и добавь строку в `10-open-questions.md`.

## 8. Закономерности, найденные при реализации MVP

- **Node 24.21.0 обязателен** (`engines` в `web/package.json`). Под Node 22 `pnpm typecheck` падает с `MODULE_NOT_FOUND …/.source/source.config.mjs` — это не поломка Fumadocs, а версия Node.
- **`make test` экспортирует `.env`**, поэтому web-тесты видят локальные `NEXT_PUBLIC_*`. `web/src/test/setup.ts` обнуляет `NEXT_PUBLIC_API_URL` и `NEXT_PUBLIC_INGEST_URL` перед каждым тестом. Не читай эти переменные на уровне модуля: только внутри функций.
- **Бюджет initial JS почти исчерпан** (страницы дашборда держатся около 220–250 KB gzip). Recharts, Radix Popover, cmdk, react-day-picker, sonner и zod подключаются только через ленивые обёртки (`components/common/lazy-*.tsx`, `components/reports/lazy-charts.tsx`, динамический импорт `lib/forms/schemas.ts`). Новая клиентская зависимость в любом маршруте может сдвинуть чужую страницу за бюджет: Turbopack считает экспорты общих модулей по всему клиентскому графу. Не включай `experimental.turbopackModuleFragments`: он добавляет 25–40 KB.
- **Web-переменные на этапе сборки**: `API_INTERNAL_URL` нужен и при `pnpm build`, и при `next start`; `NEXT_PUBLIC_INGEST_URL` вшивается при сборке (сниппеты S-10). Порты E2E-стенда переопределяются через `E2E_PORT`, `E2E_API_URL`, `E2E_INGEST_URL`.
- **Lighthouse CI** (`pnpm lhci autorun`) запускается с системным Chrome через `CHROME_PATH`: Chrome for Testing из кэша Playwright даёт `NO_FCP`.
- **Новый пакет `backend/internal/*`** требует правила в `internal/archtest/rules.go` и строки в таблице `07-project-structure.md §5`, иначе `go test ./...` падает.
- **Новая публичная операция Read API** требует запроса `# @name <operationId>` в `contracts/http/requests.http` (проверяет `internal/api/spec_test.go`).
- **Сниппеты MDX проверяются тестами**: код в `web/content/docs/**` извлекает `internal/docscheck` (Go — `go vet`, curl — запрос в ingest под тегом `integration`). Правишь рецепт — гоняй `go test ./internal/docscheck/...` с тегом и без.
- **Golden-файлы** обновляются флагом `-update` только локально и коммитятся вместе с причиной изменения. `testdata/fixtures/scenario.ndjson` руками не правится, его генерирует `go run ./testdata/gen -seed 42`.
- **Интеграционные тесты пакета** делят контейнеры через `main_integration_test.go` (TestMain). `golangci-lint` линтует их и без флага: `build-tags: [integration]` задан в `backend/.golangci.yml`.
- **Задачи worker с побочными эффектами** (close_day, system_check) берут advisory lock Postgres (`pgstore`): второй экземпляр worker пропускает цикл, а не дублирует алерты и закрытие дня.
- **Допущения** по ходу тикета записываются в `10-open-questions.md` следующим `OQ-NNN`, а при расхождении с discovery-доком правится сам документ (строка в его Changelog).
- **Две темы (макет «Dashboard v2», 2026-10-14).** Цвета — только семантические токены из `web/src/app/globals.css`: значения заданы и в `:root`, и в `:root[data-theme="dark"]`, контраст пар проверяет `globals.test.ts` (новая пара текст/фон — добавь её в `TEXT_PAIRS`). Не пиши `text-white`, hex и `dark:`-хаки в компонентах: вариант `dark:` привязан к `data-theme`, сырая статусная палитра доступна утилитам через `@theme inline reference`.
- **«/» — публичный лендинг, портфель — `/portfolio`.** Ссылки «в портфель» и редиректы после входа (`safeNext`) ведут на `/portfolio`.
- **Демо-режим (cookie `am_demo=1`)** обслуживает `apiFetch` из `web/src/lib/demo` без сети. Новая GET-операция, которую вызывает UI, должна появиться и в демо-датасете (контракт-тест zod в `src/lib/demo/*.test.ts`), иначе экран в демо получит 404; мутации в демо всегда 403 `demo_read_only`.
