# Project Structure: appmetrica.online

**Версия**: 1.0 | **Дата**: 2026-10-13

> Файловая и модульная структура монорепозитория: Go-бэкенд (`backend/`), Next.js-дашборд с документацией (`web/`),
> контракты (`contracts/`) и инфраструктура (`deploy/`). Документ объясняет, **почему** структура именно такая,
> и отвечает на вопрос «куда положить новый файл» без угадывания. Источник имён — канонический якорь пакета:
> пути из этого документа используются в [13-tickets.md](./13-tickets.md), [12-dev-runbook.md](./12-dev-runbook.md)
> и bootstrap kit [00-bootstrap/](./00-bootstrap/README.md).

---

## 1. Высокоуровневое дерево

```
appmetrica.online/
├── AGENTS.md                         # правила для AI-агентов (TDD, Validation Commands, запреты)
├── CLAUDE.md -> AGENTS.md            # симлинк, отдельно не редактируется
├── README.md                         # что это, как запустить за 5 минут, ссылки на discovery
├── Makefile                          # единая точка входа: dev, gen, test, lint, e2e, seed, passkey-link
├── lefthook.yml                      # git hooks (fmt/lint/commit-msg/pre-push tests)
├── .editorconfig  .gitignore  .env.example
├── docker-compose.dev.yml            # локальные ClickHouse 26.8.25.14 + Postgres 18.6
├── .github/workflows/
│   ├── ci.yml                        # backend → web → e2e → images (GHCR)
│   ├── deploy.yml                    # Dokploy API через Tailscale, ручной откат по тегу
│   └── geo-refresh.yml               # ежемесячная пересборка ingest со свежей DB-IP
├── contracts/openapi/
│   ├── ingest.yaml                   # OAS 3.0.3, POST /v1/events (i.appmetrica.online)
│   └── api.yaml                      # OAS 3.0.3, /v1 (Read API) + /admin/v1 (x-internal: true)
├── backend/                          # Go-модуль appmetrica.online/backend → один бинарник appmetrica
├── web/                              # Next.js 16.4: дашборд (S-01…S-12, S-14) + документация (S-13)
├── deploy/
│   ├── clickhouse/                   # compose-стек Dokploy «clickhouse-stack» + config.d / users.d
│   ├── clickhouse-backup/config.yml  # инкрементальные бэкапы в Hetzner Object Storage
│   └── dokploy/README.md             # ручные шаги в UI Dokploy (приложения, домены, healthcheck JSON)
└── docs/
    ├── discovery/2026-10-13-appmetrica-online/   # этот пакет
    ├── plans/20261013-appmetrica-online.md       # ralphex-план
    └── adr/                                      # Architecture Decision Records (MADR)
```

**Почему монорепозиторий без pnpm workspace.** В репозитории ровно один TS-пакет (`web/`) и один Go-модуль
(`backend/`). Workspace-инструменты (Turborepo, pnpm workspaces, go.work) решают задачу многих пакетов и
здесь только добавили бы конфигурацию. Связь между частями — контракты в `contracts/openapi/`, из которых
генерируется и Go-сервер, и TS-клиент; один `Makefile` в корне оркестрирует обе части.

**Почему контракты в отдельном каталоге.** OpenAPI — единственный источник правды для трёх потребителей:
oapi-codegen (Go), orval (TS-клиент, Zod, MSW-моки) и Fumadocs (страницы API reference). Если бы спека жила
внутри `backend/` или `web/`, одна из сторон зависела бы от путей другой.

---

## 2. `web/src/app/` — Next.js App Router

```
web/src/app/
├── layout.tsx                    # <html lang="ru">, шрифт Inter (next/font, subsets latin+cyrillic), globals.css
├── globals.css                   # Tailwind 4 (@import "tailwindcss"; @theme — токены из 14-ux-ui-spec)
├── not-found.tsx                 # S-14: 404
├── error.tsx                     # S-14: граница ошибок сегмента (500)
├── global-error.tsx              # фатальная ошибка корневого layout
├── api/
│   └── health/route.ts           # GET /api/health → {"status":"ok"} (healthcheck контейнера web; приоритет над rewrite)
├── (auth)/                       # публичные страницы без шапки дашборда
│   ├── layout.tsx                # центрированная карточка
│   ├── login/page.tsx            # S-01: вход по passkey (кнопка + conditional UI)
│   └── setup/[token]/page.tsx    # S-02: регистрация passkey по одноразовой ссылке
├── (dashboard)/                  # всё, что требует сессии владельца
│   ├── layout.tsx                # AppHeader, QueryClientProvider, NuqsAdapter, футер с атрибуцией DB-IP
│   ├── page.tsx                  # S-03: портфель
│   ├── loading.tsx               # skeleton портфеля
│   ├── settings/page.tsx         # S-12: вкладки Passkeys / API-ключи / Telegram / Система / Удаления
│   └── apps/
│       ├── new/page.tsx          # S-04: создание приложения
│       └── [appId]/
│           ├── layout.tsx        # AppTabs (Аудитория, Retention, Realtime, Профили, Интеграция, Настройки)
│           ├── loading.tsx
│           ├── error.tsx         # 404 app_not_found → «Приложение не найдено»
│           ├── page.tsx          # S-05: аудитория
│           ├── retention/page.tsx          # S-06
│           ├── realtime/page.tsx           # S-07
│           ├── profiles/page.tsx           # S-08
│           ├── profiles/[personKey]/page.tsx   # S-09
│           ├── integration/page.tsx        # S-10
│           └── settings/page.tsx           # S-11
└── (docs)/
    └── docs/
        ├── layout.tsx            # Fumadocs DocsLayout; серверная проверка сессии (redirect /login)
        └── [[...slug]]/page.tsx  # S-13: MDX-гайды + API reference (fumadocs-openapi)
```

