# Project Structure: appmetrica.online

**Версия**: 1.1 | **Дата**: 2026-10-14

> Файловая и модульная структура монорепозитория: Go-бэкенд (`backend/`), Next.js-дашборд с документацией (`web/`),
> контракты (`contracts/`) и инфраструктура (`deploy/`). Документ объясняет, **почему** структура именно такая,
> и отвечает на вопрос «куда положить новый файл» без угадывания. Источник имён — канонический якорь пакета:
> пути из этого документа используются в [12-dev-runbook.md](./12-dev-runbook.md).

---

## 1. Высокоуровневое дерево

```
appmetrica.online/
├── CONTRIBUTING.md                   # правила разработки (TDD, Validation Commands, запреты)
├── README.md                         # что это, как запустить за 5 минут, ссылки на discovery
├── Makefile                          # единая точка входа: dev, gen, test, lint, e2e, seed, passkey-link, perf, reconcile
├── lefthook.yml                      # git hooks (fmt/lint/commit-msg/pre-push tests)
├── renovate.json                     # обновления зависимостей (группы, точные версии)
├── .editorconfig  .gitignore  .env.example
├── docker-compose.dev.yml            # локальные ClickHouse 26.8.25.14 + Postgres 18.6
├── .github/
│   ├── pull_request_template.md      # тикет, допущения, чек-лист Validation Commands
│   └── workflows/
│       ├── ci.yml                    # backend → web → e2e → images (GHCR)
│       ├── deploy.yml                # Dokploy API через Tailscale, ручной откат по тегу
│       └── geo-refresh.yml           # ежемесячная пересборка ingest со свежей DB-IP
├── contracts/
│   ├── openapi/
│   │   ├── ingest.yaml               # OAS 3.0.3, POST /v1/events (i.appmetrica.online)
│   │   └── api.yaml                  # OAS 3.0.3, /v1 (Read API) + /admin/v1 (x-internal: true)
│   └── http/requests.http            # ручные запросы к ingest и Read API (REST Client / JetBrains HTTP)
├── backend/                          # Go-модуль appmetrica.online/backend → один бинарник appmetrica
├── web/                              # Next.js 16.4: дашборд (S-01…S-12, S-14) + документация (S-13)
├── scripts/                          # shell-скрипты CI и деплоя + Go-модуль сверки с AppMetrica (раздел 8)
├── deploy/
│   ├── clickhouse/                   # compose-стек Dokploy «clickhouse-stack» + config.d / users.d
│   ├── clickhouse-backup/config.yml  # инкрементальные бэкапы в Hetzner Object Storage
│   └── dokploy/README.md             # ручные шаги в UI Dokploy (приложения, домены, healthcheck JSON)
└── docs/
    ├── discovery/2026-10-13-appmetrica-online/   # этот пакет
    └── reconciliation/README.md                  # процедура 14-дневной сверки с AppMetrica (NFR-010)
```

**Почему монорепозиторий без pnpm workspace.** В репозитории один TS-пакет (`web/`) и один основной Go-модуль
(`backend/`); второй Go-модуль `scripts/reconcile` — отдельная утилита без импорта `backend/internal`. Workspace-инструменты (Turborepo, pnpm workspaces, go.work) решают задачу многих пакетов и
здесь только добавили бы конфигурацию. Связь между частями — контракты в `contracts/openapi/`, из которых
генерируется и Go-сервер, и TS-клиент; один `Makefile` в корне оркестрирует обе части.

**Почему контракты в отдельном каталоге.** OpenAPI — единственный источник правды для трёх потребителей:
oapi-codegen (Go), orval (TS-клиент, Zod, MSW-моки) и Fumadocs (страницы API reference). Если бы спека жила
внутри `backend/` или `web/`, одна из сторон зависела бы от путей другой.

---

## 2. `web/src/app/` — Next.js App Router

```
web/src/app/
├── layout.tsx                    # <html lang="ru" data-theme>, inline-скрипт темы (lib/theme.ts), Manrope + JetBrains Mono, globals.css
├── globals.css                   # Tailwind 4; токены светлой (:root) и тёмной (:root[data-theme="dark"]) тем, 14-ux-ui-spec «Обновление 1.2»
├── shadcn.css                    # варианты и утилиты стиля radix-nova из shadcn@4.21.4 (CLI в зависимостях не держим)
├── not-found.tsx                 # S-14: 404
├── error.tsx                     # S-14: граница ошибок сегмента (500)
├── global-error.tsx              # фатальная ошибка корневого layout
├── (public)/
│   └── page.tsx                  # S-00: лендинг на «/» (components/landing), без сессии
├── demo/
│   ├── route.ts                  # GET /demo: cookie am_demo=1 → /portfolio (S-15 демо-режим)
│   └── exit/route.ts             # GET /demo/exit: снять am_demo → /
├── api/
│   └── health/route.ts           # GET /api/health → {"status":"ok"} (healthcheck контейнера web; приоритет над rewrite)
├── (auth)/                       # публичные страницы без шапки дашборда
│   ├── layout.tsx                # центрированная карточка
│   ├── login/page.tsx            # S-01: вход по passkey (кнопка + conditional UI)
│   └── setup/[token]/page.tsx    # S-02: регистрация passkey по одноразовой ссылке
├── (dashboard)/                  # всё, что требует сессии владельца
│   ├── layout.tsx                # SkipLink → [DemoBanner] + AppHeader(demo) (под Suspense, cookie am_demo) → <main id="main"> → AppFooter
│   ├── providers.tsx             # 'use client': QueryClientProvider + NuqsAdapter
│   ├── portfolio/page.tsx        # S-03: портфель на /portfolio (оболочка под Suspense)
│   ├── portfolio-view.tsx        # S-03: клиентский вид, URL-состояние ?date ?env ?unit ?sort
│   ├── loading.tsx               # skeleton портфеля
│   ├── settings/
│   │   ├── page.tsx              # S-12
│   │   └── settings-view.tsx     # вкладки Passkeys / API-ключи / Telegram / Система / Удаления (?tab=)
│   └── apps/
│       ├── new/
│       │   ├── page.tsx          # S-04: создание приложения
│       │   └── create-app-form.tsx
│       └── [appId]/
│           ├── layout.tsx        # AppShell: название, платформа, AppTabs, ArchivedBanner
│           ├── loading.tsx
│           ├── error.tsx         # 404 app_not_found → «Приложение не найдено»
│           ├── page.tsx          # S-05: аудитория
│           ├── audience-view.tsx # S-05: тулбар, KPI, графики, разбивки (url-state)
│           ├── retention/page.tsx          # S-06
│           ├── retention/retention-view.tsx
│           ├── realtime/page.tsx           # S-07
│           ├── profiles/page.tsx           # S-08
│           ├── profiles/[personKey]/page.tsx   # S-09
│           ├── integration/page.tsx        # S-10
│           └── settings/page.tsx           # S-11
└── (docs)/
    ├── docs.css                  # стили Fumadocs UI, подключаются только в документации
    └── docs/
        ├── layout.tsx            # Fumadocs DocsLayout; серверная проверка сессии (redirect /login)
        ├── not-found.tsx         # 404 внутри документации
        ├── search/route.ts       # поиск Fumadocs, отдаётся только владельцу с сессией
        └── [[...slug]]/page.tsx  # S-13: MDX-гайды + API reference (fumadocs-openapi)
```

