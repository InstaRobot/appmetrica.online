# Техническое задание (ТЗ): appmetrica.online — внутренняя аналитика приложений без SDK

**Версия**: 1.0 | **Дата**: 2026-10-13

> Этот документ — технический развёрток [01-prd.md](./01-prd.md).
> Фокус на user stories с acceptance criteria в формате, пригодном для
> оценки и реализации разработчиками.
>
> Источник истины для имён таблиц, колонок, enum-значений, эндпоинтов, operationId, путей экранов и ID требований —
> [05-data-model.md](./05-data-model.md) и [06-api-contracts.md](./06-api-contracts.md). Этот документ описывает
> **поведение**: алгоритмы, граничные случаи, ошибки и проверяемые критерии приёмки.
> Подход к тестированию: **TDD** (решение владельца от 2026-10-13). Каждый AC ниже — это failing test, который пишется
> и коммитится до реализации.

---

## 1. Область применения

### 1.1. Что входит

MVP за 4–6 недель включает следующие функциональные блоки (ID требований совпадают с PRD):

1. **Ingestion API** `POST https://i.appmetrica.online/v1/events` (FR-001..FR-006, FR-024): приём батчей до 500 системных
   событий `session_start` / `session_end`, валидация по OpenAPI-контракту `contracts/openapi/ingest.yaml`, коррекция
   часов устройства через `sent_at`, окно досыла 7 дней, идемпотентность через `insert_id`, гео-обогащение по IP без
   сохранения IP, rate limiting по приложению и по установке, защита от PII в `user_id`, частичный успех с явными
   причинами отказа, разделение `production` / `development`.
2. **Идентичность** (FR-007): `install_id` + необязательный `user_id`, ретро-склейка «первый владелец», режим
   `unit=persons|installs` для сверки с AppMetrica.
3. **Аутентификация владельца** (FR-008): только passkey (WebAuthn), первая регистрация и восстановление через
   CLI-ссылку `appmetrica admin passkey-link`, несколько passkey, cookie-сессии, CSRF-защита.
4. **Приложения и ключи** (FR-009..FR-011): создание / изменение / архивирование / удаление приложений, публичные
   write-ключи `pk_…`, секретные Read API ключи `sk_…` со scopes `read` / `delete`.
5. **Отчёты** (FR-012..FR-017): портфель, аудитория (KPI, временные ряды, DAU/WAU/MAU, разбивки, фильтры, сравнение
   периодов), retention-когорты (дневные D0..D30, недельные W0..W12), realtime, профили, экран «Интеграция».
6. **Read API** (FR-018) и **GDPR-удаление** (FR-019) по `install_id`, `user_id` или всему приложению.
7. **Документация API за логином** (FR-023): Fumadocs внутри Next.js, страницы из OpenAPI + MDX-рецепты на Swift,
   Kotlin, JavaScript, Go, curl и для ботов.
8. **Worker** (FR-020..FR-022, FR-025): ежедневный Telegram-дайджест, system check каждые 5 минут с Telegram-алертами
   и dead-man switch healthchecks.io, закрытие дня D−8 в `daily_stats`, настройки владельца и статус системы.

### 1.2. Что НЕ входит

| Не входит | Почему | Когда вернёмся |
|---|---|---|
| Кастомные события и отчёт по ним | Решение владельца: в MVP только системные события. API спроектирован forward-compatible: `name` — enum, расширение не ломает клиентов | P1, после снятия AppMetrica SDK |
| Воронки | Требуют кастомных событий | P2 |
| Крэши и ошибки приложений | Отдельная таблица и группировка стеков, вне 6 недель | P1 |
| Выручка, push, атрибуция рекламы | Не нужны владельцу, противоречат решению «только системные события» | Не планируется |
| Импорт истории из AppMetrica | Решение владельца: старт с чистого листа, параллельная работа 14 дней | Не планируется |
| Тёмная тема, мобильная вёрстка, i18n | Решение владельца: светлая тема, только десктоп ≥ 1280px, только русский | Не планируется |
| Мультипользовательский доступ, роли | Единственный пользователь — владелец | Не планируется |
| CSV-экспорт отчётов | Read API покрывает выгрузку скриптами | P1 |
| Официальные SDK | Принцип продукта «без SDK»; вместо SDK — рецепты в документации | Не планируется |

### 1.3. Предположения

- Владелец сам встраивает HTTP-вызовы в свои приложения по рецептам из документации (Swift, Kotlin, JavaScript, Go,
  боты); клиентская офлайн-очередь и логика сессий — ответственность клиентского кода.
- Суммарная нагрузка: 10k–100k DAU, 10–150 млн событий в месяц, пик ~500 запросов/с на ingest, средний батч 2–20 событий.
- Единственная production-среда на одном выделенном сервере Hetzner AX102-1 (FSN1); отдельного staging нет
  (`# Deviation`, обоснование в §6.1).
- AppMetrica SDK остаётся в приложениях на время сверки (14 дней); сверка ведётся в режиме `unit=installs`.
- Часы устройств могут быть неверными на часы и годы; часы сервера синхронизированы NTP (chrony на хосте).
- `user_id` — непрозрачный внутренний идентификатор из бэкенда приложения; для Telegram-ботов — HMAC-SHA256 от
  Telegram ID на стороне бота (рекомендация документации, сервер это не проверяет).

### 1.4. Зависимости

- Внешние сервисы: DB-IP City Lite (mmdb, CC BY 4.0, ежемесячно, скачивается при сборке образа ingest),
  Telegram Bot API (`sendMessage`), healthchecks.io Hobbyist (dead-man switch), Hetzner Object Storage fsn1 (S3-бэкапы
  ClickHouse и Postgres), GitHub (Actions, GHCR), Tailscale (доступ к панели Dokploy и SSH из CI).
- Внутренние команды: нет — владелец один; реализацию ведёт AI-агент по тикетам из [13-tickets.md](./13-tickets.md),
  владелец ревьюит и деплоит (10–15 часов в неделю).
- Лицензии: DB-IP Lite — CC BY 4.0 (обязательная атрибуция «IP Geolocation by DB-IP» со ссылкой https://db-ip.com в
  футере дашборда); ClickHouse — Apache-2.0; PostgreSQL — PostgreSQL License; Dokploy — Apache-2.0 ядро (проприетарные
  модули не используются); go-webauthn — BSD-3-Clause; Fumadocs — MIT; shadcn/ui — MIT.

---

## 2. Функциональные требования (детально)

### 2.0. Общие правила для всех stories

**Общий Definition of Done (TDD-verified)** — применяется к каждой story ниже, в story перечислены только дополнительные пункты:

- [ ] Failing tests для каждого AC закоммичены ДО implementation (порядок коммитов: `test:` → `feat:`)
- [ ] Все тесты из AC проходят в CI (`make test`)
- [ ] Coverage изменённых пакетов ≥ 85 % (Go: `go test -coverprofile`, web: Vitest v8 coverage)
- [ ] Integration-тест через testcontainers-go, если затронуты ClickHouse или Postgres
- [ ] E2E-тест Playwright для P0-сценариев UI
- [ ] Для UI: 0 нарушений `@axe-core/playwright` и проход только с клавиатуры
- [ ] Perf budget выдержан (если в story указан NFR)
- [ ] Ревью: AI-ревью (ralphex review agents) + approve владельца; внимание к качеству тестов
- [ ] Задеплоено в production через Dokploy с успешным healthcheck, ручной smoke владельцем
- [ ] Владелец принял результат (product owner acceptance)

**Нотация test path**: `<путь файла>::<имя теста>`. Go — функция `TestXxx` или подтест `TestXxx/case`; web — строка `it(...)`.

**Общие обозначения в AC**: `T0 = 2026-10-13T12:00:00.000Z` — фиксированное серверное время, задаваемое через
`internal/clock` (`clock.Fixed`); `APP_A` — приложение `id=0192f000-0000-7000-8000-00000000000a`, `platform=ios`,
`timezone=Europe/Moscow`, `status=active`; `PK_A` — активный write-ключ `pk_` + 32 символа base62 этого приложения.

### Epic 1: Приём событий (E1 Ingestion — FR-001..FR-006, FR-024)

#### Story 1.1: Приём батча `POST /v1/events` с частичным успехом (FR-001)

**Как** владелец приложения без SDK, **я хочу** отправлять пачку системных событий одним HTTP-запросом и получать
явный ответ по каждому событию, **чтобы** клиентский код точно знал, что удалить из офлайн-очереди, а что повторить.

**Контекст**: без SDK любая ошибка интеграции иначе остаётся невидимой. Конвейер обработки строго упорядочен:

1. **Транспорт.** `Content-Encoding` ∈ {отсутствует, `identity`, `gzip`}, иначе 400 `unsupported_content_encoding`.
   `Content-Type` (без параметров) ∈ {`application/json`, `text/plain`}, иначе 415 `unsupported_media_type`.
   Тело читается через `http.MaxBytesReader` поверх `gzip.Reader` с лимитом `INGEST_MAX_BODY_BYTES + 1`
   (1 048 577 байт распакованного JSON); превышение → 413 `payload_too_large`.
2. **Ключ.** Если есть заголовок `X-App-Key`, ключ проверяется до разбора JSON; иначе JSON разбирается, ключ берётся
   из `app_key`. Нет ключа → 401 `missing_app_key`. Ключа нет в кэше или он отозван → 401 `invalid_app_key`.
   Приложение `archived` → 403 `app_archived`. Ключ в query string не читается никогда.
3. **Батч.** `json.Decoder` без `DisallowUnknownFields` (неизвестные поля игнорируются ради forward-compatibility).
   Невалидный JSON → 400 `invalid_json`. Нет `sent_at` или он не RFC 3339 с offset, `events` отсутствует, пуст или
   длиннее `INGEST_MAX_EVENTS` (500) → 400 `invalid_batch`, в `details` — `{ "field": "...", "reason": "..." }`.
4. **Лимит приложения.** `limiter.AllowN(now, len(events))` по бакету приложения (FR-005); отказ → 429 `app_rate_limited`.
5. **Событие за событием** в порядке массива: валидация (Story 1.4, 1.5), коррекция времени (Story 1.2), дедуп внутри
   батча (Story 1.3), лимит установки (FR-005), обогащение контекстом и гео (Story 1.4).
6. **Запись.** Все принятые строки одним вызовом `chstore.InsertEvents(ctx, rows)` (clickhouse-go v2.49.0,
   `clickhouse.WithAsync(true)` — ожидание подтверждения async insert). Ошибка вставки → 503 `storage_unavailable` с
   `Retry-After: 5`; при этом весь батч считается не принятым (клиент повторит, дубли погасит `insert_id`).
7. **Отклонённые.** Каждое отклонение пишется в `rejected_events` асинхронно (буферный канал на 10 000 записей,
   сброс раз в 2 с или по 1000 записей, без ожидания подтверждения); переполнение буфера увеличивает
   `ingest_rejected_log_dropped_total` и не влияет на ответ клиенту.
8. **Ответ.** 200 `IngestResult { accepted, rejected[], warnings[], server_time }` — даже если `accepted = 0`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

Каждое AC должно быть достаточно конкретным, чтобы стать failing test ДО реализации.
Формат **обязательный** для каждого AC:
- Given: конкретное состояние (роли/данные/env), не абстракции
- When: точное действие (метод + endpoint / click + target)
- Then: observable результат + инварианты + test file path где этот AC покрыт

```
AC-1.1.1 (happy path):
  Given: APP_A active, PK_A в кэше ключей, ClickHouse доступен (testcontainers 26.8.25.14), clock = T0
  When:  POST /v1/events, X-App-Key: PK_A, Content-Type: application/json,
         body {sent_at: T0, context: {platform: "ios", os_name: "iOS", os_version: "18.1", app_version: "2.4.0",
         environment: "production"}, events: [session_start(insert_id U1, install_id I1, session_id S1, timestamp T0-5s),
         session_end(insert_id U2, install_id I1, session_id S1, timestamp T0-1s, duration_ms 4000)]}
  Then:  200, body.accepted = 2, body.rejected = [], body.warnings = [], body.server_time = T0
         в appmetrica.events ровно 2 строки app_id = APP_A, insert_id ∈ {U1, U2}, write_key_id = id ключа PK_A
         метрика ingest_events_accepted_total{app_id="…000a"} увеличилась на 2
  Test:  backend/internal/ingest/integration_test.go::TestIngest_HappyPath_InsertsTwoEvents
```

```
AC-1.1.2 (edge case — частичный успех):
  Given: APP_A, PK_A, clock = T0
  When:  POST /v1/events с 3 событиями: [валидный session_start, событие без session_id, session_end без duration_ms]
  Then:  200, accepted = 1, rejected = [{index: 1, code: "missing_field", field: "events[1].session_id", retryable: false},
         {index: 2, code: "duration_required", field: "events[2].duration_ms", retryable: false}]
         в events 1 строка; в rejected_events (после flush) 2 строки с code из ответа и payload_sample ≤ 4096 байт
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_PartialSuccess
```

```
AC-1.1.3 (error path — нет ключа / отозванный ключ / архив):
  Given: ключ PK_REVOKED с revoked_at = T0-1h; приложение APP_ARCH со status = archived и ключом PK_ARCH
  When:  POST /v1/events (a) без X-App-Key и без app_key; (b) с X-App-Key: PK_REVOKED; (c) с X-App-Key: PK_ARCH
  Then:  (a) 401 error.code = "missing_app_key"; (b) 401 "invalid_app_key"; (c) 403 "app_archived"
         во всех случаях в events 0 строк; error.request_id совпадает с заголовком ответа X-Request-Id
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_AuthErrors
```

```
AC-1.1.4 (транспорт — gzip, text/plain, лимиты):
  Given: APP_A, PK_A
  When:  (a) gzip-тело 2 событий, Content-Encoding: gzip; (b) то же тело, Content-Type: text/plain, ключ в поле app_key;
         (c) Content-Encoding: br; (d) Content-Type: application/xml; (e) распакованное тело 1 048 577 байт; (f) 501 событие
  Then:  (a) 200 accepted = 2; (b) 200 accepted = 2; (c) 400 "unsupported_content_encoding"; (d) 415 "unsupported_media_type";
         (e) 413 "payload_too_large"; (f) 400 "invalid_batch", details.field = "events"
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_Transport
```

```
AC-1.1.5 (invariant — 200 только после записи):
  Given: chstore-фейк, возвращающий ошибку вставки
  When:  POST /v1/events с 2 валидными событиями
  Then:  503, error.code = "storage_unavailable", заголовок Retry-After: 5
         ingest_clickhouse_insert_errors_total увеличилась на 1; ответ 200 не отправлен ни при каком исходе вставки
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_StorageUnavailable
```

```
AC-1.1.6 (CORS для браузеров):
  Given: любой Origin, например https://example.org
  When:  OPTIONS /v1/events с Access-Control-Request-Method: POST, Access-Control-Request-Headers: content-type,x-app-key
  Then:  204, Access-Control-Allow-Origin: *, Access-Control-Allow-Methods: POST, OPTIONS,
         Access-Control-Allow-Headers: Content-Type, Content-Encoding, X-App-Key, Access-Control-Max-Age: 86400,
         заголовка Access-Control-Allow-Credentials нет
  Test:  backend/internal/httpx/cors_test.go::TestCORS_IngestPreflight
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Батч с разными `install_id`: допустим (серверные прокси, боты шлют события сотен пользователей). Лимит установки
  считается для каждого `install_id` отдельно, лимит приложения — по сумме событий батча.
  Test: `backend/internal/ingest/handler_test.go::TestIngestHandler_MixedInstalls`.
- Ключ отозван, пока у клиента в очереди 300 событий: первый же запрос после обновления кэша (≤ 15 с) получает 401
  `invalid_app_key`; документация предписывает клиенту не удалять очередь при 401/403 (данные доживут до 7 дней,
  затем выпадут из окна). Test: `backend/internal/ingest/keycache_test.go::TestKeyCache_RevokedKeyDisappearsAfterRefresh`.
- Приложение архивировано посреди дня: 403 `app_archived` для всех версий клиента; отчёты по накопленным данным
  продолжают работать. Test: `backend/internal/ingest/handler_test.go::TestIngestHandler_AuthErrors`.
- Неизвестное поле в теле (`"foo": 1`, будущая версия клиента): игнорируется, 200.
  Test: `backend/internal/ingest/handler_test.go::TestIngestHandler_UnknownFieldsIgnored`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P99 ≤ 500 мс при 500 rps и батче 10 событий (включая ожидание async insert) — test
  `backend/perf/ingest-load.js` (k6), запуск `make perf-ingest` против docker compose окружения.
- Concurrency: 200 одновременных соединений без гонок — `go test -race ./internal/ingest/...` в CI.
- Idempotency: повтор того же батча даёт тот же `accepted` и не увеличивает отчётные метрики — Story 1.3.

**UX detail (observable states)**:
- Loading state: N/A для API; клиентский код ждёт ответа не дольше 10 с (таймаут в рецептах документации).
- Empty state: батч, где все события отклонены, → 200 с `accepted: 0` и полным `rejected[]`; на экране «Интеграция»
  (S-10) эти отказы видны в таблице причин в течение 10 с.
- Error state: каждый batch-level код → понятное сообщение на русском в `error.message`; на S-10 отображаются
  `code`, `message`, пример payload.
- Success state: на S-10 счётчик «Принято за 24 ч» растёт; при первом событии онбординг становится зелёным (Story 5.6).

**Tech notes (для разработчика)**:
- Подход к реализации: хендлер пишется вручную поверх `net/http` (не через strict-server: нужен потоковый gunzip и
  ранняя проверка ключа), типы запроса/ответа — из `internal/gen/ingestapi` (oapi-codegen v2.8.0, `models` only);
  валидация — собственная, отражающая схему `ingest.yaml` (contract-тест сверяет ответы со схемой).
- Связанные endpoints: `POST /v1/events`, `OPTIONS /v1/events`, `GET /health`, `GET /ready`.
- Связанные сущности БД: `appmetrica.events`, `appmetrica.rejected_events`, `write_keys`, `apps`.
- Feature flag (если нужен): `INGEST_ACCEPT_DEVELOPMENT` (по умолчанию `true`; `false` → события с
  `environment=development` молча считаются принятыми и не пишутся — аварийная разгрузка).

**Estimate**: 16 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Contract-тест: каждый ответ хендлера валиден по `contracts/openapi/ingest.yaml`
  (`backend/internal/ingest/contract_test.go::TestIngestResponses_MatchOpenAPI`)
- [ ] k6-прогон `make perf-ingest` приложен к PR (P99 ≤ 500 мс)

#### Story 1.2: Коррекция часов устройства и окно досыла (FR-002)

**Как** владелец, **я хочу**, чтобы время событий было правильным даже при сбитых часах телефона и при досыле
офлайн-очереди, **чтобы** DAU и сессии попадали в правильные дни.

**Контекст**: алгоритм (все величины в миллисекундах UTC):

```
received_at = clock.Now()                               // время приёма запроса сервером
skew        = timestamp − sent_at                       // возраст события по часам устройства
event_time  = received_at + skew
if event_time > received_at + INGEST_FUTURE_TOLERANCE (10m):
    event_time = received_at; warning timestamp_in_future_clamped