**Правила**:
- Route groups `(auth)`, `(dashboard)`, `(docs)` дают разные layouts без влияния на URL. Пути экранов — строго из таблицы S-01…S-14 якоря.
- `src/proxy.ts` (Next.js 16, бывший middleware) только проверяет **наличие** cookie `am_session` и редиректит на `/login?next=<path>`; валидность сессии проверяет Go (`401 unauthenticated` → клиентский редирект на `/login`).
- `page.tsx` — Server Component: читает `searchParams` (период, фильтры), отдаёт данные в клиентские компоненты отчётов. Запросы к API из серверных компонентов идут на `API_INTERNAL_URL` с пробросом cookie (через `src/lib/api/fetcher.ts`).
- `'use cache'` в `(dashboard)` и `(auth)` запрещён: данные персональные и зависят от cookie. Cache Components включены (`cacheComponents: true`), поэтому все динамические чтения (`cookies()`, `searchParams`) находятся под `Suspense` (`loading.tsx`).
- Route handlers в `app/api/*` не проксируют бизнес-логику: всё API — в Go, доступно через rewrite `/api/:path*` → `http://appmetrica-api:8080/:path*` (секция `afterFiles`, поэтому `app/api/health` имеет приоритет).
- Server Actions не используются: все мутации — вызовы `/admin/v1/*` через сгенерированные TanStack Query мутации. Причина — один источник бизнес-логики (Go) и одна модель авторизации (cookie + CSRF-заголовок `X-Requested-With: appmetrica-web`).

---

## 3. `web/src/components/` — React компоненты

```
web/src/components/               # имена и props — источник истины 14-ux-ui-spec.md §Component inventory
├── ui/                           # shadcn/ui (Radix: `npx shadcn@4.21.4 init -b radix`), копируются CLI
│   ├── button.tsx  card.tsx  table.tsx  tabs.tsx  dialog.tsx  alert-dialog.tsx  dropdown-menu.tsx
│   ├── select.tsx  popover.tsx  calendar.tsx  command.tsx  input.tsx  label.tsx  badge.tsx  skeleton.tsx
│   └── tooltip.tsx  separator.tsx  switch.tsx  toggle-group.tsx  sonner.tsx  chart.tsx
├── layout/
│   ├── app-header.tsx            # логотип, AppSwitcher, «Портфель», «Документация», «Настройки», меню выхода
│   ├── app-switcher.tsx          # выпадающий список приложений (listApps)
│   ├── app-tabs.tsx              # вкладки приложения: Аудитория, Retention, Realtime, Профили, Интеграция, Настройки
│   ├── page-header.tsx           # заголовок страницы + слот действий
│   ├── app-footer.tsx            # атрибуция «IP Geolocation by DB-IP» (CC BY 4.0) со ссылкой
│   ├── skip-link.tsx             # «Перейти к содержимому» (WCAG 2.4.1)
│   └── logo.tsx
├── reports/                      # строительные блоки отчётов (знают домен аналитики, не знают HTTP)
│   ├── portfolio-table.tsx       # S-03
│   ├── kpi-card.tsx  delta.tsx  sparkline.tsx  unit-badge.tsx
│   ├── metric-chart.tsx          # S-05: timeseries + compare_points (Recharts 3.10.1 через shadcn chart)
│   ├── rolling-active-chart.tsx  # S-05: DAU/WAU/MAU
│   ├── period-picker.tsx  granularity-select.tsx  compare-toggle.tsx  filter-bar.tsx  environment-switch.tsx
│   ├── breakdown-table.tsx       # S-05
│   ├── retention-heatmap.tsx     # S-06: HTML-таблица с цветовой шкалой, sticky-шапка
│   └── retention-average-chart.tsx
├── realtime/
│   └── realtime-counter.tsx  per-minute-chart.tsx  event-feed.tsx          # S-07
├── profiles/
│   └── profile-search.tsx  profile-card.tsx  installs-table.tsx  sessions-table.tsx  deletion-status-badge.tsx  # S-08, S-09
├── integration/
│   └── snippet-tabs.tsx  waiting-for-first-event.tsx  hourly-ingest-chart.tsx  rejection-reasons-table.tsx  versions-health-table.tsx  # S-10
├── apps/
│   └── app-settings-form.tsx  write-keys-table.tsx                          # S-04, S-11
├── settings/
│   └── passkeys-tab.tsx  api-keys-tab.tsx  telegram-tab.tsx  system-tab.tsx  deletions-tab.tsx  # S-12
├── auth/
│   └── passkey-button.tsx  setup-passkey-form.tsx                           # S-01, S-02 (@simplewebauthn/browser 14.0.0)
├── common/
│   └── copy-field.tsx  code-block.tsx  platform-icon.tsx  timezone-select.tsx
├── feedback/
│   └── empty-state.tsx  error-state.tsx  confirm-delete-dialog.tsx  archived-banner.tsx
├── skeletons/                    # скелетоны каждого экрана (loading-состояния из 14-ux-ui-spec)
└── icons/platform/
    └── apple.tsx  android.tsx  web.tsx  bot.tsx  server.tsx  desktop.tsx  other.tsx  # свои SVG: в lucide-react 1.x нет бренд-иконок
```