**Правила**:
- Route groups `(public)`, `(auth)`, `(dashboard)`, `(docs)` дают разные layouts без влияния на URL. «/» — публичный лендинг S-00, портфель — `/portfolio`; в демо-режиме (cookie `am_demo=1`, S-15) proxy пускает в `(dashboard)` без сессии, а `apiFetch` берёт ответы из `src/lib/demo` вместо API; `(docs)` остаётся только для владельца. Пути экранов — строго из таблицы S-01…S-14 якоря.
- `src/proxy.ts` (Next.js 16, бывший middleware) только проверяет **наличие** cookie `am_session` и редиректит на `/login?next=<path>`; валидность сессии проверяет Go (`401 unauthenticated` → клиентский редирект на `/login`).
- `page.tsx` — Server Component: читает `searchParams` (период, фильтры), отдаёт данные в клиентские компоненты отчётов. Запросы к API из серверных компонентов идут на `API_INTERNAL_URL` с пробросом cookie (через `src/lib/api/fetcher.ts`).
- `'use cache'` в `(dashboard)` и `(auth)` запрещён: данные персональные и зависят от cookie. Cache Components включены (`cacheComponents: true`), поэтому все динамические чтения (`cookies()`, `searchParams`) находятся под `Suspense` (`loading.tsx`).
- Route handlers в `app/api/*` не проксируют бизнес-логику: всё API — в Go, доступно через rewrite `/api/:path*` → `http://appmetrica-api:8080/:path*` (секция `afterFiles`, поэтому `app/api/health` имеет приоритет).
- Клиентский вид страницы, которым пользуется только она, лежит рядом со страницей как `<screen>-view.tsx` (`portfolio-view.tsx`, `audience-view.tsx`, `retention-view.tsx`, `settings-view.tsx`), а `page.tsx` остаётся тонкой серверной оболочкой с `Suspense`.
- Server Actions не используются: все мутации — вызовы `/admin/v1/*` через сгенерированные TanStack Query мутации. Причина — один источник бизнес-логики (Go) и одна модель авторизации (cookie + CSRF-заголовок `X-Requested-With: appmetrica-web`).

---

## 3. `web/src/components/` — React компоненты