if event_time < received_at − INGEST_LATE_WINDOW (168h):
    reject event_too_old (retryable=false)
client_time = timestamp (как прислал клиент), event_date = toDate(event_time UTC)
```

`timestamp` и `sent_at` обязаны содержать offset (`Z` или `±hh:mm`); строка без offset → `invalid_field`.
Дробная часть секунд — до 9 знаков, хранится с точностью до миллисекунд (усечение).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-1.2.1 (happy path — часы устройства спешат на 3 часа):
  Given: clock = T0; устройство спешит на 3 ч: sent_at = T0+3h, timestamp = T0+3h−10s
  When:  POST /v1/events с одним session_start
  Then:  200 accepted = 1; events.event_time = T0−10s; events.client_time = T0+3h−10s; events.sent_at = T0+3h;
         warnings = []
  Test:  backend/internal/ingest/timecorrect_test.go::TestCorrectTime/device_clock_ahead
```

```
AC-1.2.2 (edge case — часы устройства в 1970):
  Given: clock = T0; sent_at = 1970-01-01T00:10:00Z, timestamp = 1970-01-01T00:09:00Z
  When:  POST /v1/events с session_start
  Then:  200 accepted = 1; event_time = T0−60s; warnings = []
  Test:  backend/internal/ingest/timecorrect_test.go::TestCorrectTime/device_clock_1970
```

```
AC-1.2.3 (edge case — досыл очереди 6 дней 23 часа и 7 дней 1 минута):
  Given: clock = T0; sent_at = T0
  When:  батч из двух событий: timestamp = T0−167h и timestamp = T0−168h−1m
  Then:  200 accepted = 1; rejected = [{index: 1, code: "event_too_old", field: "events[1].timestamp", retryable: false}]
  Test:  backend/internal/ingest/timecorrect_test.go::TestCorrectTime/late_window_boundary
```

```
AC-1.2.4 (edge case — событие «из будущего» после коррекции):
  Given: clock = T0; sent_at = T0; timestamp = T0+11m (часы сбились между событием и отправкой)
  When:  POST /v1/events
  Then:  200 accepted = 1; event_time = T0; warnings = [{index: 0, code: "timestamp_in_future_clamped"}]
  Test:  backend/internal/ingest/timecorrect_test.go::TestCorrectTime/future_clamped
```

```
AC-1.2.5 (error path — timestamp без offset):
  Given: clock = T0
  When:  событие с timestamp = "2026-10-13T11:59:00" (без Z и offset)
  Then:  200 accepted = 0; rejected[0].code = "invalid_field", field = "events[0].timestamp"
  Test:  backend/internal/ingest/validate_test.go::TestValidateEvent/timestamp_without_offset
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Сессия через полночь (start 23:59:50, end 00:00:20 по TZ приложения): события попадают в разные дни; DAU засчитывает
  человека в обоих днях, сессия считается в день `session_start`, длительность — по `session_end` той же сессии.
  Test: `backend/internal/reports/audience_test.go::TestAudience_SessionAcrossMidnight`.
- Переход на летнее/зимнее время в TZ приложения: дни строятся через `toDate(event_time, tz)`, в день перехода в
  часовой разбивке 23 или 25 точек. Test: `backend/internal/reports/audience_test.go::TestTimeseries_DSTTransition`
  (TZ `Europe/Berlin`, 2026-10-25).

**Non-functional requirements для этой story** (верифицируемые):
- Latency: коррекция — чистая функция без аллокаций сверх результата; бенчмарк
  `backend/internal/ingest/timecorrect_test.go::BenchmarkCorrectTime` ≤ 200 нс/операцию.
- Concurrency: функция без общего состояния.
- Idempotency: повтор того же события в другом запросе даёт другой `received_at`, но ReplacingMergeTree оставит одну
  строку (версия — максимальный `received_at`); метрики не меняются (Story 1.3).

**UX detail (observable states)**:
- Loading state: N/A (серверная логика).
- Empty state: N/A.
- Error state: `event_too_old` и `timestamp_in_future_clamped` видны в таблице причин на S-10 с примером payload.
- Success state: realtime (S-07) показывает событие с исправленным `event_time` в колонке «Время».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/ingest/timecorrect.go`, функция
  `CorrectTime(received, sentAt, ts time.Time, cfg TimeConfig) (eventTime time.Time, clamped bool, tooOld bool)`.
- Связанные endpoints: `POST /v1/events`.
- Связанные сущности БД: `events.event_time`, `events.client_time`, `events.sent_at`, `events.received_at`, `events.event_date`.
- Feature flag (если нужен): нет; пороги — `INGEST_LATE_WINDOW`, `INGEST_FUTURE_TOLERANCE`.

**Estimate**: 4 часа.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Table-driven тест покрывает 8 комбинаций знака skew и границ окна

#### Story 1.3: Идемпотентность и дедупликация (FR-003)

**Как** владелец, **я хочу**, чтобы повторная отправка того же батча не завышала цифры, **чтобы** клиент мог смело
ретраить при сетевых ошибках и 5xx.

**Контекст**: три уровня защиты. (1) Внутри батча — первый `insert_id` выигрывает, повторы пропускаются с warning
`duplicate_insert_id_in_batch` (не считаются ни принятыми, ни отклонёнными). (2) Между батчами — `events` это
`ReplacingMergeTree(received_at)` с `ORDER BY (app_id, event_date, install_id, insert_id)`: дубли схлопываются при
merge. (3) **Метрики идемпотентны по конструкции**: активные — `uniqExact(person_key)`, сессии —
`uniqExact(session_id)` по `session_start`, длительность — `max(duration_ms)` по `session_id` среди `session_end`;
лента realtime — `LIMIT 1 BY insert_id`. Поэтому `FINAL` в отчётных запросах не используется.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-1.3.1 (happy path — повтор батча):
  Given: батч B из session_start (U1, S1) и session_end (U2, S1, duration_ms 4000) уже принят (accepted = 2)
  When:  тот же батч B отправляется повторно; затем GET /v1/apps/{APP_A}/audience/summary за день события
  Then:  второй ответ 200 accepted = 2; summary.sessions = 1, summary.active = 1, summary.avg_duration_ms = 4000
  Test:  backend/internal/reports/audience_test.go::TestAudience_DuplicateBatchesDoNotInflate
```

```
AC-1.3.2 (edge case — дубль внутри батча):
  Given: батч из трёх событий, где events[2].insert_id = events[0].insert_id
  When:  POST /v1/events
  Then:  200 accepted = 2; warnings = [{index: 2, code: "duplicate_insert_id_in_batch"}]; в events 2 строки
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_DuplicateInBatch
```

```
AC-1.3.3 (invariant — после OPTIMIZE остаётся одна строка):
  Given: одно событие U1 вставлено трижды разными запросами
  When:  OPTIMIZE TABLE appmetrica.events FINAL; SELECT count() FROM events WHERE insert_id = U1
  Then:  count = 1, received_at = максимальный из трёх
  Test:  backend/internal/chstore/events_integration_test.go::TestEvents_ReplacingMergeTreeDedup
```

```
AC-1.3.4 (error path — insert_id не UUID):
  Given: событие с insert_id = "abc"
  When:  POST /v1/events
  Then:  rejected[0] = {code: "invalid_field", field: "events[0].insert_id", retryable: false}
  Test:  backend/internal/ingest/validate_test.go::TestValidateEvent/insert_id_not_uuid
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Два `session_end` одной сессии с разной длительностью (клиент досчитал после возврата из фона): берётся максимум.
  Test: `backend/internal/reports/audience_test.go::TestAudience_TwoSessionEndsTakeMax`.
- Дубль с другим `event_date` (тот же `insert_id`, но `sent_at` другой → другой `event_time` через полночь UTC):
  ReplacingMergeTree не схлопнет, но метрики через `uniqExact(session_id)` не завышаются. Принятый компромисс.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: дедуп внутри батча — `map[uuid.UUID]struct{}` на ≤ 500 элементов, без влияния на P99.
- Concurrency: нет общего состояния между запросами.
- Idempotency: формальный инвариант «повтор любого принятого батча N раз не меняет ни одной метрики отчётов» —
  property-тест `backend/internal/reports/idempotency_test.go::TestReports_IdempotentUnderReplays` (повтор 1..5 раз).

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: N/A.
- Error state: N/A — дубли не ошибка.
- Success state: цифры в отчётах не меняются при ретраях клиента.

**Tech notes (для разработчика)**:
- Подход к реализации: все SQL отчётов в `internal/reports/sql/*.sql` (embed) строятся только на `uniqExact` /
  `max per session_id`; линтер-тест `backend/internal/reports/sql_lint_test.go::TestReportSQL_NoCountStar` запрещает
  `count()` по `events` в отчётных запросах.
- Связанные endpoints: все отчётные `/v1/apps/{app_id}/…`.
- Связанные сущности БД: `appmetrica.events`.
- Feature flag (если нужен): нет.

**Estimate**: 4 часа.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] `TestReportSQL_NoCountStar` в CI

#### Story 1.4: Обогащение контекстом и гео без хранения IP (FR-004, FR-024)

**Как** владелец, **я хочу** видеть страну, регион и город пользователей и отделять отладочный трафик, **чтобы**
понимать географию без хранения персональных данных.

**Контекст**:
- **Слияние контекста**: батчевый `context` → поле-за-полем перекрывается `events[i].context`; если `platform` не задан
  нигде → `apps.platform`; `environment` по умолчанию `production`.
- **Валидация контекста**: длины (`os_name`, `os_version` ≤ 32; `app_version`, `app_build`, `device_model` ≤ 64;
  `locale` ≤ 35), enum `platform`, `device_type`, `environment`; нарушение → `invalid_field` с путём поля, например
  `events[3].context.platform` или `context.device_type`. Ошибка в батчевом `context` отклоняет все события батча
  с одинаковой причиной (каждое — отдельной записью `rejected[]`).
- **IP для гео**: если `context.ip == "none"` → гео пустое; если `context.ip` задан → он (невалидный → гео пустое +
  warning `geo_ip_invalid`); иначе — IP клиента: последний адрес в `X-Forwarded-For` (его добавляет Traefik), при
  отсутствии — `RemoteAddr`. Lookup — geoip2-golang v2.5.0 `City(netip.Addr)` по DB-IP City Lite.
- **Запись**: `country` = ISO 3166-1 alpha-2 (верхний регистр) или `''`; `region` = `Subdivisions[0].Names["en"]` или
  `''`; `city` = `City.Names["en"]` или `''`. **IP не попадает ни в одну колонку, ни в лог, ни в метрики, ни в
  `rejected_events.payload_sample`** (`context.ip` вырезается из сэмпла до записи; `user_id`, похожие на email или
  телефон, заменяются на `"[redacted]"` и в сэмпле тела батча при batch-ошибке — AC-1.5.2 действует до проверки событий).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-1.4.1 (happy path — IP клиента):
  Given: тестовая mmdb `backend/testdata/geo/test-city.mmdb`, где 81.2.69.142 → GB / England / London
  When:  POST /v1/events с X-Forwarded-For: 10.0.0.1, 81.2.69.142 без context.ip
  Then:  events.country = "GB", region = "England", city = "London";
         ни одна колонка events и ни одна строка лога теста (slog в буфер) не содержит "81.2.69.142"
  Test:  backend/internal/ingest/enrich_test.go::TestEnrich_GeoFromForwardedFor_NoIPPersisted
```

```
AC-1.4.2 (edge case — server-to-server с context.ip и "none"):
  Given: бот шлёт от имени пользователя
  When:  (a) context.ip = "81.2.69.142"; (b) context.ip = "none"; (c) context.ip = "999.1.1.1"
  Then:  (a) country = "GB"; (b) country = "", city = ""; (c) country = "", warnings[0].code = "geo_ip_invalid"
         payload_sample в rejected_events (если событие отклонено по другой причине) не содержит ключа "ip"
  Test:  backend/internal/ingest/enrich_test.go::TestEnrich_ContextIPOverride
```

```
AC-1.4.3 (happy path — слияние контекста и environment):
  Given: APP_A с platform = ios
  When:  батч context {app_version: "2.4.0"}, events[0].context {app_version: "2.5.0", environment: "development"},
         events[1] без context
  Then:  events[0]: app_version = "2.5.0", environment = "development", platform = "ios";
         events[1]: app_version = "2.4.0", environment = "production", platform = "ios"
  Test:  backend/internal/ingest/enrich_test.go::TestEnrich_ContextMerge
```

```
AC-1.4.4 (error path — невалидный enum в батчевом context):
  Given: батч context {platform: "symbian"} и 2 события
  When:  POST /v1/events
  Then:  200 accepted = 0; rejected содержит 2 записи с code = "invalid_field", field = "context.platform"
  Test:  backend/internal/ingest/validate_test.go::TestValidateContext/batch_platform_invalid
```

```
AC-1.4.5 (invariant — environment по умолчанию в отчётах):
  Given: за день 3 события production от I1 и 2 события development от I2
  When:  GET /v1/apps/{APP_A}/audience/summary без параметра environment и с environment=development
  Then:  без параметра: active = 1; с development: active = 1 и это I2
  Test:  backend/internal/reports/audience_test.go::TestAudience_EnvironmentFilter
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- IPv6-клиент: DB-IP Lite содержит IPv6-диапазоны (проверено 2026-10-13), lookup работает так же.
  Test: `backend/internal/geo/lookup_test.go::TestLookup_IPv6`.
- Частный адрес (10.x, 192.168.x, ::1): гео пустое, warning не выдаётся.
  Test: `backend/internal/geo/lookup_test.go::TestLookup_PrivateAddressEmpty`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: lookup ≤ 5 мкс P99 (mmdb в памяти, `maxminddb` mmap) — бенчмарк
  `backend/internal/geo/lookup_test.go::BenchmarkLookup`.
- Concurrency: reader потокобезопасен, один экземпляр на процесс.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: страна `''` отображается в разбивках как «(не определено)».
- Error state: `geo_ip_invalid` виден на S-10 среди предупреждений (счётчик `ingest_warnings_total{code}`).
- Success state: разбивка «Страна» на S-05 показывает названия стран по-русски через `Intl.DisplayNames('ru', {type: 'region'})`.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/geo` — интерфейс `Lookuper { Lookup(netip.Addr) Location }`, реализация на
  geoip2-golang/v2; при отсутствии файла `GEOIP_DB_PATH` процесс ingest не стартует (`/ready` не станет 200).
- Связанные endpoints: `POST /v1/events`.
- Связанные сущности БД: `events.country`, `events.region`, `events.city`, `events.environment`, `events.platform`.
- Feature flag (если нужен): `INGEST_ACCEPT_DEVELOPMENT`.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Тест-«детектор IP»: grep по всем строкам лога и колонкам после интеграционного прогона не находит тестовые IP

#### Story 1.5: PII-guard для `user_id` и лимиты частоты (FR-005, FR-006)

**Как** владелец, **я хочу**, чтобы сервер отклонял email и телефоны в `user_id` и защищался от спама публичным
ключом, **чтобы** PII не попали в аналитику по ошибке клиентского кода, а утёкший `pk_` не положил сервер.

**Контекст**:
- **PII-guard** (после `strings.TrimSpace`; пустая строка = `user_id` отсутствует, не ошибка):
  - email: `^[^@\s]+@[^@\s]+\.[^@\s]+$`;
  - E.164: `^\+[1-9][0-9]{6,14}$`;
  - форматированный телефон: строка содержит хотя бы один символ из `(`, `)`, `-`, пробел, состоит только из цифр и
    этих символов (и необязательного ведущего `+`) и содержит от 10 до 15 цифр.
  - Чисто цифровые строки (ID из БД, Telegram ID) разрешены.
  - Длина > 128 символов → `invalid_field`.
- **Лимит приложения**: `golang.org/x/time/rate` limiter на `app_id`, `rate = RATE_APP_EVENTS_PER_SEC (2000)`,
  `burst = RATE_APP_BURST (4000)`, расход = число событий батча; отказ → 429 `app_rate_limited`,
  `Retry-After = max(1, ceil((n − tokens) / rate))` секунд.
- **Лимит установки**: limiter на `(app_id, install_id)`, `rate = RATE_INSTALL_PER_MIN / 60` (100 событий/мин),
  `burst = RATE_INSTALL_BURST (1000)` — покрывает досыл недельной очереди; отказ → событие в `rejected[]` с
  `install_rate_limited`, `retryable = true`. Лимитеры хранятся в шардированной map (64 шарда) с вытеснением после
  10 минут простоя; проверка — после валидации события, до дедупа между батчами.
- Лимиты локальны для процесса; во время rolling update (2 реплики на ≤ 30 с) фактический лимит удваивается — принято.
- **Лимит по IP не применяется** (CGNAT мобильных операторов).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-1.5.1 (happy path — допустимые user_id):
  Given: события с user_id = "8f14e45f-ceea-467f-a0e6-7c9c1f2d1c01", "123456789", "user_42", "  "(пробелы)
  When:  POST /v1/events
  Then:  все 4 приняты; у четвёртого events.user_id = ""
  Test:  backend/internal/ingest/pii_test.go::TestPIIGuard/allowed
```

```
AC-1.5.2 (error path — PII):
  Given: user_id = "ivan@example.com", "+79161234567", "+7 (916) 123-45-67", "8-916-123-45-67"
  When:  POST /v1/events
  Then:  все 4 отклонены: code = "user_id_looks_like_pii", field = "events[i].user_id", retryable = false;
         значение user_id не попадает в лог; в rejected_events.payload_sample поле user_id заменено на "[redacted]"
  Test:  backend/internal/ingest/pii_test.go::TestPIIGuard/rejected
```

```
AC-1.5.3 (edge case — лимит установки при досыле очереди):
  Given: clock фиксирован; install I1 ещё не присылал событий
  When:  батч из 500 событий I1, затем сразу второй батч из 500 событий I1, затем третий из 500
  Then:  1-й и 2-й: accepted = 500 каждый (burst 1000); 3-й: accepted = 0, rejected 500 штук с
         code = "install_rate_limited", retryable = true; через 60 с (clock.Advance) батч из 100 событий принят
  Test:  backend/internal/ratelimit/limiter_test.go::TestInstallLimiter_BurstAndRefill
```

```
AC-1.5.4 (error path — лимит приложения):
  Given: RATE_APP_EVENTS_PER_SEC = 10, RATE_APP_BURST = 20 (тестовый конфиг)
  When:  батч из 15 событий, затем сразу батч из 15 событий
  Then:  первый 200; второй 429, error.code = "app_rate_limited", Retry-After: 1;
         ingest_rate_limited_total{scope="app"} = 1
  Test:  backend/internal/ingest/handler_test.go::TestIngestHandler_AppRateLimited
```