**Правила**:
- `ui/` — только примитивы shadcn. Генерируются `pnpm dlx shadcn@4.21.4 add <name>`, после чего правятся под токены. Домена не знают.
- `reports/`, `realtime/`, `profiles/`, `integration/`, `apps/`, `settings/`, `auth/`, `layout/`, `common/`, `feedback/` — доменные компоненты. Получают данные пропсами или через хуки из `src/hooks/`; прямых вызовов API внутри них нет, кроме мутаций, оформленных сгенерированными хуками orval.
- Тест рядом с компонентом: `retention-heatmap.tsx` + `retention-heatmap.test.tsx`.
- Компонент, используемый одной страницей и меньше ~80 строк, живёт рядом со страницей (`app/(dashboard)/apps/[appId]/realtime/pause-button.tsx`), а не в `components/`.

---

## 4. `web/src/lib/` — утилиты и интеграции

```
web/src/lib/
├── api/
│   ├── fetcher.ts                # РУКОПИСНЫЙ mutator orval: базовый URL (браузер → "/api", сервер → API_INTERNAL_URL),
│   │                             # credentials: "include", X-Requested-With для мутаций, разбор конверта Error, 401 → /login
│   └── gen/                      # СГЕНЕРИРОВАНО orval 8.42.0 — не редактировать
│       ├── endpoints/            # TanStack Query хуки по тегам OpenAPI + *.msw.ts моки
│       ├── model/                # типы схем (AudienceSummary, RetentionMatrix, Portfolio, …)
│       └── zod/                  # Zod-схемы (*.zod.ts) для валидации форм и ответов
├── query-client.ts               # фабрика QueryClient (staleTime 30 с, retry 1, без retry на 4xx)
├── period.ts                     # пресеты периода → {from, to} в TZ приложения, лимиты 366 / 7 дней
├── search-params.ts              # парсеры nuqs: from, to, granularity, compare, environment, unit, фильтры
├── person-key.ts                 # encode/decode `u:<user_id>` | `i:<install_id>` для URL
├── format.ts                     # Intl.NumberFormat("ru-RU"), проценты, длительность, даты в TZ приложения
├── retention-color.ts            # доля 0..1 → класс цветовой шкалы heatmap
├── snippets.ts                   # шаблоны сниппетов интеграции с подстановкой write-ключа и NEXT_PUBLIC_INGEST_URL
├── webauthn.ts                   # адаптер ответов go-webauthn → optionsJSON для @simplewebauthn/browser
└── utils.ts                      # cn() (реэкспорт из пакета `cn`, используется shadcn)
```

**Правила**:
- `lib/` — чистые функции и клиенты. JSX здесь нет.
- Каждый модуль `lib/*.ts` (кроме `api/gen`) покрыт unit-тестом `*.test.ts`; порог покрытия Vitest — 70 % строк.
- Переменные окружения web читаются только в `next.config.ts` и `src/lib/api/fetcher.ts` (`API_INTERNAL_URL`), а также в `src/lib/snippets.ts` (`NEXT_PUBLIC_INGEST_URL`, `NEXT_PUBLIC_API_URL`) и `src/lib/copy-as-curl.ts` (`NEXT_PUBLIC_API_URL`).

---

## 5. `backend/` — серверный код Go