```
web/src/components/               # имена и props — источник истины 14-ux-ui-spec.md §Component inventory
├── ui/                           # shadcn/ui (Radix: `npx shadcn@4.21.4 init -b radix`), копируются CLI
│   ├── button.tsx  card.tsx  table.tsx  tabs.tsx  dialog.tsx  alert-dialog.tsx  alert.tsx  dropdown-menu.tsx
│   ├── select.tsx  popover.tsx  calendar.tsx  command.tsx  input.tsx  input-group.tsx  label.tsx  badge.tsx
│   ├── skeleton.tsx  scroll-area.tsx  textarea.tsx  tooltip.tsx  separator.tsx  switch.tsx  toggle.tsx
│   └── toggle-group.tsx  sonner.tsx  chart.tsx   # input-group, textarea, toggle — зависимости command и toggle-group
├── layout/
│   ├── app-header.tsx            # логотип, AppSwitcher, «Портфель», «Документация», «Настройки», UserMenu
│   ├── app-switcher.tsx          # выпадающий список приложений (listApps)
│   ├── app-shell.tsx             # оболочка S-05…S-11: название с платформой, AppTabs, ArchivedBanner (useGetApp)
│   ├── app-tabs.tsx              # вкладки приложения: Аудитория, Retention, Realtime, Профили, Интеграция, Настройки
│   ├── user-menu.tsx             # DropdownMenu «Выйти»
│   ├── page-header.tsx           # заголовок страницы + слот действий
│   ├── app-footer.tsx            # атрибуция «IP Geolocation by DB-IP» (CC BY 4.0) со ссылкой
│   ├── skip-link.tsx             # «Перейти к содержимому» (WCAG 2.4.1)
│   └── logo.tsx
├── reports/                      # строительные блоки отчётов (знают домен аналитики, не знают HTTP)
│   ├── portfolio-table.tsx       # S-03
│   ├── kpi-card.tsx  delta.tsx  sparkline.tsx  unit-badge.tsx
│   ├── audience-kpis.tsx         # S-05: ряд KpiCard (summary + compare)
│   ├── lazy-charts.tsx           # ленивые обёртки Recharts-графиков (вне initial JS, NFR-006)
│   ├── metric-chart.tsx          # S-05: timeseries + compare_points (Recharts 3.10.1 через shadcn chart)
│   ├── rolling-active-chart.tsx  # S-05: DAU/WAU/MAU
│   ├── period-picker.tsx  granularity-select.tsx  compare-toggle.tsx  filter-bar.tsx  environment-switch.tsx
│   ├── breakdown-table.tsx       # S-05
│   ├── retention-heatmap.tsx     # S-06: HTML-таблица с цветовой шкалой, sticky-шапка
│   ├── retention-average-chart.tsx
│   └── realtime-panel.tsx        # S-07: опрос getRealtime раз в 5 с, пауза, скрытая вкладка
├── realtime/
│   └── realtime-counter.tsx  per-minute-chart.tsx  event-feed.tsx          # S-07
├── profiles/
│   └── profile-search-panel.tsx  profile-search.tsx  profile-panel.tsx  profile-card.tsx  installs-table.tsx  sessions-table.tsx  deletion-status-badge.tsx  profile-delete-dialog.tsx  # S-08, S-09
├── integration/
│   └── integration-panel.tsx  integration-onboarding.tsx  snippet-tabs.tsx  waiting-for-first-event.tsx  hourly-ingest-chart.tsx  rejection-reasons-table.tsx  versions-health-table.tsx  rejection-rate-warning.tsx  # S-10
├── apps/
│   └── app-settings-panel.tsx  app-settings-form.tsx  write-keys-table.tsx  app-danger-zone.tsx  # S-04, S-11
├── settings/
│   └── tab-header.tsx  passkeys-tab.tsx  api-keys-tab.tsx  telegram-tab.tsx  system-tab.tsx  deletions-tab.tsx  # S-12
├── auth/
│   └── passkey-button.tsx  setup-passkey-form.tsx  passkey-link-command.tsx  # S-01, S-02 (@simplewebauthn/browser 14.0.0)
├── common/
│   ├── copy-field.tsx  code-block.tsx  platform-icon.tsx  timezone-select.tsx  form-field.tsx  ru-calendar.tsx
│   └── lazy-popover.tsx  lazy-command.tsx  lazy-calendar.tsx  lazy-toaster.tsx  mounted-toaster.tsx  # ленивые чанки (NFR-006)
├── feedback/
│   └── empty-state.tsx  error-state.tsx  confirm-delete-dialog.tsx  archived-banner.tsx
├── docs/
│   └── api-page.tsx              # S-13: страница операции API reference (fumadocs-openapi)
├── skeletons/                    # loading-состояния из 14-ux-ui-spec §8.2
│   └── kpi-skeleton.tsx  table-skeleton.tsx  retention-skeleton.tsx  realtime-skeleton.tsx  profile-skeleton.tsx  integration-skeleton.tsx
└── icons/platform/
    └── apple.tsx  android.tsx  web.tsx  bot.tsx  server.tsx  desktop.tsx  other.tsx  platform-svg.tsx  # свои SVG: в lucide-react 1.x нет бренд-иконок
```

**Правила**:
- `ui/` — только примитивы shadcn. Генерируются `pnpm dlx shadcn@4.21.4 add <name>`, после чего правятся под токены. Домена не знают.
- `reports/`, `realtime/`, `profiles/`, `integration/`, `apps/`, `settings/`, `auth/`, `layout/`, `common/`, `feedback/` — доменные компоненты. Получают данные пропсами или через хуки из `src/hooks/`; прямых вызовов API внутри них нет, кроме мутаций, оформленных сгенерированными хуками orval.
- Тест рядом с компонентом: `retention-heatmap.tsx` + `retention-heatmap.test.tsx`.
- Компонент, используемый одной страницей, живёт рядом со страницей (`app/(dashboard)/apps/new/create-app-form.tsx`, `*-view.tsx`), а не в `components/`.
- Тяжёлые клиентские библиотеки (Recharts, Radix Popover, cmdk, react-day-picker, sonner, zod) не попадают в initial JS: их подключают через ленивые обёртки `components/common/lazy-*.tsx`, `components/reports/lazy-charts.tsx` и динамический импорт `lib/forms/schemas.ts`. Бюджет — 250 KB gzip на страницу дашборда, проверка `pnpm build && pnpm check:bundle` (`web/scripts/check-bundle-size.ts`).

---

## 4. `web/src/lib/` — утилиты и интеграции

