# Tech Stack: appmetrica.online

**Версия**: 1.0 | **Дата**: 2026-10-13

> Выбор стека с обоснованием и рассмотренными альтернативами.
> Каждое решение — не просто «что выбрано», а «почему выбрано».
> Альтернативы, которые были отброшены, — с причиной отказа.
> Через 6 месяцев этот документ позволит вспомнить контекст решения.
>
> Все версии сверены 2026-10-13 по первоисточникам (npm registry, proxy.golang.org, GitHub Releases, Docker Hub, официальные сайты).
> context7 MCP в сессии был недоступен (нет авторизации), поэтому версии из памяти модели не использовались.
> Канонические имена сервисов, таблиц и эндпоинтов — в [03-architecture.md](./03-architecture.md), [05-data-model.md](./05-data-model.md), [06-api-contracts.md](./06-api-contracts.md).

---

## Общий стек (одной таблицей)

| Слой | Выбор | Версия | Alternative considered |
|---|---|---|---|
| Язык бэкенда | Go | 1.27.2 | TypeScript (Node/Bun + Hono), Rust |
| Язык фронтенда | TypeScript (strict) | 6.0.3 | TypeScript 7.0.2 (нативный компилятор) |
| Runtime фронтенда | Node.js LTS | 24.21.0 | Node.js 26.11.1 (LTS с 2026-10-28), Bun |
| Package manager (web) | pnpm | 12.11.2 | npm, bun |
| Frontend framework | Next.js (App Router) | 16.4.0 + React 19.3.0 | Vite SPA, встроенный в Go; Go + htmx |
| UI библиотека | shadcn/ui (база Radix) | CLI 4.21.4, `radix-ui` 1.8.1 | Base UI (новый дефолт shadcn), MUI, Mantine |
| CSS | Tailwind CSS | 4.3.3 | CSS Modules |
| Графики | Recharts (через shadcn `chart`) | 3.10.1 | Apache ECharts 6.1.0, visx 4.0.0 |
| Backend pattern | Go stdlib `net/http` ServeMux + oapi-codegen strict server | oapi-codegen 2.8.0 | chi-router, echo v5, ogen |
| Контракт API | OpenAPI 3.0.3 (contract-first) | — | OpenAPI 3.1, gRPC, tRPC |
| TS-клиент API | orval (TanStack Query + Zod + MSW) | 8.42.0 | openapi-typescript + openapi-fetch, hey-api |
| Доступ к Postgres | pgx + sqlc | pgx 5.11.0, sqlc 1.31.1 | GORM, ent, sqlx |
| Миграции | goose (Postgres + ClickHouse) | 3.29.0 | golang-migrate, Atlas |
| БД событий | ClickHouse LTS | 26.8.25.14 | PostgreSQL + TimescaleDB, Tinybird, ClickHouse Cloud |
| БД метаданных | PostgreSQL | 18.6 | SQLite + Litestream |
| ClickHouse-клиент | clickhouse-go/v2 | 2.49.0 | ch-go 0.74.0 |
| Auth | Passkey (WebAuthn): go-webauthn + @simplewebauthn/browser | 0.18.2 / 14.0.0 | Пароль + TOTP, GitHub OAuth, Tailscale-only |
| Payments | — (не нужны) | — | — |
| Email | — (не нужен; уведомления в Telegram) | — | — |
| Уведомления | Telegram Bot API через `net/http` | Bot API 10.3 | go-telegram/bot 1.27.0, telego 1.12.1 |
| Object storage | Hetzner Object Storage (S3) | fsn1 | AWS S3, Cloudflare R2 |
| Cache / queue | Нет (in-process буферы, ClickHouse async insert) | — | Kafka, Redis, NATS |
| Background jobs | Собственный планировщик в `appmetrica worker` | — | River, asynq, cron в Dokploy |
| Hosting | Hetzner AX102-1 (FSN1) + Dokploy | Dokploy ≥ 0.30.8 | Coolify 4.4.6, GitHub Actions + compose по SSH, Kamal 2 |
| Сеть админки | Tailscale | — | Публичная панель + 2FA, WireGuard вручную |
| Гео | DB-IP City Lite + geoip2-golang | 2026-10 / 2.5.0 | MaxMind GeoLite2, без гео |
| Бэкапы ClickHouse | Altinity clickhouse-backup | 2.8.1 | Встроенный `BACKUP … TO S3()`, бэкап volume Dokploy |
| Error tracking | Логи slog JSON + Prometheus-счётчики | client_golang 1.25.0 | Sentry, GlitchTip |
| Мониторинг доступности | Worker `system_check` + healthchecks.io | Hobbyist ($0) | UptimeRobot, Beszel, Netdata |
| Testing (Go) | `testing` + testify + testcontainers-go | 1.12.1 / 0.44.0 | ginkgo/gomega |
| Testing (unit web) | Vitest + Testing Library | 5.0.4 / 16.3.3 | — |
| Testing (e2e) | Playwright | 1.64.0 | Cypress |
| API-моки web | MSW | 3.0.3 (fallback 2.15.0) | — |
| CI/CD | GitHub Actions → GHCR → Dokploy API | — | GitLab CI, Dokploy webhook на push |
| Monorepo | Один репозиторий, `Makefile`, без workspace-тулинга | GNU Make | Turborepo, Nx |
| Lint/format (web) | Biome | 2.5.15 | ESLint 9 + Prettier |
| Lint (Go) | golangci-lint v2 | 2.14.0 | staticcheck отдельно |
| Git hooks | lefthook | 2.2.2 | husky |

---

## Детально по каждому решению

### 1. Язык бэкенда — Go 1.27.2

**Выбрано**: Go 1.27.2 (релиз 2026-10-08, содержит security-фиксы `net/http` и `crypto/tls`). В `backend/go.mod`: `go 1.27` + `toolchain go1.27.2`.
Build-образ `golang:1.27.2-alpine`, runtime-образ `gcr.io/distroless/static-debian12:nonroot`, `CGO_ENABLED=0`.