```
backend/
├── go.mod  go.sum  Dockerfile  .dockerignore  .golangci.yml  sqlc.yaml  oapi-ingest.cfg.yaml  oapi-api.cfg.yaml
├── cmd/
│   ├── appmetrica/               # единственный production-бинарник
│   │   ├── main.go               # разбор подкоманды (flag), version через -ldflags
│   │   ├── ingest.go             # `appmetrica ingest`
│   │   ├── api.go                # `appmetrica api` (+ migrate up при MIGRATE_ON_START=true)
│   │   ├── worker.go             # `appmetrica worker`
│   │   ├── migrate.go            # `appmetrica migrate up|status`
│   │   ├── admin.go              # `appmetrica admin passkey-link`
│   │   └── healthcheck.go        # `appmetrica healthcheck --url` (HEALTHCHECK в distroless без curl)
│   └── seed/main.go              # dev-only: 3 приложения, write-ключ, 35 дней событий напрямую в ClickHouse
├── internal/
│   ├── config/                   # загрузка и валидация env (stdlib os.LookupEnv), по структуре на процесс
│   ├── clock/                    # Clock interface { Now() time.Time } + Fixed для тестов
│   ├── httpx/                    # middleware: request id, recover, access-лог без IP, CORS ingest, gzip, body limit,
│   │   └── clientip.go           #   единственное место чтения RemoteAddr / X-Forwarded-For (Traefik)
│   ├── ingest/                   # POST /v1/events: decode → validate → time correct → PII guard → rate limit → geo → write
│   │   ├── handler.go  decode.go  validate.go  timecorrect.go  piiguard.go  contextmerge.go
│   │   ├── keycache.go           # map key → {write_key_id, app_id, status, platform}, перезагрузка каждые 15 с
│   │   └── writer.go             # пакетная вставка events + rejected_events через chstore
│   ├── ratelimit/                # token bucket per key (golang.org/x/time/rate) с TTL-эвикцией
│   ├── geo/                      # DB-IP mmdb → {country, region, city}; Locator interface для тестов
│   ├── chstore/                  # соединение clickhouse-go/v2, async insert (WithAsync wait), батчи
│   ├── pgstore/                  # pgxpool, транзакции
│   │   └── sqlc/                 # СГЕНЕРИРОВАНО sqlc — не редактировать
│   ├── identity/                 # person_key: SQL-фрагменты CTE links и выражение person_key, разбор u:/i:
│   ├── reports/                  # SQL отчётов ClickHouse: portfolio, audience, retention, realtime, profiles,
│   │                             #   integration health, close day; параметры from/to/TZ/фильтры → args
│   ├── apps/                     # приложения, write-ключи pk_, генерация и хэширование sk_
│   ├── auth/                     # go-webauthn, setup-токены, сессии, Bearer sk_, scopes, CSRF
│   ├── deletion/                 # создание запросов (api) и исполнение (worker): lightweight DELETE + мутации
│   ├── api/                      # реализация StrictServerInterface (gen/api), маппинг ошибок в коды якоря
│   ├── worker/                   # планировщик и задачи: system_check, digest, close_day, deletion, session_cleanup
│   ├── telegram/                 # sendMessage через net/http, обработка 429 retry_after
│   ├── testutil/                 # testcontainers ClickHouse/Postgres + применение миграций + загрузка фикстур
│   ├── archtest/                 # тест правил зависимостей между пакетами (go list -deps)
│   └── gen/
│       ├── ingestapi/            # СГЕНЕРИРОВАНО oapi-codegen: только модели ingest + embedded spec
│       └── api/                  # СГЕНЕРИРОВАНО oapi-codegen: strict std-http server + модели
├── migrations/
│   ├── embed.go                  # //go:embed postgres/*.sql clickhouse/*.sql → goose Provider
│   ├── postgres/00001_init.sql   # 13 таблиц и enum-ы из 05-data-model
│   └── clickhouse/
│       ├── 00001_init.sql        # events
│       ├── 00002_aggregates.sql  # install_first_seen(+mv), user_first_seen(+mv), identity_links(+mv)
│       └── 00003_service_tables.sql # rejected_events, daily_stats
├── queries/postgres/             # sqlc (05-data-model): owner.sql passkeys.sql auth.sql apps.sql keys.sql
│                                 #   deletion_requests.sql ops.sql
└── testdata/fixtures/            # NDJSON-события + *.golden.json ожидаемые ответы отчётов
```

**Почему ingest-хендлер написан руками, а api — сгенерирован.** Strict-сервер oapi-codegen декодирует тело целиком в
типизированную структуру: одно событие с невалидным UUID сломало бы декодирование всего батча и дало `400`, а контракт
требует `200` с `rejected[{index, code}]`. Поэтому для ingest генерируются только модели (`gen/ingestapi`), а
`internal/ingest/decode.go` разбирает `events` как `[]json.RawMessage` и валидирует каждое событие отдельно. Api же —
классический REST, strict-сервер экономит сотни строк обвязки.