```
web/src/lib/
├── api/
│   ├── fetcher.ts                # РУКОПИСНЫЙ mutator orval: базовый URL (браузер → "/api", сервер → API_INTERNAL_URL),
│   │                             # credentials: "include", X-Requested-With для мутаций, разбор конверта Error, 401 → /login
│   └── gen/                      # СГЕНЕРИРОВАНО orval 8.42.0 — не редактировать
│       ├── endpoints/            # TanStack Query хуки по тегам OpenAPI + *.msw.ts / *.faker.ts моки
│       ├── model/                # типы схем (AudienceSummary, RetentionMatrix, Portfolio, …)
│       ├── zod/                  # Zod-схемы (*.zod.ts) для валидации форм и ответов
│       └── openapi/              # копии contracts/openapi/*.yaml для API reference (образ web собирается без contracts/)
├── docs/                         # S-13: source.ts (дерево MDX + API reference), openapi-source.ts, session.ts (requireOwner), translations.ts
├── forms/
│   ├── use-zod-form.ts           # хук форм (14-ux-ui-spec §7.1) вместо react-hook-form
│   └── schemas.ts                # Zod-схемы форм; импортируется только динамически (zod вне initial JS)
├── query-client.ts               # фабрика QueryClient: 4xx без повторов, 503 — 3 раза (1/2/4 с), прочее — 1 раз
├── url-state.ts                  # парсеры nuqs S-05: период, детализация, сравнение, метрики, разбивка, фильтры, env, unit
├── period.ts                     # календарные даты YYYY-MM-DD, «сегодня» в TZ владельца/приложения, лимиты 366 / 7 дней
├── person-key.ts                 # encode/decode `u:<user_id>` | `i:<install_id>` для URL
├── format.ts                     # Intl.NumberFormat("ru-RU"), проценты, длительность, даты в TZ приложения
├── plural.ts                     # русское склонение по числу
├── metric-definitions.ts         # названия метрик и тексты tooltip `Info` (02-tech-spec FR-013–015)
├── errors.ts                     # тексты кодов ошибок API (по components.responses api.yaml)
├── rejection-codes.ts            # тексты кодов отказов ingest для RejectionReasonsTable
├── retention-color.ts            # доля 0..1 → класс цветовой шкалы heatmap
├── snippets.ts                   # шаблоны сниппетов интеграции с подстановкой write-ключа и NEXT_PUBLIC_INGEST_URL
├── copy-as-curl.ts               # «Скопировать как запрос к Read API» (FR-018) с плейсхолдером $APPMETRICA_SK
├── clipboard.ts  toast.ts        # копирование с объявлением через sonner; тосты, ждущие ленивый Toaster
├── safe-next.ts                  # проверка ?next= после входа (защита от open redirect)
├── webauthn.ts                   # адаптер ответов go-webauthn → optionsJSON для @simplewebauthn/browser
└── utils.ts                      # cn() (реэкспорт из пакета `cn`, используется shadcn)
```

**Правила**:
- `lib/` — чистые функции и клиенты. JSX здесь нет.
- Каждый модуль `lib/*.ts` (кроме `api/gen`) покрыт unit-тестом `*.test.ts`; пороги Vitest — 70 % строк, statements и функций, 60 % веток.
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
│   └── seed/main.go              # dev-only: 4 приложения (одно мультиплатформенное «Seed Notes»), write-ключ,
│                                 #   35 дней событий напрямую в ClickHouse
├── internal/
│   ├── config/                   # загрузка и валидация env (stdlib os.LookupEnv), по структуре на процесс
│   ├── clock/                    # Clock interface { Now() time.Time } + Fixed для тестов
│   ├── httpx/                    # chain, requestid, recover, accesslog (без IP), cors, gzip, bodylimit, errors, ready (/ready)
│   │   └── clientip.go           #   единственное место чтения RemoteAddr / X-Forwarded-For (Traefik)
│   ├── ingest/                   # POST /v1/events: decode → validate → time correct → PII guard → rate limit → geo → write
│   │   ├── handler.go  decode.go  validate.go  timecorrect.go  piiguard.go  contextmerge.go  enrich.go
│   │   ├── ratelimit.go          # лимиты FR-005: бакет приложения (батч) и установки (событие)
│   │   ├── keycache.go           # снимок write_keys + apps в atomic.Pointer, перезагрузка KEY_CACHE_REFRESH (15 с),
│   │   │                         #   last_used_at копится в памяти и сбрасывается раз в 5 мин
│   │   ├── writer.go             # пакетная вставка events + rejected_events через chstore
│   │   ├── errors.go  health.go  metrics.go   # батч-ошибки Error, /ready ingest, метрики ingest_*
│   ├── ratelimit/                # token bucket per key (golang.org/x/time/rate) с TTL-эвикцией
│   ├── geo/                      # DB-IP mmdb → {country, region, city}; Locator interface для тестов
│   ├── chstore/                  # соединение clickhouse-go/v2, async insert (WithAsync wait), батчи, миграции CH,
│   │                             #   system.disks / system.parts (system.go), список бэкапов clickhouse-backup (backup.go)
│   ├── pgstore/                  # pgxpool, транзакции, advisory lock, goose-миграции Postgres
│   │   └── sqlc/                 # СГЕНЕРИРОВАНО sqlc — не редактировать
│   ├── identity/                 # person_key: SQL-фрагменты CTE links и выражение person_key, разбор u:/i:
│   ├── reports/                  # SQL отчётов ClickHouse: portfolio, audience, retention, realtime, profiles,
│   │                             #   integration_health, closeday; filters.go, sql.go (общие фрагменты), observe.go (метрики)
│   ├── apps/                     # приложения (service.go), write-ключи pk_ (writekeys.go), API-ключи sk_ (keys.go)
│   ├── auth/                     # go-webauthn, setup-токены, сессии, Bearer sk_, scopes, CSRF; e2e.go — сброс для APP_ENV=e2e
│   ├── deletion/                 # создание запросов (api) и исполнение (worker): lightweight DELETE + мутации
│   ├── merge/                    # слияние приложений: service.go (preview/create/retry для api), executor.go
│   │                             #   (шаги move_keys → wait_keys → copy → purge_source → reclose для worker),
│   │                             #   copy.go (INSERT … SELECT по партициям ClickHouse), metrics.go (merge_*), errors.go
│   ├── api/                      # реализация StrictServerInterface (gen/api): <area>_handlers.go (Admin API),
│   │                             #   v1_<report>.go (Read API), errors.go, params.go, metrics.go, e2e_handlers.go
│   ├── worker/                   # scheduler.go (журнал job_runs) и задачи: jobs.go (deletion),
│   │                             #   closeday.go, digest.go, systemcheck.go, cleanup.go (session_cleanup),
│   │                             #   merge.go (app_merge под advisory lock + алерт «Слияние упало»)
│   ├── telegram/                 # sendMessage через net/http, обработка 429 retry_after
│   ├── testutil/                 # testcontainers ClickHouse/Postgres, миграции, NDJSON-фикстуры, golden, генератор
│   │                             #   сценария, тестовая mmdb, программный WebAuthn-аутентификатор, синтетика перф-теста
│   ├── archtest/                 # rules.go + arch_test.go: правила зависимостей между пакетами (go list)
│   ├── docscheck/                # извлекает сниппеты из web/content/docs: Go — go vet, curl — запрос в ingest (T-048)
│   └── gen/
│       ├── ingestapi/            # СГЕНЕРИРОВАНО oapi-codegen: только модели ingest + embedded spec
│       └── api/                  # СГЕНЕРИРОВАНО oapi-codegen: strict std-http server + модели
├── migrations/
│   ├── embed.go                  # //go:embed postgres/*.sql clickhouse/*.sql → goose Provider
│   ├── postgres/
│   │   ├── 00001_init.sql        # 13 таблиц и enum-ы из 05-data-model
│   │   └── 00002_multi_platform.sql # apps.platforms, write_keys.platform, job_name + app_merge, app_merges
│   └── clickhouse/
│       ├── 00001_init.sql        # events
│       ├── 00002_aggregates.sql  # install_first_seen(+mv), user_first_seen(+mv), identity_links(+mv)
│       └── 00003_service_tables.sql # rejected_events, daily_stats
├── queries/postgres/             # sqlc (05-data-model): owner.sql passkeys.sql auth.sql apps.sql keys.sql
│                                 #   deletion_requests.sql merges.sql ops.sql
├── perf/
│   ├── ingest-load.js            # k6-сценарий `make perf-ingest` (NFR-001)
│   └── reports/baseline.json     # эталон `make perf-reports` (NFR-003); latest-файлы — результат прогона
└── testdata/
    ├── fixtures/                 # NDJSON-события + *.golden.json ожидаемые ответы отчётов (README.md — состав)
    ├── geo/test-city.mmdb        # синтетическая гео-база (`go run ./testdata/gen geo`), единственный коммитимый .mmdb
    └── gen/main.go               # генератор scenario.ndjson и test-city.mmdb