**Rationale**:
- Владелец уверенно пишет и деплоит Go — может ревьюить код агента и чинить инциденты сам.
- Ingestion — горячий путь: до ~500 запросов/с в пике и батчи по 500 событий. Go даёт предсказуемую память, один статический бинарник и goroutine-воркеры для батч-вставок в ClickHouse.
- Один бинарник `appmetrica` с подкомандами `ingest`, `api`, `worker`, `migrate`, `admin`, `healthcheck` — три контейнера из одного образа, один набор зависимостей и тестов.
- Distroless-образ без shell уменьшает поверхность атаки; HEALTHCHECK выполняется самим бинарником (`appmetrica healthcheck --url …`), curl не нужен.

**Альтернативы**:
- **TypeScript везде (Node/Bun + Hono)**: один язык с дашбордом, но владелец выбрал Go для API (интервью, вопрос «Язык»): надёжнее под нагрузкой, меньше RAM, проще эксплуатация.
- **Rust (axum)**: максимум производительности, но у владельца нет экспертизы, а нагрузка в 500 rps не требует Rust.

**Риски**:
- Supply chain Go-модулей. Mitigation: `go.sum` в репо, `govulncheck` в CI, Renovate с ревью.

---

### 2. Runtime фронтенда — Node.js 24.21.0 LTS

**Выбрано**: Node.js 24.21.0 LTS «Krypton» (2026-09-07), образ `node:24.21.0-alpine`. pnpm 12.11.2 закреплён в `web/package.json` через `"packageManager": "pnpm@12.11.2"`.

**Rationale**:
- Next.js 16.4 требует Node ≥ 20.9; vitest 5 — Node ^22.12 || ^24 || ≥26; orval 8 — Node ≥ 22.18. Node 24 закрывает все требования.
- Ветка 24.x поддерживается до 2028-04-30 (maintenance с 2026-10-20).
- В Node 25+ нет Corepack — pnpm ставится в Dockerfile явно (`npm i -g pnpm@12.11.2` в build-стадии образа, не в CI-шаге сборки зависимостей).

**Альтернативы**:
- **Node.js 26.11.1**: станет LTS 2026-10-28 — через 15 дней после старта проекта. Переход оформлен open question OQ в [10-open-questions.md](./10-open-questions.md): обновить после выхода LTS и зелёного CI.
- **Bun**: быстрее установка и старт, но Next.js 16 в проде официально ориентирован на Node; выигрыш для внутреннего дашборда не оправдывает риск несовместимостей.

---

### 3. Frontend framework — Next.js 16.4.0 + React 19.3.0

**Выбрано**: Next.js 16.4.0 (2026-10-06), App Router, `output: 'standalone'`, React 19.3.0. TypeScript 6.0.3 strict.

**Rationale**:
- Выбор владельца (интервью, вопрос «Фронтенд»): отдельный сервис, а не SPA внутри Go.
- Документация API (Fumadocs) нативно встраивается в App Router в route group `(docs)` того же приложения — один деплой для дашборда и доков.
- RSC для первой отрисовки отчётов (данные с Go API на сервере), Client Components только для интерактивных фильтров, графиков и realtime-поллинга.
- `src/proxy.ts` (переименованный `middleware.ts` в Next 16) выполняет дешёвую проверку наличия cookie `am_session` и редирект на `/login`.
- Cache Components в 16.4 включены по умолчанию — **страницы с данными владельца не используют `'use cache'`**, иначе ответы с данными сессии попадут в кэш. Это правило закреплено в `AGENTS.md`.