**Правила зависимостей между Go-пакетами** (проверяются тестом `internal/archtest/arch_test.go`, который запускается в
`go test ./...` и падает при запрещённом импорте):

| Пакет | Может импортировать (internal) | Не может импортировать |
|---|---|---|
| `cmd/appmetrica` | любой `internal/*` | — |
| `internal/api` | `gen/api`, `auth`, `apps`, `reports`, `deletion`, `identity`, `pgstore`, `chstore`, `httpx`, `clock`, `config` | `ingest`, `worker` |
| `internal/ingest` | `gen/ingestapi`, `geo`, `ratelimit`, `chstore`, `pgstore`, `httpx`, `clock`, `config` | `api`, `reports`, `auth`, `worker` |
| `internal/worker` | `reports`, `deletion`, `telegram`, `chstore`, `pgstore`, `clock`, `config` | `api`, `ingest` |
| `internal/reports` | `chstore`, `identity`, `clock` | `pgstore`, `api`, `ingest`, `worker` |
| `internal/deletion` | `chstore`, `pgstore`, `identity`, `clock` | `api`, `ingest`, `worker` |
| `internal/auth`, `internal/apps` | `pgstore`, `clock`, `config` | `chstore`, `reports`, `api` |
| `internal/identity`, `internal/clock`, `internal/geo`, `internal/ratelimit`, `internal/telegram` | только stdlib и сторонние библиотеки | любой `internal/*` |
| `internal/chstore`, `internal/pgstore`, `internal/httpx`, `internal/config` | `clock` | доменные пакеты |
| `internal/gen/*`, `internal/pgstore/sqlc` | — (сгенерированы) | редактирование руками |

Циклы запрещены; доменная логика не знает про HTTP (`reports` возвращает Go-структуры, преобразование в модели OpenAPI — в `internal/api`).

---

## 6. Хуки и типы: `web/src/hooks/`, типы из OpenAPI

```
web/src/hooks/
├── use-app-params.ts             # appId из useParams + валидация UUID
├── use-report-filters.ts         # nuqs: from, to, granularity, compare, environment, unit, фильтры (из lib/search-params)
├── use-realtime-polling.ts       # refetchInterval 5000 мс, пауза по кнопке и при скрытой вкладке
├── use-first-event-waiter.ts     # S-10: опрос integration/health раз в 3 с до появления first_event_at
└── use-passkey.ts                # обёртки begin/finish login/setup/register над сгенерированными мутациями

Типы:
- доменные типы API — только из src/lib/api/gen/model (генерируются из contracts/openapi/api.yaml);
- локальные типы компонента — в том же файле;
- глобальные декларации — web/src/types/global.d.ts (только при необходимости, сейчас не требуется).
```

**Правило**: тип, описывающий данные API, никогда не пишется руками. Если в UI нужен производный тип — `Pick`/`Omit` от сгенерированного.

---

## 7. Тесты

```
backend/
├── internal/**/<name>_test.go                 # unit: чистая логика, table-driven, без Docker
├── internal/**/<name>_integration_test.go     # //go:build integration — testcontainers ClickHouse 26.8.25.14 / Postgres 18.6
├── internal/archtest/arch_test.go             # правила импортов
└── testdata/fixtures/
    ├── audience_basic.ndjson + audience_basic.golden.json
    ├── retention_day_cohorts.ndjson + retention_day_cohorts.golden.json
    ├── retention_week_cohorts.ndjson + retention_week_cohorts.golden.json
    ├── identity_first_owner.ndjson + identity_first_owner.golden.json
    ├── timezone_boundaries.ndjson + timezone_boundaries.golden.json
    ├── realtime_active_now.ndjson + realtime_active_now.golden.json
    └── deletion_user.ndjson + deletion_user.golden.json

web/
├── src/**/<name>.test.ts(x)                    # Vitest + Testing Library + MSW (моки из lib/api/gen/endpoints/*.msw.ts)
├── src/test/setup.ts                           # @testing-library/jest-dom/vitest, MSW server lifecycle
├── src/test/msw-server.ts  src/test/render.tsx # renderWithProviders (QueryClient + NuqsTestingAdapter)
└── e2e/
    ├── global-setup.ts                         # passkey-link → виртуальный аутентификатор CDP → e2e/.auth/owner.json
    ├── helpers/webauthn.ts  helpers/ingest.ts  # регистрация виртуального ключа; отправка батчей в :8081
    ├── auth.spec.ts  onboarding.spec.ts  portfolio.spec.ts  audience.spec.ts  retention.spec.ts
    ├── realtime.spec.ts  profile-deletion.spec.ts  settings.spec.ts
    └── a11y.spec.ts                            # @axe-core/playwright на S-01, S-03, S-05, S-06, S-10, S-12
```