```

ClickHouse-схема разложена на три файла `00001`–`00003` (основная таблица, агрегаты с MV, служебные таблицы);
Postgres — `00001_init.sql` (схема MVP) и `00002_multi_platform.sql` (мультиплатформенные приложения и слияние:
`apps.platforms`, `write_keys.platform`, `app_merges`). Любое изменение схемы — новый файл со следующим
номером; существующие миграции не редактируются (CONTRIBUTING.md §5).

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
| `internal/api` | `gen/api`, `auth`, `apps`, `reports`, `deletion`, `merge`, `identity`, `pgstore`, `chstore`, `httpx`, `clock`, `config` | `ingest`, `worker` |
| `internal/ingest` | `gen/ingestapi`, `geo`, `ratelimit`, `chstore`, `pgstore`, `httpx`, `clock`, `config` | `api`, `reports`, `auth`, `worker` |
| `internal/worker` | `reports`, `deletion`, `merge`, `telegram`, `identity`, `chstore`, `pgstore`, `clock`, `config` | `api`, `ingest` |
| `internal/reports` | `chstore`, `identity`, `clock` | `pgstore`, `api`, `ingest`, `worker` |
| `internal/deletion` | `chstore`, `pgstore`, `identity`, `clock` | `api`, `ingest`, `worker` |
| `internal/merge` | `pgstore`, `chstore`, `deletion`, `identity`, `clock`, `config` | `api`, `ingest`, `worker`, `reports` |
| `internal/auth`, `internal/apps` | `pgstore`, `clock`, `config` | `chstore`, `reports`, `api` |
| `internal/identity`, `internal/clock`, `internal/geo`, `internal/ratelimit`, `internal/telegram` | только stdlib и сторонние библиотеки | любой `internal/*` |
| `internal/chstore`, `internal/pgstore`, `internal/httpx`, `internal/config` | `clock` | доменные пакеты |
| `internal/testutil`, `internal/archtest`, `internal/docscheck` | любой `internal/*` (тестовая инфраструктура) | — |
| `internal/gen/*`, `internal/pgstore/sqlc` | — (сгенерированы) | редактирование руками |

Новый пакет `internal/*` без строки в `archtest/rules.go` — ошибка теста: правило добавляется вместе с пакетом и
строкой в этой таблице.

Циклы запрещены; доменная логика не знает про HTTP (`reports` возвращает Go-структуры, преобразование в модели OpenAPI — в `internal/api`).

---

## 6. Хуки и типы: `web/src/hooks/`, типы из OpenAPI

```
web/src/hooks/
├── use-prefers-reduced-motion.ts # prefers-reduced-motion: без подсветки новых строк EventFeed (опрос S-07 — в components/reports/realtime-panel.tsx, S-10 — в components/integration/integration-panel.tsx)
└── use-filter-options.ts         # значения FilterBar (getAudienceBreakdown по 4 измерениям) после открытия списка

Остальное состояние живёт рядом с потребителем: URL-состояние — парсеры nuqs в lib/url-state.ts и *-view.tsx,
церемонии passkey — в components/auth/* поверх сгенерированных мутаций и lib/webauthn.ts, формы — lib/forms/use-zod-form.ts.

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
├── internal/**/main_integration_test.go        # TestMain пакета: один набор контейнеров на пакет
├── internal/archtest/arch_test.go             # правила импортов
├── internal/docscheck/*_test.go               # сниппеты MDX: go vet и curl в ingest (integration)
└── testdata/fixtures/                          # состав и назначение — testdata/fixtures/README.md
    ├── smoke.ndjson                            # дымовая фикстура харнесса
    ├── scenario.ndjson                         # детерминированный сценарий (go run ./testdata/gen -seed 42)
    └── portfolio / audience / retention / retention_week .golden.json   # эталоны Read API по сценарию

backend/internal/worker/testdata/digest.golden.txt   # эталон текста Telegram-дайджеста
scripts/reconcile/*_test.go + testdata/              # сверка с AppMetrica (отдельный модуль, make test-scripts)
scripts/test-ci-gates.sh  scripts/test-deploy.sh     # тесты shell-скриптов CI и деплоя

web/
├── src/**/<name>.test.ts(x)                    # Vitest + Testing Library + MSW (моки из lib/api/gen/endpoints/*.msw.ts)
├── src/test/setup.ts                           # jest-dom, MSW lifecycle (запрос без хендлера — ошибка), полифилы jsdom,
│                                               #   сброс NEXT_PUBLIC_API_URL / NEXT_PUBLIC_INGEST_URL перед каждым тестом
├── src/test/msw.ts  src/test/render.tsx        # хендлеры orval по умолчанию; render с QueryClient на тест
├── src/test/factories.ts                       # детерминированные фабрики данных
└── e2e/
    ├── global-setup.ts                         # passkey-link → виртуальный аутентификатор CDP → e2e/.auth/owner.json
    ├── helpers/webauthn.ts  helpers/seed.ts  helpers/a11y.ts   # виртуальный ключ; маршруты APP_ENV=e2e; axe-core
    ├── auth-passkey.spec.ts  app-onboarding.spec.ts  portfolio.spec.ts  audience.spec.ts  retention.spec.ts
    ├── realtime.spec.ts  profiles-deletion.spec.ts  settings.spec.ts  docs.spec.ts  errors.spec.ts
    └── (a11y)                                  # отдельной спеки нет: каждая спека вызывает expectNoA11yViolations