```
AC-1.5.5 (invariant — вытеснение лимитеров):
  Given: 100 000 разных install_id прислали по одному событию
  When:  clock.Advance(11m); вызывается Sweep()
  Then:  размер map лимитеров = 0; ingest_install_limiters gauge = 0
  Test:  backend/internal/ratelimit/limiter_test.go::TestInstallLimiter_SweepIdle
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- `user_id` с внутренними пробелами и цифрами, похожий на телефон («ID 1234567890»): содержит буквы → разрешён.
  Test: `backend/internal/ingest/pii_test.go::TestPIIGuard/alnum_with_digits_allowed`.
- 128-символьный `user_id` принимается, 129-символьный → `invalid_field`.
  Test: `backend/internal/ingest/pii_test.go::TestPIIGuard/length_boundary`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: PII-проверка ≤ 1 мкс на значение (регулярки компилируются один раз) — `BenchmarkPIIGuard`.
- Concurrency: шардированная map под `sync.Mutex` на шард; `go test -race`.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: N/A.
- Error state: на S-10 причина «user_id похож на email/телефон» с подсказкой «передавайте внутренний ID, для ботов —
  HMAC-SHA256 от Telegram ID» и ссылкой на `/docs/privacy-and-gdpr`.
- Success state: N/A.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/ingest/pii.go` (`LooksLikePII(string) bool`), `internal/ratelimit` (`AppLimiter`,
  `InstallLimiter` с инжектируемым `clock.Clock`).
- Связанные endpoints: `POST /v1/events`.
- Связанные сущности БД: `events.user_id`, `rejected_events`.
- Feature flag (если нужен): нет; пороги — env `RATE_*`.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Fuzz-тест `backend/internal/ingest/pii_test.go::FuzzLooksLikePII` в CI с `-fuzztime=30s`

### Epic 2: Идентичность (FR-007)

#### Story 2.1: Склейка «первый владелец» и режим `unit` (FR-007)

**Как** владелец, **я хочу** считать людей, а не устройства, и при этом иметь режим устройств для сверки с AppMetrica,
**чтобы** видеть честный DAU и проверить корректность системы.

**Контекст**: склейка вычисляется **в момент запроса**, сырые события не переписываются:

```sql
links AS (
  SELECT install_id, argMinMerge(first_user) AS linked_user
  FROM appmetrica.identity_links WHERE app_id = {app_id:UUID} GROUP BY install_id
)
-- unit = persons:
person_key = if(e.user_id != '', concat('u:', e.user_id),
             if(l.linked_user != '', concat('u:', l.linked_user),
                concat('i:', toString(e.install_id))))
-- unit = installs:
person_key = concat('i:', toString(e.install_id))
```

`identity_links` наполняется MV `identity_links_mv` из каждого события с непустым `user_id`:
`argMinState(user_id, event_time)` — «первый владелец» по самому раннему событию с этим `user_id` на установке.
**first_seen человека**: для `u:X` — минимум из `user_first_seen[X]` и `install_first_seen[I]` по всем установкам `I`,
у которых `linked_user = X`; для `i:I` — `install_first_seen[I]`. Реализация — `UNION ALL` двух источников и
`GROUP BY person_key`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-2.1.1 (happy path — анонимные дни до логина приписываются пользователю):
  Given: install I1: события 2026-10-01 и 2026-10-02 без user_id; 2026-10-03 с user_id "u1"
  When:  GET /v1/apps/{APP_A}/audience/timeseries?from=2026-10-01&to=2026-10-03&granularity=day&metric=active
  Then:  points = [1, 1, 1]; GET …/audience/summary?from=2026-10-01&to=2026-10-03 → active = 1, new = 1
         (new в день 2026-10-01 — first_seen установки)
  Test:  backend/internal/reports/identity_test.go::TestPersonKey_AnonymousDaysMergedIntoFirstOwner
```

```
AC-2.1.2 (edge case — два аккаунта на одном устройстве, «первый владелец»):
  Given: I1: 10-01 анонимно; 10-02 user_id "A"; 10-03 анонимно (логаут); 10-04 user_id "B"; 10-05 анонимно
  When:  summary за 10-01..10-05 (unit=persons)
  Then:  active = 2 (u:A и u:B); анонимные дни 10-01, 10-03, 10-05 приписаны u:A; день 10-04 — u:B
  Test:  backend/internal/reports/identity_test.go::TestPersonKey_FirstOwnerWinsForAnonymous
```

```
AC-2.1.3 (happy path — два устройства одного человека, unit persons vs installs):
  Given: I1 и I2, оба шлют события 10-05 с user_id "u1"
  When:  summary за 10-05 с unit=persons и unit=installs
  Then:  persons: active = 1; installs: active = 2
  Test:  backend/internal/reports/identity_test.go::TestUnit_PersonsVsInstalls
```

```
AC-2.1.4 (error path — неизвестный unit):
  Given: любой
  When:  GET /v1/apps/{APP_A}/audience/summary?unit=devices
  Then:  400 error.code = "invalid_request", details.param = "unit"
  Test:  backend/internal/api/params_test.go::TestParseCommonParams/invalid_unit
```

```
AC-2.1.5 (invariant — new для человека с несколькими установками):
  Given: u1 впервые на I1 2026-09-01 (анонимно, позже залогинился), 2026-10-05 ставит I2 и сразу логинится как u1
  When:  summary за 2026-10-05 (unit=persons) и (unit=installs)
  Then:  persons: new = 0; installs: new = 1
  Test:  backend/internal/reports/identity_test.go::TestNew_PersonFirstSeenAcrossInstalls
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Человек появляется только с явным `user_id` на чужом устройстве (второй аккаунт): его `first_seen` берётся из
  `user_first_seen`. Test: `backend/internal/reports/identity_test.go::TestNew_UserOnlyViaExplicitUserID`.
- Пустой `user_id` (`""` после trim) не создаёт связки. Test:
  `backend/internal/chstore/mv_integration_test.go::TestIdentityLinksMV_IgnoresEmptyUserID`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: JOIN с `links` при 2 млн связок укладывается в NFR-003 — перф-тест
  `backend/internal/reports/perf_integration_test.go::TestReportsPerf_90Days100kDAU` (тег `perf`, генератор данных
  `backend/testdata/gen/main.go`).
- Concurrency: только чтение.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: N/A.
- Error state: N/A.
- Success state: в профиле (S-09) человек `u:X` показывает все свои установки, включая анонимную историю.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/identity` — функции генерации SQL-фрагментов `PersonKeyExpr(unit)`,
  `LinksCTE()`, `PersonFirstSeenCTE(unit)`; парсер `ParsePersonKey("u:…" | "i:<uuid>")`.
- Связанные endpoints: все отчётные, параметр `unit`.
- Связанные сущности БД: `identity_links`, `install_first_seen`, `user_first_seen`, `events`.
- Feature flag (если нужен): нет.

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Golden-фикстура `backend/testdata/fixtures/identity.ndjson` + `identity.golden.json`

### Epic 3: Аутентификация владельца (FR-008)

#### Story 3.1: Первая регистрация и восстановление passkey по CLI-ссылке (FR-008)

**Как** владелец, **я хочу** зарегистрировать passkey по одноразовой ссылке из CLI, **чтобы** не было пароля и
был понятный путь восстановления.

**Контекст**:
1. `appmetrica admin passkey-link` (exec в контейнер `appmetrica-api` через Dokploy): генерирует 32 случайных байта,
   кодирует base64url без padding, сохраняет `sha256` в `passkey_setup_tokens` (`expires_at = now + 15m`), печатает
   `${PUBLIC_BASE_URL}/setup/<token>`. Предыдущие неиспользованные токены не отзываются.
2. `POST /admin/v1/auth/setup/{token}/begin` (beginSetup): токен существует, `used_at IS NULL`, `expires_at > now`, иначе
   410 `setup_token_expired`. Если строки `owner` нет — создаётся (`webauthn_user_handle` = 64 случайных байта).
   `webauthn.BeginRegistration(owner, ResidentKey: required, UserVerification: required, exclude: существующие credential_id)`;
   `session_data` → `webauthn_ceremonies(kind=registration, setup_token_hash, expires_at = now + 5m)`.
   Ответ `{ceremony_id, options}`.
3. Браузер: `startRegistration({ optionsJSON: options.publicKey })` (@simplewebauthn/browser 14.0.0).
4. `POST /admin/v1/auth/setup/{token}/finish` (finishSetup) `{ceremony_id, credential, name}`: церемония существует, не
   истекла, принадлежит этому токену; `webauthn.CreateCredential` на сырых байтах; insert `passkeys`; `used_at = now`;
   удалить церемонию; создать сессию (Story 3.3); 204 + `Set-Cookie`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-3.1.1 (happy path — первый запуск):
  Given: пустая БД (owner отсутствует); выполнена команда admin passkey-link, получен token
  When:  E2E: открыть /setup/<token>, ввести «MacBook», нажать «Создать passkey» (виртуальный аутентификатор CDP)
  Then:  редирект на /; в owner 1 строка; в passkeys 1 строка name = "MacBook"; passkey_setup_tokens.used_at не NULL;
         cookie am_session установлена (HttpOnly, Secure в prod, SameSite=Lax)
  Test:  web/e2e/auth-passkey.spec.ts::"first setup registers passkey and lands on portfolio"
```

```
AC-3.1.2 (error path — повторное использование и истечение):
  Given: token использован (used_at задан) и token2 с expires_at = now − 1s
  When:  POST /admin/v1/auth/setup/{token}/begin и …/{token2}/begin
  Then:  оба 410, error.code = "setup_token_expired"; новых строк в webauthn_ceremonies нет
  Test:  backend/internal/auth/setup_test.go::TestBeginSetup_ExpiredOrUsedToken
```

```
AC-3.1.3 (edge case — восстановление при существующем owner):
  Given: owner и 1 passkey существуют; новая ссылка из CLI
  When:  beginSetup + finishSetup с новым устройством
  Then:  owner по-прежнему 1 строка; passkeys = 2; options.excludeCredentials содержит credential_id старого passkey
  Test:  backend/internal/auth/setup_test.go::TestSetup_RecoveryAddsSecondPasskey
```

```
AC-3.1.4 (error path — чужая церемония):
  Given: церемония C1 создана для token1
  When:  POST /admin/v1/auth/setup/{token2}/finish с ceremony_id = C1
  Then:  422 error.code = "webauthn_verification_failed"; passkeys не изменились
  Test:  backend/internal/auth/setup_test.go::TestFinishSetup_CeremonyBoundToToken
```

```
AC-3.1.5 (invariant — токен хранится только хэшем):
  Given: команда admin passkey-link выполнена
  When:  SELECT * FROM passkey_setup_tokens
  Then:  token_hash = sha256(token), открытого токена нет ни в одной колонке и ни в логе
  Test:  backend/internal/auth/setup_test.go::TestPasskeyLink_StoresOnlyHash
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Церемония истекла (> 5 мин между begin и finish): 422 `webauthn_verification_failed`, токен остаётся годным до
  своих 15 минут. Test: `backend/internal/auth/setup_test.go::TestFinishSetup_CeremonyExpired`.
- Две вкладки с одной ссылкой: второй finish после успешного первого → 410 `setup_token_expired`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: begin/finish ≤ 200 мс P95 (без сетевых зависимостей кроме PG).
- Concurrency: finishSetup — транзакция `SELECT … FOR UPDATE` по токену; гонка двух finish даёт ровно один passkey.
  Test: `backend/internal/auth/setup_test.go::TestFinishSetup_ConcurrentOnlyOneWins`.
- Idempotency: повтор finish с той же церемонией → 422 (церемония удалена после успеха).

**UX detail (observable states)**:
- Loading state: кнопка «Создать passkey» в состоянии `aria-busy="true"` и disabled на время церемонии.
- Empty state: N/A.
- Error state: 410 → экран «Ссылка недействительна или устарела. Выполните `appmetrica admin passkey-link` ещё раз»
  (`role="alert"`); отмена системного диалога → «Создание passkey отменено», кнопка снова активна.
- Success state: редирект на `/`, toast «Passkey «MacBook» сохранён».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/auth/setup.go`, `internal/auth/ceremony.go`; go-webauthn v0.18.2
  (`BeginRegistration`, `CreateCredential` на сырых байтах — strict-server уже разобрал тело).
- Связанные endpoints: `POST /admin/v1/auth/setup/{token}/begin`, `POST /admin/v1/auth/setup/{token}/finish`,
  `GET /admin/v1/auth/state`.
- Связанные сущности БД: `owner`, `passkeys`, `passkey_setup_tokens`, `webauthn_ceremonies`, `auth_sessions`.
- Feature flag (если нужен): нет.

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Ручная проверка на реальном устройстве (iCloud Keychain + YubiKey) до релиза

#### Story 3.2: Вход по passkey, управление passkey, выход (FR-008)

**Как** владелец, **я хочу** входить одним касанием Touch ID и управлять своими passkey, **чтобы** доступ был
быстрым и устойчивым к потере одного устройства.

**Контекст**:
- `beginLogin` → `webauthn.BeginDiscoverableLogin(UserVerification: required)`; церемония `kind=login`; ответ поддерживает
  conditional UI (`mediation: "conditional"` на клиенте через `useBrowserAutofill: true`).
- `finishLogin` → `ValidatePasskeyLogin(handler, session, parsed)`; handler находит owner по `userHandle` и passkey по
  `credential_id`; обновляет `sign_count`, `backup_state`, `last_used_at`. Если `CloneWarning = true` (счётчик
  уменьшился при ненулевых значениях) → 422 `webauthn_verification_failed` + лог WARN `passkey_clone_warning`.
- Глобальный лимит auth-эндпоинтов: 30 запросов/мин на процесс (без привязки к IP) → 429 `rate_limited`.
- Управление: `listPasskeys`, `beginPasskeyRegistration` / `finishPasskeyRegistration` (как setup, но по сессии),
  `renamePasskey` (1..64 символа), `deletePasskey` — последний удалить нельзя (409 `last_passkey`).
- `logout` → `auth_sessions.revoked_at = now`, cookie `am_session` очищается (`Max-Age=0`).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-3.2.1 (happy path — вход):
  Given: owner с passkey на виртуальном аутентификаторе
  When:  E2E: /login → «Войти с passkey»
  Then:  редирект на /; passkeys.last_used_at обновлён; sign_count увеличился
  Test:  web/e2e/auth-passkey.spec.ts::"login with passkey"
```

```
AC-3.2.2 (error path — удаление последнего passkey):
  Given: у owner 1 passkey
  When:  DELETE /admin/v1/passkeys/{id} с валидной сессией и X-Requested-With: appmetrica-web
  Then:  409 error.code = "last_passkey"; passkey не удалён
  Test:  backend/internal/auth/passkeys_test.go::TestDeletePasskey_LastForbidden
```

```
AC-3.2.3 (edge case — clone warning):
  Given: passkey с sign_count = 10
  When:  finishLogin с assertion, где signCount = 5
  Then:  422 "webauthn_verification_failed"; сессия не создана; лог содержит event=passkey_clone_warning
  Test:  backend/internal/auth/login_test.go::TestFinishLogin_CloneWarningRejected
```

```
AC-3.2.4 (happy path — выход):
  Given: активная сессия
  When:  POST /admin/v1/auth/logout с X-Requested-With: appmetrica-web
  Then:  204; Set-Cookie am_session=; Max-Age=0; повторный GET /admin/v1/auth/state → authenticated = false
  Test:  backend/internal/auth/session_test.go::TestLogout_RevokesSession
```

```
AC-3.2.5 (error path — лимит попыток):
  Given: 30 вызовов beginLogin за минуту
  When:  31-й вызов
  Then:  429 error.code = "rate_limited", Retry-After ≥ 1
  Test:  backend/internal/auth/login_test.go::TestAuthEndpoints_GlobalRateLimit
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Passkey удалён в настройках, а браузер всё ещё предлагает его: finishLogin → 422, UI предлагает другой passkey.
- Повторная регистрация того же аутентификатора: `excludeCredentials` не даёт создать дубль.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: finishLogin ≤ 200 мс P95.
- Concurrency: обновление `sign_count` — `UPDATE … WHERE sign_count < $new` (без отката счётчика гонкой).
- Idempotency: повтор finishLogin той же церемонии → 422.

**UX detail (observable states)**:
- Loading state: кнопка входа `aria-busy`; поле с `autocomplete="username webauthn"` для conditional UI.
- Empty state: N/A.
- Error state: 422 → «Не удалось подтвердить passkey. Попробуйте ещё раз или используйте другой ключ»; 429 → «Слишком
  много попыток, подождите минуту».