**Стратегия** (подробно — [22-test-strategy.md](./22-test-strategy.md)):
- TDD: тест пишется до кода; путь теста указан в каждом тикете.
- SQL отчётов проверяется только интеграционными тестами на настоящем ClickHouse с golden-файлами: моков ClickHouse нет.
- Фикстуры задают события с явными `event_time` и `received_at`; часы в тестах фиксированные (`clock.Fixed`).

---

## 8. Скрипты и кодогенерация

Отдельного каталога `scripts/` нет: все операции — цели `Makefile` и подкоманды бинарника.

| Операция | Команда | Что делает |
|---|---|---|
| Кодоген всего | `make gen` | `gen-go-api` + `gen-sqlc` + `gen-web` |
| Go-сервер из OpenAPI | `make gen-go-api` | `go tool oapi-codegen` (v2.8.0, tool-директива в go.mod): `contracts/openapi/ingest.yaml` → `backend/internal/gen/ingestapi/ingestapi.gen.go`; `contracts/openapi/api.yaml` → `backend/internal/gen/api/api.gen.go` |
| Доступ к Postgres | `make gen-sqlc` | Docker-образ `sqlc/sqlc:1.31.1`: `backend/migrations/postgres` + `backend/queries/postgres` → `backend/internal/pgstore/sqlc` |
| TS-клиент | `make gen-web` | `pnpm gen:api` (orval 8.42.0): `contracts/openapi/api.yaml` → `web/src/lib/api/gen/{endpoints,model,zod}` |
| Проверка актуальности | `make gen-check` | `make gen` + `git diff --exit-code` по трём каталогам (то же в CI) |
| Миграции | `make migrate-up` / `make migrate-status` | `go run ./cmd/appmetrica migrate up|status` (goose, embed.FS, оба диалекта) |
| Демо-данные | `make seed` | `go run ./cmd/seed --apps 3 --days 35 --daily-installs 400 --write-key $DEV_WRITE_KEY` |
| Гео-база локально | `make geo-db` | скачивает DB-IP City Lite текущего (или прошлого) месяца в `backend/.geo/` |
| Ссылка passkey | `make passkey-link` | `go run ./cmd/appmetrica admin passkey-link` |

**Что коммитится**: весь сгенерированный код (`backend/internal/gen/**`, `backend/internal/pgstore/sqlc/**`,
`web/src/lib/api/gen/**`), `go.sum`, `pnpm-lock.yaml`. **Что не коммитится**: `.env`, `backend/.geo/`, `*.mmdb`,
`web/.next/`, `web/.source/` (генерирует `fumadocs-mdx` в postinstall), отчёты покрытия и Playwright.

Почему сгенерированный код коммитится: сборка Docker-образа и `go build` не требуют генераторов; ревьюер видит
дифф контракта в виде изменений кода; CI ловит расхождение спеки и кода через `gen-check`.

---

## 9. `docs/`

```
docs/
├── discovery/2026-10-13-appmetrica-online/   # discovery-пакет (00-summary … 22-test-strategy, 00-bootstrap/)
├── plans/20261013-appmetrica-online.md       # ralphex-план (чекбоксы задач)
└── adr/
    ├── 0001-clickhouse-for-events.md
    ├── 0002-person-resolution-at-query-time.md
    ├── 0003-handwritten-ingest-handler.md
    ├── 0004-passkey-only-auth.md
    └── 0005-dokploy-with-tailscale.md

web/content/docs/                              # пользовательская документация API (S-13, за логином)
├── meta.json                                  # порядок разделов в сайдбаре
├── index.mdx                                  # Быстрый старт
├── concepts/identity.mdx  concepts/sessions.mdx  concepts/time-and-offline.mdx
├── recipes/ios-swift.mdx  recipes/android-kotlin.mdx  recipes/web-js.mdx
├── recipes/bots-and-backends.mdx  recipes/go-and-curl.mdx
├── errors-and-retries.mdx  privacy-and-gdpr.mdx
└── read-api/overview.mdx
```

Операционные runbook-и живут в discovery-пакете ([12-dev-runbook.md](./12-dev-runbook.md),
[21-ops-runbooks.md](./21-ops-runbooks.md)) и при изменении процедур обновляются там же. ADR пишутся в формате MADR для
каждого решения, меняющего контракт, схему хранения или инфраструктуру.

---

## 10. Config files (корень и пакеты)