```

**Стратегия** (подробно — [22-test-strategy.md](./22-test-strategy.md)):
- TDD: тест пишется до кода; путь теста указан в каждом тикете.
- SQL отчётов проверяется только интеграционными тестами на настоящем ClickHouse с golden-файлами: моков ClickHouse нет.
- Фикстуры задают события с явными `event_time` и `received_at`; часы в тестах фиксированные (`clock.Fixed`).
- `make test` экспортирует `.env`, поэтому web-тесты не должны зависеть от локальных `NEXT_PUBLIC_*`: `src/test/setup.ts` обнуляет их перед каждым тестом.
- E2E и Lighthouse CI гоняются на собранном web (`pnpm build` + `next start`) против api с `APP_ENV=e2e`: служебные маршруты сброса и выпуска ссылки passkey (`internal/api/e2e_handlers.go`) существуют только в этом режиме.

---

## 8. Скрипты и кодогенерация

Точка входа — цели `Makefile` и подкоманды бинарника. Логика, которую неудобно держать в Makefile, вынесена в `scripts/`:

| Файл | Назначение |
|---|---|
| `scripts/check-bootstrap.sh` | проверка каркаса (`--infra` — с инфраструктурой) |
| `scripts/coverage-gate.sh` | гейт покрытия Go ≥ 80 % по ключевым пакетам (`make coverage`, CI) |
| `scripts/check-tdd-order.sh` | порядок коммитов `test(...)` → `feat(...)` относительно BASE (`make tdd-order`, CI) |
| `scripts/perf-compare.sh` | сравнение двух отчётов k6: рост p99 больше `PERF_MAX_P99_GROWTH` (20 %) — exit 1 |
| `scripts/deploy-plan.sh`, `scripts/dokploy-deploy.sh` | план и выполнение деплоя через Dokploy API (`deploy.yml`) |
| `scripts/geo-month.sh` | месяц DB-IP для сборки (текущий, иначе предыдущий) |
| `scripts/test-ci-gates.sh`, `scripts/test-deploy.sh` | тесты shell-скриптов (`make test-scripts`) |
| `scripts/reconcile/` | отдельный Go-модуль: сверка с AppMetrica (NFR-010, `make reconcile`, процедура — `docs/reconciliation/README.md`) |
| `web/scripts/check-bundle-size.ts` | бюджет initial JS 250 KB gz на страницу (`pnpm check:bundle` после `pnpm build`) |

| Операция | Команда | Что делает |
|---|---|---|
| Кодоген всего | `make gen` | `gen-go-api` + `gen-sqlc` + `gen-web` |
| Go-сервер из OpenAPI | `make gen-go-api` | `go tool oapi-codegen` (v2.8.0, tool-директива в go.mod): `contracts/openapi/ingest.yaml` → `backend/internal/gen/ingestapi/ingestapi.gen.go`; `contracts/openapi/api.yaml` → `backend/internal/gen/api/api.gen.go` |
| Доступ к Postgres | `make gen-sqlc` | Docker-образ `sqlc/sqlc:1.31.1`: `backend/migrations/postgres` + `backend/queries/postgres` → `backend/internal/pgstore/sqlc` |
| TS-клиент | `make gen-web` | `pnpm gen:api` (orval 8.42.0): `contracts/openapi/api.yaml` → `web/src/lib/api/gen/{endpoints,model,zod}` |
| Проверка актуальности | `make gen-check` | `make gen` + `git diff --exit-code` по трём каталогам (то же в CI) |
| Миграции | `make migrate-up` / `make migrate-status` | `go run ./cmd/appmetrica migrate up|status` (goose, embed.FS, оба диалекта) |
| Демо-данные | `make seed` | `go run ./cmd/seed --apps 4 --days 35 --daily-installs 400 --write-key $DEV_WRITE_KEY [--seed 1]` |
| Гео-база локально | `make geo-db` | скачивает DB-IP City Lite текущего (или прошлого) месяца в `backend/.geo/` |
| Ссылка passkey | `make passkey-link` | `go run ./cmd/appmetrica admin passkey-link` |
| Тестовые данные | `go run ./testdata/gen [-seed 42 \| geo]` | `testdata/fixtures/scenario.ndjson` и `testdata/geo/test-city.mmdb` |
| Тесты | `make test` / `make test-integration` / `make coverage` / `make fuzz` | unit Go + web + скрипты; testcontainers; покрытие с гейтом; fuzz PII-guard |
| E2E | `make e2e` | Playwright против запущенных ingest, api (`APP_ENV=e2e`) и worker |
| Перф | `make perf-ingest` / `make perf-reports` | k6 500 rps × 10 мин (NFR-001); отчёты 100k DAU × 90 дней (NFR-003) |
| Сверка | `make reconcile ARGS="-csv … -api … -app …"` | `scripts/reconcile`, ключ `sk_` в `AM_SK` |

**Что коммитится**: весь сгенерированный код (`backend/internal/gen/**`, `backend/internal/pgstore/sqlc/**`,
`web/src/lib/api/gen/**`), `go.sum`, `pnpm-lock.yaml`. **Что не коммитится**: `.env`, `backend/.geo/`, `*.mmdb`,
`web/.next/`, `web/.source/` (генерирует `fumadocs-mdx` в postinstall), отчёты покрытия и Playwright.

Почему сгенерированный код коммитится: сборка Docker-образа и `go build` не требуют генераторов; ревьюер видит
дифф контракта в виде изменений кода; CI ловит расхождение спеки и кода через `gen-check`.

---

## 9. `docs/`

```
docs/
├── discovery/2026-10-13-appmetrica-online/   # discovery-пакет (00-summary … 22-test-strategy)
└── reconciliation/README.md                  # процедура 14-дневной сверки с AppMetrica (NFR-010)

web/content/docs/                              # пользовательская документация API (S-13, за логином)
├── meta.json                                  # порядок разделов в сайдбаре (+ meta.json в каждом подразделе)
├── index.mdx                                  # Быстрый старт
├── concepts/initialization.mdx  concepts/identity.mdx  concepts/sessions.mdx  concepts/time-and-offline.mdx
├── recipes/curl.mdx  recipes/ios-swift.mdx  recipes/android-kotlin.mdx  recipes/web-js.mdx
├── recipes/bots-and-backends.mdx  recipes/go.mdx
├── errors-and-retries.mdx  privacy-and-gdpr.mdx
└── read-api/overview.mdx
```

Операционные runbook-и живут в discovery-пакете ([12-dev-runbook.md](./12-dev-runbook.md),
[21-ops-runbooks.md](./21-ops-runbooks.md)) и при изменении процедур обновляются там же. Отдельного каталога `docs/adr/`
нет: решения ADR-001…ADR-014 записаны в формате MADR в [03-architecture.md §5](./03-architecture.md); новое решение,
меняющее контракт, схему хранения или инфраструктуру, добавляется туда следующим номером. Допущения, принятые по ходу
реализации, записываются в [10-open-questions.md](./10-open-questions.md) (OQ-NNN).

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
| `scripts/reconcile/go.mod` | Отдельный модуль скрипта сверки; линтуется конфигом `backend/.golangci.yml` |
| `renovate.json` | Еженедельные PR обновлений, точные версии, автомерж patch после зелёного CI |
| `web/Dockerfile` | Образ `appmetrica-web`: Next.js standalone |
| `web/package.json` | Зависимости с точными версиями, `packageManager: pnpm@12.11.2`, скрипты |
| `web/patches/fumadocs-openapi@12.4.2.patch` | Патч pnpm к fumadocs-openapi (подключён в `pnpm-workspace.yaml`) |
| `web/lighthouserc.json` | Lighthouse CI: LCP ≤ 2,5 с, CLS < 0,1, TBT ≤ 200 мс (NFR-006) |
| `web/pnpm-workspace.yaml` | Настройки pnpm 12 (`allowBuilds: esbuild`) |
| `web/tsconfig.json` | strict, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, алиасы `@/*` и `fumadocs-mdx:collections/*` |
| `web/biome.json` | Biome 2.5.15: lint + format + organize imports, игнор `src/lib/api/gen` |
| `web/next.config.ts` | standalone, typedRoutes, cacheComponents, rewrites `/api/*`, security headers, Fumadocs MDX |
| `web/source.config.ts` | Коллекция документации Fumadocs (`content/docs`) |
| `web/orval.config.ts` | Генерация хуков TanStack Query, Zod, MSW-моков |
| `web/vitest.config.ts`, `web/playwright.config.ts` | Unit (jsdom, пороги 70/70/70/60 %) и E2E (Chromium 1440×900) |
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
| Новый эндпоинт Read API / admin API | 1) `contracts/openapi/api.yaml` (operationId из якоря) → 2) `make gen` → 3) метод в `backend/internal/api/v1_<report>.go` (Read API) или `<area>_handlers.go` (Admin API) → 4) тест рядом (`v1_<report>_test.go`, `<area>_handlers_test.go`) → 5) запрос `# @name <operationId>` в `contracts/http/requests.http` для публичной операции |
| Изменение формата ingest | `contracts/openapi/ingest.yaml` → `make gen-go-api` → `backend/internal/ingest/{decode,validate}.go` |
| Новый отчёт или метрика ClickHouse | `backend/internal/reports/<report>.go` + фикстура `testdata/fixtures/<scenario>.ndjson/.golden.json` + `<report>_integration_test.go` |
| Новая таблица / колонка Postgres | `backend/migrations/postgres/NNNNN_<change>.sql` + запросы в `backend/queries/postgres/<entity>.sql` + `make gen-sqlc` |
| Новая таблица / колонка ClickHouse | `backend/migrations/clickhouse/NNNNN_<change>.sql` (+ обновить `05-data-model.md`) |
| Фоновая задача | `backend/internal/worker/<name>.go` (функция, возвращающая `worker.Job`) + подключение в `cmd/appmetrica/worker.go` (`NewScheduler`) + имя в `config.Jobs` (`WORKER_JOBS_ENABLED`) + значение в enum `job_name` (новая миграция Postgres) |
| Новая env-переменная | `backend/internal/config/` + `.env.example` + раздел env в `12-dev-runbook.md` + Dokploy |
| Новый экран | `web/src/app/(dashboard)/<path>/page.tsx` (+ `loading.tsx`) по screen inventory из `14-ux-ui-spec.md` |
| Доменный компонент отчёта | `web/src/components/reports/<name>.tsx` + `<name>.test.tsx` |
| Примитив UI | `pnpm dlx shadcn@4.21.4 add <name>` → `web/src/components/ui/` |
| Чистая функция web | `web/src/lib/<topic>.ts` + `<topic>.test.ts` |
| Хук | `web/src/hooks/use-<name>.ts`, если нужен нескольким компонентам; иначе — в файле компонента |
| Клиентский вид одной страницы | рядом с `page.tsx`: `<screen>-view.tsx` |
| Тяжёлая клиентская библиотека | ленивая обёртка по образцу `components/common/lazy-*.tsx` + `pnpm build && pnpm check:bundle` |
| Страница документации | `web/content/docs/<section>/<slug>.mdx` + `meta.json` |
| E2E-сценарий | `web/e2e/<flow>.spec.ts` |
| Конфиг ClickHouse | `deploy/clickhouse/config.d/` или `users.d/` (+ перезапуск стека в Dokploy) |
| Архитектурное решение | новый `ADR-NNN` в `03-architecture.md §5`; допущение по ходу тикета — `10-open-questions.md` |
| Shell-скрипт CI / деплоя | `scripts/<name>.sh` + тест в `scripts/test-ci-gates.sh` или `scripts/test-deploy.sh` |

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
| 2026-10-14 | 1.1 | Сверка с кодом после MVP (Task 52): каталоги `scripts/`, `docs/reconciliation/`, `contracts/http/`; вместо `docs/adr/` — ADR в 03-architecture §5; страницы с `*-view.tsx`; компоненты по 14-ux-ui-spec §4 плюс оболочки (`app-shell`, `user-menu`, `audience-kpis`, `realtime-panel`, `app-settings-panel`, `app-danger-zone`, `tab-header`) и ленивые обёртки `lazy-*`; `lib/` (`url-state` вместо `search-params`, `forms/`, `docs/`, `errors`, `rejection-codes` и др.), хуки — только два; backend: `keycache.go` в `internal/ingest` (снимок в atomic.Pointer, last_used_at раз в 5 мин), три миграции ClickHouse 00001–00003, пакет `docscheck`, `perf/`, `testdata/{geo,gen}`; правила archtest для тестовых пакетов; фикстуры `smoke`/`scenario` + golden; E2E-спеки и a11y в каждой спеке; таблица `scripts/`; «куда положить» — имена хендлеров `v1_<report>.go` / `<area>_handlers.go` и задач worker |
| 2026-10-14 | 1.2 | Ревью MVP: `internal/worker` может импортировать `identity` (лист без зависимостей; алиас `reports.UnitPersons` убран); экранирование параметров ClickHouse — общее в `chstore/params.go`; флаг `--seed` у `cmd/seed` |
| 2026-10-14 | 1.x | Макет «Dashboard v2»: `(public)/page.tsx` (S-00 лендинг), `(dashboard)/portfolio/page.tsx` (портфель на /portfolio), `app/demo/**` и `lib/demo/**` + `lib/demo-mode.ts` (S-15 демо-режим), `components/landing/**`, `lib/theme.ts`, `hooks/use-theme.ts`, `components/layout/{theme-toggle,demo-banner}.tsx` |
| 2026-10-14 | 1.x | Публичный релиз: `deploy/selfhost/` (docker-compose.yml, .env.example, Caddyfile — установка на любой сервер), `README.md` (EN) и `README.ru.md` (RU), `LICENSE` (AGPL-3.0) |
| 2026-10-15 | 1.x | Мультиплатформенные приложения и слияние: пакет `internal/merge` (строка в §5, импортируется `api` и `worker`), `worker/merge.go` (`app_merge`), `migrations/postgres/00002_multi_platform.sql`, `queries/postgres/merges.sql`; seed — 4 приложения (`--apps 4`), у Postgres больше не одна миграция |