- Success state: редирект на `next` из query (только относительные пути, начинающиеся с `/`), иначе `/`.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/auth/login.go`, `internal/auth/passkeys.go`; web — `src/app/(auth)/login/page.tsx`,
  `src/lib/webauthn.ts`.
- Связанные endpoints: `POST /admin/v1/auth/login/begin`, `POST /admin/v1/auth/login/finish`, `POST /admin/v1/auth/logout`,
  `GET /admin/v1/passkeys`, `POST /admin/v1/passkeys/register/begin`, `POST /admin/v1/passkeys/register/finish`,
  `PATCH /admin/v1/passkeys/{passkey_id}`, `DELETE /admin/v1/passkeys/{passkey_id}`.
- Связанные сущности БД: `passkeys`, `webauthn_ceremonies`, `auth_sessions`.
- Feature flag (если нужен): нет.

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

#### Story 3.3: Сессии, CSRF и защита маршрутов (FR-008)

**Как** владелец, **я хочу**, чтобы сессия жила разумно долго и не могла быть использована со стороннего сайта,
**чтобы** дашборд был безопасен при доступе из интернета.

**Контекст**:
- Токен — 32 байта `crypto/rand`, cookie `am_session` = base64url(token); в `auth_sessions.token_hash` = sha256.
- Валидность: `revoked_at IS NULL AND now < expires_at AND now < last_seen_at + idle_timeout`; абсолютный TTL
  `SESSION_TTL` (720 ч), idle `SESSION_IDLE_TTL` (168 ч); `last_seen_at` обновляется, если старше 5 минут.
- CSRF: для `POST`/`PATCH`/`DELETE` на `/admin/v1/*` и для `POST /v1/apps/{app_id}/deletion-requests` по cookie —
  обязателен заголовок `X-Requested-With: appmetrica-web`; если присутствует `Origin`, он обязан совпадать с
  `PUBLIC_BASE_URL`; иначе 403 `csrf_check_failed`. Bearer-запросы с `sk_` CSRF-проверку не проходят (не нужна).
- Next.js `src/proxy.ts`: без cookie `am_session` и вне `/login`, `/setup/*`, `/api/*`, `/_next/*` → редирект
  `/login?next=<path>`. Валидность сессии проверяет только Go; клиентский fetcher при 401 делает
  `router.replace('/login?next=…')`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-3.3.1 (happy path — sliding idle):
  Given: сессия создана в T0; clock.Advance(6 дней); GET /admin/v1/auth/state
  When:  clock.Advance(6 дней); GET /admin/v1/auth/state
  Then:  оба ответа authenticated = true (idle продлён на первом запросе)
  Test:  backend/internal/auth/session_test.go::TestSession_IdleSliding
```

```
AC-3.3.2 (error path — абсолютный TTL и idle):
  Given: (a) сессия старше 30 дней при регулярной активности; (b) сессия без запросов 7 дней + 1 с
  When:  GET /v1/apps с cookie
  Then:  обе — 401 error.code = "unauthenticated"
  Test:  backend/internal/auth/session_test.go::TestSession_Expiry
```

```
AC-3.3.3 (error path — CSRF):
  Given: валидная сессия
  When:  (a) POST /admin/v1/apps без X-Requested-With; (b) с заголовком, но Origin: https://evil.example
  Then:  обе — 403 "csrf_check_failed"; приложение не создано
  Test:  backend/internal/auth/csrf_test.go::TestCSRF_AdminMutations
```

```
AC-3.3.4 (happy path — защита маршрута в Next.js):
  Given: нет cookie am_session
  When:  GET /apps/123/retention
  Then:  307 Location: /login?next=%2Fapps%2F123%2Fretention
  Test:  web/src/proxy.test.ts::"redirects anonymous users to login with next"
```

```
AC-3.3.5 (invariant — open redirect невозможен):
  Given: /login?next=https://evil.example
  When:  успешный вход
  Then:  редирект на /, а не на внешний URL
  Test:  web/src/lib/safe-next.test.ts::"rejects absolute and protocol-relative next"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Cookie есть, но сессия отозвана: proxy.ts пропускает, API отвечает 401, клиент редиректит на `/login`.
  Test: `web/src/lib/api/fetcher.test.ts::"redirects to login on 401"`.
- Ключ `sk_` в cookie-запросе не используется; cookie в Bearer-запросе игнорируется.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: проверка сессии ≤ 2 мс P95 (поиск по PK).
- Concurrency: обновление `last_seen_at` — `UPDATE … WHERE last_seen_at < now() - interval '5 minutes'`.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: N/A.
- Error state: 401 на любой странице → редирект `/login?next=…` и toast «Сессия истекла, войдите снова».
- Success state: N/A.

**Tech notes (для разработчика)**:
- Подход к реализации: middleware `auth.RequireSession`, `auth.RequireAPIKeyOrSession(scope)`, `auth.CSRF` в
  `internal/auth/middleware.go`; страницы дашборда не используют `'use cache'` (Next.js 16.4 Cache Components).
- Связанные endpoints: все `/admin/v1/*` и `/v1/*`.
- Связанные сущности БД: `auth_sessions`.
- Feature flag (если нужен): нет.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

### Epic 4: Приложения и ключи (FR-009, FR-010, FR-011)

#### Story 4.1: Управление приложениями (FR-009)

**Как** владелец, **я хочу** создавать, переименовывать, архивировать и удалять приложения, **чтобы** портфель
отражал мои реальные продукты.

**Контекст**:
- `createApp {name, platform, timezone}`: `name` после trim 1..64, уникален без учёта регистра (409 `app_name_taken`);
  `platform` из enum `app_platform`; `timezone` проходит `time.LoadLocation` (иначе 400 `invalid_request`,
  `details.field = "timezone"`); по умолчанию в UI подставляется `settings.owner_timezone`. В одной транзакции
  создаются `apps` и первый `write_keys` (label «Основной»). Ответ 201 `{app, write_key}`.
- `updateApp {name?, timezone?}`: смена TZ влияет на будущие расчёты; закрытые дни (`closed_days`) не
  пересчитываются — UI показывает предупреждение.
- `archiveApp` → `status=archived`, `archived_at=now`; ingest отвечает 403 `app_archived` не позднее чем через 15 с.
  `unarchiveApp` — обратное.
- `deleteApp {confirm_name}`: точное совпадение с `name`, иначе 409 `confirm_name_mismatch`; приложение сразу
  `archived`, создаётся `deletion_requests(subject_type=app, source=dashboard)`, ответ 202 `DeletionRequest`;
  строку `apps` удаляет worker после очистки ClickHouse (Story 6.2).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-4.1.1 (happy path — создание):
  Given: сессия владельца; приложений нет
  When:  POST /admin/v1/apps {name: "Notes iOS", platform: "ios", timezone: "Europe/Moscow"}
  Then:  201; body.app.status = "active"; body.write_key.key соответствует ^pk_[A-Za-z0-9]{32}$;
         в apps 1 строка, в write_keys 1 строка label = "Основной"
  Test:  backend/internal/apps/service_test.go::TestCreateApp_CreatesAppAndFirstKey
```

```
AC-4.1.2 (error path — дубликат имени и неверный TZ):
  Given: существует приложение "Notes iOS"
  When:  (a) createApp name = "notes ios"; (b) createApp name = "Bot", timezone = "Mars/Olympus"
  Then:  (a) 409 "app_name_taken"; (b) 400 "invalid_request", details.field = "timezone"
  Test:  backend/internal/apps/service_test.go::TestCreateApp_Validation
```

```
AC-4.1.3 (happy path — архив останавливает приём):
  Given: APP_A active, ключ PK_A
  When:  POST /admin/v1/apps/{APP_A}/archive; keycache.Refresh(); POST /v1/events с PK_A
  Then:  архив 200 App.status = "archived"; ingest 403 "app_archived"
  Test:  backend/internal/ingest/keycache_test.go::TestKeyCache_ArchivedAppRejected
```

```
AC-4.1.4 (error path — удаление без подтверждения):
  Given: APP_A с name "Notes iOS"
  When:  DELETE /admin/v1/apps/{APP_A} {confirm_name: "Notes"}
  Then:  409 "confirm_name_mismatch"; приложение не изменилось; deletion_requests пуст
  Test:  backend/internal/apps/service_test.go::TestDeleteApp_ConfirmNameMismatch
```

```
AC-4.1.5 (happy path — удаление запускает фоновую очистку):
  Given: APP_A
  When:  DELETE /admin/v1/apps/{APP_A} {confirm_name: "Notes iOS"}
  Then:  202, body.subject_type = "app", body.status = "pending"; apps.status = "archived"
  Test:  backend/internal/apps/service_test.go::TestDeleteApp_CreatesDeletionRequest
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Смена TZ с `Europe/Moscow` на `Asia/Tokyo`: последующие отчёты строят дни в новом поясе; закрытые дни остаются как
  есть; на S-11 предупреждение «Уже закрытые дни (старше 8 дней) посчитаны в прежнем поясе».
  Test: `web/src/components/apps/app-settings-form.test.tsx::"warns about closed days on timezone change"`.
- Переименование в имя другого приложения в ином регистре → 409 `app_name_taken`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: CRUD ≤ 100 мс P95.
- Concurrency: уникальность имени обеспечивается индексом `apps_name_lower_uq`, гонка двух create → один 201, один 409.
- Idempotency: повторный archive архивного приложения → 200 без изменений.

**UX detail (observable states)**:
- Loading state: кнопка «Создать» disabled + спиннер; форма блокируется.
- Empty state: портфель без приложений → EmptyState «Добавьте первое приложение» с кнопкой (S-03).
- Error state: 409 → ошибка под полем «Имя» `aria-describedby`; 400 TZ → под полем «Часовой пояс».
- Success state: после create — редирект на `/apps/[appId]/integration` (S-10).

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/apps/service.go`, sqlc-запросы `backend/queries/postgres/apps.sql`.
- Связанные endpoints: `POST /admin/v1/apps`, `PATCH /admin/v1/apps/{app_id}`, `POST /admin/v1/apps/{app_id}/archive`,
  `POST /admin/v1/apps/{app_id}/unarchive`, `DELETE /admin/v1/apps/{app_id}`, `GET /v1/apps`, `GET /v1/apps/{app_id}`.
- Связанные сущности БД: `apps`, `write_keys`, `deletion_requests`.
- Feature flag (если нужен): нет.

**Estimate**: 8 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/app-onboarding.spec.ts::"create app redirects to integration"`

#### Story 4.2: Write-ключи `pk_` и кэш в ingest (FR-010)

**Как** владелец, **я хочу** иметь несколько write-ключей на приложение и отзывать их, **чтобы** ротировать ключ без
поломки старых версий приложения.

**Контекст**: `pk_` + 32 символа base62 из `crypto/rand` (≈ 190 бит). Ключ публичный (вшит в клиент), хранится открыто
в `write_keys.key` — это позволяет показывать сниппеты с подставленным ключом. Ingest держит кэш
`map[key]KeyInfo{WriteKeyID, AppID, AppStatus, AppPlatform}`, полностью перезагружаемый из PG каждые
`KEY_CACHE_REFRESH` (15 с) с атомарной заменой указателя; до первой успешной загрузки `/ready` = 503. Ошибка
перезагрузки — старый кэш остаётся, `ingest_key_cache_refresh_errors_total++`. `last_used_at` накапливается в памяти и
сбрасывается одним `UPDATE … FROM unnest(...)` раз в 5 минут.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-4.2.1 (happy path — второй ключ):
  Given: APP_A с ключом K1
  When:  POST /admin/v1/apps/{APP_A}/write-keys {label: "Android 3.x"}; keycache.Refresh()
  Then:  201 с новым ключом K2; события с K1 и с K2 принимаются; events.write_key_id различается
  Test:  backend/internal/ingest/keycache_test.go::TestKeyCache_MultipleActiveKeys
```

```
AC-4.2.2 (error path — отозванный ключ):
  Given: K1 отозван (POST …/write-keys/{K1}/revoke)
  When:  keycache.Refresh(); POST /v1/events с K1
  Then:  401 "invalid_app_key"
  Test:  backend/internal/ingest/keycache_test.go::TestKeyCache_RevokedKeyDisappearsAfterRefresh
```

```
AC-4.2.3 (edge case — PG недоступен при refresh):
  Given: кэш загружен; Postgres остановлен
  When:  keycache.Refresh()
  Then:  ошибка залогирована; ingest_key_cache_refresh_errors_total = 1; события с K1 по-прежнему принимаются
  Test:  backend/internal/ingest/keycache_test.go::TestKeyCache_KeepsStaleOnError
```

```
AC-4.2.4 (invariant — /ready до первой загрузки):
  Given: ingest стартует, PG недоступен
  When:  GET /ready
  Then:  503 {status: "not_ready", checks: {key_cache: "not_loaded"}}; GET /health → 200
  Test:  backend/internal/ingest/health_test.go::TestReady_WaitsForKeyCache
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Отзыв последнего активного ключа разрешён; UI предупреждает «Приём событий остановится для всех версий с этим ключом».
- `last_used_at` при падении процесса до сброса теряется максимум за 5 минут — принято.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: поиск ключа — O(1) map lookup без блокировок (`atomic.Pointer`).
- Concurrency: `go test -race` с параллельными Refresh и Lookup.
- Idempotency: повторный revoke → 200 без изменения `revoked_at`.

**UX detail (observable states)**:
- Loading state: строка таблицы ключей со скелетоном при создании.
- Empty state: N/A (у приложения всегда есть хотя бы один ключ при создании).
- Error state: ошибка revoke → toast «Не удалось отозвать ключ».
- Success state: новый ключ подсвечен 3 секунды, кнопка «Скопировать».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/ingest/keycache.go` (кэш write-ключей живёт в ingest по правилам импорта 07), `internal/apps/keys.go`.
- Связанные endpoints: `GET/POST /admin/v1/apps/{app_id}/write-keys`, `POST /admin/v1/apps/{app_id}/write-keys/{key_id}/revoke`.
- Связанные сущности БД: `write_keys`, `apps`.
- Feature flag (если нужен): нет.

**Estimate**: 5 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

#### Story 4.3: Read API ключи `sk_` (FR-011)

**Как** владелец, **я хочу** выпускать секретные ключи с ограниченными правами, **чтобы** скрипты и бэкенды могли
читать отчёты и удалять данные пользователей.

**Контекст**: `sk_` + 40 символов base62; показывается один раз в ответе `createApiKey`; в БД `key_prefix`
(`sk_` + 8 символов) и `key_hash = HMAC-SHA256(KEY_HASH_PEPPER, secret)`. Аутентификация: `Authorization: Bearer sk_…`
→ HMAC → поиск по `key_hash` → `revoked_at IS NULL` → проверка scope (`read` для GET `/v1/*`, `delete` для
`createDeletionRequest`) → 403 `insufficient_scope`; если `api_keys.app_id` задан — любой другой `app_id` → 404
`app_not_found`, `listApps` и `getPortfolio` возвращают только это приложение. `last_used_at` обновляется не чаще
раза в минуту на ключ. Лимит: 10 запросов/с на ключ, burst 30 → 429 `rate_limited`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-4.3.1 (happy path):
  Given: POST /admin/v1/api-keys {label: "telegram-digest-script", scopes: ["read"]} вернул secret S
  When:  GET /v1/apps с Authorization: Bearer S
  Then:  200; в api_keys нет колонки или значения, равного S; key_prefix = первые 11 символов S
  Test:  backend/internal/auth/apikey_test.go::TestAPIKey_CreateAndAuthenticate
```

```
AC-4.3.2 (error path — scope):
  Given: ключ только с ["read"]
  When:  POST /v1/apps/{APP_A}/deletion-requests {subject_type: "install", subject_value: I1}
  Then:  403 "insufficient_scope"
  Test:  backend/internal/auth/apikey_test.go::TestAPIKey_ScopeEnforced
```

```
AC-4.3.3 (edge case — ключ на одно приложение):
  Given: ключ с app_id = APP_A; существует APP_B
  When:  GET /v1/apps; GET /v1/apps/{APP_B}/audience/summary?from=…&to=…
  Then:  listApps содержит только APP_A; второй запрос 404 "app_not_found"
  Test:  backend/internal/auth/apikey_test.go::TestAPIKey_AppRestriction
```

```
AC-4.3.4 (error path — отозванный и неверный ключ):
  Given: ключ отозван; второй ключ — случайная строка sk_…
  When:  GET /v1/apps
  Then:  оба 401 "unauthenticated"; api_auth_failures_total{reason="invalid_key"} = 2
  Test:  backend/internal/auth/apikey_test.go::TestAPIKey_RevokedAndUnknown
```

```
AC-4.3.5 (error path — rate limit):
  Given: ключ с read
  When:  31 запрос за 1 секунду (clock фиксирован)
  Then:  31-й — 429 "rate_limited", Retry-After: 1
  Test:  backend/internal/auth/apikey_test.go::TestAPIKey_RateLimited
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Смена `KEY_HASH_PEPPER` инвалидирует все `sk_` — задокументировано в [21-ops-runbooks.md](./21-ops-runbooks.md),
  ротация pepper не предусмотрена в MVP.
- Пустой массив scopes → 400 `invalid_request`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: проверка ключа ≤ 2 мс P95 (кэш ключей в памяти api на 60 с, инвалидация при revoke в том же процессе).
- Concurrency: N/A.
- Idempotency: повторный revoke → 200.

**UX detail (observable states)**:
- Loading state: диалог создания с disabled-кнопкой.
- Empty state: «API-ключей пока нет. Создайте ключ для скриптов и бэкендов».
- Error state: 400 → inline-ошибки полей.
- Success state: диалог «Скопируйте ключ сейчас — он больше не будет показан» с кнопкой копирования.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/auth/apikey.go`; генерация — `internal/auth/random.go` (`RandomBase62(n)`).
- Связанные endpoints: `GET/POST /admin/v1/api-keys`, `POST /admin/v1/api-keys/{key_id}/revoke`, все `/v1/*`.
- Связанные сущности БД: `api_keys`.
- Feature flag (если нужен): нет.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

### Epic 5: Отчёты (FR-012..FR-017)

#### Story 5.1: Портфель (FR-012)

**Как** владелец, **я хочу** утром видеть одну таблицу по всем приложениям, **чтобы** за 10 секунд понять, где что-то
пошло не так.

**Контекст**: `GET /v1/portfolio?date&environment&unit` (date по умолчанию — вчера в `settings.owner_timezone`).
Для каждого приложения (дни — в TZ приложения):
- `active_today` — активные сегодня (live, неполный день);
- `active_date`, `active_same_day_prev_week` (date − 7), `active_delta_pct = round((a − b) / b × 100, 1)`, `null` при `b = 0`;
- `new_date`, `sessions_date`;
- `retention_d1` — доля когорты (date − 1), вернувшейся в date; `retention_d7` — когорта (date − 7), вернувшаяся в date;
  `null`, если когорта пуста;
- `sparkline` — 30 точек `{day, active}` за date − 29 … date: закрытые дни из `daily_stats`, открытые — live.
Расчёт по приложениям параллельно (`errgroup` с лимитом 4). Архивные приложения включаются с `app.status = archived`,
UI скрывает их по умолчанию.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.1.1 (happy path — golden):
  Given: фикстура backend/testdata/fixtures/portfolio.ndjson (2 приложения, 40 дней, известные ответы)
  When:  GET /v1/portfolio?date=2026-10-12
  Then:  ответ совпадает с backend/testdata/fixtures/portfolio.golden.json (все поля, включая sparkline)
  Test:  backend/internal/reports/portfolio_test.go::TestPortfolio_Golden
```

```
AC-5.1.2 (edge case — нулевая база):
  Given: приложение создано 3 дня назад, на прошлой неделе событий нет
  When:  GET /v1/portfolio?date=<вчера>
  Then:  active_delta_pct = null; retention_d7 = null; sparkline содержит 30 точек, ранние — active = 0
  Test:  backend/internal/reports/portfolio_test.go::TestPortfolio_NoBaseline
```

```
AC-5.1.3 (happy path — смешение закрытых и открытых дней):
  Given: daily_stats за дни ≤ date − 8 содержит active = 999 (подменённое значение), сырые события дают 10
  When:  GET /v1/portfolio
  Then:  sparkline для закрытых дней = 999 (из daily_stats), для открытых — live-значения
  Test:  backend/internal/reports/portfolio_test.go::TestPortfolio_UsesDailyStatsForClosedDays
```

```
AC-5.1.4 (error path — неверная дата):
  Given: любой
  When:  GET /v1/portfolio?date=2026-13-01
  Then:  400 "invalid_request", details.param = "date"
  Test:  backend/internal/api/params_test.go::TestParseCommonParams/invalid_date
```

```
AC-5.1.5 (UI — таблица и сортировка):
  Given: MSW-мок getPortfolio с 3 приложениями
  When:  рендер PortfolioTable, клик по заголовку «DAU»
  Then:  строки отсортированы по active_date по убыванию; Δ отрицательная окрашена токеном --color-negative и имеет
         текст «−1,2 %» (не только цвет); строка — ссылка на /apps/<id>
  Test:  web/src/components/reports/portfolio-table.test.tsx::"sorts by DAU and shows signed delta"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Приложения в разных TZ: «вчера» вычисляется для каждого приложения в его поясе; заголовок таблицы показывает дату
  в TZ владельца с подсказкой «даты считаются в часовом поясе каждого приложения».
- Приложение без единого события: строка с нулями и бейджем «Нет данных» со ссылкой на S-10.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 1 с при 10 приложениях и 100k DAU — `TestReportsPerf_90Days100kDAU/portfolio`.
- Concurrency: не более 4 параллельных CH-запросов на один вызов портфеля.
- Idempotency: N/A (GET).

**UX detail (observable states)**:
- Loading state: скелетон таблицы из 5 строк (`aria-busy="true"` на таблице).
- Empty state: «Добавьте первое приложение» + кнопка «Добавить приложение» → `/apps/new`.
- Error state: 503 → карточка «ClickHouse недоступен, данные временно не загружаются» + кнопка «Повторить».
- Success state: таблица, по умолчанию сортировка по `active_date` по убыванию.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/portfolio.go` + `internal/reports/sql/portfolio_*.sql`.
- Связанные endpoints: `GET /v1/portfolio`.
- Связанные сущности БД: `events`, `daily_stats`, `install_first_seen`, `user_first_seen`, `identity_links`, `apps`.
- Feature flag (если нужен): нет.

**Estimate**: 12 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/portfolio.spec.ts::"portfolio shows apps and navigates to app"`

#### Story 5.2: Аудитория (FR-013)

**Как** владелец, **я хочу** видеть активных, новых, сессии и длительность с разбивками и сравнением периодов,
**чтобы** понимать динамику каждого приложения.

**Контекст**:
- Общие параметры: `from`, `to` (включительно, TZ приложения), макс. 366 дней (иначе 400 `range_too_large`);
  `granularity=hour` только при диапазоне ≤ 7 дней; `week` — ISO-неделя с понедельника; `month` — календарный месяц.
- Фильтры `platform`, `app_version`, `os_name`, `country` (повторяемые; OR внутри параметра, AND между параметрами).
- `summary`: `active`, `new`, `sessions`, `avg_duration_ms` (сумма `max(duration_ms)` по сессиям с `session_end` /
  их число), `median_duration_ms` (`quantileExact(0.5)` по тем же значениям); `compare=previous_period` — период той
  же длины непосредственно перед `from`, плюс `delta_pct`.
- `timeseries`: метрика по бакетам гранулярности; пустые бакеты заполняются нулями (`WITH FILL`).
- `rolling-active`: для каждого дня D: `dau` = активные в D, `wau` = в [D−6, D], `mau` = в [D−29, D].
- `breakdown`: `dimension` из enum, `metric` ∈ {active, new, sessions}, `limit` 1..200 (по умолчанию 50); `share =
  count / total`, где `total` — значение метрики без разбивки (сумма долей для active может превышать 1, потому что
  человек мог пользоваться двумя версиями — это показано подписью в UI).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.2.1 (happy path — golden):
  Given: фикстура backend/testdata/fixtures/audience.ndjson
  When:  summary, timeseries (day, hour), rolling-active и breakdown(app_version) за 2026-10-01..2026-10-07
  Then:  ответы совпадают с backend/testdata/fixtures/audience.golden.json
  Test:  backend/internal/reports/audience_test.go::TestAudience_Golden
```

```
AC-5.2.2 (edge case — сессии без session_end):
  Given: 3 сессии, у одной нет session_end, у двух длительности 10 000 и 20 000 мс
  When:  summary
  Then:  sessions = 3; avg_duration_ms = 15000; median_duration_ms = 15000
  Test:  backend/internal/reports/audience_test.go::TestAudience_DurationIgnoresSessionsWithoutEnd
```

```
AC-5.2.3 (error path — диапазоны):
  Given: любой
  When:  (a) from=2025-01-01&to=2026-10-01; (b) granularity=hour на 8 днях; (c) from > to
  Then:  (a) 400 "range_too_large"; (b) 400 "range_too_large"; (c) 400 "invalid_request"
  Test:  backend/internal/api/params_test.go::TestParseCommonParams/ranges
```

```
AC-5.2.4 (happy path — фильтры OR/AND):
  Given: события: ios 2.4.0, ios 2.5.0, android 2.4.0
  When:  summary?platform=ios&app_version=2.4.0&app_version=2.5.0
  Then:  active учитывает только ios 2.4.0 и ios 2.5.0
  Test:  backend/internal/reports/filters_test.go::TestFilters_OrWithinAndAcross
```

```
AC-5.2.5 (UI — KPI-карточки и сравнение):
  Given: MSW-мок getAudienceSummary с compare
  When:  рендер AudienceKpis
  Then:  4 карточки «Активные», «Новые», «Сессии», «Ср. длительность»; дельта с знаком и стрелкой,
         aria-label вида «Активные: 12 480, +4,1 % к предыдущему периоду»
  Test:  web/src/components/reports/audience-kpis.test.tsx::"renders KPIs with accessible deltas"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- DST-переход в TZ приложения: часовой ряд за 2026-10-25 в `Europe/Berlin` содержит 25 точек.
  Test: `TestTimeseries_DSTTransition`.
- Значение измерения `''` → в ответе `value = ""`, UI показывает «(не указано)».

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 1,5 с на 90 днях при 100k DAU — `TestReportsPerf_90Days100kDAU/audience`.
- Concurrency: клиент делает до 4 параллельных запросов на экран; api ограничивает CH-запросы семафором 16 на процесс.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: скелетоны карточек и графика, высота зарезервирована (CLS < 0,1).
- Empty state: «За выбранный период событий нет» + ссылка «Проверить интеграцию» → S-10.
- Error state: 400 `range_too_large` → inline-подсказка у выбора периода; 503 → карточка с «Повторить».
- Success state: график Recharts, переключатель метрики, таблицы разбивок во вкладках.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/audience.go`, SQL в `internal/reports/sql/audience_*.sql`; web —
  `src/app/(dashboard)/apps/[appId]/page.tsx`, URL-state через nuqs (`from`, `to`, `g`, `cmp`, `env`, фильтры).
- Связанные endpoints: `getAudienceSummary`, `getAudienceTimeseries`, `getRollingActive`, `getAudienceBreakdown`.
- Связанные сущности БД: `events`, `identity_links`, `install_first_seen`, `user_first_seen`.
- Feature flag (если нужен): нет.

**Estimate**: 20 часов (backend 12, web 8).

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/audience.spec.ts::"audience filters persist in URL"`

#### Story 5.3: Retention (FR-014)

**Как** владелец, **я хочу** видеть классический retention по когортам, **чтобы** сравнить его с AppMetrica и
оценивать релизы.

**Контекст**: когорта — люди (или установки при `unit=installs`), чей `first_seen` попадает в день/неделю когорты в
диапазоне `[from, to]`. `values[n] = returned_n / size`, где `returned_n` — когорта с ≥ 1 событием в день
`cohort_start + n` (для недель — в ISO-неделю `+ n`). `values[0] = 1.0`. `null`, если `cohort_start + n ≥ сегодня`
(в TZ приложения) — показываются только полностью завершившиеся периоды. `periods`: по умолчанию 30 (day, максимум 90)
и 12 (week, максимум 52). `average[n] = Σ returned_n / Σ size` по когортам с не-null значением.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.3.1 (happy path — golden):
  Given: фикстура backend/testdata/fixtures/retention.ndjson (когорты 10 дней, известная матрица)
  When:  GET /v1/apps/{APP_A}/retention?from=2026-09-20&to=2026-09-29&cohort_granularity=day&periods=14
  Then:  ответ совпадает с retention.golden.json; values[0] = 1 во всех когортах
  Test:  backend/internal/reports/retention_test.go::TestRetention_DayGolden
```

```
AC-5.3.2 (happy path — недельные когорты):
  Given: та же фикстура
  When:  cohort_granularity=week&periods=4
  Then:  ответ совпадает с retention_week.golden.json; cohort_start — понедельники
  Test:  backend/internal/reports/retention_test.go::TestRetention_WeekGolden
```

```
AC-5.3.3 (edge case — незавершённые периоды и пустая когорта):
  Given: когорта вчерашнего дня; день без новых пользователей
  When:  retention по дням
  Then:  у вчерашней когорты values[1..] = null; пустая когорта size = 0, values все null; average игнорирует null
  Test:  backend/internal/reports/retention_test.go::TestRetention_NullsForFutureAndEmpty
```

```
AC-5.3.4 (error path — periods вне диапазона):
  Given: любой
  When:  periods=91 (day) и periods=53 (week)
  Then:  оба 400 "invalid_request", details.param = "periods"
  Test:  backend/internal/api/params_test.go::TestParseRetentionParams
```

```
AC-5.3.5 (UI — heatmap как таблица):
  Given: MSW-мок getRetention
  When:  рендер RetentionHeatmap
  Then:  <table> с <caption>, заголовки D0..D14 (th scope="col"), когорты (th scope="row"), ячейки null пустые с
         aria-label «нет данных»; цвет ячейки из шкалы токенов --color-retention-0..9
  Test:  web/src/components/reports/retention-heatmap.test.tsx::"renders accessible cohort table"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Когорта, где человек «вернулся» только событием `session_end` старой сессии, начатой в день когорты, но
  завершённой после полуночи: засчитывается как возврат на следующий день (любое событие). Совпадает с поведением
  «любое событие» AppMetrica; задокументировано в `/docs/concepts/sessions`.
- Сверка с AppMetrica (NFR-010) выполняется с `unit=installs`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 1,5 с для 30 дневных когорт × 30 периодов при 100k DAU — `TestReportsPerf_90Days100kDAU/retention`.
- Concurrency: N/A.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: скелетон таблицы 10×15.
- Empty state: «В выбранном периоде нет новых пользователей».
- Error state: 503 → карточка с «Повторить».
- Success state: heatmap + линия средней кривой (Recharts) под таблицей.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/retention.go`, SQL `internal/reports/sql/retention.sql`
  (`arrayJoin` по дням активности, `groupUniqArray` нет — только `uniqExact` по (cohort, n, person_key)).
- Связанные endpoints: `getRetention`.
- Связанные сущности БД: `events`, `install_first_seen`, `user_first_seen`, `identity_links`.
- Feature flag (если нужен): нет.

**Estimate**: 14 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/retention.spec.ts::"retention switches day and week cohorts"`

#### Story 5.4: Realtime (FR-015)

**Как** владелец, **я хочу** в день релиза видеть, что происходит прямо сейчас, **чтобы** сразу заметить проблему.

**Контекст**: `GET /v1/apps/{app_id}/realtime?environment`:
- `active_now` — `uniqExact(person_key)` по `session_start` за последние 30 минут, у которых нет `session_end` с тем же
  `session_id`;
- `per_minute` — 30 точек `{minute, sessions_started}` (минуты UTC, последняя — текущая неполная), пустые — 0;
- `recent_events` — 50 последних по `received_at DESC`, `LIMIT 1 BY insert_id`, поля `RecentEvent`;
- `generated_at` — серверное время. Клиент опрашивает каждые 5 с (TanStack Query `refetchInterval: 5000`,
  пауза кнопкой и при `document.hidden`).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.4.1 (happy path):
  Given: clock = T0; сессии S1 (start T0−10m, без end), S2 (start T0−5m, end T0−1m), S3 (start T0−40m, без end)
  When:  GET /v1/apps/{APP_A}/realtime
  Then:  active_now = 1 (только S1); per_minute содержит 30 точек, сумма sessions_started = 2
  Test:  backend/internal/reports/realtime_test.go::TestRealtime_ActiveNow
```

```
AC-5.4.2 (invariant — событие видно ≤ 10 с):
  Given: ingest и api подняты в integration-тесте против одного ClickHouse
  When:  POST /v1/events (200), затем опрос getRealtime каждые 500 мс
  Then:  событие появляется в recent_events не позднее 10 с после ответа 200
  Test:  backend/internal/reports/realtime_integration_test.go::TestRealtime_VisibleWithin10s
```

```
AC-5.4.3 (edge case — дубли в ленте):
  Given: одно событие вставлено дважды (ретрай)
  When:  getRealtime
  Then:  в recent_events одна запись этого insert_id
  Test:  backend/internal/reports/realtime_test.go::TestRealtime_FeedDeduplicated
```

```
AC-5.4.4 (UI — пауза и скрытая вкладка):
  Given: MSW-мок getRealtime
  When:  рендер RealtimePage, клик «Пауза», затем симуляция document.visibilityState = "hidden"
  Then:  после паузы запросов нет 10 с (fake timers); кнопка меняется на «Продолжить» с aria-pressed="true"
  Test:  web/src/components/reports/realtime-panel.test.tsx::"pauses polling"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Сессия длиннее 30 минут без `session_end` выпадает из `active_now` — ограничение модели без heartbeat,
  подпись в UI «активные — начали сессию за последние 30 минут и ещё не закрыли её».
- В ленте показывается `install_id` целиком в `title`, в ячейке — первые 8 символов.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 300 мс (сканирование только сегодняшнего `event_date` и вчерашнего для перехода через полночь UTC).
- Concurrency: при открытой вкладке 0,2 запроса/с на клиента.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: большие цифры-скелетоны.
- Empty state: «За последние 30 минут событий нет» + ссылка на S-10.
- Error state: сбой опроса — бейдж «Нет связи, повтор через 5 с» (`role="status"`), данные не очищаются.
- Success state: число «Активны сейчас», поминутные столбцы, таблица ленты.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/realtime.go`; web — `src/app/(dashboard)/apps/[appId]/realtime/page.tsx`.
- Связанные endpoints: `getRealtime`.
- Связанные сущности БД: `events`, `identity_links`.
- Feature flag (если нужен): нет.

**Estimate**: 8 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/realtime.spec.ts::"realtime shows freshly ingested event"`

#### Story 5.5: Профили (FR-016)

**Как** владелец, **я хочу** найти пользователя по `install_id` или `user_id` и увидеть его устройства и сессии,
**чтобы** разбираться с обращениями в поддержку и отлаживать интеграцию.

**Контекст**:
- `searchProfiles?q`: если `q` — UUID → поиск установки в `install_first_seen` (оба environment); найденная установка
  отображается как её `person_key` (с учётом склейки). Иначе `q` трактуется как `user_id` (поиск в `user_first_seen` и
  `identity_links`). Пустой `q` или длиннее 128 → 400 `invalid_request`. Максимум 20 результатов.
- `getProfile/{person_key}`: `person_key` — `u:<user_id>` или `i:<uuid>` (URL-encoded); невалидный формат → 400;
  не найден → 404 `profile_not_found`. `installs` для `u:X` — установки с `linked_user = X` плюс установки, где
  встречались события с `user_id = X`; для каждой — последние значения `platform`, `os_name`, `os_version`,
  `device_model`, `app_version`, `country`, `city` (`argMax` по `event_time`), `first_seen`, `last_seen`.
  `sessions` — по `session_start` в порядке убывания, `sessions_limit` 1..500 (по умолчанию 100), курсор `before`
  (`started_at` последней показанной сессии), `next_before` — если есть ещё.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.5.1 (happy path — поиск по install_id связанной установки):
  Given: I1 связан с u1
  When:  GET /v1/apps/{APP_A}/profiles?q=<I1>
  Then:  items[0].person_key = "u:u1", installs_count ≥ 1
  Test:  backend/internal/reports/profiles_test.go::TestSearchProfiles_InstallResolvesToPerson
```

```
AC-5.5.2 (happy path — карточка и пагинация):
  Given: u1 с 2 установками и 150 сессиями
  When:  getProfile u:u1?sessions_limit=100, затем с before = next_before
  Then:  первая страница 100 сессий, next_before задан; вторая 50 сессий, next_before отсутствует; installs = 2
  Test:  backend/internal/reports/profiles_test.go::TestGetProfile_PaginationAndInstalls
```

```
AC-5.5.3 (error path — не найден и неверный ключ):
  Given: любой
  When:  getProfile "u:nobody"; getProfile "x:123"
  Then:  404 "profile_not_found"; 400 "invalid_request"
  Test:  backend/internal/reports/profiles_test.go::TestGetProfile_Errors
```

```
AC-5.5.4 (UI — удаление из карточки):
  Given: MSW-мок getProfile и createDeletionRequest
  When:  клик «Удалить данные», ввод person_key в поле подтверждения, «Удалить»
  Then:  вызван POST /api/v1/apps/{id}/deletion-requests {subject_type: "user", subject_value: "u1"} с
         X-Requested-With; показан статус «Удаление запланировано»
  Test:  web/src/components/profiles/profile-delete-dialog.test.tsx::"requests deletion for user"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Для `i:<uuid>` удаление создаёт `subject_type=install`; для `u:<id>` — `subject_type=user`.
- `user_id` со спецсимволами (`/`, `:`) корректно кодируется `encodeURIComponent` в пути.
  Test: `web/src/lib/person-key.test.ts::"encodes and decodes person keys"`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 1 с на 25 месяцах данных (bloom-индексы `idx_install_id`, `idx_user_id`) —
  `TestReportsPerf_90Days100kDAU/profile`.
- Concurrency: N/A.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: скелетон карточки.
- Empty state: поиск без результатов — «Никого не нашли. Проверьте install_id или user_id».
- Error state: 404 → «Профиль не найден» с возвратом к поиску.
- Success state: карточка, таблица устройств, таблица сессий с кнопкой «Показать ещё».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/profiles.go`; web — `src/app/(dashboard)/apps/[appId]/profiles/…`.
- Связанные endpoints: `searchProfiles`, `getProfile`, `createDeletionRequest`.
- Связанные сущности БД: `events`, `install_first_seen`, `user_first_seen`, `identity_links`.
- Feature flag (если нужен): нет.

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/profiles-deletion.spec.ts::"search profile and request deletion"`

#### Story 5.6: Экран «Интеграция» (FR-017)

**Как** владелец без SDK, **я хочу** сразу видеть, приходят ли события и почему часть отклоняется, **чтобы** чинить
интеграцию за минуты, а не за дни.

**Контекст**: `GET /v1/apps/{app_id}/integration/health?hours=24` (`hours` 1..168):
- `first_event_at` — `min(first_seen)` из `install_first_seen` по приложению (любой environment) или `null`;
- `accepted_total`, `rejected_total`, `hourly[{hour, accepted, rejected}]` — accepted по `events.received_at`,
  rejected по `rejected_events.received_at`;
- `reasons[{code, count, last_seen, example_payload, example_message}]` — `argMax(payload_sample, received_at)`;
- `versions[{app_version, accepted, rejected, last_seen}]`.
UI онбординга (если `first_event_at = null`): ключ с копированием, вкладки сниппетов curl, Swift, Kotlin, JavaScript, Go
с подставленными `write_keys.key` и `NEXT_PUBLIC_INGEST_URL`, статус «Ожидаем первое событие…» с опросом раз в 3 с;
при появлении — зелёный статус «Первое событие получено в HH:MM:SS», опрос останавливается.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-5.6.1 (happy path — health):
  Given: за 24 ч: 100 принятых событий версии 2.4.0, 5 отклонённых event_too_old, 3 — user_id_looks_like_pii
  When:  GET /v1/apps/{APP_A}/integration/health?hours=24
  Then:  accepted_total = 100, rejected_total = 8, reasons отсортированы по count убыванию,
         example_payload не содержит "ip" и содержит "user_id":"[redacted]" для PII
  Test:  backend/internal/reports/integration_health_test.go::TestIntegrationHealth_Aggregates
```

```
AC-5.6.2 (happy path — онбординг до первого события):
  Given: MSW: getIntegrationHealth → first_event_at = null, затем (третий запрос) → "2026-10-13T12:00:03Z"
  When:  рендер IntegrationOnboarding с fake timers, продвинуть 9 с
  Then:  сначала статус «Ожидаем первое событие…» (role="status"), затем «Первое событие получено в 15:00:03»
         (TZ приложения Europe/Moscow); сниппет curl содержит ключ приложения и https://i.appmetrica.online/v1/events
  Test:  web/src/components/integration/integration-onboarding.test.tsx::"switches to success after first event"
```

```
AC-5.6.3 (edge case — отклонения с нераспознанным ключом):
  Given: события с неизвестным pk_ (app_id nil в rejected_events)
  When:  health для APP_A
  Then:  эти отклонения не попадают в ответ APP_A
  Test:  backend/internal/reports/integration_health_test.go::TestIntegrationHealth_IgnoresUnknownApp
```

```
AC-5.6.4 (error path — hours вне диапазона):
  Given: любой
  When:  hours=0 и hours=169
  Then:  400 "invalid_request", details.param = "hours"
  Test:  backend/internal/api/params_test.go::TestParseIntegrationParams
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Отклонения хранятся 7 дней (TTL `rejected_events`), поэтому `hours` ограничен 168.
- Батчевые ошибки 401 (`invalid_app_key`, в том числе отозванный ключ) в `rejected_events` не пишутся: кэш ключей ingest
  держит только действующие ключи, и отозванный не резолвится в приложение (OQ-033 (3)). Старую сборку с отозванным
  ключом владелец видит по `last_used_at` ключа и метрике `ingest_events_rejected_total{code}`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: P95 ≤ 500 мс.
- Concurrency: опрос 0,33 запроса/с только при открытом онбординге.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: скелетоны графика и таблиц.
- Empty state: онбординг (описан выше).
- Error state: 503 → карточка «Статус интеграции временно недоступен».
- Success state: график accepted/rejected по часам, таблица причин с раскрываемым JSON-примером, таблица версий.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/reports/integration_health.go`; сниппеты — `web/src/components/integration/snippets.ts`
  (шаблоны строк, тест на подстановку ключа).
- Связанные endpoints: `getIntegrationHealth`, `listWriteKeys`.
- Связанные сущности БД: `events`, `rejected_events`, `install_first_seen`, `write_keys`.
- Feature flag (если нужен): нет.

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/app-onboarding.spec.ts::"onboarding turns green after curl event"`

### Epic 6: Read API и GDPR (FR-018, FR-019)

#### Story 6.1: Публичный Read API (FR-018)

**Как** владелец, **я хочу** получать те же отчёты HTTP-запросами со `sk_`-ключом, **чтобы** строить свои скрипты и
сверять данные с AppMetrica.

**Контекст**: `/v1/*` на `api.appmetrica.online` принимает `Bearer sk_…`; через дашборд те же пути доступны как
`https://appmetrica.online/api/v1/*` с cookie. Ответы и ошибки строго по `contracts/openapi/api.yaml` (OAS 3.0.3,
`operationId` из якоря). Кнопка «Скопировать как запрос к Read API» на каждом отчёте формирует curl с текущими
параметрами и плейсхолдером ключа `$APPMETRICA_SK`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-6.1.1 (happy path — контракт):
  Given: сид-данные фикстуры audience.ndjson
  When:  вызвать каждый GET /v1/* с валидными параметрами через Bearer
  Then:  каждый ответ валиден по api.yaml (kin-openapi openapi3filter.ValidateResponse)
  Test:  backend/internal/api/contract_test.go::TestReadAPI_ResponsesMatchOpenAPI
```

```
AC-6.1.2 (error path — без авторизации):
  Given: нет Authorization и cookie
  When:  GET /v1/apps
  Then:  401 "unauthenticated", заголовок WWW-Authenticate: Bearer
  Test:  backend/internal/api/handlers_test.go::TestReadAPI_Unauthenticated
```

```
AC-6.1.3 (UI — копирование запроса):
  Given: S-05 с period 2026-10-01..2026-10-07, platform=ios
  When:  клик «Скопировать как запрос к Read API»
  Then:  в буфере строка curl -H "Authorization: Bearer $APPMETRICA_SK"
         "https://api.appmetrica.online/v1/apps/<id>/audience/summary?from=2026-10-01&to=2026-10-07&platform=ios&environment=production&unit=persons"
  Test:  web/src/lib/copy-as-curl.test.ts::"builds curl for current report"
```

```
AC-6.1.4 (invariant — одинаковый результат cookie и Bearer):
  Given: одна фикстура
  When:  getAudienceSummary через cookie-сессию и через Bearer
  Then:  тела ответов идентичны
  Test:  backend/internal/api/handlers_test.go::TestReadAPI_SameResultForCookieAndBearer
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Ключ на одно приложение и `getPortfolio`: в ответе одна строка.
- Неизвестный query-параметр игнорируется (forward-compatible).

**Non-functional requirements для этой story** (верифицируемые):
- Latency: как у соответствующих отчётов (NFR-003).
- Concurrency: 10 запросов/с на ключ.
- Idempotency: GET идемпотентны.

**UX detail (observable states)**:
- Loading state: N/A (API).
- Empty state: N/A.
- Error state: единый конверт `Error`.
- Success state: N/A.

**Tech notes (для разработчика)**:
- Подход к реализации: oapi-codegen strict-server `internal/gen/api`, реализация интерфейса в `internal/api/server.go`.
- Связанные endpoints: все `/v1/*`.
- Связанные сущности БД: см. отчёты.
- Feature flag (если нужен): `API_PUBLIC_BEARER_ENABLED` (по умолчанию `true`; `false` → Bearer-запросы получают 401,
  дашборд по cookie работает — аварийное закрытие внешнего доступа).

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

#### Story 6.2: GDPR-удаление (FR-019)

**Как** владелец, **я хочу** удалить все данные пользователя по запросу, **чтобы** выполнять право на удаление по GDPR.

**Контекст**:
- `createDeletionRequest {subject_type: install|user, subject_value}`: для `install` — UUID, для `user` — 1..128 символов;
  иначе 400. Ответ 202 `DeletionRequest(status=pending)`. Источник `api` (Bearer, scope `delete`) или `dashboard` (cookie).
- Worker job `deletion` (каждую минуту): `SELECT … WHERE status='pending' ORDER BY requested_at LIMIT 10 FOR UPDATE
  SKIP LOCKED` → `running`, `attempts++`, `started_at`:
  - `install`: `DELETE FROM appmetrica.events WHERE app_id = A AND install_id = I`; `ALTER TABLE` `install_first_seen`,
    `identity_links`, `rejected_events` (по `payload_sample LIKE '%<I>%'`) `DELETE WHERE …` с `mutations_sync = 2`;
  - `user`: установки `I*` с `linked_user = U` + события с `user_id = U`; удаление как выше для всех `I*`, плюс
    `user_first_seen` по `U`;
  - `app`: все таблицы по `app_id`, затем `DELETE FROM apps WHERE id = A` (каскад ключей, `closed_days`,
    `deletion_requests`) — перед этим копия запроса в `job_runs.details`.
  - Успех → `done`, `completed_at`; ошибка → `pending` до 5 попыток, затем `failed` + Telegram-сообщение.
- `daily_stats` не очищается (агрегаты без персональных данных). Новые события после удаления снова создадут данные —
  документация предписывает клиенту сбросить `install_id`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-6.2.1 (happy path — удаление пользователя):
  Given: u1 с установками I1 (связана), I2 (явный user_id u1), события 30 дней; u2 — посторонний
  When:  createDeletionRequest {user, "u1"}; worker.RunOnce("deletion")
  Then:  status = "done"; в events, install_first_seen, user_first_seen, identity_links нет строк I1, I2, u1;
         данные u2 не тронуты; getProfile u:u1 → 404
  Test:  backend/internal/deletion/executor_integration_test.go::TestDeletion_UserRemovesAllLinkedData
```

```
AC-6.2.2 (error path — валидация):
  Given: ключ с delete
  When:  {subject_type: "install", subject_value: "not-a-uuid"}; {subject_type: "user", subject_value: ""}
  Then:  оба 400 "invalid_request"
  Test:  backend/internal/api/deletion_test.go::TestCreateDeletionRequest_Validation
```

```
AC-6.2.3 (edge case — повторные сбои):
  Given: CH-фейк возвращает ошибку
  When:  worker.RunOnce("deletion") 5 раз
  Then:  status = "failed", attempts = 5, error заполнен; отправлено 1 Telegram-сообщение
  Test:  backend/internal/deletion/executor_test.go::TestDeletion_FailsAfterFiveAttempts
```

```
AC-6.2.4 (happy path — удаление приложения):
  Given: APP_B с данными, deleteApp выполнен
  When:  worker.RunOnce("deletion")
  Then:  строк app_id = APP_B нет ни в одной таблице CH; apps без APP_B; job_runs содержит копию запроса
  Test:  backend/internal/deletion/executor_integration_test.go::TestDeletion_AppRemovesEverything
```

```
AC-6.2.5 (invariant — SLA):
  Given: 50 pending-запросов
  When:  worker работает 60 минут модельного времени (clock.Advance по минуте)
  Then:  все done; ни один не ждал дольше 24 ч
  Test:  backend/internal/deletion/executor_test.go::TestDeletion_DrainsQueue
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Два одновременных воркера (rolling update): `SKIP LOCKED` гарантирует, что запрос выполняет один.
- Запрос на несуществующего пользователя → `done` без ошибок (удалять нечего).

**Non-functional requirements для этой story** (верифицируемые):
- Latency: создание запроса ≤ 100 мс; исполнение ≤ 10 мин на запрос при 2 млрд строк (верхняя граница с кастомными событиями; lightweight delete).
- Concurrency: `FOR UPDATE SKIP LOCKED`.
- Idempotency: повторное исполнение удаления безопасно (DELETE идемпотентен).

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: вкладка «Удаления» в S-12 без запросов — «Запросов на удаление не было».
- Error state: `failed` — красный бейдж и текст ошибки, кнопка «Повторить» (создаёт новый запрос).
- Success state: статус «Выполнено HH:MM».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/deletion/executor.go`, `internal/worker/jobs_deletion.go`.
- Связанные endpoints: `createDeletionRequest`, `getDeletionRequest`, `listDeletionRequests`, `deleteApp`.
- Связанные сущности БД: `deletion_requests`, `job_runs`, все таблицы CH.
- Feature flag (если нужен): `WORKER_JOBS_ENABLED` (исключение `deletion` из списка останавливает исполнение).

**Estimate**: 10 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

### Epic 7: Документация API (FR-023)

#### Story 7.1: Документация за логином (FR-023)

**Как** владелец (и AI-агент, интегрирующий мои приложения), **я хочу** полную документацию с рецептами,
**чтобы** встроить отправку событий в любое приложение без SDK.

**Контекст**: Fumadocs (core/ui 16.17.2, mdx 15.4.7, openapi 12.4.2) в route group `(docs)`, путь `/docs`. Страницы:
index (Быстрый старт), concepts/identity, concepts/sessions, concepts/time-and-offline, recipes/ios-swift,
recipes/android-kotlin, recipes/web-js, recipes/bots-and-backends, recipes/go-and-curl, errors-and-retries,
privacy-and-gdpr, read-api/overview, api-reference (ingest), api-reference (read). API reference генерируется из
`contracts/openapi/ingest.yaml` и `api.yaml` (`/admin/v1` помечен `x-internal: true` и исключается). Playground выключен.
Доступ: `proxy.ts` + серверная проверка `GET /admin/v1/auth/state` в `app/(docs)/layout.tsx`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-7.1.1 (happy path):
  Given: вход выполнен
  When:  открыть /docs/recipes/ios-swift
  Then:  страница содержит Swift-код с URLSession, Keychain-хранением install_id и офлайн-очередью; h1 «iOS (Swift)»
  Test:  web/e2e/docs.spec.ts::"docs recipe page renders for owner"
```

```
AC-7.1.2 (error path — без входа):
  Given: нет cookie
  When:  GET /docs
  Then:  редирект на /login?next=%2Fdocs
  Test:  web/e2e/docs.spec.ts::"docs require login"
```

```
AC-7.1.3 (invariant — admin-эндпоинты не публикуются):
  Given: сборка docs
  When:  поиск по сгенерированным страницам api-reference
  Then:  нет путей /admin/v1
  Test:  web/src/lib/docs/openapi-source.test.ts::"excludes x-internal operations"
```

```
AC-7.1.4 (invariant — сниппеты компилируются):
  Given: go-сниппет recipes/go-and-curl
  When:  CI извлекает блоки ```go из MDX и запускает go vet
  Then:  0 ошибок
  Test:  backend/internal/docscheck/snippets_test.go::TestDocsGoSnippets_Vet
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Атрибуция DB-IP в футере docs и дашборда. Test: `web/e2e/docs.spec.ts::"footer shows DB-IP attribution"`.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: страницы docs статически пререндерятся на сборке, но отдаются только после проверки сессии; TTFB ≤ 300 мс.
- Concurrency: N/A.
- Idempotency: N/A.

**UX detail (observable states)**:
- Loading state: N/A (SSR).
- Empty state: N/A.
- Error state: 404 docs → страница Fumadocs «Страница не найдена» со ссылкой на /docs.
- Success state: навигация слева, поиск по документации (встроенный Fumadocs).

**Tech notes (для разработчика)**:
- Подход к реализации: `web/source.config.ts`, `web/content/docs/*.mdx`, `web/src/app/(docs)/docs/[[...slug]]/page.tsx`.
- Связанные endpoints: `getAuthState`.
- Связанные сущности БД: нет.
- Feature flag (если нужен): нет.

**Estimate**: 14 часов (контент рецептов — основная часть).

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] Каждый рецепт проверен владельцем на реальном приложении соответствующей платформы

### Epic 8: Worker и эксплуатация (FR-020, FR-021, FR-022, FR-025)

#### Story 8.1: Ежедневный дайджест в Telegram (FR-020)

**Как** владелец, **я хочу** каждое утро получать сводку портфеля в Telegram, **чтобы** не открывать дашборд без повода.

**Контекст**: job `digest` проверяется каждую минуту: если `digest_enabled`, задан `telegram_chat_id`, текущее время в
`owner_timezone` ≥ `digest_time` и сегодня ещё нет `job_runs(job=digest, status=ok)` с `started_at` в сегодняшних
сутках владельца → строится портфель за вчера (`unit=persons`, `environment=production`) и отправляется
`sendMessage` с `parse_mode=HTML`:

```
<b>Портфель за 12.10.2026</b> · DAU 21 382
<b>Notes iOS</b> · DAU 12 480 (▲4,1 % к пн) · новые 1 204 · сессии 31 022 · D1 38 %
<b>Notes Android</b> · DAU 8 902 (▼1,2 % к пн) · новые 861 · сессии 20 114 · D1 31 %
<a href="https://appmetrica.online/">Открыть портфель</a>
```

Формат — OQ-012 (решён в Task 48): строки по убыванию DAU, не больше 30, остальные — «и ещё N приложений».

Ответ Telegram 429 → ждать `parameters.retry_after` и повторить, всего 3 попытки; иначе `job_runs.status = failed`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-8.1.1 (happy path):
  Given: settings: digest_enabled = true, chat_id = "123", digest_time = 09:00, owner_timezone = Europe/Moscow;
         clock = 2026-10-13T06:00:30Z (09:00:30 MSK); httptest-сервер вместо api.telegram.org
  When:  worker.RunOnce("digest")
  Then:  отправлен 1 POST /bot<token>/sendMessage с chat_id = "123", текст совпадает с golden
         backend/internal/worker/testdata/digest.golden.txt; job_runs: digest ok
  Test:  backend/internal/worker/digest_test.go::TestDigest_SendsOncePerDay
```

```
AC-8.1.2 (edge case — повторный запуск в тот же день):
  Given: digest уже ok сегодня
  When:  worker.RunOnce("digest") в 09:05
  Then:  сообщений не отправлено
  Test:  backend/internal/worker/digest_test.go::TestDigest_SendsOncePerDay
```

```
AC-8.1.3 (error path — Telegram 429 и 400):
  Given: httptest отвечает 429 retry_after=1, затем 200; во втором кейсе 400 chat not found
  When:  RunOnce("digest")
  Then:  первый кейс — ok после 2 попыток; второй — job_runs failed, worker_telegram_send_errors_total = 1
  Test:  backend/internal/worker/digest_test.go::TestDigest_TelegramErrors
```

```
AC-8.1.4 (happy path — тестовое сообщение):
  Given: chat_id задан
  When:  POST /admin/v1/settings/telegram/test
  Then:  200 TelegramTestResult {sent: true, telegram_message_id: <int>}; сообщение «Тестовое сообщение appmetrica.online» отправлено;
         при ошибке Telegram (400 chat not found, 401 bad token, сеть) — 200 {sent: false, error: "<описание от Telegram>"}, не 5xx (контракт 06-api-contracts.md, operationId sendTelegramTest)
  Test:  backend/internal/api/settings_test.go::TestSendTelegramTest
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Нет активных приложений → сообщение «Нет активных приложений» не отправляется, job ok с `details.skipped = "no_apps"`.
- Смена `digest_time` на более раннее после отправки — повторной отправки в тот же день нет.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: построение дайджеста ≤ 10 с.
- Concurrency: один экземпляр worker; при rolling update дубль исключён проверкой `job_runs` в транзакции с
  advisory lock `pg_try_advisory_xact_lock(hashtext('digest'))`.
- Idempotency: не более одного сообщения в сутки.

**UX detail (observable states)**:
- Loading state: кнопка «Отправить тест» disabled на время запроса.
- Empty state: chat_id не задан — поле с подсказкой «Напишите боту /start и вставьте chat_id».
- Error state: 502 → «Telegram отклонил сообщение: проверьте chat_id и токен бота».
- Success state: toast «Тестовое сообщение отправлено».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/telegram/client.go` (чистый `net/http`), `internal/worker/jobs_digest.go`.
- Связанные endpoints: `getSettings`, `updateSettings`, `sendTelegramTest`.
- Связанные сущности БД: `settings`, `job_runs`.
- Feature flag (если нужен): `WORKER_JOBS_ENABLED`.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

#### Story 8.2: System check, алерты и dead-man switch (FR-021)

**Как** владелец, **я хочу** узнавать о заполнении диска, сбоях вставки, устаревших бэкапах и падении сервисов в
течение 10 минут, **чтобы** выполнить критерий «ноль дней простоя приёма».

**Контекст**: job `system_check` каждые 5 минут:

| Проверка | Источник | Условие firing | key в `alert_states` |
|---|---|---|---|
| Свободное место ClickHouse | `SELECT free_space, total_space FROM system.disks WHERE name='default'` | free < `DISK_FREE_ALERT_PCT` (20 %) | `disk_free` |
| ClickHouse жив | ping за 5 с | ошибка | `clickhouse_down` |
| Ошибки вставки | `INGEST_METRICS_URL`: прирост `ingest_clickhouse_insert_errors_total` / прирост `ingest_clickhouse_insert_duration_seconds_count` с прошлого цикла | > 1 % | `insert_errors` |
| Свежесть бэкапа | `GET BACKUP_API_URL/backup/list/remote` → максимальный `created` | старше 26 ч, бэкапов нет или список недоступен | `backup_stale` |
| Ingest жив | `GET INGEST_HEALTH_URL` за 3 с | не 200 | `ingest_down` |
| API жив | `GET API_HEALTH_URL` за 3 с | не 200 | `api_down` |

Алерт срабатывает, когда условие выполнено два цикла подряд (T-052): первый неудачный цикл только записывает
`first_fired_at`. Переход `firing false → true` → Telegram «🔴 <описание> (значение)»; пока firing — повтор раз в 6 ч;
`true → false` → «🟢 Восстановлено: <описание>». При `alerts_enabled = false` состояния обновляются, сообщения не
шлются. После завершения цикла без исключений — `GET HEALTHCHECKS_PING_URL`; при панике/исключении цикла —
`GET HEALTHCHECKS_PING_URL/fail`. Если worker или сервер мертвы, healthchecks.io сам алертит в Telegram по тишине
(период 5 мин, grace 10 мин — настраивается в healthchecks.io).

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-8.2.1 (happy path — алерт по диску и восстановление):
  Given: фейк CH: free 15 %; Telegram — httptest
  When:  RunOnce("system_check") дважды; затем free 30 %, RunOnce
  Then:  после первого сообщений нет; после второго — 1 сообщение «🔴 На диске ClickHouse свободно 15 %»,
         alert_states.disk_free firing = true; третье — «🟢 Восстановлено: свободное место на диске ClickHouse (30 %)»,
         firing = false
  Test:  backend/internal/worker/systemcheck_test.go::TestSystemCheck_DiskAlertAndRecovery
```

```
AC-8.2.2 (edge case — повтор не чаще 6 ч):
  Given: disk_free firing, last_notified_at = now − 5 ч
  When:  RunOnce; clock.Advance(1h + 1m); RunOnce
  Then:  первое — сообщений 0; второе — 1 повтор
  Test:  backend/internal/worker/systemcheck_test.go::TestSystemCheck_RepeatEvery6h
```

```
AC-8.2.3 (happy path — dead-man ping):
  Given: все проверки ok; httptest вместо hc-ping.com
  When:  RunOnce("system_check")
  Then:  1 GET на HEALTHCHECKS_PING_URL; при панике проверки — 1 GET на …/fail
  Test:  backend/internal/worker/systemcheck_test.go::TestSystemCheck_HealthchecksPing
```

```
AC-8.2.4 (error path — бэкап устарел):
  Given: backup list remote: последний бэкап 27 ч назад
  When:  RunOnce дважды
  Then:  alert backup_stale firing, сообщение «🔴 Последний бэкап ClickHouse 27 ч назад»
  Test:  backend/internal/worker/systemcheck_test.go::TestSystemCheck_BackupStale
```

```
AC-8.2.5 (edge case — флаппинг ingest):
  Given: ingest /health: 503, затем 200
  When:  два цикла
  Then:  алерта нет (нужно 2 подряд неуспешных)
  Test:  backend/internal/worker/systemcheck_test.go::TestSystemCheck_RequiresTwoFailures
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Telegram недоступен: состояние алерта фиксируется, `last_notified_at` не обновляется, отправка повторяется в
  следующем цикле.
- `HEALTHCHECKS_PING_URL` пуст (dev) — пинг пропускается без ошибки.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: цикл ≤ 30 с (таймауты проверок 3–5 с).
- Concurrency: один цикл одновременно (`sync.Mutex` + advisory lock).
- Idempotency: повтор цикла не дублирует сообщения (машина состояний).

**UX detail (observable states)**:
- Loading state: вкладка «Система» (S-12) — скелетоны.
- Empty state: нет firing-алертов — «Всё в порядке» с временем последней проверки.
- Error state: firing-алерты красными строками с временем начала.
- Success state: таблица проверок и последних запусков jobs.

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/worker/systemcheck.go` (проверки и машина состояний), `internal/worker/cleanup.go`; допущения — OQ-067.
- Связанные endpoints: `getSystemStatus`.
- Связанные сущности БД: `alert_states`, `job_runs`, `settings`.
- Feature flag (если нужен): `WORKER_JOBS_ENABLED`, `settings.alerts_enabled`.

**Estimate**: 8 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] В healthchecks.io создан check «appmetrica-worker» (период 5 мин, grace 10 мин, интеграция Telegram)

#### Story 8.3: Закрытие дня и TTL (FR-022)

**Как** владелец, **я хочу**, чтобы история DAU сохранялась бессрочно, а сырые данные удалялись через 25 месяцев,
**чтобы** соблюдать минимизацию данных и не терять графики.

**Контекст**: job `close_day` каждые 15 минут: для каждого приложения (включая архивные) вычисляется `local_now` в его
TZ; если `local_now ≥ 03:30` и в `closed_days` нет `D = local_today − 8` → расчёт `daily_stats` для D по
`unit ∈ {persons, installs}` × `environment ∈ {production, development}` (4 строки), запись `closed_days(app_id, D)`.
Первый запуск для приложения закрывает все дни от `apps.created_at` (в TZ) до D, не более 400 дней за проход.
Повторное закрытие безопасно (ReplacingMergeTree по `closed_at`). TTL `events` — `event_date + INTERVAL 25 MONTH`,
выполняется ClickHouse фоново; `daily_stats` без TTL.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-8.3.1 (happy path):
  Given: APP_A (Europe/Moscow), события 2026-10-04; clock = 2026-10-13T00:31:00Z (03:31 MSK)
  When:  RunOnce("close_day")
  Then:  daily_stats содержит 4 строки app_id = APP_A, day = 2026-10-05 (13 − 8); closed_days (APP_A, 2026-10-05);
         значения active/new/sessions совпадают с getAudienceSummary за этот день
  Test:  backend/internal/worker/closeday_test.go::TestCloseDay_WritesFourRows
```

```
AC-8.3.2 (edge case — до 03:30 и разные TZ):
  Given: APP_A Europe/Moscow, APP_T Asia/Tokyo; clock = 2026-10-13T00:00:00Z (03:00 MSK, 09:00 JST)
  When:  RunOnce
  Then:  закрыт только APP_T (день 2026-10-05); APP_A — нет
  Test:  backend/internal/worker/closeday_test.go::TestCloseDay_RespectsAppTimezone
```

```
AC-8.3.3 (invariant — идемпотентность):
  Given: день уже закрыт
  When:  RunOnce дважды
  Then:  после OPTIMIZE FINAL в daily_stats ровно 4 строки за день
  Test:  backend/internal/worker/closeday_test.go::TestCloseDay_Idempotent
```

```
AC-8.3.4 (error path — CH недоступен):
  Given: CH-фейк с ошибкой
  When:  RunOnce
  Then:  closed_days не записан; job_runs close_day failed; следующий успешный запуск закрывает день
  Test:  backend/internal/worker/closeday_test.go::TestCloseDay_RetriesAfterFailure
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- Удаление пользователя после закрытия дня не меняет `daily_stats` — агрегаты без PII, задокументировано.
- Смена TZ приложения: уже закрытые дни не пересчитываются.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: закрытие одного дня одного приложения ≤ 5 с при 100k DAU.
- Concurrency: advisory lock `close_day`.
- Idempotency: см. AC-8.3.3.

**UX detail (observable states)**:
- Loading state: N/A.
- Empty state: N/A.
- Error state: failed запуски видны во вкладке «Система» (S-12).
- Success state: N/A (фоновая задача).

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/worker/jobs_closeday.go`, SQL `internal/reports/sql/daily_stats_close.sql`
  (переиспользует выражения отчётов аудитории).
- Связанные endpoints: нет.
- Связанные сущности БД: `daily_stats`, `closed_days`, `events`.
- Feature flag (если нужен): `WORKER_JOBS_ENABLED`.

**Estimate**: 6 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0

#### Story 8.4: Настройки владельца и статус системы (FR-025)

**Как** владелец, **я хочу** управлять часовым поясом, Telegram и видеть состояние системы, **чтобы** настраивать
всё в одном месте.

**Контекст**: `updateSettings` валидирует: `owner_timezone` — IANA (`time.LoadLocation`); `telegram_chat_id` —
`^-?[0-9]{1,20}$` или `^@[A-Za-z0-9_]{5,32}$`; `digest_time` — `HH:MM`; булевы флаги. `getSystemStatus` собирает:
ClickHouse (`ok`, `disk_free_pct`, `events_rows` из `system.parts` активных частей), Postgres (`ok`), бэкап
(`last_backup_at`, `last_backup_name` из clickhouse-backup REST), `alert_states`, последние 20 `job_runs`.

**Acceptance criteria (TDD — Given/When/Then, executable)**:

```
AC-8.4.1 (happy path):
  Given: сессия
  When:  PATCH /admin/v1/settings {owner_timezone: "Asia/Almaty", digest_enabled: true, digest_time: "08:30"}
  Then:  200, GET /admin/v1/settings возвращает те же значения; updated_at изменился
  Test:  backend/internal/api/settings_test.go::TestUpdateSettings_Persists
```

```
AC-8.4.2 (error path — валидация):
  Given: сессия
  When:  {telegram_chat_id: "abc"}; {digest_time: "25:00"}; {owner_timezone: "Moscow"}
  Then:  все 400 "invalid_request" с details.field
  Test:  backend/internal/api/settings_test.go::TestUpdateSettings_Validation
```

```
AC-8.4.3 (happy path — статус):
  Given: CH и PG подняты, clickhouse-backup — httptest с одним бэкапом
  When:  GET /admin/v1/system/status
  Then:  clickhouse.ok = true, postgres.ok = true, backup.last_backup_at задан, jobs — массив ≤ 20
  Test:  backend/internal/api/system_test.go::TestSystemStatus
```

```
AC-8.4.4 (UI — вкладки настроек):
  Given: MSW-моки settings, passkeys, api-keys, system, deletion-requests
  When:  рендер SettingsPage, навигация стрелками по вкладкам
  Then:  tablist с 5 вкладками «Passkeys», «API-ключи», «Telegram», «Система», «Удаления», клавиатурная навигация
         по WAI-ARIA Tabs
  Test:  web/src/app/(dashboard)/settings/settings-page.test.tsx::"tabs are keyboard navigable"
```

**Edge cases (дополнительно, с AC-форматом если не покрыто выше)**:
- clickhouse-backup недоступен: `backup = {}` и предупреждение в UI, статус остальных компонентов показывается.
- Смена `owner_timezone` меняет «вчера» в портфеле по умолчанию и время дайджеста, но не дни отчётов приложений.

**Non-functional requirements для этой story** (верифицируемые):
- Latency: `getSystemStatus` ≤ 2 с (параллельные проверки с таймаутом 1,5 с).
- Concurrency: N/A.
- Idempotency: PATCH идемпотентен.

**UX detail (observable states)**:
- Loading state: скелетоны каждой вкладки.
- Empty state: см. отдельные вкладки.
- Error state: inline-ошибки полей; 503 → «Статус недоступен».
- Success state: toast «Настройки сохранены».

**Tech notes (для разработчика)**:
- Подход к реализации: `internal/api/settings.go`, `internal/api/system.go`; web — `src/app/(dashboard)/settings/page.tsx`.
- Связанные endpoints: `getSettings`, `updateSettings`, `sendTelegramTest`, `getSystemStatus`, `listDeletionRequests`.
- Связанные сущности БД: `settings`, `alert_states`, `job_runs`, `deletion_requests`.
- Feature flag (если нужен): нет.

**Estimate**: 8 часов.

**Definition of Done (TDD-verified)**:
- [ ] Общий DoD §2.0
- [ ] E2E `web/e2e/settings.spec.ts::"owner updates telegram settings"`

---

## 3. Нефункциональные требования (детально)

### 3.1. Производительность

| Метрика | Target | Измеряется как |
|---|---|---|
| API P50 | < 300 мс (отчёты), < 20 мс (ingest без ожидания CH) | `api_request_duration_seconds{operation}` и `ingest_batch_duration_seconds` (Prometheus histogram) |
| API P95 | < 1,5 с отчёты на 90 днях (NFR-003), < 1 с портфель | Перф-тест `TestReportsPerf_90Days100kDAU` + production-гистограммы |
| API P99 | ≤ 500 мс ingest при 500 rps (NFR-001) | k6 `backend/perf/ingest-load.js` |
| Page LCP | < 2.5s | Lighthouse CI (desktop preset) на S-03, S-05, S-06 в CI против docker compose |
| Page CLS | < 0.1 | Lighthouse CI |
| Time to interactive | < 3 с (desktop, кабель) | Lighthouse CI; INP ≤ 200 мс (NFR-006) |
| DB query P95 | ClickHouse отчёт ≤ 1,2 с, Postgres ≤ 20 мс | `api_clickhouse_query_duration_seconds{report}`, `pgx` tracer → `api_postgres_query_duration_seconds` |
| Realtime-задержка | ≤ 10 с от ответа 200 до появления в ленте (NFR-004) | `TestRealtime_VisibleWithin10s` |
| Initial JS | ≤ 250 KB gz на страницах дашборда (NFR-006) | `next build` + `@next/bundle-analyzer` отчёт, проверка скриптом `web/scripts/check-bundle-size.ts` в CI |

### 3.2. Нагрузка

- Target concurrent users: 1 пользователь дашборда (владелец) + до 5 скриптов Read API; на ingest — до 100 000
  устройств, присылающих батчи независимо.
- Target RPS at peak: 500 запросов/с на ingest (≈ 5 000 событий/с), 20 запросов/с на api.
- Data write rate: средние 60 событий/с (150 млн/мес), пиковые 5 000 событий/с; ClickHouse async insert объединяет
  вставки в блоки (настройки `async_insert_busy_timeout_max_ms = 200`, `async_insert_max_data_size = 10485760`;
  200, а не 1000: ingest ждёт ack, и под нагрузкой адаптивный таймаут уходит к максимуму — при 1000 P99 ≈ 920 мс,
  при 200 — ≈ 210 мс на k6 500 rps, OQ-034).
- Data read:write ratio: по запросам 1:100 (ingest доминирует), по объёму сканируемых данных 50:1 (отчёты).

### 3.3. Security requirements (детально)

#### Authentication
- Provider: собственная реализация WebAuthn на go-webauthn v0.18.2 (единственный пользователь, внешний IdP избыточен).
- Session mechanism: opaque cookie `am_session` (HttpOnly, Secure, SameSite=Lax, host-only `appmetrica.online`),
  токен 32 байта, в БД — sha256; Read API — `Authorization: Bearer sk_…`; ingest — публичный `X-App-Key: pk_…`.
- Session lifetime: абсолютный 30 дней, idle 7 дней (sliding).
- Session rotation: новая сессия на каждый вход; при выходе — revoke; при удалении passkey активные сессии остаются
  (одна учётная запись, пароля нет).
- 2FA: passkey с обязательной user verification — сам по себе двухфакторный (владение устройством + биометрия/PIN).

#### Authorization
- Model: единственный владелец (сессия = полный доступ) + scopes API-ключей (`read`, `delete`) и опциональная
  привязка ключа к одному приложению.
- Roles: `owner` (сессия); API-ключ с `read`; API-ключ с `delete`; публичный write-ключ (только `POST /v1/events`
  своего приложения).
- Permissions: матрица — [06-api-contracts.md](./06-api-contracts.md), раздел безопасности операций.
- Multi-tenant isolation: N/A — один владелец; изоляция приложений внутри Read API — фильтр `app_id` в каждом запросе
  и проверка привязки ключа.

#### Data protection
- PII поля: `events.user_id` (псевдоним), `events.install_id` (псевдоним), `events.city`, `events.region`,
  `events.device_model`, `rejected_events.payload_sample`, `auth_sessions.user_agent`. IP не хранится (NFR-007).
- Encryption at rest: `# Deviation`: диски сервера без LUKS (нужна перезагрузка без ручного ввода ключа; данные —
  псевдонимизированная аналитика); бэкапы в приватных бакетах Hetzner Object Storage с versioning и object lock;
  секретные `sk_` — только HMAC-SHA256 с pepper.
- Encryption in transit: TLS 1.3 на Traefik (Let's Encrypt HTTP-01) для трёх доменов; внутренний трафик между
  контейнерами — overlay-сеть Docker Swarm на одном хосте.
- Secrets management: переменные окружения Dokploy (Applications → Environment) и GitHub Actions secrets; в репозитории
  только `.env.example`.
- Access logging: access-лог Traefik выключен на `i.appmetrica.online` (содержит IP); api логирует `operation`,
  `status`, `duration_ms`, `auth_kind` (session|api_key), `api_key_prefix` — без IP. В поле `path` — шаблон
  маршрута `ServeMux` (`/admin/v1/auth/setup/{token}/begin`), а при 404/405 — константа `<unmatched>`:
  сырой путь не пишется никогда, в нём может стоять токен ссылки регистрации passkey.

#### Threat model highlights
- Утечка/извлечение `pk_` из приложения и спам событиями → лимиты per app и per install, отзыв ключа, флаг
  `INGEST_ACCEPT_DEVELOPMENT`, видимость на S-10.
- Захват панели Dokploy (критические CVE 2026) → панель только через Tailscale, порт 3000 закрыт в Robot firewall,
  ручные обновления ≤ 48 ч для security-релизов.
- CSRF на `/admin/v1` → SameSite=Lax + обязательный `X-Requested-With` + проверка `Origin`.
- Подбор `sk_` → 238 бит энтропии, HMAC-поиск, лимит 10 rps на ключ, метрика `api_auth_failures_total`.
- SQL-инъекции в отчёты через фильтры → только параметризованные запросы ClickHouse (`{name:Type}`), значения
  фильтров не конкатенируются; `dimension` — enum с whitelisting колонок.
- PII в `user_id` по ошибке клиента → PII-guard (Story 1.5) и `[redacted]` в сэмплах.

### 3.4. Observability requirements

#### Logs
- Level conventions: DEBUG / INFO / WARN / ERROR
- Structured logging: JSON (`log/slog` JSONHandler) с полями `time`, `level`, `msg`, `service` (ingest|api|worker),
  `request_id`, `operation` (operationId или `ingest_events`), `app_id`, `status`, `duration_ms`, `events`,
  `accepted`, `rejected`, `auth_kind`, `api_key_prefix`, `job`, `error`.
- Retention: логи контейнеров Docker `json-file` с ротацией `max-size=50m`, `max-file=5` (≈ 7 дней при штатной
  нагрузке); централизованного хранения логов нет (`# Deviation`: solo-проект, метрики + алерты покрывают обнаружение).
- PII redaction: в логи никогда не пишутся IP, `user_id`, `install_id`, `session_id`, тела запросов, `sk_`/`pk_` целиком
  (только `api_key_prefix` и `write_key_id`), токены сессий и setup-ссылок. Тест
  `backend/internal/httpx/logging_test.go::TestLogging_NoSensitiveFields`.

#### Metrics
- Business metrics: `ingest_events_accepted_total{app_id}`, `ingest_events_rejected_total{code}`,
  `ingest_warnings_total{code}` (ожидающие удаления и отправленные дайджесты видны в `job_runs.details` и на S-12,
  отдельных метрик нет — OQ-069).
- System metrics: `ingest_requests_total{status}`, `ingest_batch_duration_seconds`, `ingest_batch_events`,
  `ingest_clickhouse_insert_duration_seconds`, `ingest_clickhouse_insert_errors_total`,
  `ingest_rate_limited_total{scope}`, `ingest_install_limiters`, `ingest_key_cache_size`,
  `ingest_key_cache_refresh_errors_total`, `ingest_key_cache_last_refresh_timestamp_seconds`,
  `ingest_rejected_log_dropped_total`, `api_request_duration_seconds{operation,status}`,
  `api_clickhouse_query_duration_seconds{report}`, `api_auth_failures_total{reason}`, `api_rate_limited_total`,
  `worker_job_duration_seconds{job}`, `worker_job_failures_total{job}` (вместо `worker_job_runs_total{job,status}`:
  успешные запуски считаются по `worker_job_duration_seconds_count`), `worker_alerts_firing{key}`,
  `worker_telegram_send_errors_total`; стандартные `go_*` и `process_*`. Гео-ошибки отдельной метрики не имеют:
  невалидный `context.ip` — warning `geo_ip_invalid` в `ingest_warnings_total`.
- Alert thresholds: диск ClickHouse < 20 % свободно; доля ошибок вставки > 1 % за 5 мин; бэкап старше 26 ч; ingest/api
  `/health` не 200 два цикла подряд; тишина healthchecks.io > 15 мин; deletion `failed`.

#### Traces
- Provider: `# Deviation` — распределённая трассировка OpenTelemetry в MVP не включается: три процесса на одном хосте,
  вызовы между сервисами только Next.js → api; корреляция через `request_id` (заголовок `X-Request-Id` пробрасывается
  из Next.js в api и возвращается клиенту). Подключение OpenTelemetry Go SDK + OTLP — open question в
  [10-open-questions.md](./10-open-questions.md).
- Sampling rate: N/A в MVP (трассировка не собирается).

### 3.5. Compliance checklist

- [x] GDPR: consent flow — ответственность приложений владельца (privacy policy приложений обновляется, аналитика без
  рекламных ID и IP); data export — профиль (S-09) и Read API `getProfile`; right to be forgotten — FR-019.
- [ ] CCPA: N/A — аудитория приложений владельца в РФ/СНГ/ЕС, продажи данных нет.
- [ ] PCI DSS: N/A — платежей нет.
- [ ] SOC 2: N/A — внутренний инструмент.
- [x] Accessibility: WCAG 2.2 AA (NFR-008).
- [ ] 152-ФЗ (локализация ПДн граждан РФ): открытый юридический вопрос, данные хранятся в ЕС —
  [09-risks-and-mitigations.md](./09-risks-and-mitigations.md), [10-open-questions.md](./10-open-questions.md).

---

## 4. API / Integration Requirements

### 4.1. Внутренние API

Детали — в [06-api-contracts.md](./06-api-contracts.md).

- Protocol: REST/JSON, контракты OpenAPI 3.0.3 (`contracts/openapi/ingest.yaml`, `contracts/openapi/api.yaml`),
  Go-сервер — oapi-codegen v2.8.0 (strict-server для api), TS-клиент — orval 8.42.0 (TanStack Query + zod + MSW).
- Auth: ingest — `X-App-Key: pk_…` или `app_key` в теле; Read API — `Bearer sk_…`; admin — cookie `am_session` +
  `X-Requested-With: appmetrica-web`.
- Versioning: путь `/v1/`, `/admin/v1/`; изменения только аддитивные; breaking → `/v2/` с параллельной поддержкой `/v1`
  ≥ 12 месяцев (клиенты вшиты в приложения в сторах).
- Pagination: курсор `before` (timestamp) только для сессий профиля; остальные списки ограничены по размеру
  (приложения ≤ 100, ключи ≤ 50, удаления — последние 200).
- Rate limiting: ingest 2000 событий/с на приложение и 100/мин (burst 1000) на установку; Read API 10 req/s на ключ
  (burst 30); сессия 30 req/s; auth-эндпоинты 30 req/мин на процесс.
- Body limit: ingest — `INGEST_MAX_BODY_BYTES` (1 048 576 байт после распаковки); api — 65 536 байт на любой запрос
  (`api.MaxBodyBytes`, `http.MaxBytesReader` до разбора тела) → 413 `payload_too_large`.

### 4.2. Внешние интеграции

| Сервис | Назначение | Критичность | Fallback |
|---|---|---|---|
| DB-IP City Lite (mmdb в образе) | Гео по IP | P1 | Файл в образе; без файла ingest не стартует; ошибки lookup → пустое гео, приём продолжается |
| Telegram Bot API | Дайджест и алерты | P1 | 3 попытки с `retry_after`; сбой фиксируется в `job_runs`; dead-man healthchecks.io алертит независимо |
| healthchecks.io | Dead-man switch worker/сервера | P1 | Сам является fallback-ом; при недоступности пинги теряются без влияния на продукт |
| Hetzner Object Storage | Бэкапы CH (clickhouse-backup) и PG (Dokploy) | P0 для DR | Локальные снапшоты отсутствуют; `backup_stale` алерт через 26 ч; ручной бэкап по runbook |
| GHCR | Хранение образов | P1 | Образ предыдущего деплоя закэширован на хосте; откат работает без GHCR |
| Tailscale | Доступ к Dokploy и SSH, деплой из CI | P1 | Аварийный доступ через Hetzner Robot KVM-консоль (runbook) |

### 4.3. Webhooks

**Входящие** (мы принимаем):
- Нет. Деплой инициируется GitHub Actions вызовом Dokploy API (`/api/application.deploy`), а не входящим вебхуком
  продукта; вебхуки Dokploy от GitHub не используются (панель закрыта Tailscale).

**Исходящие** (мы отправляем):
- Формат: N/A — исходящих вебхуков продукт не отправляет; Telegram и healthchecks.io вызываются как клиентские API.
- Signature: N/A.
- Retry policy: для Telegram — 3 попытки с `retry_after`; для healthchecks.io — без ретраев (следующий цикл через 5 мин).
- Idempotency: дайджест — не более одного в сутки (`job_runs` + advisory lock); алерты — машина состояний `alert_states`.

---

## 5. Data Requirements

Полная схема — в [05-data-model.md](./05-data-model.md). Ниже — highlights.

### 5.1. Ключевые сущности
- `appmetrica.events` (ClickHouse) — сырые системные события `session_start` / `session_end` с контекстом и гео.
- `appmetrica.identity_links`, `install_first_seen`, `user_first_seen` (ClickHouse) — агрегаты для склейки и first_seen.
- `appmetrica.rejected_events` (ClickHouse) — отклонённые события для экрана «Интеграция».
- `appmetrica.daily_stats` (ClickHouse) — бессрочные дневные агрегаты закрытых дней.
- `apps`, `write_keys`, `api_keys` (Postgres) — приложения и ключи.
- `owner`, `passkeys`, `passkey_setup_tokens`, `webauthn_ceremonies`, `auth_sessions` (Postgres) — аутентификация.
- `deletion_requests`, `settings`, `job_runs`, `alert_states`, `closed_days` (Postgres) — операционные данные.

### 5.2. Объёмы
- `events`: ≈ 182 млн строк к концу первого года при 100k DAU и одних системных событиях (≈ 19 ГБ сжатых за 25 месяцев); верхняя граница с кастомными событиями (P1) — 1–2 млрд строк в год, ≈ 210 ГБ за 25 месяцев.
- `install_first_seen`: до 5 млн строк в год; `identity_links` и `user_first_seen`: до 2 млн строк в год.
- `rejected_events`: ≤ 10 млн строк в скользящем окне 7 дней.
- `daily_stats`: 4 строки × приложение × день ≈ 15 000 строк в год на 10 приложений.
- Postgres: < 100 МБ к концу первого года.
- Attachments/файлы: нет; гео-база 121 МБ внутри образа ingest.

### 5.3. Data retention
| Сущность | Retention | Удаление | Архивация |
|---|---|---|---|
| `events` | 25 месяцев | TTL ClickHouse + GDPR-удаление ≤ 24 ч | Нет; агрегаты в `daily_stats` |
| `install_first_seen`, `user_first_seen`, `identity_links` | Пока есть события установки/пользователя | GDPR-удаление; осиротевшие строки удаляет worker ежемесячно (open question по автоматизации) | Нет |
| `rejected_events` | 7 дней | TTL ClickHouse | Нет |
| `daily_stats` | Бессрочно | Только при удалении приложения | Нет |
| `auth_sessions`, `webauthn_ceremonies`, `passkey_setup_tokens` | До истечения | job `session_cleanup` ежечасно | Нет |
| `job_runs` | 90 дней | job `session_cleanup` | Нет |
| `deletion_requests` | Бессрочно (журнал исполнения GDPR) | Каскадно при удалении приложения | Копия в `job_runs.details` |
| Логи контейнеров | ≈ 7 дней (ротация 5 × 50 МБ) | Docker json-file | Нет |

### 5.4. Backup / recovery
- Backup частота: ClickHouse — clickhouse-backup 2.8.1 в режиме `server --watch`: инкремент каждые 24 ч (`WATCH_INTERVAL`),
  полный раз в 7 дней (`FULL_INTERVAL`); Postgres — ежедневный бэкап Dokploy в S3.
- Backup retention: ClickHouse — 35 удалённых бэкапов (`BACKUPS_TO_KEEP_REMOTE`); Postgres — 30 бэкапов.
- Тест восстановления: ежеквартально по runbook [21-ops-runbooks.md](./21-ops-runbooks.md) на временном сервере
  Hetzner Cloud; первый тест — до снятия AppMetrica SDK.
- RPO: 24 часа (катастрофа сервера); 0 для подтверждённых событий при отказе одного диска (RAID1).
- RTO: 4 часа.

---

## 6. Deployment & Operations

Детали — в [03-architecture.md § Deployment](./03-architecture.md#deployment).

### 6.1. Environments
- `dev` — локальный: `make dev` поднимает ClickHouse 26.8.25.14 и Postgres 18.6 через `docker compose`
  (`docker-compose.dev.yml` в корне репо), Go-процессы `go run ./cmd/appmetrica <role>`, web — `pnpm dev`; WebAuthn RP ID `localhost`.
- `preview` — `# Deviation`: preview-окружений нет (Dokploy не делает preview для приложений из образов); замена —
  E2E Playwright в CI против полного стека в docker compose.
- `staging` — `# Deviation`: отдельного staging нет (solo, один сервер); замена — integration-тесты на testcontainers,
  rolling update с `FailureAction: rollback` и проверкой healthcheck.
- `prod` — Hetzner AX102-1 (FSN1), Dokploy ≥ v0.30.8, домены `appmetrica.online`, `i.appmetrica.online`,
  `api.appmetrica.online`.

### 6.2. CI/CD
- Platform: GitHub Actions (`.github/workflows/ci.yml`, `deploy.yml`, `geo-refresh.yml`).
- Pipeline stages: lint (golangci-lint v2.14.0, Biome 2.5.15, `tsc --noEmit`) → test (Go unit + integration на
  testcontainers, Vitest, contract) → build (образы `appmetrica-backend`, `appmetrica-web` в GHCR, тег — git SHA) →
  e2e (Playwright против docker compose) → deploy (только `main`).
- Auto-deploy: `main` → prod автоматически после зелёного CI (trunk-based, короткие ветки); `geo-refresh.yml` —
  ежемесячная пересборка ingest 2-го числа в 06:00 UTC.
- Rollback strategy: автоматический — Swarm `FailureAction: rollback` при провале healthcheck; ручной — Dokploy
  Deployments → Rollback на предыдущий образ (≤ 2 мин) или `deploy.yml` с `workflow_dispatch` и тегом предыдущего SHA.

### 6.3. Monitoring / Alerting
- On-call policy: владелец — единственный получатель алертов; реакция на 🔴 в рабочее время ≤ 2 ч, ночью — при
  `ingest_down` / `clickhouse_down` / `disk_free` в течение утра (клиентские очереди держат данные 7 дней).
- Alert channels: Telegram (worker `system_check`, Dokploy notifications: App Deploy, App Build Error, Database Backup,
  Docker Cleanup, Dokploy Restart), healthchecks.io → Telegram.
- SLOs:
  - Availability: 99.9 % ingest в месяц (NFR-002), измеряется пингами healthchecks.io и `ingest_requests_total{status="5xx"}`.
  - Error rate: < 0,1 % ответов 5xx на ingest; < 1 % ошибок вставки в CH за 5 мин.
  - Latency P95: ingest ≤ 300 мс, отчёты ≤ 1,5 с.

---

## 7. Quality & Testing

**Full test strategy** — в отдельном документе [22-test-strategy.md](./22-test-strategy.md) (pyramid, frameworks, coverage targets, CI gates, fixtures).

Ниже — коротко highlights, специфичные этому проекту.

### 7.1. Testing pyramid (резюме)

| Уровень | Target coverage | Tools | Когда blocker-gate |
|---|---|---|---|
| Unit | **≥ 90%** core business logic (`ingest`, `identity`, `reports` SQL-билдеры), ≥ 80% overall (NFR-009) | Go `testing` + testify v1.12.1; Vitest 5.0.4 | Каждый PR |
| Integration (API ↔ DB) | 100% endpoints happy + errors | testcontainers-go v0.44.0 (ClickHouse 26.8.25.14, Postgres 18.6) + golden-фикстуры | Каждый PR |
| Component | 100% новых компонентов отчётов | Testing Library + MSW 3.0.3 (моки orval) | Каждый PR |
| Contract | 100% endpoints | kin-openapi `openapi3filter` (валидация ответов Go-хендлеров по OpenAPI) + orval-типы | Каждый PR |
| E2E (smoke) | 9 critical journeys (`web/e2e/*.spec.ts`) | Playwright 1.64.0 + виртуальный WebAuthn authenticator (CDP) | Каждый PR |
| E2E (full) | all P0 ACs с пометкой E2E | Playwright | Перед релизом (каждый merge в `main`) |
| Accessibility | 0 axe violations на новых screens | @axe-core/playwright | Каждый PR |
| Visual regression | no unreviewed diffs на S-03, S-05, S-06, S-07, S-10 | Playwright `toHaveScreenshot` (`# Deviation` от Chromatic: solo, без внешнего сервиса) | Каждый PR |
| Performance | NFR-001, NFR-003 | k6 (`backend/perf/ingest-load.js`), `TestReportsPerf_90Days100kDAU` (тег `perf`) | Еженедельно (cron в CI) + перед снятием AppMetrica SDK |
| Security (SAST/SCA) | 0 criticals | golangci-lint (gosec), govulncheck, `pnpm audit --audit-level high`, Dependabot | Каждый PR |

### 7.2. TDD discipline (обязательна для FR с verifiable AC)

1. **Red**: failing test для каждого AC (как в §2 story template) — commit-ится ПЕРВЫМ
2. **Green**: минимальная implementation для passing tests — следующий commit
3. **Refactor**: improve quality без изменения behaviour — tests остаются green

Review rejects PR где implementation commit предшествует test commit (git log check).

Специфика проекта: для отчётов «Red» — это golden-файл с ожидаемым ответом, посчитанным вручную по фикстуре
(таблица в комментарии фикстуры), ДО написания SQL. Golden-файлы не перегенерируются автоматически из кода
(флаг `-update` запрещён в CI), изменение golden — отдельный коммит с объяснением.

### 7.3. Quality gates (CI blocks merge)

- [ ] Lint + typecheck pass (golangci-lint v2.14.0, Biome 2.5.15, `tsc --noEmit` на TypeScript 6.0.3 strict)
- [ ] Unit + integration + component tests pass
- [ ] E2E smoke suite pass
- [ ] A11y tests: 0 violations на новых/изменённых screens
- [ ] Coverage не опустился (diff check, ≥ 80% overall maintained)
- [ ] Visual regression: no unreviewed diffs
- [ ] Security scan: 0 critical/high vulns
- [ ] Code review approved (1+ reviewer)
- [ ] Perf budget не нарушен (если NFR в story указан)

Дополнительно: `make gen && git diff --exit-code` — сгенерированный код (oapi-codegen, sqlc, orval) соответствует
контрактам; Conventional Commits проверяются `commitlint`-эквивалентом в lefthook (`commit-msg`).

**Feature flags** (`# Deviation` от LaunchDarkly / Statsig / Unleash: solo-проект на одном сервере, внешний сервис флагов
добавляет зависимость без пользы; флаги — переменные окружения, переключение — redeploy через Dokploy за ≤ 2 мин):

| Флаг | По умолчанию | Сервис | Назначение |
|---|---|---|---|
| `INGEST_ACCEPT_DEVELOPMENT` | `true` | ingest | Kill switch dev-трафика при перегрузке |
| `API_PUBLIC_BEARER_ENABLED` | `true` | api | Kill switch внешнего Read API |
| `WORKER_JOBS_ENABLED` | `system_check,digest,close_day,deletion,session_cleanup` | worker | Включённые фоновые задачи |
| `MIGRATE_ON_START` | `true` | api | Автомиграции при старте |

Незавершённая работа в trunk прячется не флагами, а отсутствием ссылки в навигации до готовности экрана
(страница существует, но недостижима из UI), что достаточно для одного пользователя.

---

## 8. Glossary / Термины

| Термин | Определение |
|---|---|
| Установка (install) | Экземпляр приложения на устройстве/в браузере, идентифицируется `install_id` (UUID, генерируется клиентом при первом запуске; iOS — Keychain, Android — SharedPreferences + Auto Backup, web — localStorage) |
| Пользователь (user_id) | Непрозрачный внутренний ID из бэкенда приложения, передаётся после логина |
| Человек (person) | Единица подсчёта метрик при `unit=persons`; ключ `person_key` = `u:<user_id>` или `i:<install_id>` |
| Первый владелец | Правило склейки: анонимные события установки приписываются первому `user_id`, когда-либо вошедшему на ней |
| Сессия | Интервал от `session_start` до `session_end` с общим `session_id`, генерируемым клиентом при выходе приложения на передний план |
| Environment | `production` или `development`; отчёты по умолчанию показывают `production` |
| Write-ключ (`pk_`) | Публичный ключ приложения для ingest, вшивается в клиент |
| API-ключ (`sk_`) | Секретный ключ Read API со scopes `read` / `delete` |
| Окно досыла | 7 дней: события старше отклоняются `event_too_old` |
| Закрытый день | День D ≤ сегодня − 8 (в TZ приложения), агрегаты которого записаны в `daily_stats` и больше не меняются |
| Когорта | Люди с `first_seen` в одном дне/неделе; основа retention |
| Day-N retention | Доля когорты с хотя бы одним событием ровно в день когорты + N (classic, как AppMetrica) |
| Отклонённое событие | Событие, не принятое ingest с кодом `RejectionCode`, хранится 7 дней в `rejected_events` |
| `insert_id` | UUID события от клиента для идемпотентности |
| `sent_at` | Время отправки батча по часам устройства, используется для коррекции времени событий |
| Сверка | 14-дневное сравнение с AppMetrica в режиме `unit=installs` (NFR-010) |

---

## 9. References

- PRD: [01-prd.md](./01-prd.md)
- Architecture: [03-architecture.md](./03-architecture.md)
- Stack rationale: [04-tech-stack.md](./04-tech-stack.md)
- Data model: [05-data-model.md](./05-data-model.md)
- API contracts: [06-api-contracts.md](./06-api-contracts.md)
- Roadmap: [08-roadmap.md](./08-roadmap.md)
- Risks: [09-risks-and-mitigations.md](./09-risks-and-mitigations.md)
- UX/UI: [14-ux-ui-spec.md](./14-ux-ui-spec.md)
- Ops runbooks: [21-ops-runbooks.md](./21-ops-runbooks.md)
- Test strategy: [22-test-strategy.md](./22-test-strategy.md)
- Tickets: [13-tickets.md](./13-tickets.md)