| Файл | Назначение |
|---|---|
| `Makefile` | Все команды (см. раздел 8 и `make help`) |
| `.env.example` | 100 % переменных окружения (коммитится); `.env` — локальная копия (gitignored) |
| `docker-compose.dev.yml` | ClickHouse 26.8.25.14 + Postgres 18.6 для разработки, интеграционных и E2E-тестов |
| `lefthook.yml` | pre-commit (fmt, lint, Biome), commit-msg (Conventional Commits), pre-push (тесты) |
| `.editorconfig` | Отступы: tab для Go и Makefile, 2 пробела для остального |
| `backend/go.mod` | Go 1.27, toolchain go1.27.2, tool oapi-codegen v2.8.0 |
| `backend/.golangci.yml` | golangci-lint v2.14.0, forbidigo (`RemoteAddr`, `fmt.Print*`, `time.Now`), gofumpt + goimports |
| `backend/sqlc.yaml` | sqlc v2-конфиг, pgx/v5, `google/uuid`, указатели для NULL |
| `backend/oapi-ingest.cfg.yaml`, `backend/oapi-api.cfg.yaml` | Настройки генерации Go из OpenAPI |
| `backend/Dockerfile` | Образ `appmetrica-backend`: golang:1.27.2-alpine → distroless/static-debian12:nonroot, DB-IP по `GEO_MONTH` |
| `web/package.json` | Зависимости с точными версиями, `packageManager: pnpm@12.11.2`, скрипты |
| `web/pnpm-workspace.yaml` | Настройки pnpm 12 (`allowBuilds: esbuild`) |
| `web/tsconfig.json` | strict, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, алиасы `@/*` и `fumadocs-mdx:collections/*` |
| `web/biome.json` | Biome 2.5.15: lint + format + organize imports, игнор `src/lib/api/gen` |
| `web/next.config.ts` | standalone, typedRoutes, cacheComponents, rewrites `/api/*`, security headers, Fumadocs MDX |
| `web/source.config.ts` | Коллекция документации Fumadocs (`content/docs`) |
| `web/orval.config.ts` | Генерация хуков TanStack Query, Zod, MSW-моков |
| `web/vitest.config.ts`, `web/playwright.config.ts` | Unit (jsdom, пороги 70 %) и E2E (Chromium 1440×900) |
| `web/components.json` | Конфиг shadcn (создаётся `shadcn init -b radix`) |
| `web/postcss.config.mjs` | `@tailwindcss/postcss` |
| `deploy/clickhouse/compose.yaml` | Стек ClickHouse + clickhouse-backup для Dokploy (образы с digest) |
| `deploy/clickhouse/config.d/appmetrica.xml`, `users.d/appmetrica.xml` | Серверные настройки и пользователи `appmetrica_rw`/`appmetrica_ro` |
| `deploy/clickhouse-backup/config.yml` | S3 Hetzner, 35 бэкапов, watch 24h / full 168h |

---

## 11. Конвенции именования

### Файлы
- **Go**: `snake_case.go` (`timecorrect.go`, `job_close_day.go`); тесты `<name>_test.go`, интеграционные — `<name>_integration_test.go` с первой строкой `//go:build integration`.
- **SQL-миграции**: `NNNNN_snake_case.sql` с пятизначным номером (`00001_init.sql`, `00002_add_alert_states_index.sql`) — отдельные последовательности в `migrations/postgres` и `migrations/clickhouse`. ClickHouse-миграции начинаются с `-- +goose NO TRANSACTION`, по одному DDL на блок `-- +goose StatementBegin/End`.
- **sqlc-запросы**: `queries/postgres/<entity_plural>.sql`, имя запроса `-- name: VerbEntity :one|:many|:exec` (`GetAppByID`, `ListActiveWriteKeys`).
- **TS/TSX**: kebab-case для всех файлов, включая компоненты (`retention-heatmap.tsx`) — так генерирует shadcn, держим единообразие; тесты `<name>.test.ts(x)`, E2E `<flow>.spec.ts`.
- **Фикстуры**: `<scenario>.ndjson` + `<scenario>.golden.json`.

### Директории
- Go-пакеты — одно короткое слово в нижнем регистре без подчёркиваний (`ratelimit`, `chstore`).
- Web — kebab-case (`app-header`, `[personKey]` — исключение по правилам Next.js для динамических сегментов).

### Идентификаторы
- Go: `MixedCaps`; аббревиатуры целиком (`AppID`, `UserID`, `HTTPClient`, `TTL`); ошибки-сентинелы `ErrXxx`; интерфейсы по поведению (`Locator`, `Clock`).
- TS: `camelCase` для переменных и функций, `PascalCase` для компонентов и типов, `SCREAMING_SNAKE_CASE` для env и констант модулей.
- SQL и JSON API: `snake_case` (`install_id`, `retention_d1`) — совпадает с якорем и OpenAPI.
- Коды ошибок и причин отказа — `snake_case` из якоря (`event_too_old`, `insufficient_scope`), не переводятся.

### Типы vs интерфейсы (TS)
- `type` — для unions, пересечений и производных от сгенерированных типов.
- `interface` — для пропсов компонентов.

---

## 12. Правила «куда положить?»