**TypeScript 6.0.3, а не 7.0.2**: TS 7 (нативный компилятор на Go) не имеет JS API; на нём не работают openapi-typescript (#2841), hey-api (#4235); typescript-eslint требует `<6.1.0`; страница настройки TS 7 в доках Next.js помечена experimental. Переход — после TS 7.1 со стабильным API.

**Альтернативы**:
- **Vite SPA, вшитая в Go-бинарник (embed)**: один процесс без Node в проде — рекомендовался, владелец выбрал Next.js. Цена выбора — второй runtime в проде.
- **Go + templ + htmx**: минимум JS, но интерактивные фильтры, графики и heatmap retention делать неудобно.
- **TanStack Start / React Router 7**: моложе по экосистеме доков (Fumadocs заточен под Next.js).

**Risk**: второй runtime и сборка ~1–2 минуты. Mitigation: standalone-образ, сборка только в GitHub Actions, не на прод-сервере.

---

### 4. UI — shadcn/ui (Radix) + Tailwind CSS 4.3.3

**Выбрано**: shadcn CLI 4.21.4, инициализация **явно с Radix**: `pnpm dlx shadcn@4.21.4 init -b radix`. Tailwind 4.3.3 + `@tailwindcss/postcss` 4.3.3, CSS-first конфиг через `@theme`. Иконки lucide-react 1.56.0. Анимации tw-animate-css 1.4.0.

**Rationale**:
- Копируемые компоненты в собственном репо (`web/src/components/ui`) — агент правит их как обычный код, без борьбы с чужой темой.
- Radix — проверенная accessibility (WCAG 2.2 AA, фокус-менеджмент, клавиатура) и больше обучающих примеров у кодинг-агентов, чем у Base UI, ставшего дефолтом shadcn в июле 2026.
- Стиль AppMetrica (светлая тема, карточки-виджеты, синий акцент) собирается из `Card`, `Table`, `Tabs`, `Select`, `Popover`, `Calendar`, `Tooltip`, `Dialog`, `Command` + токены в [14-ux-ui-spec.md](./14-ux-ui-spec.md).
- С сентября 2026 shadcn берёт `cn` из пакета `cn` 0.4.0 — фиксируем его в зависимостях.
- Зависимости примитивов shadcn (ставятся CLI при `add`, версии фиксируются точно): `cmdk` 1.1.1 — основа `command` (AppSwitcher, поиск часового пояса); `react-day-picker` 10.0.2 — основа `calendar` (PeriodPicker, локаль `ru` из date-fns). Альтернатива — писать комбобокс и календарь на голых Radix-примитивах: больше кода и своя доступность клавиатуры, выигрыша нет.
- Что CLI ставит, но мы **не** берём: `shadcn` как runtime-зависимость (из него нужен только `shadcn/tailwind.css` с кастомными вариантами `data-open:` и т. п. — файл скопирован в `web/src/app/shadcn.css`), `next-themes` у `sonner` (тёмной темы нет, тема Toaster зафиксирована `light`). После `add chart` CLI откатывает recharts на 3.8.0 — возвращаем 3.10.1.

**Альтернативы**:
- **Base UI** (новый дефолт shadcn): современнее, но меньше наработанных паттернов; Radix выбран ради предсказуемости.
- **MUI**: Material-вид не совпадает со стилем AppMetrica, тяжелее бандл.
- **Mantine**: batteries-included, но кастомизация под точный стиль сложнее, чем правка исходников shadcn.

**Иконки**: в lucide-react v1 удалены бренд-иконки (Apple, Android, GitHub) — логотипы платформ хранятся собственными SVG в `web/src/components/icons/platform/*.tsx`.

---

### 5. Backend pattern — Go `net/http` + oapi-codegen strict server

**Выбрано**: stdlib `net/http` ServeMux (паттерны `GET /v1/apps/{app_id}`), код серверов генерируется oapi-codegen 2.8.0 (`strict-server` + `std-http-server`) из `contracts/openapi/ingest.yaml` и `contracts/openapi/api.yaml`; runtime 1.7.0; nullable 1.1.0 (`nullable.Nullable[T]` для `nullable: true` в сгенерированных моделях); валидация запросов по спеке — `nethttp-middleware` 1.2.0 (kin-openapi 0.149.0). Middleware — цепочка `func(http.Handler) http.Handler`; из chi подключается только пакет `github.com/go-chi/chi/v5/middleware` 5.4.0 (RequestID, Recoverer).

**Rationale**:
- Контракт-первым: одна спека → Go-сервер, TS-клиент (orval), доки (Fumadocs). Расхождение кода и доков исключено конструктивно.
- Strict server даёт типизированные request/response — агент не может забыть поле ответа.
- Горячий хендлер `POST /v1/events` пишется руками поверх сгенерированных типов (нужен контроль над gzip, лимитом тела, частичным успехом), остальные — через strict server.
- WebAuthn-эндпоинты используют `CreateCredential` / `ValidatePasskeyLogin` go-webauthn на сырых байтах тела: `Finish*`-хелперы сами читают `*http.Request`, а strict server к этому моменту тело уже разобрал.
- Сгенерированный код коммитится в `backend/internal/gen/{ingestapi,api}`; CI проверяет, что `make gen` не даёт diff.

**Альтернативы**:
- **chi-router 5.4.0**: `Route/Group/Mount` не нужны — stdlib-паттернов достаточно; chi берём только ради middleware.
- **echo v5**: собственный `echo.Context` ломает совместимость с net/http-экосистемой; ветка v4 получает фиксы только до 2026-12-31.
- **ogen 1.24.0**: быстрее, OTel из коробки, но собственный роутер и громоздкие обёртки `Opt*/Nil*`; поддержка OAS 3.1 частичная.
- **OpenAPI 3.1**: в oapi-codegen 2.8 поддержка «initial»; 3.0.3 одинаково понимают oapi-codegen, orval и fumadocs-openapi (последний сам поднимает 3.0 → 3.1 для рендера).

**`github.com/oapi-codegen/nullable` 1.1.0** (добавлено в T-002): runtime-зависимость сгенерированного кода — конфиги `oapi-*.cfg.yaml` включают `output-options.nullable-type: true`, и поля `nullable: true` генерируются как `nullable.Nullable[T]` (различает «поле отсутствует» и `null`, нужно для PATCH `updateApp`/`updateSettings`). Версия — та, что выбирает MVS вместе с oapi-codegen 2.8.0. Альтернатива — выключить `nullable-type` и получать `*T`, но тогда PATCH не отличает `null` от пропущенного поля.

**`github.com/getkin/kin-openapi` 0.149.0** — прямая зависимость с T-002: структурная валидация спек (`openapi3.Loader` + `doc.Validate`) и проверка ответов в контрактных тестах (`openapi3filter`).

**Требование безопасности**: oapi-codegen не ниже 2.8.0 — в 2.7.x были advisories о code injection (GHSA).

---

### 6. Доступ к данным — pgx 5.11.0 + sqlc 1.31.1 + goose 3.29.0; clickhouse-go 2.49.0

**Выбрано**:
- Postgres: `pgxpool` (pgx/v5 5.11.0), запросы в `backend/queries/postgres/*.sql`, код генерирует sqlc 1.31.1 (`sql_package: pgx/v5`) в `backend/internal/pgstore/sqlc`. sqlc запускается Docker-образом `sqlc/sqlc:1.31.1` из `make gen` (у sqlc cgo-зависимость pg_query_go — не тянем её в `go tool`).
- Миграции: goose 3.29.0, встроены в бинарник через `embed.FS`, диалекты `postgres` и `clickhouse`. Каталоги `backend/migrations/postgres` и `backend/migrations/clickhouse`. Каждая ClickHouse-миграция помечена `-- +goose NO TRANSACTION`, по одному DDL в блоке `StatementBegin/End`.
- ClickHouse: `clickhouse-go/v2` 2.49.0, native-протокол (порт 9000), вставка батчами через `PrepareBatch` с контекстом `clickhouse.WithAsync(true)` (ожидание подтверждения async insert). Устаревший `AsyncInsert()` не используется.

**Rationale**:
- SQL-first: аналитика — это SQL, и владельцу (без опыта ClickHouse) проще читать явные запросы, чем DSL.
- sqlc даёт типобезопасность для метаданных без ORM-магии.
- goose — один инструмент и один формат (SQL + embed) для обеих БД; служебная таблица версий goose в ClickHouse создаётся на MergeTree и попадает в бэкап clickhouse-backup.

**Альтернативы**:
- **golang-migrate 4.20.1**: для ClickHouse служебная таблица по умолчанию на TinyLog (не бэкапится clickhouse-backup), multi-statement режим режет по `;`.
- **Atlas 1.3.0**: драйвер ClickHouse только в Pro.
- **GORM / ent**: ORM поверх аналитики не помогает, а для 13 таблиц метаданных избыточен.

**Риски**: sqlc 1.31.1 использует грамматику PostgreSQL 17 — синтаксис, появившийся только в PG 18, может не разобраться. Mitigation: в запросах не используем PG18-специфичный синтаксис; `uuidv7()` вызывается только в `DEFAULT` миграций (goose, не sqlc), а в запросах — с явным `::uuid`.

---

### 7. Database — ClickHouse 26.8.25.14 LTS + PostgreSQL 18.6

**Выбрано**:
- **ClickHouse 26.8.25.14 LTS** (`clickhouse/clickhouse-server:26.8.25.14`, GA ветки 2026-08-30) — события, связки identity, агрегаты, отклонённые события. Compose-сервис в Dokploy с named volumes `clickhouse-data`, `clickhouse-logs`.
- **PostgreSQL 18.6** (`postgres:18.6-trixie`) — managed-база Dokploy: приложения, ключи, passkeys, сессии, запросы удаления, настройки, журнал задач. Volume монтируется в `/var/lib/postgresql` (в образах 18+ PGDATA = `/var/lib/postgresql/18/docker`).

**Rationale для ClickHouse**:
- Объём: ≈ 18 млн событий в месяц (≈ 182 млн строк в год) при 100k DAU и одних системных событиях; верхняя граница с кастомными событиями (P1) — 150 млн в месяц, 1–2 млрд строк в год; хранение 25 месяцев. Колоночное хранение со сжатием даёт ~100 ГБ/год и секундные `uniqExact` по десяткам миллионов строк.
- `ReplacingMergeTree` + `AggregatingMergeTree` + materialized views закрывают дедупликацию, first_seen и связки identity без отдельного сервиса.
- LTS-ветка: security-фиксы получают 26.9, 26.8, 26.7, 26.3; 25.8 LTS уже без поддержки.

**Rationale для PostgreSQL**:
- Транзакционные метаданные (ключи, passkeys, сессии) не место для ClickHouse.
- Dokploy умеет managed Postgres с бэкапами в S3 из коробки — меньше ops, чем SQLite + Litestream.
- 18.6 — актуальная стабильная, EOL 2030-11-14.

**Альтернативы**:
- **PostgreSQL + TimescaleDB для событий**: проще одна БД, но `count distinct` по сотням миллионов строк и retention-матрицы на ней медленнее на порядок.
- **ClickHouse Cloud / Yandex Managed ClickHouse**: меньше ops, но от ~$100–300/мес, данные у третьей стороны (противоречит мотиву «уйти от зависимости»).
- **SQLite + Litestream для метаданных**: работает, но требует отдельного процесса бэкапа; Dokploy его не обслуживает.

**Риски**: нет экспертизы ClickHouse у владельца. Mitigation: [21-ops-runbooks.md](./21-ops-runbooks.md) (диск, мутации, бэкап/восстановление, медленные запросы) + все аналитические запросы покрыты тестами на фикстурах через testcontainers.

---

### 8. Auth — Passkey (go-webauthn 0.18.2 + @simplewebauthn/browser 14.0.0)

**Выбрано**: только passkey, без пароля. Сервер — `github.com/go-webauthn/webauthn` 0.18.2 (discoverable credentials, conditional UI через `BeginDiscoverableMediatedLogin`). Браузер — `@simplewebauthn/browser` 14.0.0: `startRegistration({ optionsJSON: resp.publicKey })`, `startAuthentication({ optionsJSON: resp.publicKey, useBrowserAutofill: true })`. Сессия — cookie `am_session` (HttpOnly, Secure, SameSite=Lax), токен 32 байта, в БД только sha256.

**Rationale**:
- Выбор владельца (интервью, вопрос «Вход»): фишинг-устойчиво, Face ID / Touch ID, нечего утекать.
- Один пользователь — нет нужды в Clerk/Auth.js и внешних провайдерах; auth живёт в Go рядом с данными.
- Первая регистрация и восстановление — `appmetrica admin passkey-link` (одноразовая ссылка на 15 минут), не зависит от email.

**Альтернативы**:
- **Пароль + TOTP**: рекомендовался как самый независимый; владелец выбрал passkey.
- **GitHub OAuth**: вход зависит от GitHub.
- **Tailscale-only дашборд**: с телефона нужен клиент Tailscale; владелец не выбрал.
- **Clerk / Auth.js / Better Auth**: избыточны для одного аккаунта и тянут auth в Next.js, а данные и API — в Go.

**Риски**: go-webauthn версии 0.x, в 0.18.0 были breaking changes; требует Go ≥ 1.26. Mitigation: версия закреплена, обёртка `internal/auth/webauthn.go` изолирует API библиотеки, e2e-тест входа через виртуальный аутентификатор Playwright (CDP WebAuthn).

**Cost projection**: $0 при любом числе устройств.

**Exit strategy**: схема `passkeys` хранит стандартные поля WebAuthn (credential_id, public_key, sign_count, aaguid) — переносится в любую WebAuthn-библиотеку без перерегистрации.

---

### 9. Payments — не применимо

> **N/A для этого проекта.** Причина: внутренний инструмент одного владельца, монетизации и биллинга нет (интервью, вопрос «Цель»: «Свои приложения»).
> (Относится к: категория монетизации; пересмотр — только при решении сделать публичный SaaS, см. [10-open-questions.md](./10-open-questions.md).)

---

### 10. Hosting — Hetzner AX102-1 + Dokploy ≥ 0.30.8

**Выбрано**: выделенный сервер **Hetzner AX102-1** в Falkenstein (FSN1): AMD Ryzen 9 7950X3D (16 ядер), 128 ГБ DDR5, 2 × 1,92 ТБ NVMe Datacenter в программном RAID1. PaaS — **Dokploy ≥ 0.30.8** (Docker Swarm + Traefik v3.6). Панель Dokploy и SSH доступны только через Tailscale.

**Rationale**:
- Пользователи в ЕС, бюджет не ограничен, выбор по технике: RAID1 даёт ~1,92 ТБ полезного места — ClickHouse за 25 месяцев займёт ~210 ГБ, остаётся запас под merge (×2 на крупные части), Postgres, образы и локальные артефакты бэкапа.
- 128 ГБ RAM: ClickHouse `max_memory_usage=8G` на запрос API с запасом на page cache и параллельные отчёты.
- Dokploy: выбор владельца. Applications из образов GHCR получают zero-downtime через Swarm (`"Order":"start-first","FailureAction":"rollback"`) и ручной откат на предыдущий образ.

**Cost**: см. раздел «Инфраструктура и стоимость» ниже — около 265,49 € в месяц плюс 129 € единоразово.

**Альтернативы**:
- **AX42-1 (97,30 €/мес)**: 2 × 512 ГБ в RAID1 — 512 ГБ полезного места, впритык через 2 года с учётом merge.
- **EX63-1 (147,30 €/мес)**: 64 ГБ ECC, 2 × 1 ТБ — ~0,93 ТиБ, на границе требования «≥ 1 ТБ полезного».
- **Coolify 4.4.6** (рекомендовался в интервью): стабильная ветка v4 с апреля 2026, полностью Apache-2.0, managed ClickHouse с бэкапом в S3, встроенные алерты «диск > 80 %» и «контейнер упал». **Владелец выбрал Dokploy вопреки рекомендации** — мы фиксируем это решение и закрываем его слабые места: нет алертов по диску и контейнерам → worker `system_check` + healthchecks.io; ClickHouse только шаблоном с `:latest` → свой compose с закреплённым тегом; CVE без авторизации (CVE-2026-45631, CVSS 10; CVE-2025-53825, CVSS 9.8) → панель только через Tailscale, версия ≥ 0.30.8.
- **GitHub Actions + docker compose по SSH**: минимум слоёв, но без UI, без откатов в один клик; владелец предпочёл PaaS.
- **Kamal 2**: zero-downtime из коробки, но Ruby-тулинг и ещё один инструмент.

**Exit**: все сервисы — обычные Docker-образы + compose-файл ClickHouse; переезд на Coolify или голый compose — 1–2 дня (перенос env, доменов и volumes).

---

### 11-N. Остальные решения

#### 11. Графики — Recharts 3.10.1 через shadcn `chart`
- **Почему**: SVG, декларативно, темизация через CSS-переменные shadcn; линии, столбцы, area, sparkline (`LineChart` без осей). После `shadcn add chart` (пинит recharts 3.8.0) версия обновляется до 3.10.1.
- **Retention-heatmap — HTML-таблица** с цветовой шкалой Tailwind, а не график: sticky-шапка, копирование значений, доступность для скринридеров.
- **Альтернативы**: ECharts 6.1.0 — Canvas, dataZoom, но тяжёлый (сотни КБ) и обёртка echarts-for-react 3.0.6 обновляется редко; нужен только при сериях > 10 000 точек, чего в MVP нет (максимум 7 дней × 24 часа = 168 точек на hour-графике, 366 точек на day). visx 4.0.0 — слишком низкоуровневый.

#### 12. TS-клиент API — orval 8.42.0
- **Почему**: из `contracts/openapi/api.yaml` генерирует хуки TanStack Query v5, `fetch`-клиент с собственным mutator (`web/src/lib/api/fetcher.ts`: `credentials: 'include'`, заголовок `X-Requested-With: appmetrica-web`, редирект на `/login` при 401), Zod-схемы и MSW-хендлеры для тестов. Не зависит от компилятора TS — готов к TS 7.
- **Альтернативы**: openapi-typescript 7.13.0 + openapi-fetch 0.17.0 — изящнее API, но без релизов с февраля 2026 и с проблемами на TS 6/7; @hey-api/openapi-ts 0.99.0 — версия 0.x с частыми breaking changes.
- **Риск**: MSW-моки orval не проверены с MSW 3.0.3. Mitigation: fallback на msw 2.15.0 описан в [12-dev-runbook.md](./12-dev-runbook.md).

#### 13. Документация API — Fumadocs (core/ui 16.17.2, mdx 15.4.7, openapi 12.4.2)
- **Почему**: MDX-гайды с вкладками кода (Swift, Kotlin, JS, Go, curl) и подсветкой shiki плюс страницы API-reference из тех же `ingest.yaml` и `api.yaml` — нативно в App Router, route group `(docs)`, за логином (проверка сессии в `app/(docs)/layout.tsx`, без `'use cache'`).
- **Playground** (`@scalar/api-client-react`) выключен: доки за логином, отправка запросов из браузера на боевой ingest не нужна.
- **Альтернативы**: Scalar (`@scalar/nextjs-api-reference` 0.12.10) — хорош как одностраничный справочник, но системы гайдов нет; Mintlify/ReadMe — внешний хостинг, противоречит «данные у себя».

#### 14. Гео — DB-IP City Lite (2026-10) + geoip2-golang/v2 2.5.0
- **Почему**: CC BY 4.0, без регистрации и ключа, IPv4 + IPv6 (проверено в данных за октябрь 2026), ~121 МБ mmdb, ежемесячное обновление; схема близка к GeoIP2 City → типизированный `City(netip.Addr)`.
- **Поставка**: mmdb скачивается в CI при сборке образа (`https://download.db-ip.com/free/dbip-city-lite-YYYY-MM.mmdb.gz`, кэш по месяцу), кладётся в `/app/geo/dbip-city-lite.mmdb`; workflow `geo-refresh.yml` (cron `0 6 2 * *`) пересобирает и деплоит ingest.
- **Атрибуция**: «IP Geolocation by DB-IP» со ссылкой https://db-ip.com в футере дашборда — обязательное условие лицензии.
- **Альтернативы**: MaxMind GeoLite2 — нужен аккаунт, license key и EULA с обязанностью обновлять базу; без гео — владелец выбрал «страна + город».
- **Тестовая mmdb**: `backend/testdata/geo/test-city.mmdb` собирает `github.com/maxmind/mmdbwriter` 1.2.0 (официальный писатель формата MaxMind DB, Apache-2.0/MIT, сам читает через maxminddb-golang/v2) из `internal/testutil/geodb.go`; перегенерация — `go run ./testdata/gen geo`. В production-бинарник не попадает (импортируется только тестовой инфраструктурой). Альтернативы: копия `GeoIP2-City-Test.mmdb` из MaxMind-DB — нет IPv6 Яндекса и другой `database_type`; собственный писатель формата — лишний код без проверки.

#### 15. Очереди — не используем
- **Почему**: при 500 rps и записи батчами ClickHouse async insert сам буферизует вставки; ответ 200 отдаётся после подтверждения записи. Kafka/Redis добавили бы два stateful-компонента ради нагрузки, которой нет.
- **Триггер пересмотра**: устойчивые > 5 000 событий/с или требование принимать события при недоступном ClickHouse дольше, чем клиенты держат очередь (7 дней).

#### 16. Rate limiting — golang.org/x/time/rate 0.16.0
- Token bucket в процессе ingest: по приложению 2 000 событий/с (burst 4 000) → 429 на весь батч; по `install_id` 100 событий/мин (burst 1 000) → отказ события `install_rate_limited` с `retryable=true`. Карта лимитеров с TTL-эвикцией.
- **Не по IP**: за мобильным CGNAT тысячи пользователей делят один IP. Traefik `rateLimit` (по IP, в памяти) не используется.

#### 17. Уведомления — Telegram Bot API через `net/http`
- Один `POST https://api.telegram.org/bot<token>/sendMessage` с JSON `{chat_id, text, parse_mode}`, обработка 429 по `retry_after`, тесты через `httptest`. Библиотека (go-telegram/bot 1.27.0) понадобится только для входящих команд — их нет.

#### 18. Бэкапы — clickhouse-backup 2.8.1 + Hetzner Object Storage
- Sidecar `altinity/clickhouse-backup:2.8.1` в режиме `server --watch`: инкрементальный бэкап каждые 24 ч (`WATCH_INTERVAL`), полный раз в 168 ч (`FULL_INTERVAL`), хранить 35 удалённых (`BACKUPS_TO_KEEP_REMOTE`). REST API на 7171 — worker читает `/backup/list/remote` для проверки свежести.
- Postgres — встроенный бэкап Dokploy в бакет `appmetrica-pg-backup`, хранить 30.
- **Альтернатива** — встроенный `BACKUP DATABASE … TO S3()` с `base_backup`: работает, но инкременты и ротацию пришлось бы писать самим.

#### 19. Наблюдаемость — slog JSON + Prometheus client_golang 1.25.0 + healthchecks.io
- Логи: `log/slog` JSONHandler, поля `request_id`, `service`, `app_id`, `status`, `duration_ms`; IP клиента в логи не пишется (NFR-007).
- Метрики: `/metrics` на `METRICS_LISTEN_ADDR` (:9090), не публикуется наружу; ключевые: `ingest_events_total{result}`, `ingest_batch_duration_seconds`, `ingest_rejections_total{code}`, `clickhouse_insert_errors_total`, `report_query_duration_seconds{report}`.
- Доступность: worker `system_check` каждые 5 мин → Telegram-алерты; после успешного цикла — ping `https://hc-ping.com/<uuid>`; тишина > 10 мин → healthchecks.io сам шлёт алерт в Telegram (сервер мёртв целиком).

#### 20. Тестирование
- Go: `testing` + testify 1.12.1 (`require` для фатальных проверок) + testcontainers-go 0.44.0 с модулями `clickhouse` и `postgres` (образы те же, что в проде: 26.8.25.14 и 18.6). Golden-фикстуры отчётов в `backend/testdata/fixtures`.
- Web: Vitest 5.0.4 + @testing-library/react 16.3.3 + @testing-library/user-event 14.6.8 + @testing-library/jest-dom 7.0.1 (матчеры DOM, совместимы с Vitest) + jsdom 30.1.2; MSW 3.0.3; @faker-js/faker 10.6.0 (devDependency: на нём построены MSW-моки orval `*.faker.ts`; в тестах — `faker.seed(...)`, см. 22-test-strategy.md §6).
- E2E: Playwright 1.64.0 + @axe-core/playwright 4.13.0; вход через виртуальный WebAuthn-аутентификатор.
- Перф-бюджеты (T-044, NFR-006): @lhci/cli 0.15.1 (devDependency, Lighthouse 12.6.1) — `pnpm lhci autorun` по `web/lighthouserc.json` в job e2e; бюджет initial JS — `web/scripts/check-bundle-size.ts`, запускается `node` 24 без tsx (Node сам снимает типы), читает `.next/diagnostics/route-bundle-stats.json` сборки Turbopack.
- Нагрузка: k6 2.3.0 — Docker-образ `grafana/k6:2.3.0` (тег подтверждён `docker pull` 2026-10-13, `k6 version` → v2.3.0), не зависимость модулей; сценарий `backend/perf/ingest-load.js`, запуск `make perf-ingest`.
- Подход — TDD (решение владельца), детали в [22-test-strategy.md](./22-test-strategy.md).

#### 21. Lint/format и git-хуки
- Web: Biome 2.5.15 (`css.parser.tailwindDirectives: true` для Tailwind 4). Go: golangci-lint 2.14.0 (конфиг `version: "2"`), ставится бинарём/образом, не как зависимость модуля. Хуки: lefthook 2.2.2 (pre-commit: biome check, gofmt; commit-msg: Conventional Commits).

#### 22. CI/CD — GitHub Actions → GHCR → Dokploy API
- Сборка образов только в CI. Деплой: `tailscale/github-action` (вход в tailnet по OAuth-клиенту) → `POST $DOKPLOY_URL/api/application.saveDockerProvider` (новый тег `<sha>`) → `POST $DOKPLOY_URL/api/application.deploy` с `x-api-key`.
- Политика хранения GHCR: держать последние 10 тегов каждого образа (`actions/delete-package-versions`).

#### 23. URL-state, даты, валидация
- nuqs 2.10.2 (`nuqs/adapters/next/app`) — период, детализация, фильтры в URL; ссылку на отчёт можно сохранить.
- date-fns 4.4.0 — форматирование дат на русском (`ru` locale); арифметика по TZ приложения делается на сервере (ClickHouse), клиент только форматирует.
- Zod 4.6.7 — схемы, сгенерированные orval, + формы.

#### 24. Разбор User-Agent — не нужен
- Клиенты присылают контекст устройства явно (`EventContext`: platform, os_name, os_version, device_model). Рецепт для веба в доках собирает его из `navigator.userAgentData`/`navigator.userAgent` на клиенте. Сервер не парсит UA — меньше кода и нет зависимости (medama-io/go-useragent 1.2.5 рассмотрен и отложен).

---

## Полный список зависимостей

### backend/go.mod

```go
module appmetrica.online/backend

go 1.27

toolchain go1.27.2

require (
	github.com/ClickHouse/clickhouse-go/v2 v2.49.0
	github.com/getkin/kin-openapi v0.149.0
	github.com/go-chi/chi/v5 v5.4.0
	github.com/go-webauthn/webauthn v0.18.2
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/maxmind/mmdbwriter v1.2.0
	github.com/oapi-codegen/nethttp-middleware v1.2.0
	github.com/oapi-codegen/nullable v1.1.0
	github.com/oapi-codegen/runtime v1.7.0
	github.com/oschwald/geoip2-golang/v2 v2.5.0
	github.com/oschwald/maxminddb-golang/v2 v2.8.0
	github.com/pressly/goose/v3 v3.29.0
	github.com/prometheus/client_golang v1.25.0
	github.com/stretchr/testify v1.12.1
	github.com/testcontainers/testcontainers-go v0.44.0
	github.com/testcontainers/testcontainers-go/modules/clickhouse v0.44.0
	github.com/testcontainers/testcontainers-go/modules/postgres v0.44.0
	golang.org/x/time v0.16.0
)

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
```

Инструменты вне `go.mod`: sqlc 1.31.1 (Docker-образ `sqlc/sqlc:1.31.1`), golangci-lint 2.14.0 (бинарь в CI, `golangci/golangci-lint-action`), `govulncheck` v1.9.0 (через `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` в job `backend` CI).
`oapi-codegen` добавляется командой `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0`.

### web/package.json

```json
{
  "name": "appmetrica-web",
  "private": true,
  "type": "module",
  "packageManager": "pnpm@12.11.2",
  "engines": { "node": ">=24.21.0 <25" },
  "dependencies": {
    "@simplewebauthn/browser": "14.0.0",
    "@tanstack/react-query": "5.104.2",
    "class-variance-authority": "0.7.1",
    "clsx": "2.1.1",
    "cmdk": "1.1.1",
    "cn": "0.4.0",
    "date-fns": "4.4.0",
    "fumadocs-core": "16.17.2",
    "fumadocs-mdx": "15.4.7",
    "fumadocs-openapi": "12.4.2",
    "fumadocs-ui": "16.17.2",
    "lucide-react": "1.56.0",
    "next": "16.4.0",
    "nuqs": "2.10.2",
    "radix-ui": "1.8.1",
    "react": "19.3.0",
    "react-day-picker": "10.0.2",
    "react-dom": "19.3.0",
    "recharts": "3.10.1",
    "tailwind-merge": "3.7.0",
    "zod": "4.6.7"
  },
  "devDependencies": {
    "@axe-core/playwright": "4.13.0",
    "@lhci/cli": "0.15.1",
    "@biomejs/biome": "2.5.15",
    "@next/bundle-analyzer": "16.4.0",
    "@playwright/test": "1.64.0",
    "@tailwindcss/postcss": "4.3.3",
    "@testing-library/jest-dom": "7.0.1",
    "@testing-library/react": "16.3.3",
    "@testing-library/user-event": "14.6.8",
    "@faker-js/faker": "10.6.0",
    "@types/mdx": "2.0.14",
    "@types/node": "24.19.2",
    "@types/react": "19.3.0",
    "@types/react-dom": "19.3.0",
    "@vitejs/plugin-react": "6.1.2",
    "jsdom": "30.1.2",
    "lefthook": "2.2.2",
    "msw": "3.0.3",
    "orval": "8.42.0",
    "tailwindcss": "4.3.3",
    "tw-animate-css": "1.4.0",
    "typescript": "6.0.3",
    "vitest": "5.0.4"
  }
}
```

Версии зафиксированы точно (без `^`), lockfile `pnpm-lock.yaml` коммитится, установка в CI и Dockerfile — `pnpm install --frozen-lockfile`.
`@types/node` взят из ветки 24.x (24.19.2), а не latest 26.6.5, чтобы типы совпадали с runtime Node 24.

### Docker-образы

| Образ | Тег | Где используется |
|---|---|---|
| `golang` | `1.27.2-alpine` | build-стадия `backend/Dockerfile` |
| `gcr.io/distroless/static-debian12` | `nonroot` | runtime ingest / api / worker |
| `node` | `24.21.0-alpine` | build и runtime `web/Dockerfile` |
| `clickhouse/clickhouse-server` | `26.8.25.14` (+ `@sha256` при первом деплое) | compose `clickhouse-stack` |
| `altinity/clickhouse-backup` | `2.8.1` (+ `@sha256`) | compose `clickhouse-stack` |
| `postgres` | `18.6-trixie` | Dokploy managed DB, testcontainers |
| `sqlc/sqlc` | `1.31.1` | `make gen` |

---

## Инфраструктура и стоимость

| Позиция | Параметры | €/мес (без НДС) | Единоразово |
|---|---|---|---|
| Hetzner AX102-1 | FSN1, Ryzen 9 7950X3D, 128 ГБ DDR5, 2 × 1,92 ТБ NVMe RAID1 | 257,30 | 129,00 (setup) |
| Основной IPv4 | для AX102-1 | 1,70 | — |
| Hetzner Object Storage | fsn1, база включает 1 ТБ хранения + 1 ТБ egress; бэкапы CH (~200–400 ГБ) и PG помещаются | 6,49 | — |
| healthchecks.io | Hobbyist, 20 checks | 0,00 | — |
| Tailscale | Personal | 0,00 | — |
| GitHub Actions + GHCR | приватный репо; политика 10 последних тегов | 0,00 при укладывании в бесплатные лимиты аккаунта | — |
| DB-IP City Lite | CC BY 4.0 | 0,00 | — |
| Домен `appmetrica.online` | уже у владельца | продление по счёту регистратора (вне расчёта) | — |
| **Итого инфраструктура** | | **≈ 265,49 €/мес** | **129,00 €** |

Масштаб для расчёта: 100k DAU → ~150 млн событий/мес → ~100 ГБ/год в ClickHouse после сжатия → ~210 ГБ при хранении 25 месяцев. Пересмотр железа — при занятости диска > 60 % или стабильном p95 отчётов > 1,5 с (NFR-003).

---

## Соответствие modern baseline

| Практика baseline | Статус | Как реализовано |
|---|---|---|
| Type-safe end-to-end | ✅ | OpenAPI 3.0.3 → oapi-codegen (Go) + orval (TS + Zod); TS strict + `noUncheckedIndexedAccess` + `exactOptionalPropertyTypes` |
| Server Components first | ✅ | RSC для первичной загрузки отчётов, Client Components для фильтров/графиков/поллинга |
| Testing discipline | ✅ TDD | test → fail → implement → pass; coverage ≥ 80 % для `internal/{ingest,reports,auth,identity,deletion}` (NFR-009) |
| Linter/formatter | ✅ | Biome 2.5.15, golangci-lint 2.14.0 |
| Conventional Commits | ✅ | commit-msg хук lefthook + проверка в CI |
| Renovate / Dependabot | ✅ | `renovate.json`: gomod, npm, dockerfile, github-actions; patch — automerge после зелёного CI |
| ADR | ✅ | ADR-001…ADR-014 в [03-architecture.md](./03-architecture.md) |
| AGENTS.md / CLAUDE.md | ✅ | `AGENTS.md` в корне, `CLAUDE.md` — симлинк |
| Security baseline | ✅ | OWASP API Top 10 2023, секреты только в env Dokploy/GitHub, `govulncheck` + `pnpm audit` в CI, CSP с nonce на web |
| Perf budgets | ✅ | NFR-003 (P95 ≤ 1,5 с), NFR-006 (LCP ≤ 2,5 с, initial JS ≤ 250 KB gz, INP ≤ 200 мс) |
| Accessibility | ✅ | WCAG 2.2 AA, axe-core в Playwright |
| Infrastructure as Code | ✅ частично | Dockerfile, compose ClickHouse, workflows, конфиги в git; настройки Dokploy (домены, env) описаны пошагово в `deploy/dokploy/README.md` |
| OpenTelemetry tracing | # Deviation | Три процесса на одном сервере у одного владельца: distributed tracing не окупается. Вместо него — `request_id` сквозь ingest/api/web, slog JSON, Prometheus-метрики. Триггер пересмотра: появление второго сервера или межсервисных цепочек > 2 хопов |
| Feature flags | # Deviation | Нет внешнего сервиса флагов: один пользователь, нет аудитории для постепенного раската. Незавершённые фичи — за env-флагами `FEATURE_<NAME>=true` в конфиге Go/web и короткими PR |
| Preview deployments | # Deviation | Dokploy не делает preview для compose, а прод-сервер один. Замена: `make dev` (docker compose локально) + CI-задача e2e против поднятого в CI стека |
| Error tracking / RUM (Sentry) | # Deviation | Данные не уходят третьим сторонам (мотив проекта). Серверные ошибки — slog + счётчики; клиентские ошибки дашборда логируются в консоль; пересмотр, если ошибки UI начнут теряться |
| Monorepo tooling (Turborepo) | # Deviation | Один Go-модуль и один web-пакет — Makefile достаточно |

---

## Итого: зачем такой стек

**Сильные стороны**:
- Данные целиком у владельца: свой сервер в ЕС, без SaaS-аналитики, без внешних трекеров ошибок; IP не хранится нигде.
- Контракт-первым: одна OpenAPI-спека порождает Go-сервер, TS-клиент, Zod-схемы, MSW-моки и страницы документации — расхождение доков и API невозможно конструктивно.
- Минимум движущихся частей: один Go-бинарник (3 контейнера), Next.js, ClickHouse, Postgres. Нет Kafka, Redis, внешних auth-провайдеров.
- ClickHouse выдерживает 100k DAU и 25 месяцев истории на одном сервере с многократным запасом.
- Стек совпадает с экспертизой владельца (Go, TS/React, Linux/Docker); слабое место (ClickHouse) закрыто runbook-ами и тестами.

**Слабые стороны (признаём)**:
- Два runtime в проде (Go и Node) — цена выбора Next.js вместо SPA внутри Go.
- Один сервер: RPO 24 ч, RTO 4 ч ([09-risks-and-mitigations.md](./09-risks-and-mitigations.md)).
- Dokploy моложе Coolify, с историей критических CVE — смягчено Tailscale и ручными обновлениями.
- Молодые версии: msw 3 (2 недели), vitest 5, go-webauthn 0.x — для каждой описан fallback.

---

## Когда пересматривать

Пересматривать весь стек при:
- Устойчивом росте выше 300k DAU суммарно или > 5 000 событий/с (нужны очередь перед ClickHouse и/или второй сервер).
- Решении превратить инструмент в публичный SaaS (мультитенантность, биллинг, публичные доки, юр. вопросы товарного знака).
- Появлении кастомных событий и воронок (отчёты по произвольным свойствам потребуют новых таблиц и индексов).
- Выходе Next.js 17 / React 20 / ClickHouse 27.x LTS.

Точечно пересматривать:
- Node.js — переход на 26 LTS после 2026-10-28 и зелёного CI.
- TypeScript — переход на 7.x после TS 7.1 со стабильным JS API и поддержки в orval/Fumadocs.
- MSW — откат на 2.15.0, если моки orval не работают с 3.0.3.
- Dokploy — при новой критической CVE без авторизации: немедленное обновление или переезд на Coolify (exit 1–2 дня).
- Железо — при занятости диска > 60 % или p95 отчётов > 1,5 с.

---

## Changelog

| Date | Version | Changes |
|---|---|---|
| 2026-10-13 | 1.0 | Initial stack selection по итогам discovery-интервью (Go + ClickHouse + Postgres + Next.js + Dokploy на Hetzner AX102-1) |
| 2026-10-13 | 1.1 | T-002: добавлены `github.com/oapi-codegen/nullable` 1.1.0 (runtime сгенерированных моделей при `nullable-type: true`) и прямая зависимость `github.com/getkin/kin-openapi` 0.149.0 (тесты спек); web devDependency `@faker-js/faker` 10.6.0 (MSW-моки orval; альтернатива — `mock: false` в orval, но тогда теряются сгенерированные MSW-хендлеры из 22-test-strategy.md) |
| 2026-10-13 | 1.2 | T-007: web-зависимости примитивов shadcn `cmdk` 1.1.1 (`command`) и `react-day-picker` 10.0.2 (`calendar`); `shadcn/tailwind.css` скопирован в `web/src/app/shadcn.css` вместо зависимости `shadcn`; `next-themes` не используется |
| 2026-10-13 | 1.3 | T-012: `github.com/maxmind/mmdbwriter` 1.2.0 — генерация тестовой mmdb `backend/testdata/geo/test-city.mmdb` (только тестовая инфраструктура `internal/testutil`) |
| 2026-10-14 | 1.4 | T-044: web devDependency `@lhci/cli` 0.15.1 (версия из тикета) — Lighthouse CI для бюджетов LCP/CLS/TBT портфеля; альтернатива — `lighthouse` CLI напрямую, но тогда ассерты и повторы прогонов пришлось бы писать самим. `tsx` не добавлен: `web/scripts/check-bundle-size.ts` запускается Node 24 (`node scripts/check-bundle-size.ts`, встроенное снятие типов) |