| Что | Куда |
|---|---|
| Новый эндпоинт Read API / admin API | 1) `contracts/openapi/api.yaml` (operationId из якоря) → 2) `make gen` → 3) метод в `backend/internal/api/handlers_<area>.go` → 4) тест `handlers_<area>_test.go` |
| Изменение формата ingest | `contracts/openapi/ingest.yaml` → `make gen-go-api` → `backend/internal/ingest/{decode,validate}.go` |
| Новый отчёт или метрика ClickHouse | `backend/internal/reports/<report>.go` + фикстура `testdata/fixtures/<scenario>.ndjson/.golden.json` + `<report>_integration_test.go` |
| Новая таблица / колонка Postgres | `backend/migrations/postgres/NNNNN_<change>.sql` + запросы в `backend/queries/postgres/<entity>.sql` + `make gen-sqlc` |
| Новая таблица / колонка ClickHouse | `backend/migrations/clickhouse/NNNNN_<change>.sql` (+ обновить `05-data-model.md`) |
| Фоновая задача | `backend/internal/worker/job_<name>.go` + регистрация в `scheduler.go` + значение в enum `job_name` (миграция) |
| Новая env-переменная | `backend/internal/config/` + `.env.example` + раздел env в `12-dev-runbook.md` + Dokploy |
| Новый экран | `web/src/app/(dashboard)/<path>/page.tsx` (+ `loading.tsx`) по screen inventory из `14-ux-ui-spec.md` |
| Доменный компонент отчёта | `web/src/components/reports/<name>.tsx` + `<name>.test.tsx` |
| Примитив UI | `pnpm dlx shadcn@4.21.4 add <name>` → `web/src/components/ui/` |
| Чистая функция web | `web/src/lib/<topic>.ts` + `<topic>.test.ts` |
| Хук | `web/src/hooks/use-<name>.ts` |
| Страница документации | `web/content/docs/<section>/<slug>.mdx` + `meta.json` |
| E2E-сценарий | `web/e2e/<flow>.spec.ts` |
| Конфиг ClickHouse | `deploy/clickhouse/config.d/` или `users.d/` (+ перезапуск стека в Dokploy) |
| Архитектурное решение | `docs/adr/NNNN-<slug>.md` |

---

## 13. Anti-patterns (не делаем)

- ❌ **Правка сгенерированного кода руками** (`internal/gen`, `pgstore/sqlc`, `lib/api/gen`) — правка теряется при следующем `make gen`, а `gen-check` в CI падает.
- ❌ **Пакеты-корзины** `utils`, `common`, `helpers` в Go. Функция живёт в пакете своего домена.
- ❌ **SQL ClickHouse вне `internal/reports` и `internal/deletion`/`internal/ingest/writer.go`**. Хендлеры не собирают SQL.
- ❌ **Моки ClickHouse в тестах отчётов**. Только testcontainers + golden-файлы.
- ❌ **Чтение `RemoteAddr`/`X-Forwarded-For` вне `httpx/clientip.go`** и запись IP куда-либо.
- ❌ **`time.Now()` в доменном коде** — только `clock.Clock`.
- ❌ **Ручные `fetch` к API из компонентов** вместо сгенерированных хуков.
- ❌ **Бизнес-логика в Next.js route handlers / Server Actions** — вся логика в Go.
- ❌ **Файлы длиннее ~400 строк** (кроме сгенерированных) — делить по ответственности.
- ❌ **Циклические импорты** и нарушения таблицы зависимостей раздела 5 (ловит `archtest`).
- ❌ **Абстракции «на будущее»**: интерфейсы с одной реализацией вводятся только там, где нужна подмена в тестах (`Clock`, `Locator`, отправка Telegram).

---

## 14. Экспорт / импорт

- **Path aliases** (`web/tsconfig.json`):
  ```json
  "paths": {
    "@/*": ["./src/*"],
    "fumadocs-mdx:collections/*": ["./.source/*"]
  }
  ```
  Импорты внутри web — только через `@/` (`import { formatCount } from "@/lib/format"`), относительные пути — только внутри одной папки.
- **Named exports** во всех модулях; default export — только там, где его требует Next.js (`page.tsx`, `layout.tsx`, `loading.tsx`, `error.tsx`, `not-found.tsx`) и конфиги.
- **Barrel files** (`index.ts` с реэкспортами) не создаём: они раздувают бандл и скрывают зависимости. Исключение — то, что генерирует orval.
- **Go**: импорты группируются goimports (stdlib, сторонние, `appmetrica.online/backend/...`); экспортируются только идентификаторы, используемые другими пакетами; конструкторы `NewXxx(deps...) *Xxx`.

---

## Changelog

| Date | Version | Changes |
|---|---|---|
| 2026-10-13 | 1.0 | Initial project structure (monorepo backend + web + contracts + deploy) |
