# API Contracts: appmetrica.online

**Версия**: 1.0 | **Дата**: 2026-10-13

> Контракты endpoints, events, webhooks. Достаточно детализировано, чтобы
> фронтенд / мобильный клиент мог начать интеграцию до готовности backend.
>
> В системе два публичных контракта и один внутренний:
> - **Ingestion API** (`https://i.appmetrica.online`, файл `contracts/openapi/ingest.yaml`) — приём системных событий `session_start` / `session_end` от приложений **без SDK**, аутентификация публичным write-ключом `pk_…`.
> - **Read API** (`https://api.appmetrica.online/v1`, файл `contracts/openapi/api.yaml`, теги без `x-internal`) — отчёты, профили, здоровье интеграции, GDPR-удаление; аутентификация секретным ключом `sk_…` (Bearer) или cookie-сессией дашборда.
> - **Admin API** (`/admin/v1/*` в том же `api.yaml`, все операции помечены `x-internal: true`) — только для дашборда `appmetrica.online`: passkey-вход, приложения, ключи, настройки, статус системы. В публичную документацию (Fumadocs, S-13) не попадает.
>
> Единый источник имён — канонический якорь пакета; связанные документы: [05-data-model.md](./05-data-model.md) (таблицы Postgres/ClickHouse, на которые опираются ответы), [02-tech-spec.md](./02-tech-spec.md) (FR-001…FR-025), [22-test-strategy.md](./22-test-strategy.md) (фреймворки и фикстуры), [14-ux-ui-spec.md](./14-ux-ui-spec.md) (экраны S-01…S-14, потребляющие эти эндпоинты).

---

## 1. Общие соглашения

### 1.1. Стиль API
- **Protocol**: REST поверх HTTPS (HTTP/1.1 и HTTP/2 через Traefik v3.6 в Dokploy). GraphQL/tRPC/gRPC не используются: клиенты — приложения без SDK, им нужен самый простой контракт «один POST с JSON».
- **Формат**: JSON, UTF-8.
- **Content-Type**: `application/json` для всех эндпоинтов. Исключение — `POST /v1/events` дополнительно принимает `text/plain` (тело всё равно JSON): `navigator.sendBeacon` с `Blob` типа `text/plain` не вызывает CORS-preflight.
- **Сжатие запроса**: `Content-Encoding: gzip` поддерживается только на `POST /v1/events`. Другие значения → `400 unsupported_content_encoding`.
- **Versioning**: URL path `/v1/...` (ingest и Read API) и `/admin/v1/...` (admin). Breaking changes выпускаются только как `/v2` с параллельной поддержкой `/v1` минимум 12 месяцев (ingest-клиенты вшиты в бинарники приложений и обновляются медленно). Устаревающие операции получают заголовки ответа `Deprecation: @<unix-ts>` (RFC 9745) и `Sunset: <HTTP-date>` (RFC 8594) минимум за 6 месяцев до отключения, плюс запись в Changelog этого документа.
- **Что считается breaking**: удаление/переименование поля ответа, новое обязательное поле запроса, сужение enum во входных данных, смена семантики метрики (например, определения `new`). **Не breaking**: новые необязательные поля запроса, новые поля ответа, новые значения enum в ответах (клиенты обязаны игнорировать неизвестные поля и значения), новые `RejectionCode`/`WarningCode` (клиент трактует неизвестный код как `retryable` из ответа).
- **Character case в JSON**: `snake_case` (Deviation от шаблонного camelCase: совпадает с колонками ClickHouse/Postgres и с привычным форматом аналитических API — Amplitude, Mixpanel, AppMetrica Logs API; один стиль на ingest, Read API и admin).
- **Timestamps**: RFC 3339 / ISO 8601 с миллисекундами. Сервер всегда отвечает в UTC (`2026-10-13T09:15:02.318Z`), кроме полей-«корзин» отчётов (`ts` в timeseries, `minute` в realtime), которые отдаются с offset часового пояса приложения (`2026-10-13T00:00:00+03:00`), чтобы граница суток совпадала с отчётом. Клиент ingest обязан передавать `timestamp` и `sent_at` **с явным offset** (`Z` или `±hh:mm`).
- **Даты отчётов**: параметры `from`, `to`, `date`, `day`, `cohort_start` — `YYYY-MM-DD` в часовом поясе приложения (`apps.timezone`), оба конца включительно.
- **IDs**: UUID как строка в нижнем регистре. Серверные ID (`apps.id`, `api_keys.id`, `deletion_requests.id` и прочие строки Postgres) — UUIDv7 (`uuidv7()` PG 18). Клиентские `insert_id`, `install_id`, `session_id` — любая версия UUID кроме nil (`00000000-0000-0000-0000-000000000000`); рекомендуется UUIDv4 для `install_id` и UUIDv7 для `insert_id`/`session_id`.
- **person_key**: идентификатор «человека» в отчётах — `u:<user_id>` или `i:<install_id>`; в пути URL кодируется через `encodeURIComponent` (двоеточие допускается, `/` внутри `user_id` → `%2F`).
- **Числа**: счётчики — integer (int64), доли — number 0..1 (не проценты), `*_delta_pct` — number в процентах (−100..+∞), `null` если база сравнения равна 0.

### 1.2. Authentication
- **Ingestion API**: публичный write-ключ приложения `pk_` + 32 символа base62 в заголовке `X-App-Key`, **или** в поле тела `app_key` (для `sendBeacon`, который не умеет заголовки). Ключ в query string не принимается (утекает в логи прокси). Ключ не секретен (вшит в клиент), поэтому защита от злоупотреблений — rate limit по приложению и по `install_id` (§1.6), а не секретность. Кэш ключей в ingest обновляется каждые 15 с (`KEY_CACHE_REFRESH`), значит отзыв ключа вступает в силу не позже чем через 15 с.
- **Read API (`/v1/*`)**: `Authorization: Bearer sk_<40 символов base62>`. В Postgres хранится только `key_prefix` (`sk_` + 8 символов) и `HMAC-SHA256(KEY_HASH_PEPPER, secret)`; секрет показывается один раз при создании. Scopes: `read` (все `GET /v1/*`), `delete` (`POST /v1/apps/{app_id}/deletion-requests`; `GET` статуса удаления требует `read` или `delete`). Ключ, выпущенный с `app_id`, видит только это приложение: чужие `app_id` → `404 app_not_found` (а не 403 — не раскрываем существование приложений), `GET /v1/apps` и `GET /v1/portfolio` возвращают только его.
- **Session cookie** для дашборда: `am_session` (HttpOnly, Secure, SameSite=Lax, Path=/, host-only на `appmetrica.online`). Токен 32 байта, в БД — sha256. Абсолютный TTL 30 дней (`SESSION_TTL=720h`), idle 7 дней (`SESSION_IDLE_TTL=168h`). Дашборд ходит в API через rewrites Next.js `/api/:path*` → `http://appmetrica-api:8080/:path*`, поэтому cookie остаётся same-origin. Cookie принимается и на `/v1/*` (дашборд использует те же отчётные эндпоинты), и на `/admin/v1/*` (только cookie).
- **CSRF**: все `POST`/`PATCH`/`DELETE`, аутентифицированные cookie (включая `/admin/v1/auth/*`), обязаны нести заголовок `X-Requested-With: appmetrica-web`; иначе `403 csrf_check_failed`. Запросы с Bearer-ключом заголовок не требуют.
- **Passkey (WebAuthn)**: единственный способ входа в дашборд. RP ID `appmetrica.online` (dev `localhost`), origin `https://appmetrica.online` (dev `http://localhost:3000`). Ceremony-данные лежат в `webauthn_ceremonies` (TTL 5 мин), клиент передаёт `ceremony_id` обратно в `finish`. Формат `options` — JSON, который возвращает go-webauthn v0.18.2 (`{"publicKey": {…}, "mediation": "conditional"?}`); фронт передаёт `options.publicKey` в `@simplewebauthn/browser` 14.0.0: `startRegistration({ optionsJSON: options.publicKey })` и `startAuthentication({ optionsJSON: options.publicKey, useBrowserAutofill })`.
- **API keys для service-to-service** — это и есть `sk_` (Bearer), отдельного `X-API-Key` нет.

### 1.3. Error response format

Все 4xx / 5xx (кроме `204`) в ingest, Read API и admin возвращают один конверт:

```json
{
  "error": {
    "code": "range_too_large",
    "message": "Диапазон для granularity=hour не может превышать 7 дней (запрошено 14).",
    "request_id": "01JA8Z3K7Q2M9V4T6R1X5N8C0B",
    "details": {
      "field": "to",
      "max_days": 7
    }
  }
}
```

- `code` — стабильный машиночитаемый код из каталога §9.3 (snake_case). Клиент ветвится только по `code` и HTTP-статусу, не по `message`.
- `message` — человекочитаемый текст на русском (ingest — на русском тоже; при необходимости клиент логирует как есть).
- `request_id` — ULID, совпадает с заголовком ответа `X-Request-Id` и полем `request_id` в JSON-логах slog; по нему владелец находит запрос в логах.
- `details` — необязательный объект; для `invalid_request`/`invalid_batch` содержит `field` (JSON-path, например `events[3].timestamp`) и `issue`.
- Ссылки на документацию (`docsUrl` из шаблона) не отдаются: доки закрыты логином; каталог кодов живёт в `content/docs/errors-and-retries.mdx`.

Частичный успех ingest (`200` с `rejected[]`) — **не ошибка**, конверт `error` не используется; формат — `IngestResult` (§2.4.1).

### 1.4. Стандартные HTTP codes

| Code | Когда |
|---|---|
| 200 | Успех с body. На ingest — батч обработан (даже если часть событий отклонена, см. `rejected[]`) |
| 201 | Создано: `createApp`, `createWriteKey`, `createApiKey` |
| 202 | Принято к асинхронному исполнению: `createDeletionRequest`, `deleteApp` |
| 204 | Успех без body: `finishLogin`, `finishSetup`, `logout`, `deletePasskey`, `revokeWriteKey`, `revokeApiKey`, `archiveApp`, `unarchiveApp`, CORS preflight |
| 400 | Невалидный запрос: `invalid_json`, `invalid_batch`, `unsupported_content_encoding`, `invalid_request`, `range_too_large` |
| 401 | Нет или неверная аутентификация: `missing_app_key`, `invalid_app_key`, `unauthenticated` |
| 403 | Аутентифицирован, но нельзя: `app_archived` (ingest), `insufficient_scope`, `csrf_check_failed` |
| 404 | Не найдено или нет доступа по `app_id` ключа: `app_not_found`, `profile_not_found`, `not_found` |
| 409 | Конфликт: `last_passkey`, `app_name_taken`, `confirm_name_mismatch` |
| 410 | Ссылка установки passkey истекла или использована: `setup_token_expired` |
| 413 | Тело ingest > 1 048 576 байт после распаковки: `payload_too_large` |
| 415 | Неподдерживаемый Content-Type: `unsupported_media_type` |
| 422 | WebAuthn-проверка не прошла: `webauthn_verification_failed` |
| 429 | Rate limit: `app_rate_limited` (ingest), `rate_limited` (api); всегда с `Retry-After` |
| 500 | Ошибка сервера (баг): `internal_error` |
| 503 | ClickHouse/Postgres недоступен: `storage_unavailable`; с `Retry-After` |

### 1.5. Pagination

Почти все списки в системе малы (приложения, ключи, passkeys — десятки строк) и отдаются целиком в конверте `{"items": [<все элементы>]}` без пагинации. Единственный потенциально длинный список — сессии в профиле (`GET /v1/apps/{app_id}/profiles/{person_key}`): **cursor-based по времени**.

Request:
```
GET /v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f/profiles/u%3Aa3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90?sessions_limit=100&before=2026-10-01T12:00:00.000Z
```

Response (фрагмент):
```json
{
  "person_key": "u:a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
  "sessions": [
    { "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f", "started_at": "2026-10-01T11:58:10.120Z" }
  ],
  "next_before": "2026-09-14T08:02:41.007Z"
}
```

- Сессии отсортированы по `started_at` DESC; `before` — исключающая граница (строго меньше).
- `next_before` = `started_at` последней сессии страницы, если вернулось ровно `sessions_limit` строк; иначе `null` (страниц больше нет).
- `sessions_limit`: 1..500, default 100.
- `GET /admin/v1/deletion-requests` возвращает последние 200 запросов (по `requested_at` DESC) без курсора; журнал удалений за всю историю проекта ожидается в пределах сотен записей.

### 1.6. Rate limiting

Token bucket в памяти процесса (`golang.org/x/time/rate`), не распределённый (каждый сервис — одна реплика на одном сервере).

| Контур | Ключ | Лимит | Реакция |
|---|---|---|---|
| Ingest, приложение | `app_id` | 2000 событий/с, burst 4000 (`RATE_APP_EVENTS_PER_SEC`, `RATE_APP_BURST`); стоимость запроса = число событий в батче | Весь батч → `429 app_rate_limited`, `Retry-After: <секунды до пополнения, минимум 1>` |
| Ingest, устройство | `app_id` + `install_id` | burst 1000 событий, пополнение 100 событий/мин (`RATE_INSTALL_BURST`, `RATE_INSTALL_PER_MIN`) | Только события этого `install_id` → `rejected[]` с `code=install_rate_limited`, `retryable=true`; остальные события батча принимаются |
| Read API | `api_keys.id` | 10 запросов/с, burst 30 | `429 rate_limited` + `Retry-After` |
| Дашборд | `auth_sessions.token_hash` | 30 запросов/с, burst 60 | `429 rate_limited` + `Retry-After` |
| Admin auth (без сессии) | весь процесс api | `beginLogin`/`finishLogin`/`beginSetup`/`finishSetup`: 20 запросов/мин суммарно | `429 rate_limited` + `Retry-After` |

- **Лимиты по IP не используются**: за мобильным CGNAT тысячи честных пользователей делят один IP. Traefik `rateLimit` в Dokploy не включается.
- Заголовки `X-RateLimit-Limit/Remaining/Reset` **не** отдаются (Deviation от шаблона: in-memory token bucket не имеет фиксированного окна, а ingest-клиентам достаточно `Retry-After`). Единственный контракт — `Retry-After` в секундах (integer) на каждом `429` и `503`.
- Burst 1000 на устройство рассчитан на досыл офлайн-очереди за 7 дней (≈2 события на сессию × до 500 сессий).

### 1.7. Idempotency

Заголовок `Idempotency-Key` **не используется** (Deviation от шаблона): у каждой мутации есть естественный ключ идемпотентности.

| Операция | Механизм | Поведение при повторе |
|---|---|---|
| `POST /v1/events` | `insert_id` каждого события. Дубликат внутри батча → принимается первый, остальные получают warning `duplicate_insert_id_in_batch`. Повтор между батчами → вставляется повторно, но схлопывается `ReplacingMergeTree(received_at)` по ключу `(app_id, event_date, install_id, insert_id)`, а все метрики считаются через `uniqExact(session_id)`/`uniqExact(person_key)` и поэтому не завышаются ещё до слияния кусков | Повторная отправка того же батча после сетевой ошибки или `5xx` безопасна и обязательна (at-least-once + дедуп = effectively-once) |
| `POST /v1/apps/{app_id}/deletion-requests` | Естественный ключ `(app_id, subject_type, subject_value)` среди запросов со статусом `pending` или `running` | Возвращается существующий запрос с тем же `id` и статусом `202`; новый не создаётся. После `done`/`failed` повтор создаёт новый запрос |
| `DELETE /admin/v1/apps/{app_id}` | Тот же естественный ключ (`subject_type=app`, `subject_value=<app_id>`) | Возвращает существующий pending/running запрос |
| `POST /admin/v1/apps` | Уникальность `lower(name)` | Повтор с тем же именем → `409 app_name_taken` |
| `revoke*`, `archiveApp`, `unarchiveApp` | Операции идемпотентны по состоянию | Повторный вызов → `204` без изменений (`revoked_at` не перезаписывается) |
| `createWriteKey`, `createApiKey` | Не идемпотентны | UI блокирует кнопку на время запроса; лишний ключ отзывается вручную |
| `PATCH` (`updateApp`, `updateSettings`, `renamePasskey`) | Last-write-wins | Одинаковое тело → тот же результат |

### 1.8. Test vectors (TDD — обязательно для каждого endpoint)

Каждый endpoint ниже (в §2) имеет таблицу **Test vectors (TDD)**:

| Case | Input (headers+body) | Expected (status + body) | Test file |
|---|---|---|---|

Правила для этого проекта (testing_approach = **tdd**):
- Тест из вектора пишется **до** реализации и падает (Red), затем реализация до Green, затем refactor.
- Go-тесты: стандартный `testing` + `testify/require` v1.12.1; HTTP — через `httptest.NewRecorder` и сгенерированный strict-server; хранилища — testcontainers-go v0.44.0 с образами `clickhouse/clickhouse-server:26.8.25.14` и `postgres:18.6-trixie` (те же, что в проде). Часы — интерфейс `internal/clock.Clock` с `clock.Fixed(...)` в тестах; `received_at` во всех векторах ниже = `2026-10-13T09:15:02.000Z`, если не сказано иное.
- Отчётные векторы используют NDJSON-фикстуры `backend/testdata/fixtures/*.ndjson` и эталоны `*.golden.json`.
- Минимальный набор на endpoint: happy path, нет аутентификации (401), чужой/запрещённый ресурс (404 или 403 — с обоснованием), валидация каждого ограничения, граница rate limit, идемпотентность для мутаций, гонка для мутаций общего ресурса.
- E2E-векторы дашборда — Playwright 1.64.0 с виртуальным WebAuthn-аутентификатором (CDP `WebAuthn.addVirtualAuthenticator`) в `web/e2e/`.

---

## 2. Endpoints (детально)

### 2.1. Auth

Вход в дашборд — только passkey. Эндпоинты живут в `/admin/v1/auth/*` (`x-internal: true`), вызываются фронтом через `/api/admin/v1/auth/*`. Все `POST` требуют `X-Requested-With: appmetrica-web`.

#### GET /admin/v1/auth/state (getAuthState)

Без аутентификации. Ответ `200`:
```json
{ "authenticated": false, "setup_required": true }
```
- `authenticated` — валидна ли cookie `am_session` (не истекла, не отозвана, idle < 7 дней).
- `setup_required` — в таблице `passkeys` нет ни одной строки (первый запуск или все passkey удалены вручную через SQL). Страница `/login` при `setup_required=true` показывает инструкцию «выполните `appmetrica admin passkey-link` на сервере».

#### POST /admin/v1/auth/login/begin (beginLogin)

Без тела. Создаёт `webauthn_ceremonies` (kind=`login`, discoverable login — `BeginDiscoverableMediatedLogin` для conditional UI). Ответ `200`:
```json
{
  "ceremony_id": "0199d6b0-0c11-7a2b-9c3d-4e5f6a7b8c9d",
  "options": {
    "publicKey": {
      "challenge": "q3bYb8nY7v3m0lZ2l6h0Sg",
      "timeout": 300000,
      "rpId": "appmetrica.online",
      "userVerification": "required"
    },
    "mediation": "conditional"
  }
}
```

#### POST /admin/v1/auth/login/finish (finishLogin)

Тело: `{"ceremony_id": "<uuid>", "credential": <AuthenticationResponseJSON от @simplewebauthn/browser>}`. Сервер вызывает `ValidatePasskeyLogin` на сырых байтах credential, обновляет `passkeys.sign_count` и `last_used_at`, создаёт `auth_sessions`, удаляет ceremony. Ответ `204` + `Set-Cookie: am_session=<token>; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=2592000`.

Ошибки: `404 not_found` (ceremony нет или истекла), `422 webauthn_verification_failed` (подпись, challenge, origin, неизвестный credential), `429 rate_limited`.

#### POST /admin/v1/auth/setup/{token}/begin (beginSetup)

`token` — 43 символа base64url (32 байта) из ссылки `https://appmetrica.online/setup/<token>`, напечатанной `appmetrica admin passkey-link`. Сервер ищет `sha256(token)` в `passkey_setup_tokens`: нет → `404 not_found`; `used_at` не null или `expires_at < now()` → `410 setup_token_expired`. Иначе создаёт ceremony kind=`registration` (resident key required, UV required, `excludeCredentials` = уже зарегистрированные), ответ `200` `WebAuthnCeremonyOptions`.

#### POST /admin/v1/auth/setup/{token}/finish (finishSetup)

Тело: `{"ceremony_id": "<uuid>", "credential": <RegistrationResponseJSON>, "name": "MacBook Touch ID"}`. В одной транзакции: создаёт `owner`, если его нет (singleton), вставляет `passkeys`, ставит `passkey_setup_tokens.used_at = now()`, создаёт `auth_sessions`. Ответ `204` + `Set-Cookie`. Ошибки: `410 setup_token_expired` (токен использован другим запросом — гонка двух вкладок, побеждает первая транзакция), `422 webauthn_verification_failed`, `400 invalid_request` (`name` пустое или > 64 символов).

#### POST /admin/v1/auth/logout (logout)

Требует cookie. Ставит `auth_sessions.revoked_at = now()`, ответ `204` + `Set-Cookie: am_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax`. Без cookie → `204` (идемпотентно).

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| state_fresh_install | пустая БД, без cookie | 200 `{"authenticated":false,"setup_required":true}` | `backend/internal/api/admin_auth_test.go::TestAuthState_FreshInstall` |
| state_logged_in | валидная cookie | 200 `{"authenticated":true,"setup_required":false}` | `backend/internal/api/admin_auth_test.go::TestAuthState_LoggedIn` |
| state_idle_expired | cookie, `last_seen_at` = now−8d | 200 `authenticated=false` | `backend/internal/auth/session_test.go::TestSession_IdleExpiry` |
| setup_happy | валидный токен, credential от виртуального аутентификатора, name="MacBook" | 204, Set-Cookie `am_session`, 1 строка `passkeys`, `used_at` заполнен | `backend/internal/api/admin_auth_test.go::TestSetup_Happy` |
| setup_token_reused | тот же токен второй раз | begin → 410 `setup_token_expired` | `backend/internal/api/admin_auth_test.go::TestSetup_TokenReused` |
| setup_token_expired | токен создан now−16m | 410 `setup_token_expired` | `backend/internal/api/admin_auth_test.go::TestSetup_TokenExpired` |
| setup_unknown_token | случайные 32 байта | 404 `not_found` | `backend/internal/api/admin_auth_test.go::TestSetup_UnknownToken` |
| setup_race | два finish параллельно с одним токеном | ровно один 204, второй 410; одна строка `passkeys` | `backend/internal/api/admin_auth_test.go::TestSetup_ConcurrentFinish` |
| setup_name_too_long | name из 65 символов | 400 `invalid_request`, `details.field="name"` | `backend/internal/api/admin_auth_test.go::TestSetup_NameTooLong` |
| login_happy | begin → подписанный credential | 204, Set-Cookie, `sign_count` увеличен | `backend/internal/api/admin_auth_test.go::TestLogin_Happy` |
| login_bad_signature | credential с испорченной подписью | 422 `webauthn_verification_failed`, сессия не создана | `backend/internal/api/admin_auth_test.go::TestLogin_BadSignature` |
| login_ceremony_expired | finish через 6 мин после begin | 404 `not_found` | `backend/internal/api/admin_auth_test.go::TestLogin_CeremonyExpired` |
| login_rate_limit | 21-й вызов beginLogin за минуту | 429 `rate_limited`, `Retry-After` ≥ 1 | `backend/internal/api/admin_auth_test.go::TestLogin_RateLimited` |
| csrf_missing | POST login/begin без `X-Requested-With` | 403 `csrf_check_failed` | `backend/internal/httpx/csrf_test.go::TestCSRF_MissingHeader` |
| logout_twice | logout, затем logout с той же cookie | оба 204; `revoked_at` выставлен один раз | `backend/internal/api/admin_auth_test.go::TestLogout_Idempotent` |
| e2e_login | Playwright: виртуальный аутентификатор, `/setup/<token>` → `/` | портфель отрисован, cookie выставлена | `web/e2e/auth.spec.ts::"setup and login with passkey"` |

---

### 2.2. Users

Пользователь системы один — владелец (`owner`, singleton). Профиля, email и смены роли нет; «управление пользователем» = управление его passkey-ами. Все операции — `/admin/v1/passkeys*`, cookie-only.

#### GET /admin/v1/passkeys (listPasskeys)

Ответ `200`:
```json
{
  "items": [
    {
      "id": "0199d6b2-1111-7aaa-8bbb-0c0d0e0f1011",
      "name": "MacBook Touch ID",
      "created_at": "2026-10-13T09:20:00.000Z",
      "last_used_at": "2026-10-13T10:02:11.512Z",
      "backup_eligible": true,
      "backup_state": true,
      "transports": ["internal", "hybrid"]
    }
  ]
}
```
`credential_id` и `public_key` наружу не отдаются никогда.

#### POST /admin/v1/passkeys/register/begin (beginPasskeyRegistration) и POST /admin/v1/passkeys/register/finish (finishPasskeyRegistration)

Добавление ещё одного passkey (телефон, аппаратный ключ) из залогиненной сессии. `begin` без тела → `WebAuthnCeremonyOptions` (`excludeCredentials` = все текущие). `finish` тело `{"ceremony_id", "credential", "name"}` → `201` `Passkey`. Ошибки как у `finishSetup`, плюс `401 unauthenticated`.

#### PATCH /admin/v1/passkeys/{passkey_id} (renamePasskey)

Тело `{"name": "iPhone"}` (1..64 символа) → `200` `Passkey`. Нет такого → `404 not_found`.

#### DELETE /admin/v1/passkeys/{passkey_id} (deletePasskey)

`204`. Если это последний passkey → `409 last_passkey` (иначе владелец заблокирует себе вход; восстановление только через CLI). Проверка и удаление — в одной транзакции с `SELECT … FOR UPDATE` по `owner`, чтобы два параллельных удаления двух последних ключей не оставили ноль.

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| list_happy | cookie, 2 passkeys | 200, `items` длиной 2, нет полей `credential_id`/`public_key` | `backend/internal/api/admin_passkeys_test.go::TestListPasskeys` |
| list_noauth | без cookie | 401 `unauthenticated` | `backend/internal/api/admin_passkeys_test.go::TestListPasskeys_NoAuth` |
| list_bearer_rejected | `Authorization: Bearer sk_…` (scope read) | 401 `unauthenticated` (admin принимает только cookie) | `backend/internal/api/admin_passkeys_test.go::TestAdmin_RejectsBearer` |
| add_second | begin → finish с name="YubiKey" | 201, в списке 2 элемента | `backend/internal/api/admin_passkeys_test.go::TestAddPasskey` |
| add_duplicate_credential | finish с credential_id, уже существующим | 422 `webauthn_verification_failed` | `backend/internal/api/admin_passkeys_test.go::TestAddPasskey_Duplicate` |
| rename_happy | PATCH `{"name":"iPhone"}` | 200, `name="iPhone"` | `backend/internal/api/admin_passkeys_test.go::TestRenamePasskey` |
| rename_empty | PATCH `{"name":""}` | 400 `invalid_request`, field=`name` | `backend/internal/api/admin_passkeys_test.go::TestRenamePasskey_Empty` |
| delete_last | единственный passkey | 409 `last_passkey`, строка на месте | `backend/internal/api/admin_passkeys_test.go::TestDeletePasskey_Last` |
| delete_race | 2 passkeys, два DELETE параллельно | один 204, другой 409 `last_passkey`; остаётся 1 | `backend/internal/api/admin_passkeys_test.go::TestDeletePasskey_ConcurrentLastTwo` |
| delete_unknown | случайный uuid | 404 `not_found` | `backend/internal/api/admin_passkeys_test.go::TestDeletePasskey_Unknown` |

---

### 2.3. Organizations

Организаций, команд и мультиарендности нет (один владелец, без RBAC). Роль «контейнера данных» играет **приложение** (`apps`), поэтому здесь — CRUD приложений, write-ключей `pk_` и Read API ключей `sk_`. Всё — `/admin/v1/*`, cookie-only, `x-internal: true`. Чтение списка приложений для дашборда и скриптов — публичный `GET /v1/apps` (§2.4.2).

#### POST /admin/v1/apps (createApp)

Request:
```json
{ "name": "Notes iOS", "platform": "ios", "timezone": "Europe/Moscow" }
```
Validation: `name` 1..64 символа после trim, уникальность по `lower(name)`; `platform` ∈ `ios, android, web, macos, windows, linux, bot, server, other`; `timezone` — валидное имя IANA (проверка `time.LoadLocation`), по умолчанию UI подставляет `settings.owner_timezone`.

Response `201` (`CreateAppResult`): приложение + первый write-ключ, который UI сразу показывает на экране «Интеграция» (S-10):
```json
{
  "app": {
    "id": "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
    "name": "Notes iOS",
    "platform": "ios",
    "timezone": "Europe/Moscow",
    "status": "active",
    "created_at": "2026-10-13T09:30:00.000Z",
    "updated_at": "2026-10-13T09:30:00.000Z",
    "archived_at": null
  },
  "write_key": {
    "id": "0199d5f0-1a2c-7d00-9e10-112233445566",
    "app_id": "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
    "key": "pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2",
    "label": "default",
    "created_at": "2026-10-13T09:30:00.000Z",
    "revoked_at": null,
    "last_used_at": null
  }
}
```
Ошибки: `400 invalid_request`, `409 app_name_taken`.

#### PATCH /admin/v1/apps/{app_id} (updateApp)

Тело — хотя бы одно из `name`, `timezone` (`minProperties: 1`). `platform` не меняется (влияет на значения по умолчанию в уже собранных событиях). Смена `timezone` влияет на все отчёты, начиная со следующего запроса, но уже закрытые дни `daily_stats` (D-8 и старше) не пересчитываются — UI предупреждает. `200` `App`.

#### POST /admin/v1/apps/{app_id}/archive (archiveApp), POST /admin/v1/apps/{app_id}/unarchive (unarchiveApp)

`204`. Архив: `status=archived`, `archived_at=now()`; ingest отвечает `403 app_archived` на все его ключи (кэш ≤15 с); данные и отчёты остаются; приложение скрыто из портфеля без `include_archived=true`. Разархивирование возвращает приём.

#### DELETE /admin/v1/apps/{app_id} (deleteApp)

Тело `{"confirm_name": "Notes iOS"}` — должно совпасть с `apps.name` точно (с учётом регистра), иначе `409 confirm_name_mismatch`. Создаёт `deletion_requests` (`subject_type=app`, `subject_value=<app_id>`, `source=dashboard`), сразу архивирует приложение, ответ `202` `DeletionRequest`. Worker (job `deletion`) удаляет данные ClickHouse, затем строку `apps` (каскадом — ключи, `closed_days`, сами `deletion_requests` этого приложения; итог копируется в `job_runs.details`).

#### Write keys: GET/POST /admin/v1/apps/{app_id}/write-keys, POST …/write-keys/{key_id}/revoke

- `listWriteKeys` → `200 {"items": WriteKey[]}` включая отозванные (UI показывает серым).
- `createWriteKey` тело `{"label": "android-2026"}` (0..64) → `201 WriteKey`. Активных ключей у приложения может быть несколько — ротация без поломки старых версий приложений.
- `revokeWriteKey` → `204`; повторно — `204`. После отзыва ingest отвечает `401 invalid_app_key` не позже чем через 15 с.
- `pk_` хранится открыто (`write_keys.key`) — он публичный и нужен для подстановки в сниппеты.

#### API keys: GET/POST /admin/v1/api-keys, POST /admin/v1/api-keys/{key_id}/revoke

- `createApiKey` тело `{"label": "telegram-bot", "scopes": ["read"], "app_id": null}` → `201`:
```json
{
  "api_key": {
    "id": "0199d6c0-2222-7bbb-8ccc-0d0e0f101112",
    "key_prefix": "sk_Q8mT2wXa",
    "label": "telegram-bot",
    "scopes": ["read"],
    "app_id": null,
    "created_at": "2026-10-13T10:00:00.000Z",
    "revoked_at": null,
    "last_used_at": null
  },
  "secret": "sk_Q8mT2wXaR4nP7kL1cV9bZ3hJ6dF0gS5yU2eW8tAq"
}
```
- `secret` возвращается только в этом ответе; в БД — HMAC. `scopes` непустой, без повторов, ⊂ {`read`, `delete`}.
- `revokeApiKey` → `204`, действует немедленно (проверка ключа идёт в Postgres по хэшу на каждый запрос с LRU-кэшем на 10 с).

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| create_happy | `{"name":"Notes iOS","platform":"ios","timezone":"Europe/Moscow"}` | 201, `write_key.key` соответствует `^pk_[A-Za-z0-9]{32}$` | `backend/internal/api/admin_apps_test.go::TestCreateApp` |
| create_dup_name | повтор с `"name":"notes ios"` | 409 `app_name_taken` | `backend/internal/api/admin_apps_test.go::TestCreateApp_DuplicateName` |
| create_bad_tz | `"timezone":"Mars/Olympus"` | 400 `invalid_request`, field=`timezone` | `backend/internal/api/admin_apps_test.go::TestCreateApp_BadTimezone` |
| create_bad_platform | `"platform":"symbian"` | 400 `invalid_request`, field=`platform` | `backend/internal/api/admin_apps_test.go::TestCreateApp_BadPlatform` |
| create_name_empty | `"name":"   "` | 400 `invalid_request`, field=`name` | `backend/internal/api/admin_apps_test.go::TestCreateApp_EmptyName` |
| create_race | два одинаковых createApp параллельно | один 201, второй 409 `app_name_taken` | `backend/internal/api/admin_apps_test.go::TestCreateApp_Concurrent` |
| update_empty | PATCH `{}` | 400 `invalid_request` | `backend/internal/api/admin_apps_test.go::TestUpdateApp_Empty` |
| archive_twice | archive, archive | 204, 204; `archived_at` не меняется | `backend/internal/api/admin_apps_test.go::TestArchiveApp_Idempotent` |
| delete_mismatch | `{"confirm_name":"notes ios"}` | 409 `confirm_name_mismatch` | `backend/internal/api/admin_apps_test.go::TestDeleteApp_ConfirmMismatch` |
| delete_twice | два DELETE с правильным именем | оба 202 с одинаковым `id` | `backend/internal/api/admin_apps_test.go::TestDeleteApp_Idempotent` |
| writekey_revoke_effect | revoke, через 16 с POST /v1/events с этим ключом | ingest 401 `invalid_app_key` | `backend/internal/ingest/keycache_test.go::TestKeyCache_RevocationWithin15s` |
| apikey_secret_once | create, затем list | create содержит `secret`; list не содержит `secret` | `backend/internal/api/admin_apikeys_test.go::TestApiKey_SecretShownOnce` |
| apikey_bad_scope | `"scopes":["admin"]` | 400 `invalid_request`, field=`scopes` | `backend/internal/api/admin_apikeys_test.go::TestApiKey_BadScope` |
| apikey_empty_scopes | `"scopes":[]` | 400 `invalid_request` | `backend/internal/api/admin_apikeys_test.go::TestApiKey_EmptyScopes` |

---

### 2.4. Core Domain Endpoints

#### 2.4.1. POST /v1/events (ingestEvents) — `i.appmetrica.online`

Основной контракт продукта (FR-001…FR-006, FR-024). Клиенты — собственный код приложений на Swift, Kotlin, JS, Go, curl, без SDK.

**Headers**:
- `X-App-Key: pk_…` — либо поле тела `app_key`. Если есть оба и они различаются → `400 invalid_batch` (`details.field="app_key"`).
- `Content-Type: application/json` или `text/plain` (иначе `415 unsupported_media_type`).
- `Content-Encoding: gzip` — необязательно.

**Request** (`EventBatch`):
```json
{
  "sent_at": "2026-10-13T09:15:02.120Z",
  "context": {
    "platform": "ios",
    "os_name": "iOS",
    "os_version": "26.0.1",
    "app_version": "3.4.0",
    "app_build": "341",
    "device_model": "iPhone17,3",
    "device_type": "phone",
    "locale": "ru-RU",
    "environment": "production"
  },
  "events": [
    {
      "insert_id": "0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61",
      "name": "session_start",
      "timestamp": "2026-10-13T09:11:58.004Z",
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f",
      "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90"
    },
    {
      "insert_id": "0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b",
      "name": "session_end",
      "timestamp": "2026-10-13T09:15:01.990Z",
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f",
      "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
      "duration_ms": 183986
    }
  ]
}
```

**Обработка по шагам** (порядок важен для тестов):
1. Ограничение тела 1 048 576 байт **после** распаковки gzip (`http.MaxBytesReader` поверх `gzip.Reader`) → `413 payload_too_large`.
2. Парсинг JSON → `400 invalid_json`.
3. Ключ: заголовок или `app_key`; отсутствует → `401 missing_app_key`; не найден в кэше или отозван → `401 invalid_app_key`; приложение `archived` → `403 app_archived`.
4. Батч-валидация: `sent_at` обязателен и парсится, `events` — массив 1..500 → иначе `400 invalid_batch` с `details.field`.
5. Rate limit приложения на `len(events)` → `429 app_rate_limited`.
6. Для каждого события (индекс сохраняется): слияние контекста (событие перекрывает батч поле за полем, нет `platform` → `apps.platform`, нет `environment` → `production`); валидация полей → `missing_field` / `invalid_field` / `unknown_event_name` / `duration_required`; PII-guard `user_id` → `user_id_looks_like_pii`; коррекция времени `event_time = received_at + (timestamp − sent_at)`; будущее (> received_at + 10 мин) → `event_time = received_at` + warning `timestamp_in_future_clamped`; прошлое (< received_at − 168 ч) → `event_too_old`; дубль `insert_id` в батче → warning `duplicate_insert_id_in_batch`, событие пропускается без rejection; rate limit `install_id` → `install_rate_limited`.
7. Гео: IP — `context.ip` (если задан и не `"none"`), иначе адрес клиента (`X-Forwarded-For` от Traefik, последний доверенный хоп); `"none"` — гео не определяется. Невалидный `context.ip` → warning `geo_ip_invalid`, гео пустое. IP после lookup отбрасывается: не пишется в ClickHouse, логи и `payload_sample`.
8. Вставка принятых событий в `events` через clickhouse-go async insert **с ожиданием подтверждения**; ошибка или таймаут 5 с → весь запрос `503 storage_unavailable` + `Retry-After: 5` (клиент повторит весь батч — безопасно благодаря `insert_id`).
9. Отклонённые события и batch-ошибки известных приложений → `rejected_events` (best effort, ошибка вставки только логируется).
10. Ответ `200 IngestResult`.

**Response 200** (частичный успех):
```json
{
  "accepted": 1,
  "rejected": [
    {
      "index": 1,
      "insert_id": "0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b",
      "code": "duration_required",
      "field": "events[1].duration_ms",
      "message": "Для session_end обязательно поле duration_ms (0..86400000).",
      "retryable": false
    }
  ],
  "warnings": [],
  "server_time": "2026-10-13T09:15:02.318Z"
}
```

**Семантика полей и ограничения** — полностью в YAML §9.1 (`EventBatch`, `EventContext`, `Event`). Ключевое:
- `install_id` — UUID, сгенерированный приложением один раз (iOS — Keychain, переживает переустановку; Android — SharedPreferences + Auto Backup; Web — `localStorage`; боты — стабильный UUID на чат).
- `session_id` — UUID, генерируется при выходе на передний план; один `session_id` у `session_start` и `session_end`.
- `user_id` — непрозрачный ID из бэкенда приложения (1..128). Email и телефоны отклоняются; чисто цифровые строки разрешены (Telegram ID рекомендуется передавать как HMAC-SHA256 на стороне бота).
- `duration_ms` — время на переднем плане, измеренное клиентом; обязателен для `session_end`, игнорируется у `session_start`.
- `context.ip` — только для server-to-server источников (бот передаёт IP пользователя, если знает), иначе не заполнять.

**Retry-политика клиента** (дублируется в `content/docs/errors-and-retries.mdx`):

| Ответ | Действие клиента |
|---|---|
| `200`, `rejected` пуст | Удалить батч из очереди |
| `200`, есть `rejected` с `retryable=false` | Удалить эти события навсегда, залогировать `code` |
| `200`, есть `rejected` с `retryable=true` (`install_rate_limited`) | Оставить эти события в очереди, отправить следующим батчем через ≥ 60 с |
| `400`, `413`, `415` | Не повторять; батч удалить, ошибку залогировать (баг клиента) |
| `401`, `403` | Не повторять батч; очередь не очищать, прекратить отправку до обновления приложения или ключа; события старше 7 дней удалять |
| `429`, `503` | Повторить весь батч через `Retry-After` секунд |
| `500`, сетевая ошибка, таймаут | Повторить весь батч: экспонента 1 с → ×2 → максимум 300 с, jitter ±20 % |
| Любые события старше 7 дней в очереди | Удалить до отправки (сервер всё равно ответит `event_too_old`) |

**curl**:
```bash
curl -sS https://i.appmetrica.online/v1/events \
  -H 'Content-Type: application/json' \
  -H 'X-App-Key: pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2' \
  --data-binary @- <<'JSON'
{
  "sent_at": "2026-10-13T09:15:02.120Z",
  "context": { "platform": "server", "app_version": "1.0.0", "environment": "development" },
  "events": [
    {
      "insert_id": "0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61",
      "name": "session_start",
      "timestamp": "2026-10-13T09:15:01.000Z",
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f"
    }
  ]
}
JSON

# gzip-вариант
gzip -c batch.json | curl -sS https://i.appmetrica.online/v1/events \
  -H 'Content-Type: application/json' -H 'Content-Encoding: gzip' \
  -H 'X-App-Key: pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2' --data-binary @-
```

**Test vectors (TDD)** — все в `backend/internal/ingest/`, `received_at = 2026-10-13T09:15:02.000Z`, ключ `pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2` активен, приложение `active`, platform `ios`:

| # | Case | Input | Expected | Test file |
|---|---|---|---|---|
| I-01 | valid_batch | 2 события (start+end) как в примере, `sent_at=09:15:02.120Z` | 200 `accepted=2`, `rejected=[]`; в CH 2 строки, `platform=ios`, `environment=production` | `handler_test.go::TestIngest_ValidBatch` |
| I-02 | clock_correction | `sent_at=2026-10-13T12:15:02.000Z` (часы устройства +3 ч), `timestamp=2026-10-13T12:14:02.000Z` | 200; `event_time=2026-10-13T09:14:02.000Z`, `client_time=12:14:02` | `clock_test.go::TestCorrectEventTime_SkewedDevice` |
| I-03 | future_clamp | `sent_at=09:15:02.000Z`, `timestamp=2026-10-13T09:40:02.000Z` | 200 `accepted=1`, warning `timestamp_in_future_clamped`, `event_time=09:15:02.000Z` | `clock_test.go::TestCorrectEventTime_FutureClamped` |
| I-04 | future_within_tolerance | `timestamp` = `sent_at` + 9 мин 59 с | 200, без warning, `event_time` = received_at + 9:59 | `clock_test.go::TestCorrectEventTime_FutureWithinTolerance` |
| I-05 | too_old | `sent_at=09:15:02.000Z`, `timestamp=2026-10-06T09:15:01.999Z` (7 дн + 1 мс) | 200 `accepted=0`, rejected `event_too_old`, `retryable=false` | `clock_test.go::TestCorrectEventTime_TooOld` |
| I-06 | exactly_7_days | `timestamp` = `sent_at` − 168 ч ровно | 200 `accepted=1` (граница включительно) | `clock_test.go::TestCorrectEventTime_Boundary7d` |
| I-07 | pii_email | `user_id="ivan@example.com"` | rejected `user_id_looks_like_pii`, field `events[0].user_id` | `pii_test.go::TestPIIGuard_Email` |
| I-08 | pii_e164 | `user_id="+79161234567"` | rejected `user_id_looks_like_pii` | `pii_test.go::TestPIIGuard_E164` |
| I-09 | pii_formatted_phone | `user_id="8 (916) 123-45-67"` | rejected `user_id_looks_like_pii` | `pii_test.go::TestPIIGuard_FormattedPhone` |
| I-10 | digits_allowed | `user_id="123456789012"` | accepted, `user_id` сохранён как есть | `pii_test.go::TestPIIGuard_DigitsAllowed` |
| I-11 | nil_install_id | `install_id="00000000-0000-0000-0000-000000000000"` | rejected `invalid_field`, field `events[0].install_id` | `validate_test.go::TestValidate_NilInstallID` |
| I-12 | dup_insert_id | 2 события с одинаковым `insert_id` | 200 `accepted=1`, warning `duplicate_insert_id_in_batch` (index 1) | `validate_test.go::TestValidate_DuplicateInsertID` |
| I-13 | end_without_duration | `session_end` без `duration_ms` | rejected `duration_required` | `validate_test.go::TestValidate_SessionEndRequiresDuration` |
| I-14 | unknown_name | `name="purchase"` | rejected `unknown_event_name`, `retryable=false` | `validate_test.go::TestValidate_UnknownEventName` |
| I-15 | missing_session_id | событие без `session_id` | rejected `missing_field`, field `events[0].session_id` | `validate_test.go::TestValidate_MissingSessionID` |
| I-16 | gzip_body | тот же батч I-01, `Content-Encoding: gzip` | 200 `accepted=2` | `handler_test.go::TestIngest_Gzip` |
| I-17 | bad_encoding | `Content-Encoding: br` | 400 `unsupported_content_encoding` | `handler_test.go::TestIngest_UnsupportedEncoding` |
| I-18 | text_plain_key_in_body | `Content-Type: text/plain`, без заголовка, `"app_key":"pk_7Hq2…"` в теле | 200 `accepted=2` | `handler_test.go::TestIngest_TextPlainKeyInBody` |
| I-19 | key_mismatch | заголовок и `app_key` разные | 400 `invalid_batch`, `details.field="app_key"` | `handler_test.go::TestIngest_KeyMismatch` |
| I-20 | missing_key | без заголовка и без `app_key` | 401 `missing_app_key` | `handler_test.go::TestIngest_MissingKey` |
| I-21 | revoked_key | ключ с `revoked_at` | 401 `invalid_app_key`; в CH 0 строк | `handler_test.go::TestIngest_RevokedKey` |
| I-22 | archived_app | приложение `archived` | 403 `app_archived` | `handler_test.go::TestIngest_ArchivedApp` |
| I-23 | too_many_events | 501 событие | 400 `invalid_batch`, `details.field="events"` | `handler_test.go::TestIngest_TooManyEvents` |
| I-24 | body_too_large | распакованное тело 1 048 577 байт (gzip 20 КБ) | 413 `payload_too_large` | `handler_test.go::TestIngest_BodyTooLargeAfterGunzip` |
| I-25 | invalid_json | тело `{"sent_at":` | 400 `invalid_json` | `handler_test.go::TestIngest_InvalidJSON` |
| I-26 | missing_sent_at | без `sent_at` | 400 `invalid_batch`, `details.field="sent_at"` | `handler_test.go::TestIngest_MissingSentAt` |
| I-27 | wrong_media_type | `Content-Type: application/x-www-form-urlencoded` | 415 `unsupported_media_type` | `handler_test.go::TestIngest_UnsupportedMediaType` |
| I-28 | install_rate_limit | 1001 событие одного `install_id` в 3 батчах подряд (500+500+1) | 3-й батч: 200, rejected `install_rate_limited`, `retryable=true` | `ratelimit_test.go::TestInstallLimiter_Burst1000` |
| I-29 | app_rate_limit | burst 4000 исчерпан, батч из 10 событий | 429 `app_rate_limited`, `Retry-After` ≥ 1 | `ratelimit_test.go::TestAppLimiter_429` |
| I-30 | context_override | батч `context.app_version=3.4.0`, событие `context.app_version=3.5.0` | в CH `app_version=3.5.0`, остальные поля из батча | `enrich_test.go::TestMergeContext_EventOverridesBatch` |
| I-31 | platform_default | нет `context.platform` | в CH `platform=ios` (из `apps.platform`) | `enrich_test.go::TestMergeContext_PlatformFromApp` |
| I-32 | geo_from_ip | `context.ip="81.2.69.142"` (тестовая mmdb) | `country="GB"`, `city` заполнен; IP нет ни в одной колонке и в `payload_sample` | `geo_test.go::TestGeo_FromContextIP_NotPersisted` |
| I-33 | geo_none | `context.ip="none"` | `country=""`, `city=""` | `geo_test.go::TestGeo_NoneDisables` |
| I-34 | geo_invalid | `context.ip="999.1.1.1"` | accepted, warning `geo_ip_invalid` | `geo_test.go::TestGeo_InvalidIPWarning` |
| I-35 | storage_down | ClickHouse остановлен | 503 `storage_unavailable`, `Retry-After: 5` | `integration_test.go::TestIngest_ClickHouseDown` |
| I-36 | retry_same_batch | I-01 дважды | обе 200; `uniqExact(session_id)=1` в отчёте сессий | `integration_test.go::TestIngest_RetryIsIdempotentForMetrics` |
| I-37 | cors_preflight | `OPTIONS /v1/events` с `Origin: https://site.ru` | 204, `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Headers` содержит `X-App-Key` | `handler_test.go::TestIngest_CORSPreflight` |
| I-38 | rejected_logged | I-07 | строка в `rejected_events` с `code=user_id_looks_like_pii`, `payload_sample` ≤ 4096 байт | `integration_test.go::TestIngest_RejectedEventsLogged` |

---

#### 2.4.2. GET /v1/apps (listApps), GET /v1/apps/{app_id} (getApp)

**Auth**: Bearer `sk_` (scope `read`) или cookie. Query `include_archived` (boolean, default `false`).

**Response 200** (`AppList`):
```json
{
  "items": [
    {
      "id": "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
      "name": "Notes iOS",
      "platform": "ios",
      "timezone": "Europe/Moscow",
      "status": "active",
      "created_at": "2026-10-13T09:30:00.000Z",
      "updated_at": "2026-10-13T09:30:00.000Z",
      "archived_at": null
    }
  ]
}
```
Сортировка по `name` (регистронезависимо). `getApp` для чужого (по `api_keys.app_id`) или несуществующего → `404 app_not_found`.

**curl**:
```bash
curl -sS https://api.appmetrica.online/v1/apps -H "Authorization: Bearer $AM_SK"
curl -sS https://api.appmetrica.online/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| list_happy | sk_ read, 2 active + 1 archived | 200, 2 элемента | `backend/internal/api/v1_apps_test.go::TestListApps` |
| list_archived | `include_archived=true` | 200, 3 элемента | `backend/internal/api/v1_apps_test.go::TestListApps_IncludeArchived` |
| list_scoped_key | sk_ с `app_id=A` | 200, только A | `backend/internal/api/v1_apps_test.go::TestListApps_ScopedKey` |
| noauth | без заголовка | 401 `unauthenticated` | `backend/internal/api/v1_apps_test.go::TestListApps_NoAuth` |
| revoked_key | sk_ отозван | 401 `unauthenticated` | `backend/internal/api/v1_apps_test.go::TestListApps_RevokedKey` |
| delete_only_scope | sk_ только `delete` | 403 `insufficient_scope` | `backend/internal/api/v1_apps_test.go::TestListApps_DeleteScopeOnly` |
| get_foreign | sk_ с `app_id=A`, запрос B | 404 `app_not_found` | `backend/internal/api/v1_apps_test.go::TestGetApp_ForeignApp` |
| rate_limit | 31-й запрос за 1 с одним ключом | 429 `rate_limited`, `Retry-After` | `backend/internal/api/ratelimit_test.go::TestApiKeyLimiter_Burst30` |

---

#### 2.4.3. GET /v1/portfolio (getPortfolio)

Главный экран S-03 (FR-012). Одна строка на приложение.

**Query**: `date` (YYYY-MM-DD, default — вчера в `settings.owner_timezone`; для каждого приложения дата интерпретируется в его `timezone`), `environment` (default `production`), `unit` (default `persons`), `include_archived` (default `false`).

**Response 200**:
```json
{
  "date": "2026-10-12",
  "environment": "production",
  "unit": "persons",
  "rows": [
    {
      "app": { "id": "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f", "name": "Notes iOS", "platform": "ios", "status": "active" },
      "active_today": 9120,
      "active_date": 12480,
      "active_same_day_prev_week": 11988,
      "active_delta_pct": 4.1,
      "new_date": 1204,
      "sessions_date": 31022,
      "retention_d1": 0.38,
      "retention_d7": 0.14,
      "sparkline": [
        { "day": "2026-09-13", "active": 11502 },
        { "day": "2026-09-14", "active": 11730 }
      ]
    }
  ]
}
```
- `active_today` — live за текущие сутки приложения; `active_date` — за `date`; `active_same_day_prev_week` — за `date − 7`; `active_delta_pct = (active_date − active_same_day_prev_week) / active_same_day_prev_week × 100`, `null` при нулевой базе.
- `retention_d1` — доля когорты `date − 1`, вернувшейся в `date`; `retention_d7` — доля когорты `date − 7`, вернувшейся в `date`; `null`, если когорта пуста.
- `sparkline` — ровно 30 точек `date − 29 … date`: закрытые дни из `daily_stats`, последние 8 открытых — live.
- Сортировка строк — по `active_date` DESC; UI может пересортировать.

**curl**:
```bash
curl -sS "https://api.appmetrica.online/v1/portfolio?date=2026-10-12&environment=production" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)** — фикстура `backend/testdata/fixtures/portfolio_two_apps.ndjson` + `portfolio_two_apps.golden.json`:

| Case | Input | Expected | Test file |
|---|---|---|---|
| golden | `date=2026-10-12` | ответ совпадает с golden (2 строки, 30 точек sparkline) | `backend/internal/reports/portfolio_test.go::TestPortfolio_Golden` |
| delta_null | у приложения 0 активных за `date−7` | `active_delta_pct=null` | `backend/internal/reports/portfolio_test.go::TestPortfolio_DeltaNullOnZeroBase` |
| tz_boundary | событие в 23:30 UTC 11.10 у приложения Europe/Moscow | считается в `2026-10-12` | `backend/internal/reports/portfolio_test.go::TestPortfolio_AppTimezoneBoundary` |
| closed_plus_live | `daily_stats` за D-8 и старше + сырые события за последние 8 дней | sparkline склеен без дыр и дублей | `backend/internal/reports/portfolio_test.go::TestPortfolio_SparklineClosedAndLive` |
| unit_installs | `unit=installs`, человек с 2 устройствами | `active_date` на 1 больше, чем при `persons` | `backend/internal/reports/portfolio_test.go::TestPortfolio_UnitInstalls` |
| dev_excluded | события `environment=development` | не учитываются при `environment=production` | `backend/internal/reports/portfolio_test.go::TestPortfolio_EnvironmentFilter` |
| bad_date | `date=2026-13-01` | 400 `invalid_request`, field=`date` | `backend/internal/api/v1_portfolio_test.go::TestPortfolio_BadDate` |

---

#### 2.4.4. Audience: summary, timeseries, rolling-active, breakdown

Экран S-05 (FR-013). Общие параметры: `from`, `to` (обязательны, YYYY-MM-DD в TZ приложения, включительно, `to − from` ≤ 365 дней → иначе `400 range_too_large`), `environment`, `unit`, фильтры `platform`, `app_version`, `os_name`, `country` (повторяемые; OR внутри одного параметра, AND между разными).

Определения (совпадают с AppMetrica — нужно для сверки NFR-010):
- `active` — `uniqExact(person_key)` по любым событиям периода.
- `new` — люди, чей `first_seen` (минимум по всем их install и `user_id`) попадает в период.
- `sessions` — `uniqExact(session_id)` по `session_start`.
- `avg_duration_ms`, `median_duration_ms` — по `session_end.duration_ms`; сессии без `session_end` в длительность не входят; `null`, если таких сессий нет.

**GET /v1/apps/{app_id}/audience/summary (getAudienceSummary)** — дополнительно `compare` = `previous_period` (по умолчанию) | `none`. Предыдущий период — той же длины, сразу перед `from`.
```json
{
  "from": "2026-10-06",
  "to": "2026-10-12",
  "active": 41230,
  "new": 8120,
  "sessions": 201554,
  "avg_duration_ms": 214500.5,
  "median_duration_ms": 98000,
  "compare": {
    "from": "2026-09-29",
    "to": "2026-10-05",
    "active": 40102,
    "new": 7998,
    "sessions": 195020,
    "avg_duration_ms": 209870.1,
    "median_duration_ms": 96500,
    "delta_pct": { "active": 2.81, "new": 1.53, "sessions": 3.35, "avg_duration_ms": 2.21, "median_duration_ms": 1.55 }
  }
}
```

**GET /v1/apps/{app_id}/audience/timeseries (getAudienceTimeseries)** — `granularity` = `hour` | `day` | `week` | `month` (default `day`; для `hour` диапазон ≤ 7 дней), `metric` = `active` | `new` | `sessions` | `avg_duration_ms` (default `active`), `compare` как выше.
```json
{
  "metric": "active",
  "granularity": "day",
  "points": [
    { "ts": "2026-10-06T00:00:00+03:00", "value": 12011 },
    { "ts": "2026-10-07T00:00:00+03:00", "value": 12480 }
  ],
  "compare_points": [
    { "ts": "2026-09-29T00:00:00+03:00", "value": 11870 },
    { "ts": "2026-09-30T00:00:00+03:00", "value": 11902 }
  ]
}
```
Точки без данных возвращаются со значением `0` (для `avg_duration_ms` — `null`), ряд всегда полный от `from` до `to`. Недели — ISO (понедельник), месяцы — календарные; неполная первая/последняя корзина помечается не отдельным полем, а обрезается по `from`/`to`.

**GET /v1/apps/{app_id}/audience/rolling-active (getRollingActive)** — для каждого дня периода: `dau` (день), `wau` (7 дней, заканчивая днём), `mau` (30 дней, заканчивая днём).
```json
{ "points": [ { "day": "2026-10-12", "dau": 12480, "wau": 41230, "mau": 98411 } ] }
```

**GET /v1/apps/{app_id}/audience/breakdown (getAudienceBreakdown)** — `dimension` (обязателен) ∈ `platform, os_name, os_version, app_version, device_model, device_type, country, city, locale`; `metric` = `active` | `new` | `sessions` (default `active`); `limit` 1..200 (default 50). `share = count / total`; `total` — значение метрики без разбивки (для `active` сумма `count` может быть больше `total`: человек с двумя версиями приложения за период попадает в обе строки). Пустое значение измерения отдаётся как `""` (UI: «Не определено»).
```json
{
  "dimension": "app_version",
  "metric": "active",
  "total": 41230,
  "rows": [
    { "value": "3.4.0", "count": 30110, "share": 0.7303 },
    { "value": "3.3.2", "count": 11920, "share": 0.2891 }
  ]
}
```

**curl**:
```bash
APP=0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f
curl -sS "https://api.appmetrica.online/v1/apps/$APP/audience/summary?from=2026-10-06&to=2026-10-12&compare=previous_period" -H "Authorization: Bearer $AM_SK"
curl -sS "https://api.appmetrica.online/v1/apps/$APP/audience/timeseries?from=2026-10-06&to=2026-10-12&granularity=day&metric=active&platform=ios&app_version=3.4.0" -H "Authorization: Bearer $AM_SK"
curl -sS "https://api.appmetrica.online/v1/apps/$APP/audience/rolling-active?from=2026-09-13&to=2026-10-12" -H "Authorization: Bearer $AM_SK"
curl -sS "https://api.appmetrica.online/v1/apps/$APP/audience/breakdown?from=2026-10-06&to=2026-10-12&dimension=country&metric=new&limit=20" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)** — фикстуры `audience_identity.ndjson` (3 install, 2 из них склеены одним `user_id`; один анонимный день до логина), `audience_durations.ndjson`:

| Case | Input | Expected | Test file |
|---|---|---|---|
| active_persons | `from=to=2026-10-10`, unit persons | `active=2` (склеенные install считаются одним человеком, включая анонимный день до логина) | `backend/internal/reports/audience_test.go::TestSummary_ActivePersonsRetroMerge` |
| active_installs | то же, `unit=installs` | `active=3` | `backend/internal/reports/audience_test.go::TestSummary_ActiveInstalls` |
| first_owner | install I1: user A (день 1), user B (день 3); анонимное событие I1 день 5 | анонимное событие → `u:A` | `backend/internal/reports/audience_test.go::TestPersonKey_FirstOwnerWins` |
| new_users | человек впервые 2026-10-08 на I1, 2026-10-10 на I2 | `new` за 10.10 = 0, за 08.10 = 1 | `backend/internal/reports/audience_test.go::TestSummary_NewUsesPersonFirstSeen` |
| sessions_dedup | 2 одинаковых `session_start` (повтор батча) | `sessions=1` | `backend/internal/reports/audience_test.go::TestSummary_SessionsUniq` |
| duration_without_end | 3 сессии, 2 с `session_end` (60000, 120000) | `avg_duration_ms=90000`, `median_duration_ms=90000` | `backend/internal/reports/audience_test.go::TestSummary_DurationIgnoresOpenSessions` |
| compare | `compare=previous_period` | `compare.from/to` — предыдущие 7 дней, `delta_pct` посчитан | `backend/internal/reports/audience_test.go::TestSummary_ComparePreviousPeriod` |
| filters_or_and | `platform=ios&platform=android&app_version=3.4.0` | (ios OR android) AND 3.4.0 | `backend/internal/reports/filters_test.go::TestFilters_OrWithinAndAcross` |
| timeseries_fill | день без событий в середине периода | точка со значением 0 | `backend/internal/reports/audience_test.go::TestTimeseries_FillsGaps` |
| hour_range | `granularity=hour`, 8 дней | 400 `range_too_large` | `backend/internal/api/v1_audience_test.go::TestTimeseries_HourRangeTooLarge` |
| range_366 | `from=2025-10-12&to=2026-10-12` | 400 `range_too_large` | `backend/internal/api/v1_audience_test.go::TestSummary_RangeTooLarge` |
| from_after_to | `from=2026-10-12&to=2026-10-06` | 400 `invalid_request`, field=`from` | `backend/internal/api/v1_audience_test.go::TestSummary_FromAfterTo` |
| rolling | фикстура 35 дней | `wau`/`mau` совпадают с golden | `backend/internal/reports/audience_test.go::TestRollingActive_Golden` |
| breakdown_empty_value | события без `country` | строка `{"value":""}` | `backend/internal/reports/audience_test.go::TestBreakdown_EmptyValue` |
| bad_dimension | `dimension=ip` | 400 `invalid_request`, field=`dimension` | `backend/internal/api/v1_audience_test.go::TestBreakdown_BadDimension` |

---

#### 2.4.5. GET /v1/apps/{app_id}/retention (getRetention)

Экран S-06 (FR-014). Классический retention N-го дня (как AppMetrica): когорта — люди с `first_seen` в день (или ISO-неделю) `cohort_start`; значение периода N — доля когорты, у которой было ≥ 1 событие ровно в день (неделю) `cohort_start + N`.

**Query**: `from`, `to` — диапазон дат начала когорт; `cohort_granularity` = `day` (default) | `week`; `periods` — для `day` 1..90 (default 30), для `week` 1..52 (default 12); `environment`, `unit`, фильтры (фильтры применяются к событию, определяющему когорту, и к событиям возврата одинаково).

**Response 200**:
```json
{
  "cohort_granularity": "day",
  "periods": 30,
  "cohorts": [
    {
      "cohort_start": "2026-10-10",
      "size": 1204,
      "values": [1.0, 0.38, 0.29, null]
    }
  ],
  "average": [1.0, 0.37, 0.28, 0.24]
}
```
- `values` длиной `periods + 1` (D0..D`periods`), D0 всегда `1.0` при `size > 0`; периоды, которые ещё не наступили (в TZ приложения), — `null`. Пример выше сокращён до 4 элементов для читаемости; реальный ответ при `periods=30` содержит 31 элемент.
- `average[N]` — средневзвешенное по размеру когорт среди когорт, у которых период N уже наступил; `null`, если таких когорт нет.
- Когорта с `size = 0` возвращается со `values`, заполненными `null`.

**curl**:
```bash
curl -sS "https://api.appmetrica.online/v1/apps/$APP/retention?from=2026-09-12&to=2026-10-12&cohort_granularity=day&periods=30" -H "Authorization: Bearer $AM_SK"
curl -sS "https://api.appmetrica.online/v1/apps/$APP/retention?from=2026-07-06&to=2026-10-12&cohort_granularity=week&periods=12&unit=installs" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)** — фикстура `retention_basic.ndjson` + `retention_basic.golden.json` (10 человек в когорте 2026-10-01; 4 вернулись на D1, 2 на D7, 1 на D1 и D7):

| Case | Input | Expected | Test file |
|---|---|---|---|
| golden_day | `from=to=2026-10-01`, `periods=7`, «сегодня» = 2026-10-13 | `size=10`, `values=[1.0,0.4,…,0.2]` как в golden | `backend/internal/reports/retention_test.go::TestRetention_DayGolden` |
| classic_not_rolling | человек вернулся только на D3 | учитывается в D3, не в D1/D2 | `backend/internal/reports/retention_test.go::TestRetention_ClassicNotRolling` |
| future_null | когорта 2026-10-10, «сегодня» 2026-10-13 | D4 и далее `null` | `backend/internal/reports/retention_test.go::TestRetention_FuturePeriodsNull` |
| week | `cohort_granularity=week` | недели ISO с понедельника, golden | `backend/internal/reports/retention_test.go::TestRetention_WeekGolden` |
| retro_merge | анонимный install в когорте, через 2 дня логин | человек остаётся в одной когорте (по первому дню), D2 засчитан | `backend/internal/reports/retention_test.go::TestRetention_RetroMergeKeepsCohort` |
| unit_installs | `unit=installs` | когорты по install, golden `retention_installs.golden.json` | `backend/internal/reports/retention_test.go::TestRetention_UnitInstalls` |
| periods_limit | `cohort_granularity=day&periods=91` | 400 `invalid_request`, field=`periods` | `backend/internal/api/v1_retention_test.go::TestRetention_PeriodsTooLarge` |
| average_weighted | 2 когорты 10 и 30 человек, D1 0.5 и 0.1 | `average[1]=0.2` | `backend/internal/reports/retention_test.go::TestRetention_WeightedAverage` |

---

#### 2.4.6. GET /v1/apps/{app_id}/realtime (getRealtime)

Экран S-07 (FR-015). Дашборд опрашивает каждые 5 с (TanStack Query `refetchInterval: 5000`), WebSocket/SSE не используются (§4).

**Query**: `environment` (default `production`).

**Response 200**:
```json
{
  "active_now": 37,
  "per_minute": [
    { "minute": "2026-10-13T12:46:00+03:00", "sessions_started": 4 },
    { "minute": "2026-10-13T12:47:00+03:00", "sessions_started": 6 }
  ],
  "recent_events": [
    {
      "event_time": "2026-10-13T09:15:01.990Z",
      "received_at": "2026-10-13T09:15:02.318Z",
      "name": "session_end",
      "environment": "production",
      "platform": "ios",
      "app_version": "3.4.0",
      "os_name": "iOS",
      "os_version": "26.0.1",
      "device_model": "iPhone17,3",
      "country": "RU",
      "city": "Moscow",
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "person_key": "u:a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90"
    }
  ],
  "generated_at": "2026-10-13T09:15:05.002Z"
}
```
- `active_now` — люди с `session_start` за последние 30 минут, у сессии которых нет `session_end`.
- `per_minute` — ровно 30 точек, последние 30 полных и текущая неполная минута не включается; минуты в TZ приложения.
- `recent_events` — до 50 последних по `received_at` DESC (для проверки интеграции важнее время приёма, чем время события).
- Событие видно здесь ≤ 10 с после ответа ingest `200` (NFR-004).

**curl**:
```bash
curl -sS "https://api.appmetrica.online/v1/apps/$APP/realtime?environment=development" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| active_now | 3 сессии стартовали за 30 мин, 1 из них завершена; 1 сессия стартовала 31 мин назад | `active_now=2` | `backend/internal/reports/realtime_test.go::TestRealtime_ActiveNow` |
| per_minute_len | любые данные | `len(per_minute)=30`, пустые минуты = 0 | `backend/internal/reports/realtime_test.go::TestRealtime_PerMinuteAlways30` |
| recent_limit | 80 событий | 50 элементов, отсортированы по `received_at` DESC | `backend/internal/reports/realtime_test.go::TestRealtime_RecentEventsLimit50` |
| env_filter | события dev и prod, `environment=development` | только dev | `backend/internal/reports/realtime_test.go::TestRealtime_EnvironmentFilter` |
| latency | ingest 200 → poll realtime | событие в `recent_events` ≤ 10 с | `backend/internal/ingest/integration_test.go::TestIngestToRealtime_Within10s` |

---

#### 2.4.7. Profiles: GET /v1/apps/{app_id}/profiles (searchProfiles), GET /v1/apps/{app_id}/profiles/{person_key} (getProfile)

Экраны S-08, S-09 (FR-016).

**searchProfiles**: `q` (обязателен, 1..128) — точное совпадение с `install_id` (если `q` — UUID) **или** с `user_id`. Поиск по подстроке не поддерживается (bloom-фильтры ClickHouse ускоряют только точное совпадение). Ответ до 20 элементов:
```json
{
  "items": [
    {
      "person_key": "u:a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
      "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
      "matched_by": "install_id",
      "installs_count": 2,
      "first_seen": "2026-09-01T07:12:00.000Z",
      "last_seen": "2026-10-13T09:15:01.990Z"
    }
  ]
}
```
Если `q` — install, привязанный к пользователю, возвращается профиль пользователя (`u:`), `matched_by=install_id`.

**getProfile**: `person_key` (path, `u:<user_id>` или `i:<install_id>`), `sessions_limit` (1..500, default 100), `before` (date-time). Ответ `Profile`:
```json
{
  "person_key": "u:a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
  "user_id": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
  "first_seen": "2026-09-01T07:12:00.000Z",
  "last_seen": "2026-10-13T09:15:01.990Z",
  "installs": [
    {
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "platform": "ios",
      "os_name": "iOS",
      "os_version": "26.0.1",
      "app_version": "3.4.0",
      "device_model": "iPhone17,3",
      "device_type": "phone",
      "locale": "ru-RU",
      "country": "RU",
      "city": "Moscow",
      "first_seen": "2026-09-01T07:12:00.000Z",
      "last_seen": "2026-10-13T09:15:01.990Z"
    }
  ],
  "sessions": [
    {
      "session_id": "0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f",
      "install_id": "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
      "started_at": "2026-10-13T09:11:58.004Z",
      "ended_at": "2026-10-13T09:15:01.990Z",
      "duration_ms": 183986,
      "app_version": "3.4.0",
      "platform": "ios",
      "environment": "production",
      "country": "RU",
      "city": "Moscow"
    }
  ],
  "next_before": null
}
```
- Атрибуты install (`os_version`, `app_version` и прочие) — из **последнего** события install.
- Профиль ищется по всем `environment` (отладочные сессии тоже видны, поле `environment` в сессии).
- Нет ни одного события → `404 profile_not_found`.

**curl**:
```bash
curl -sS "https://api.appmetrica.online/v1/apps/$APP/profiles?q=6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21" -H "Authorization: Bearer $AM_SK"
curl -sS "https://api.appmetrica.online/v1/apps/$APP/profiles/u%3Aa3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90?sessions_limit=50" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| search_install_linked | `q=<install I1>`, I1 привязан к user A | 1 элемент `u:A`, `matched_by=install_id` | `backend/internal/reports/profiles_test.go::TestSearch_InstallResolvesToUser` |
| search_user | `q=A` | `u:A`, `installs_count=2` | `backend/internal/reports/profiles_test.go::TestSearch_ByUserID` |
| search_anonymous | `q=<install I3>` без логина | `i:I3`, `user_id=null` | `backend/internal/reports/profiles_test.go::TestSearch_AnonymousInstall` |
| search_none | `q=unknown` | 200 `items=[]` | `backend/internal/reports/profiles_test.go::TestSearch_NoMatch` |
| profile_pagination | 150 сессий, `sessions_limit=100` | 100 сессий, `next_before` = `started_at` 100-й; вторая страница — 50 и `next_before=null` | `backend/internal/reports/profiles_test.go::TestProfile_CursorPagination` |
| open_session | сессия без `session_end` | `ended_at=null`, `duration_ms=null` | `backend/internal/reports/profiles_test.go::TestProfile_OpenSession` |
| not_found | `i:<случайный uuid>` | 404 `profile_not_found` | `backend/internal/api/v1_profiles_test.go::TestProfile_NotFound` |
| bad_person_key | `x:123` | 400 `invalid_request`, field=`person_key` | `backend/internal/api/v1_profiles_test.go::TestProfile_BadPersonKey` |
| slash_in_user_id | user_id `org/42`, путь `u%3Aorg%2F42` | 200, профиль найден | `backend/internal/api/v1_profiles_test.go::TestProfile_EncodedSlash` |

---

#### 2.4.8. GET /v1/apps/{app_id}/integration/health (getIntegrationHealth)

Экран S-10 (FR-017). Дашборд опрашивает каждые 3 с, пока `first_event_at = null` (онбординг), затем каждые 60 с.

**Query**: `hours` 1..168 (default 24).

**Response 200**:
```json
{
  "hours": 24,
  "first_event_at": "2026-10-13T09:31:12.511Z",
  "accepted_total": 18230,
  "rejected_total": 41,
  "hourly": [
    { "hour": "2026-10-13T11:00:00+03:00", "accepted": 812, "rejected": 2 }
  ],
  "reasons": [
    {
      "code": "duration_required",
      "count": 39,
      "last_seen": "2026-10-13T09:10:44.020Z",
      "example_message": "Для session_end обязательно поле duration_ms (0..86400000).",
      "example_payload": "{\"insert_id\":\"0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b\",\"name\":\"session_end\",\"timestamp\":\"2026-10-13T09:10:43.990Z\"}"
    }
  ],
  "versions": [
    { "app_version": "3.4.0", "accepted": 18001, "rejected": 2, "last_seen": "2026-10-13T09:15:01.990Z" },
    { "app_version": "3.3.2", "accepted": 229, "rejected": 39, "last_seen": "2026-10-13T08:59:12.000Z" }
  ]
}
```
- `first_event_at` — минимум `received_at` по всем событиям приложения за всю историю (`null` → UI показывает «Ожидаем первое событие»).
- `accepted` считаются по `events.received_at`, `rejected` — по `rejected_events.received_at` (хранятся 7 дней, поэтому `hours` ≤ 168).
- `reasons[].code` — значение `RejectionCode` или batch-кода (`invalid_json`, `invalid_batch`, `unsupported_content_encoding`, `payload_too_large`, `unsupported_media_type`, `app_rate_limited`, `app_archived`); `example_payload` — последний `payload_sample` (≤ 4096 байт, без IP).
- `hourly` — ровно `hours` точек.

**curl**:
```bash
curl -sS "https://api.appmetrica.online/v1/apps/$APP/integration/health?hours=24" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| waiting | новое приложение без событий | `first_event_at=null`, `accepted_total=0`, `hourly` из 24 нулей | `backend/internal/reports/integration_health_test.go::TestHealth_NoEventsYet` |
| first_event | одно принятое событие | `first_event_at` = его `received_at` | `backend/internal/reports/integration_health_test.go::TestHealth_FirstEvent` |
| reasons_grouped | 3 × `event_too_old`, 1 × `invalid_field` | 2 строки, отсортированы по `count` DESC | `backend/internal/reports/integration_health_test.go::TestHealth_ReasonsGrouped` |
| versions | события 3.3.2 и 3.4.0 | 2 строки с accepted/rejected | `backend/internal/reports/integration_health_test.go::TestHealth_ByVersion` |
| hours_limit | `hours=169` | 400 `invalid_request`, field=`hours` | `backend/internal/api/v1_integration_test.go::TestHealth_HoursLimit` |

---

#### 2.4.9. Deletion requests (FR-019)

**POST /v1/apps/{app_id}/deletion-requests (createDeletionRequest)** — scope `delete` (или cookie + `X-Requested-With`). Тело:
```json
{ "subject_type": "user", "subject_value": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90" }
```
- `subject_type` ∈ `install` | `user` (`app` — только через `DELETE /admin/v1/apps/{app_id}`).
- Для `install` `subject_value` обязан быть UUID; для `user` — 1..128 символов.
- Ответ `202` `DeletionRequest` со `status=pending`. Повтор с тем же субъектом, пока предыдущий `pending`/`running`, → `202` с существующим запросом.
- Исполнение — worker job `deletion` (каждую минуту): `user` → удаляются события с этим `user_id` и все события install, у которых первый владелец — этот user, а также строки `install_first_seen`, `user_first_seen`, `identity_links`, `rejected_events` по ним; `install` → всё по этому install. `daily_stats` не трогаются (агрегаты без персональных данных). Срок исполнения ≤ 24 ч (обычно минуты), до 5 попыток, затем `failed` с `error`.
- Запрос на субъекта без данных → всё равно `202` и затем `done` (не раскрываем, есть ли данные).

**GET /v1/apps/{app_id}/deletion-requests/{request_id} (getDeletionRequest)** — scope `read` или `delete`.
```json
{
  "id": "0199d6d0-3333-7ccc-8ddd-0e0f10111213",
  "app_id": "0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
  "subject_type": "user",
  "subject_value": "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
  "status": "done",
  "source": "api",
  "requested_at": "2026-10-13T10:00:00.000Z",
  "started_at": "2026-10-13T10:00:31.120Z",
  "completed_at": "2026-10-13T10:00:44.871Z",
  "error": null
}
```

**curl**:
```bash
curl -sS -X POST "https://api.appmetrica.online/v1/apps/$APP/deletion-requests" \
  -H "Authorization: Bearer $AM_SK_DELETE" -H 'Content-Type: application/json' \
  -d '{"subject_type":"user","subject_value":"a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90"}'
curl -sS "https://api.appmetrica.online/v1/apps/$APP/deletion-requests/0199d6d0-3333-7ccc-8ddd-0e0f10111213" -H "Authorization: Bearer $AM_SK"
```

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| create_user | sk_ delete, subject user A | 202, `status=pending`, строка в `deletion_requests` с `source=api`, `api_key_id` заполнен | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_User` |
| idempotent | повтор пока pending | 202, тот же `id`, строк в БД 1 | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_IdempotentWhilePending` |
| after_done | повтор после `done` | 202, новый `id` | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_NewAfterDone` |
| read_scope_only | sk_ только `read` | 403 `insufficient_scope` | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_RequiresDeleteScope` |
| install_not_uuid | `subject_type=install`, `subject_value="abc"` | 400 `invalid_request`, field=`subject_value` | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_InstallNotUUID` |
| subject_app_rejected | `subject_type=app` | 400 `invalid_request`, field=`subject_type` | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_AppSubjectRejected` |
| cookie_without_csrf | cookie без `X-Requested-With` | 403 `csrf_check_failed` | `backend/internal/api/v1_deletion_test.go::TestCreateDeletion_CookieRequiresCSRF` |
| executor_user | worker исполняет user A (2 install, один анонимный период до логина) | в `events` 0 строк A и его install; `status=done`; отчёты не содержат `u:A` | `backend/internal/deletion/executor_test.go::TestExecute_UserRemovesLinkedInstalls` |
| executor_keeps_daily_stats | закрытый день с A | `daily_stats` не изменён | `backend/internal/deletion/executor_test.go::TestExecute_KeepsDailyStats` |
| executor_retry | CH недоступен на 1-й попытке | `attempts=2`, затем `done` | `backend/internal/deletion/executor_test.go::TestExecute_RetriesOnStorageError` |

---

#### 2.4.10. Settings & system (admin, FR-020, FR-021, FR-025)

- **GET /admin/v1/settings (getSettings)** → `Settings`:
```json
{
  "owner_timezone": "Europe/Moscow",
  "telegram_chat_id": "123456789",
  "digest_enabled": true,
  "digest_time": "09:00",
  "alerts_enabled": true,
  "updated_at": "2026-10-13T10:05:00.000Z"
}
```
- **PATCH /admin/v1/settings (updateSettings)** — любое подмножество полей (`minProperties: 1`); `digest_time` — `HH:MM` 24-часовой; `owner_timezone` — IANA; `telegram_chat_id` — строка `^-?[0-9]{1,20}$` или `null` (отключает и дайджест, и алерты). `200 Settings`.
- **POST /admin/v1/settings/telegram/test (sendTelegramTest)** — отправляет «Тестовое сообщение appmetrica.online» в `telegram_chat_id`; `200 {"sent": true, "telegram_message_id": 4211}`; нет `chat_id` → `400 invalid_request`; Telegram вернул ошибку → `200 {"sent": false, "error": "Bad Request: chat not found"}` (ошибка внешнего сервиса не превращается в 5xx).
- **GET /admin/v1/system/status (getSystemStatus)**:
```json
{
  "clickhouse": { "ok": true, "disk_free_pct": 81.4, "events_rows": 1820331921 },
  "postgres": { "ok": true },
  "backup": { "last_backup_at": "2026-10-13T02:00:41.000Z", "last_backup_name": "increment-2026-10-13T02-00-00" },
  "alerts": [
    { "key": "disk_free", "firing": false, "first_fired_at": null, "last_notified_at": null, "last_value": "81.4" }
  ],
  "jobs": [
    { "id": "0199d6e0-4444-7ddd-8eee-0f1011121314", "job": "system_check", "started_at": "2026-10-13T10:05:00.000Z", "finished_at": "2026-10-13T10:05:01.412Z", "status": "ok", "details": { "checks": 5 } }
  ]
}
```
  `jobs` — последний запуск каждого из 5 jobs (`close_day`, `digest`, `system_check`, `deletion`, `session_cleanup`).
- **GET /admin/v1/deletion-requests (listDeletionRequests)** — `app_id` (необязательный фильтр), до 200 последних.

**Test vectors (TDD)**:

| Case | Input | Expected | Test file |
|---|---|---|---|
| patch_partial | `{"digest_enabled":true}` | 200, остальные поля без изменений | `backend/internal/api/admin_settings_test.go::TestUpdateSettings_Partial` |
| patch_bad_time | `{"digest_time":"25:00"}` | 400 `invalid_request`, field=`digest_time` | `backend/internal/api/admin_settings_test.go::TestUpdateSettings_BadTime` |
| patch_bad_chat | `{"telegram_chat_id":"@channel"}` | 400 `invalid_request`, field=`telegram_chat_id` | `backend/internal/api/admin_settings_test.go::TestUpdateSettings_BadChatID` |
| telegram_test_ok | httptest-сервер вместо api.telegram.org отвечает ok | 200 `sent=true` | `backend/internal/telegram/client_test.go::TestSendMessage_OK` |
| telegram_test_fail | Telegram отвечает 400 | 200 `sent=false`, `error` заполнен | `backend/internal/api/admin_settings_test.go::TestTelegramTest_ExternalErrorIs200` |
| status_ch_down | ClickHouse недоступен | 200, `clickhouse.ok=false`, `disk_free_pct=null` | `backend/internal/api/admin_system_test.go::TestSystemStatus_ClickHouseDown` |

---

### 2.5. Billing

> **N/A для этого проекта.** Причина: внутренний инструмент одного владельца без тарифов, оплаты и лимитов по подписке; Stripe и любые платёжные провайдеры не интегрируются. (Относится к: out of scope в 00-summary.md / 01-prd.md.)

Единственное «денежное» ограничение — rate limit (§1.6) как защита от спама публичным `pk_`, а не биллинг.

---

## 3. Webhooks (исходящие, если есть)

Исходящих webhooks клиентам нет: потребитель данных — сам владелец через дашборд и Read API. Входящих webhooks тоже нет (ни Stripe, ни Clerk). Ниже — что система отправляет наружу вместо webhooks, чтобы агент-реализатор не искал несуществующий механизм подписки.

### 3.1. Format

> **N/A для этого проекта.** Причина: подписок на события и URL-ов клиентов нет. Исходящие вызовы worker-а — фиксированные интеграции: Telegram Bot API `POST https://api.telegram.org/bot<TELEGRAM_BOT_TOKEN>/sendMessage` с JSON `{"chat_id","text","parse_mode":"HTML","disable_web_page_preview":true}` и ping `GET HEALTHCHECKS_PING_URL` / `GET HEALTHCHECKS_PING_URL/fail` (healthchecks.io).

### 3.2. Signature verification

> **N/A для этого проекта.** Причина: нет входящих webhooks. Деплой-вебхуков Dokploy в системе тоже нет: CI вызывает Dokploy API `POST /api/application.deploy` с `x-api-key` через Tailscale (см. [03-architecture.md](./03-architecture.md)), а не публичный webhook URL.

### 3.3. Retry policy

> **N/A для webhooks.** Для исходящих вызовов worker-а: Telegram — до 3 попыток с учётом `retry_after` из ответа 429 (иначе 1 с → 2 с → 4 с), после неудачи пишется `job_runs.status=failed` и алерт не повторяется до следующего цикла `system_check`; healthchecks.io — одна попытка с таймаутом 10 с (пропуск одного ping не страшен: grace period проверки 10 мин).

### 3.4. Event types

| Event | Когда |
|---|---|
| Telegram: «Портфель за вчера» | job `digest` ежедневно в `settings.digest_time` (TZ `settings.owner_timezone`), если `digest_enabled` и задан `telegram_chat_id` |
| Telegram: «🔴 Алерт: <key>» | job `system_check`: переход `alert_states.firing` false → true, повтор раз в 6 ч, пока firing |
| Telegram: «🟢 Восстановлено: <key>» | job `system_check`: переход firing true → false |
| healthchecks.io ping | после каждого успешного цикла `system_check` (раз в 5 мин); `/fail` при исключении в цикле |
| Telegram (Dokploy, вне кода) | App Deploy, App Build Error, Database Backup, Docker Cleanup, Dokploy Restart — настраивается в Dokploy Notifications |

---

## 4. Real-time (если применимо)

### 4.1. Protocol

HTTP polling, без WebSocket/SSE/Supabase Realtime/Ably. Обоснование: один зритель (владелец), обновление раз в 5 с полностью покрывает NFR-004 (событие видно ≤ 10 с), а long-lived соединения усложнили бы Traefik, rolling update Swarm (разрыв соединений при деплое) и тесты. Интервалы опроса задаются в TanStack Query `refetchInterval` и останавливаются при скрытой вкладке (`refetchIntervalInBackground: false`).

### 4.2. Channels

| Channel | Подписка | События |
|---|---|---|
| `GET /v1/apps/{app_id}/realtime` | S-07, `refetchInterval: 5000`, кнопка «Пауза» отключает | `active_now`, `per_minute`, `recent_events` |
| `GET /v1/apps/{app_id}/integration/health` | S-10, 3000 мс до первого события, затем 60000 мс | `first_event_at`, accepted/rejected |
| `GET /v1/portfolio` | S-03, `refetchInterval: 60000` (обновляет `active_today`) | строки портфеля |
| `GET /admin/v1/system/status` | S-12 вкладка «Система», 30000 мс | статус CH/PG/бэкапа/алертов |

### 4.3. Authorization

Как у обычных запросов: cookie `am_session` на каждом poll-запросе; при `401 unauthenticated` фронт останавливает опрос и редиректит на `/login?next=<текущий путь>`. Rate limit дашборда 30 req/s с запасом покрывает 4 одновременных poll-канала.

---

## 5. GraphQL / tRPC (если используется)

> **N/A для этого проекта.** Причина: REST + OpenAPI 3.0.3 выбран потому, что основные клиенты — приложения без SDK на Swift/Kotlin/JS/Go/curl, которым нужен самый простой HTTP-контракт; типобезопасность фронта обеспечивает orval-клиент, сгенерированный из того же `api.yaml`, а бэкенда — oapi-codegen strict-server. tRPC невозможен (бэкенд на Go), GraphQL избыточен для фиксированного набора отчётов.

---

## 6. OpenAPI spec

- **Источник истины** — вручную поддерживаемые файлы (design-first, не генерация из кода): `contracts/openapi/ingest.yaml` и `contracts/openapi/api.yaml`, OpenAPI **3.0.3** (Deviation от шаблонного 3.1: oapi-codegen v2.8.0 поддерживает 3.1 только «initial», orval 8.42.0 и fumadocs-openapi 12.4.2 одинаково работают с 3.0.3). Полные тексты — §9.1.
- **Кодогенерация из спеки**: Go-серверы — oapi-codegen v2.8.0 (`backend/internal/gen/ingestapi`, `backend/internal/gen/api`, сгенерированный код коммитится); TS-клиент дашборда — orval 8.42.0 (`web/src/lib/api/gen/`: хуки TanStack Query, Zod-схемы, MSW-хендлеры; рукописный `web/src/lib/api/fetcher.ts`). Команда `make gen` перегенерирует всё; CI падает, если `git diff --exit-code` после `make gen` не пуст.
- **Документация** — Fumadocs (S-13, `/docs`, только после логина): страницы API reference генерируются `fumadocs-openapi` из тех же двух файлов; операции с `x-internal: true` (весь `/admin/v1`) исключаются из генерации. Playground отключён (запросы с реальным `sk_` из браузера не нужны; примеры curl есть на каждой странице).
- **Raw-спеки**: в репозитории; наружу (`/openapi.json`) не публикуются — доки закрыты.
- **Валидация спеки в CI**: `npx @redocly/cli@2 lint contracts/openapi/*.yaml` (версию пиннить при bootstrap, тикет E0) + проверка, что каждый `operationId` из якоря присутствует.

---

## 7. Testing контрактов

- **Test vectors §2** — исполняемые AC, пишутся до кода (TDD): Go-тесты через `httptest` против сгенерированного strict-server и реальных ClickHouse/Postgres в testcontainers.
- **Contract drift**: `make gen && git diff --exit-code` в CI (`.github/workflows/ci.yml`) — сервер и клиент не могут разойтись со спекой молча.
- **Валидация ответов в тестах**: для Read/Admin API Go-тесты прогоняют каждый ответ через `kin-openapi` (`openapi3filter.ValidateResponse`, транзитивная зависимость `oapi-codegen/nethttp-middleware` v1.2.0) — ответ, не соответствующий схеме, валит тест.
- **Валидация запросов в рантайме**: `api` подключает `nethttp-middleware` (OapiRequestValidator) — некорректные параметры отклоняются `400 invalid_request` до хендлера. Ingest **не** использует валидатор kin-openapi на горячем пути (стоимость рефлексии): ручная валидация в `internal/ingest/validate.go`, покрытая векторами I-01…I-38.
- **Фронт**: Vitest 5.0.4 + MSW-хендлеры, сгенерированные orval из той же спеки (тот же контракт, что у бэкенда); Playwright 1.64.0 E2E против поднятого локально стека (`make dev`).
- **Ручная проверка**: файл `contracts/http/requests.http` (формат VS Code REST Client / JetBrains HTTP Client) с примерами всех публичных запросов — вместо Postman/Bruno-коллекции.
- **Нагрузочная проверка ingest** (NFR-001): `k6` сценарий `backend/perf/ingest-load.js` — 500 rps батчами по 20 событий 10 мин, порог `http_req_duration p(99) < 500`.

---

## 8. SDK / Client generation

- **SDK для приложений не выпускаются — это продуктовое решение** («без SDK в сборке»). Вместо SDK — рецепты в документации (`content/docs/recipes/*.mdx`): iOS (Swift, URLSession + офлайн-очередь в файле + Keychain для `install_id`), Android (Kotlin, OkHttp + WorkManager + SharedPreferences), Web (fetch + `sendBeacon` на `visibilitychange`), боты и бэкенды (сессионизация на сервере по 30-минутному тайм-ауту), Go и curl. Каждый рецепт — копипастный код ≤ 150 строк, который владелец вставляет в своё приложение.
- **Внутренние сгенерированные клиенты**:
  - TypeScript (дашборд): orval 8.42.0, конфиг §9.2, `client: 'react-query'`, `httpClient: 'fetch'`, свой mutator `web/src/lib/api/fetcher.ts` (`credentials: 'include'`, заголовок `X-Requested-With: appmetrica-web` на мутациях, базовый URL `/api`), плюс `zod` и `mock: msw`.
  - Go (бэкенд): oapi-codegen v2.8.0 — `std-http-server` + `strict-server` + `models` для `api.yaml`; для `ingest.yaml` генерируются **только модели** (`models: true`), хендлер `POST /v1/events` пишется вручную ради производительности горячего пути (один проход по JSON, без промежуточных strict-обёрток и рефлексивной валидации).
  - WebAuthn-эндпоинты (`finishLogin`, `finishSetup`, `finishPasskeyRegistration`) в strict-server получают тело как `json.RawMessage` (поле `credential` описано как свободный объект), а сервер вызывает `webauthn.CreateCredential` / `ValidatePasskeyLogin` на сырых байтах — потому что `Finish*`-методы go-webauthn сами читают `*http.Request`, который strict-server уже разобрал.

---

## 9. Готовые контракты (implementation-ready)

### 9.1. Full OpenAPI 3.1 YAML

Используется OpenAPI **3.0.3** (обоснование — §6). Два файла.

#### contracts/openapi/ingest.yaml

```yaml
openapi: 3.0.3
info:
  title: appmetrica.online Ingestion API
  version: 1.0.0
  description: |
    Приём системных событий приложений без SDK.
    Аутентификация: публичный write-ключ приложения `pk_…` в заголовке `X-App-Key`
    или в поле тела `app_key` (для navigator.sendBeacon). Ключ в query string не принимается.
    Ответ 200 отдаётся только после подтверждённой записи в хранилище.
servers:
  - url: https://i.appmetrica.online
    description: Production
  - url: http://localhost:8081
    description: Local development
tags:
  - name: Ingestion
    description: Приём батчей событий
  - name: Health
    description: Liveness и readiness
paths:
  /v1/events:
    options:
      tags: [Ingestion]
      operationId: preflightEvents
      summary: CORS preflight для браузерных клиентов
      security: []
      responses:
        '204':
          description: Preflight принят
          headers:
            Access-Control-Allow-Origin: { $ref: '#/components/headers/AccessControlAllowOrigin' }
            Access-Control-Allow-Methods:
              schema: { type: string, example: 'POST, OPTIONS' }
            Access-Control-Allow-Headers:
              schema: { type: string, example: 'Content-Type, Content-Encoding, X-App-Key' }
            Access-Control-Max-Age:
              schema: { type: integer, example: 86400 }
    post:
      tags: [Ingestion]
      operationId: ingestEvents
      summary: Принять батч событий (1..500)
      description: |
        Валидирует, обогащает (гео, контекст по умолчанию) и записывает события.
        Невалидные события не валят батч: они перечисляются в `rejected`.
        Батч целиком отклоняется только при ошибках уровня батча (400/401/403/413/415/429/503).
      security:
        - appKeyHeader: []
        - {}
      parameters:
        - $ref: '#/components/parameters/ContentEncoding'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/EventBatch' }
            examples:
              ios_session: { $ref: '#/components/examples/IosSessionBatch' }
          text/plain:
            schema: { $ref: '#/components/schemas/EventBatch' }
            examples:
              web_beacon: { $ref: '#/components/examples/WebBeaconBatch' }
      responses:
        '200':
          description: Батч обработан (возможен частичный успех)
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
            Access-Control-Allow-Origin: { $ref: '#/components/headers/AccessControlAllowOrigin' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/IngestResult' }
              examples:
                all_accepted: { $ref: '#/components/examples/ResultAllAccepted' }
                partial: { $ref: '#/components/examples/ResultPartial' }
        '400':
          description: Невалидный батч
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                invalid_json: { $ref: '#/components/examples/ErrInvalidJson' }
                invalid_batch: { $ref: '#/components/examples/ErrInvalidBatch' }
                unsupported_content_encoding: { $ref: '#/components/examples/ErrUnsupportedEncoding' }
        '401':
          description: Ключ отсутствует, не найден или отозван
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                missing_app_key: { $ref: '#/components/examples/ErrMissingKey' }
                invalid_app_key: { $ref: '#/components/examples/ErrInvalidKey' }
        '403':
          description: Приложение в архиве
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                app_archived: { $ref: '#/components/examples/ErrAppArchived' }
        '413':
          description: Тело больше 1 048 576 байт после распаковки
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                payload_too_large: { $ref: '#/components/examples/ErrPayloadTooLarge' }
        '415':
          description: Content-Type не application/json и не text/plain
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                unsupported_media_type: { $ref: '#/components/examples/ErrUnsupportedMediaType' }
        '429':
          description: Превышен лимит событий приложения
          headers:
            Retry-After: { $ref: '#/components/headers/RetryAfter' }
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                app_rate_limited: { $ref: '#/components/examples/ErrAppRateLimited' }
        '500':
          description: Внутренняя ошибка
          headers:
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                internal_error: { $ref: '#/components/examples/ErrInternal' }
        '503':
          description: Хранилище недоступно, повторить весь батч
          headers:
            Retry-After: { $ref: '#/components/headers/RetryAfter' }
            X-Request-Id: { $ref: '#/components/headers/XRequestId' }
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Error' }
              examples:
                storage_unavailable: { $ref: '#/components/examples/ErrStorageUnavailable' }
  /health:
    get:
      tags: [Health]
      operationId: ingestHealth
      summary: Liveness (без проверки зависимостей)
      security: []
      responses:
        '200':
          description: Процесс жив
          content:
            application/json:
              schema: { $ref: '#/components/schemas/HealthStatus' }
  /ready:
    get:
      tags: [Health]
      operationId: ingestReady
      summary: Readiness (ClickHouse, Postgres, кэш ключей загружен)
      security: []
      responses:
        '200':
          description: Готов принимать события
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ReadyStatus' }
        '503':
          description: Не готов
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ReadyStatus' }
components:
  securitySchemes:
    appKeyHeader:
      type: apiKey
      in: header
      name: X-App-Key
      description: Публичный write-ключ приложения `pk_` + 32 символа base62. Альтернатива — поле тела `app_key`.
  parameters:
    ContentEncoding:
      name: Content-Encoding
      in: header
      required: false
      schema: { type: string, enum: [gzip, identity] }
  headers:
    RetryAfter:
      description: Через сколько секунд повторить запрос
      schema: { type: integer, minimum: 1 }
    XRequestId:
      description: ULID запроса, совпадает с error.request_id и логами
      schema: { type: string }
    AccessControlAllowOrigin:
      schema: { type: string, enum: ['*'] }
  schemas:
    Platform:
      type: string
      enum: [ios, android, web, macos, windows, linux, bot, server, other]
    DeviceType:
      type: string
      enum: [phone, tablet, desktop, tv, watch, server, unknown]
    Environment:
      type: string
      enum: [production, development]
    EventName:
      type: string
      enum: [session_start, session_end]
    EventContext:
      type: object
      additionalProperties: false
      description: |
        Контекст устройства. Поле события `context` перекрывает поле батча `context` поле за полем.
        Отсутствующий `platform` берётся из настроек приложения, отсутствующий `environment` = production.
      properties:
        platform: { $ref: '#/components/schemas/Platform' }
        os_name: { type: string, maxLength: 32, example: iOS }
        os_version: { type: string, maxLength: 32, example: 26.0.1 }
        app_version: { type: string, maxLength: 64, example: 3.4.0 }
        app_build: { type: string, maxLength: 64, example: '341' }
        device_model: { type: string, maxLength: 64, example: 'iPhone17,3' }
        device_type: { $ref: '#/components/schemas/DeviceType' }
        locale: { type: string, maxLength: 35, description: BCP 47, example: ru-RU }
        environment: { $ref: '#/components/schemas/Environment' }
        ip:
          type: string
          maxLength: 45
          description: |
            Только для server-to-server источников: IPv4/IPv6 пользователя для гео или строка "none",
            отключающая гео. Используется для lookup и не сохраняется.
          example: none
    Event:
      type: object
      additionalProperties: false
      required: [insert_id, name, timestamp, install_id, session_id]
      properties:
        insert_id:
          type: string
          format: uuid
          description: Уникальный ID события для дедупликации повторов
        name: { $ref: '#/components/schemas/EventName' }
        timestamp:
          type: string
          format: date-time
          description: Время события по часам устройства, RFC 3339 с offset
        install_id:
          type: string
          format: uuid
          description: Стабильный ID установки, не nil-UUID
        session_id:
          type: string
          format: uuid
          description: Общий для session_start и session_end одной сессии
        user_id:
          type: string
          minLength: 1
          maxLength: 128
          description: Непрозрачный ID пользователя из бэкенда приложения. Email и телефоны отклоняются.
        duration_ms:
          type: integer
          format: int64
          minimum: 0
          maximum: 86400000
          description: Обязателен для session_end; время на переднем плане
        context: { $ref: '#/components/schemas/EventContext' }
    EventBatch:
      type: object
      additionalProperties: false
      required: [sent_at, events]
      properties:
        app_key:
          type: string
          pattern: '^pk_[A-Za-z0-9]{32}$'
          description: Альтернатива заголовку X-App-Key (для sendBeacon)
        sent_at:
          type: string
          format: date-time
          description: Момент отправки HTTP-запроса по часам устройства; используется для коррекции времени
        context: { $ref: '#/components/schemas/EventContext' }
        events:
          type: array
          minItems: 1
          maxItems: 500
          items: { $ref: '#/components/schemas/Event' }
    RejectionCode:
      type: string
      enum:
        - missing_field
        - invalid_field
        - unknown_event_name
        - event_too_old
        - user_id_looks_like_pii
        - duration_required
        - install_rate_limited
    WarningCode:
      type: string
      enum:
        - timestamp_in_future_clamped
        - duplicate_insert_id_in_batch
        - geo_ip_invalid
    Rejection:
      type: object
      required: [index, insert_id, code, field, message, retryable]
      properties:
        index: { type: integer, minimum: 0 }
        insert_id: { type: string, nullable: true }
        code: { $ref: '#/components/schemas/RejectionCode' }
        field: { type: string, example: 'events[1].duration_ms' }
        message: { type: string }
        retryable:
          type: boolean
          description: true только для install_rate_limited
    IngestWarning:
      type: object
      required: [index, code, message]
      properties:
        index: { type: integer, minimum: 0 }
        code: { $ref: '#/components/schemas/WarningCode' }
        message: { type: string }
    IngestResult:
      type: object
      required: [accepted, rejected, warnings, server_time]
      properties:
        accepted: { type: integer, minimum: 0 }
        rejected:
          type: array
          items: { $ref: '#/components/schemas/Rejection' }
        warnings:
          type: array
          items: { $ref: '#/components/schemas/IngestWarning' }
        server_time: { type: string, format: date-time }
    Error:
      type: object
      required: [error]
      properties:
        error:
          type: object
          required: [code, message, request_id]
          properties:
            code: { type: string }
            message: { type: string }
            request_id: { type: string }
            details:
              type: object
              additionalProperties: true
    HealthStatus:
      type: object
      required: [status]
      properties:
        status: { type: string, enum: [ok] }
    ReadyStatus:
      type: object
      required: [status, checks]
      properties:
        status: { type: string, enum: [ready, not_ready] }
        checks:
          type: object
          additionalProperties: { type: string, enum: [ok, fail] }
          example: { clickhouse: ok, postgres: ok, key_cache: ok }
  examples:
    IosSessionBatch:
      summary: iOS — начало и конец сессии залогиненного пользователя
      value:
        sent_at: '2026-10-13T09:15:02.120Z'
        context:
          platform: ios
          os_name: iOS
          os_version: 26.0.1
          app_version: 3.4.0
          app_build: '341'
          device_model: 'iPhone17,3'
          device_type: phone
          locale: ru-RU
          environment: production
        events:
          - insert_id: 0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61
            name: session_start
            timestamp: '2026-10-13T09:11:58.004Z'
            install_id: 6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21
            session_id: 0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f
            user_id: a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90
          - insert_id: 0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b
            name: session_end
            timestamp: '2026-10-13T09:15:01.990Z'
            install_id: 6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21
            session_id: 0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f
            user_id: a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90
            duration_ms: 183986
    WebBeaconBatch:
      summary: Web — sendBeacon (text/plain), ключ в теле
      value:
        app_key: pk_7Hq2x9AbLm3Nc8Rt5Vw1Yz4Kd6Fg0Jp2
        sent_at: '2026-10-13T09:20:00.500+03:00'
        context:
          platform: web
          os_name: macOS
          device_type: desktop
          locale: ru-RU
        events:
          - insert_id: 0199d6b9-0a0b-7c0d-8e0f-101112131415
            name: session_end
            timestamp: '2026-10-13T09:20:00.480+03:00'
            install_id: 9b2e4c61-0f3a-4d8b-9e71-5c2a6d8f0b13
            session_id: 0199d6b0-1f2e-7d3c-8b4a-596877665544
            duration_ms: 412300
    ResultAllAccepted:
      value:
        accepted: 2
        rejected: []
        warnings: []
        server_time: '2026-10-13T09:15:02.318Z'
    ResultPartial:
      summary: По одному примеру на каждый RejectionCode и WarningCode
      value:
        accepted: 1
        rejected:
          - { index: 1, insert_id: null, code: missing_field, field: 'events[1].insert_id', message: 'Поле insert_id обязательно.', retryable: false }
          - { index: 2, insert_id: 0199d6c1-0000-7000-8000-000000000002, code: invalid_field, field: 'events[2].install_id', message: 'install_id должен быть UUID и не nil-UUID.', retryable: false }
          - { index: 3, insert_id: 0199d6c1-0000-7000-8000-000000000003, code: unknown_event_name, field: 'events[3].name', message: 'Допустимые имена: session_start, session_end.', retryable: false }
          - { index: 4, insert_id: 0199d6c1-0000-7000-8000-000000000004, code: event_too_old, field: 'events[4].timestamp', message: 'Событие старше 7 дней после коррекции часов.', retryable: false }
          - { index: 5, insert_id: 0199d6c1-0000-7000-8000-000000000005, code: user_id_looks_like_pii, field: 'events[5].user_id', message: 'user_id похож на email или телефон. Передавайте непрозрачный ID.', retryable: false }
          - { index: 6, insert_id: 0199d6c1-0000-7000-8000-000000000006, code: duration_required, field: 'events[6].duration_ms', message: 'Для session_end обязательно поле duration_ms (0..86400000).', retryable: false }
          - { index: 7, insert_id: 0199d6c1-0000-7000-8000-000000000007, code: install_rate_limited, field: 'events[7].install_id', message: 'Слишком много событий с этого устройства. Повторите позже.', retryable: true }
        warnings:
          - { index: 0, code: timestamp_in_future_clamped, message: 'Время события в будущем больше чем на 10 минут; заменено временем приёма.' }
          - { index: 8, code: duplicate_insert_id_in_batch, message: 'insert_id повторяется в батче; событие пропущено.' }
          - { index: 0, code: geo_ip_invalid, message: 'context.ip не является IP-адресом; гео не определено.' }
        server_time: '2026-10-13T09:15:02.318Z'
    ErrInvalidJson:
      value: { error: { code: invalid_json, message: 'Тело запроса не является корректным JSON.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0B } }
    ErrInvalidBatch:
      value: { error: { code: invalid_batch, message: 'events: от 1 до 500 событий.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0C, details: { field: events, issue: max_items } } }
    ErrUnsupportedEncoding:
      value: { error: { code: unsupported_content_encoding, message: 'Поддерживается только Content-Encoding: gzip.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0D } }
    ErrMissingKey:
      value: { error: { code: missing_app_key, message: 'Передайте ключ в заголовке X-App-Key или в поле app_key.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0E } }
    ErrInvalidKey:
      value: { error: { code: invalid_app_key, message: 'Ключ не найден или отозван.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0F } }
    ErrAppArchived:
      value: { error: { code: app_archived, message: 'Приложение в архиве, приём событий остановлен.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0G } }
    ErrPayloadTooLarge:
      value: { error: { code: payload_too_large, message: 'Тело после распаковки больше 1 048 576 байт. Разбейте батч.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0H } }
    ErrUnsupportedMediaType:
      value: { error: { code: unsupported_media_type, message: 'Content-Type должен быть application/json или text/plain.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0J } }
    ErrAppRateLimited:
      value: { error: { code: app_rate_limited, message: 'Превышен лимит 2000 событий/с для приложения.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0K } }
    ErrInternal:
      value: { error: { code: internal_error, message: 'Внутренняя ошибка. Повторите батч позже.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0M } }
    ErrStorageUnavailable:
      value: { error: { code: storage_unavailable, message: 'Хранилище временно недоступно. Повторите весь батч.', request_id: 01JA8Z3K7Q2M9V4T6R1X5N8C0N } }
```

#### contracts/openapi/api.yaml

```yaml
openapi: 3.0.3
info:
  title: appmetrica.online API
  version: 1.0.0
  description: |
    Read API (`/v1`, Bearer `sk_…` со scopes read/delete, либо cookie дашборда)
    и внутренний Admin API (`/admin/v1`, только cookie `am_session`, все операции x-internal).
    Даты from/to/date — YYYY-MM-DD в часовом поясе приложения, включительно.
servers:
  - url: https://api.appmetrica.online
    description: Production — Read API для скриптов (Bearer sk_)
  - url: https://appmetrica.online/api
    description: Production — через rewrites дашборда (cookie am_session)
  - url: http://localhost:8080
    description: Local development
security:
  - bearerApiKey: []
  - sessionCookie: []
tags:
  - name: Apps
  - name: Portfolio
  - name: Audience
  - name: Retention
  - name: Realtime
  - name: Profiles
  - name: Integration
  - name: Deletion
  - name: Health
  - name: Admin Auth
    x-internal: true
  - name: Admin Passkeys
    x-internal: true
  - name: Admin Apps
    x-internal: true
  - name: Admin Keys
    x-internal: true
  - name: Admin Settings
    x-internal: true
  - name: Admin System
    x-internal: true
paths:
  /health:
    get:
      tags: [Health]
      operationId: apiHealth
      security: []
      responses:
        '200':
          description: Процесс жив
          content:
            application/json:
              schema: { $ref: '#/components/schemas/HealthStatus' }
  /ready:
    get:
      tags: [Health]
      operationId: apiReady
      security: []
      responses:
        '200':
          description: Готов
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ReadyStatus' }
        '503':
          description: Не готов
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ReadyStatus' }
  /v1/apps:
    get:
      tags: [Apps]
      operationId: listApps
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/IncludeArchived'
      responses:
        '200':
          description: Список приложений
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AppList' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /v1/apps/{app_id}:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Apps]
      operationId: getApp
      x-required-scope: read
      responses:
        '200':
          description: Приложение
          content:
            application/json:
              schema: { $ref: '#/components/schemas/App' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /v1/portfolio:
    get:
      tags: [Portfolio]
      operationId: getPortfolio
      x-required-scope: read
      parameters:
        - name: date
          in: query
          required: false
          description: По умолчанию — вчера в settings.owner_timezone
          schema: { type: string, format: date }
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/IncludeArchived'
      responses:
        '200':
          description: Портфель
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Portfolio' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/audience/summary:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Audience]
      operationId: getAudienceSummary
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/From'
        - $ref: '#/components/parameters/To'
        - $ref: '#/components/parameters/Compare'
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/FilterPlatform'
        - $ref: '#/components/parameters/FilterAppVersion'
        - $ref: '#/components/parameters/FilterOsName'
        - $ref: '#/components/parameters/FilterCountry'
      responses:
        '200':
          description: KPI аудитории
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AudienceSummary' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/audience/timeseries:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Audience]
      operationId: getAudienceTimeseries
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/From'
        - $ref: '#/components/parameters/To'
        - name: granularity
          in: query
          schema: { $ref: '#/components/schemas/Granularity' }
        - name: metric
          in: query
          schema: { $ref: '#/components/schemas/TimeSeriesMetric' }
        - $ref: '#/components/parameters/Compare'
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/FilterPlatform'
        - $ref: '#/components/parameters/FilterAppVersion'
        - $ref: '#/components/parameters/FilterOsName'
        - $ref: '#/components/parameters/FilterCountry'
      responses:
        '200':
          description: Временной ряд
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TimeSeries' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/audience/rolling-active:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Audience]
      operationId: getRollingActive
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/From'
        - $ref: '#/components/parameters/To'
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/FilterPlatform'
        - $ref: '#/components/parameters/FilterAppVersion'
        - $ref: '#/components/parameters/FilterOsName'
        - $ref: '#/components/parameters/FilterCountry'
      responses:
        '200':
          description: DAU/WAU/MAU по дням
          content:
            application/json:
              schema: { $ref: '#/components/schemas/RollingActive' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/audience/breakdown:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Audience]
      operationId: getAudienceBreakdown
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/From'
        - $ref: '#/components/parameters/To'
        - name: dimension
          in: query
          required: true
          schema: { $ref: '#/components/schemas/BreakdownDimension' }
        - name: metric
          in: query
          schema: { $ref: '#/components/schemas/BreakdownMetric' }
        - name: limit
          in: query
          schema: { type: integer, minimum: 1, maximum: 200, default: 50 }
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/FilterPlatform'
        - $ref: '#/components/parameters/FilterAppVersion'
        - $ref: '#/components/parameters/FilterOsName'
        - $ref: '#/components/parameters/FilterCountry'
      responses:
        '200':
          description: Разбивка по измерению
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Breakdown' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/retention:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Retention]
      operationId: getRetention
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/From'
        - $ref: '#/components/parameters/To'
        - name: cohort_granularity
          in: query
          schema: { $ref: '#/components/schemas/CohortGranularity' }
        - name: periods
          in: query
          description: day 1..90 (default 30), week 1..52 (default 12)
          schema: { type: integer, minimum: 1, maximum: 90 }
        - $ref: '#/components/parameters/EnvironmentQ'
        - $ref: '#/components/parameters/UnitQ'
        - $ref: '#/components/parameters/FilterPlatform'
        - $ref: '#/components/parameters/FilterAppVersion'
        - $ref: '#/components/parameters/FilterOsName'
        - $ref: '#/components/parameters/FilterCountry'
      responses:
        '200':
          description: Матрица retention
          content:
            application/json:
              schema: { $ref: '#/components/schemas/RetentionMatrix' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/realtime:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Realtime]
      operationId: getRealtime
      x-required-scope: read
      parameters:
        - $ref: '#/components/parameters/EnvironmentQ'
      responses:
        '200':
          description: Realtime-срез
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Realtime' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/profiles:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Profiles]
      operationId: searchProfiles
      x-required-scope: read
      parameters:
        - name: q
          in: query
          required: true
          description: Точный install_id (UUID) или user_id
          schema: { type: string, minLength: 1, maxLength: 128 }
      responses:
        '200':
          description: Найденные профили (до 20)
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ProfileSearchResult' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/profiles/{person_key}:
    parameters:
      - $ref: '#/components/parameters/AppId'
      - name: person_key
        in: path
        required: true
        description: u:<user_id> или i:<install_id>, URL-encoded
        schema: { type: string, pattern: '^(u:.{1,128}|i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$' }
    get:
      tags: [Profiles]
      operationId: getProfile
      x-required-scope: read
      parameters:
        - name: sessions_limit
          in: query
          schema: { type: integer, minimum: 1, maximum: 500, default: 100 }
        - name: before
          in: query
          description: Курсор — started_at последней сессии предыдущей страницы (исключающая граница)
          schema: { type: string, format: date-time }
      responses:
        '200':
          description: Профиль
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Profile' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/integration/health:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Integration]
      operationId: getIntegrationHealth
      x-required-scope: read
      parameters:
        - name: hours
          in: query
          schema: { type: integer, minimum: 1, maximum: 168, default: 24 }
      responses:
        '200':
          description: Здоровье интеграции
          content:
            application/json:
              schema: { $ref: '#/components/schemas/IntegrationHealth' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
        '503': { $ref: '#/components/responses/StorageUnavailable' }
  /v1/apps/{app_id}/deletion-requests:
    parameters:
      - $ref: '#/components/parameters/AppId'
    post:
      tags: [Deletion]
      operationId: createDeletionRequest
      x-required-scope: delete
      description: Идемпотентно по (app_id, subject_type, subject_value) среди pending/running.
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateDeletionRequestBody' }
      responses:
        '202':
          description: Запрос принят (новый или существующий pending/running)
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeletionRequest' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /v1/apps/{app_id}/deletion-requests/{request_id}:
    parameters:
      - $ref: '#/components/parameters/AppId'
      - name: request_id
        in: path
        required: true
        schema: { type: string, format: uuid }
    get:
      tags: [Deletion]
      operationId: getDeletionRequest
      x-required-scope: read
      responses:
        '200':
          description: Статус удаления
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeletionRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /admin/v1/auth/state:
    get:
      tags: [Admin Auth]
      operationId: getAuthState
      x-internal: true
      security: []
      responses:
        '200':
          description: Состояние аутентификации
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AuthState' }
  /admin/v1/auth/login/begin:
    post:
      tags: [Admin Auth]
      operationId: beginLogin
      x-internal: true
      security: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '200':
          description: Опции WebAuthn для входа
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebAuthnCeremonyOptions' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /admin/v1/auth/login/finish:
    post:
      tags: [Admin Auth]
      operationId: finishLogin
      x-internal: true
      security: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/FinishLoginBody' }
      responses:
        '204':
          description: Вход выполнен
          headers:
            Set-Cookie: { $ref: '#/components/headers/SetSessionCookie' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '422': { $ref: '#/components/responses/WebAuthnFailed' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /admin/v1/auth/setup/{token}/begin:
    parameters:
      - $ref: '#/components/parameters/SetupToken'
    post:
      tags: [Admin Auth]
      operationId: beginSetup
      x-internal: true
      security: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '200':
          description: Опции WebAuthn для регистрации
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebAuthnCeremonyOptions' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '410': { $ref: '#/components/responses/Gone' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /admin/v1/auth/setup/{token}/finish:
    parameters:
      - $ref: '#/components/parameters/SetupToken'
    post:
      tags: [Admin Auth]
      operationId: finishSetup
      x-internal: true
      security: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/FinishRegistrationBody' }
      responses:
        '204':
          description: Passkey зарегистрирован, сессия создана
          headers:
            Set-Cookie: { $ref: '#/components/headers/SetSessionCookie' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '410': { $ref: '#/components/responses/Gone' }
        '422': { $ref: '#/components/responses/WebAuthnFailed' }
        '429': { $ref: '#/components/responses/RateLimited' }
  /admin/v1/auth/logout:
    post:
      tags: [Admin Auth]
      operationId: logout
      x-internal: true
      security:
        - sessionCookie: []
        - {}
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: Сессия завершена
          headers:
            Set-Cookie: { $ref: '#/components/headers/ClearSessionCookie' }
        '403': { $ref: '#/components/responses/Forbidden' }
  /admin/v1/passkeys:
    get:
      tags: [Admin Passkeys]
      operationId: listPasskeys
      x-internal: true
      security:
        - sessionCookie: []
      responses:
        '200':
          description: Passkeys владельца
          content:
            application/json:
              schema: { $ref: '#/components/schemas/PasskeyList' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
  /admin/v1/passkeys/register/begin:
    post:
      tags: [Admin Passkeys]
      operationId: beginPasskeyRegistration
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '200':
          description: Опции WebAuthn для регистрации дополнительного passkey
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebAuthnCeremonyOptions' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
  /admin/v1/passkeys/register/finish:
    post:
      tags: [Admin Passkeys]
      operationId: finishPasskeyRegistration
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/FinishRegistrationBody' }
      responses:
        '201':
          description: Passkey добавлен
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Passkey' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '422': { $ref: '#/components/responses/WebAuthnFailed' }
  /admin/v1/passkeys/{passkey_id}:
    parameters:
      - name: passkey_id
        in: path
        required: true
        schema: { type: string, format: uuid }
    patch:
      tags: [Admin Passkeys]
      operationId: renamePasskey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/RenamePasskeyBody' }
      responses:
        '200':
          description: Переименован
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Passkey' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
    delete:
      tags: [Admin Passkeys]
      operationId: deletePasskey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: Удалён
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '409': { $ref: '#/components/responses/Conflict' }
  /admin/v1/apps:
    post:
      tags: [Admin Apps]
      operationId: createApp
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateAppBody' }
      responses:
        '201':
          description: Приложение и первый write-ключ
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CreateAppResult' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '409': { $ref: '#/components/responses/Conflict' }
  /admin/v1/apps/{app_id}:
    parameters:
      - $ref: '#/components/parameters/AppId'
    patch:
      tags: [Admin Apps]
      operationId: updateApp
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/UpdateAppBody' }
      responses:
        '200':
          description: Обновлено
          content:
            application/json:
              schema: { $ref: '#/components/schemas/App' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '409': { $ref: '#/components/responses/Conflict' }
    delete:
      tags: [Admin Apps]
      operationId: deleteApp
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/DeleteAppBody' }
      responses:
        '202':
          description: Удаление запланировано
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeletionRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
        '409': { $ref: '#/components/responses/Conflict' }
  /admin/v1/apps/{app_id}/archive:
    parameters:
      - $ref: '#/components/parameters/AppId'
    post:
      tags: [Admin Apps]
      operationId: archiveApp
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: В архиве
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/apps/{app_id}/unarchive:
    parameters:
      - $ref: '#/components/parameters/AppId'
    post:
      tags: [Admin Apps]
      operationId: unarchiveApp
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: Активно
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/apps/{app_id}/write-keys:
    parameters:
      - $ref: '#/components/parameters/AppId'
    get:
      tags: [Admin Keys]
      operationId: listWriteKeys
      x-internal: true
      security:
        - sessionCookie: []
      responses:
        '200':
          description: Write-ключи приложения, включая отозванные
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WriteKeyList' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '404': { $ref: '#/components/responses/NotFound' }
    post:
      tags: [Admin Keys]
      operationId: createWriteKey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateWriteKeyBody' }
      responses:
        '201':
          description: Ключ создан
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WriteKey' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/apps/{app_id}/write-keys/{key_id}/revoke:
    parameters:
      - $ref: '#/components/parameters/AppId'
      - $ref: '#/components/parameters/KeyId'
    post:
      tags: [Admin Keys]
      operationId: revokeWriteKey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: Отозван (идемпотентно)
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/api-keys:
    get:
      tags: [Admin Keys]
      operationId: listApiKeys
      x-internal: true
      security:
        - sessionCookie: []
      responses:
        '200':
          description: Read API ключи (без секретов)
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ApiKeyList' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
    post:
      tags: [Admin Keys]
      operationId: createApiKey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateApiKeyBody' }
      responses:
        '201':
          description: Ключ создан; secret показывается один раз
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CreateApiKeyResult' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/api-keys/{key_id}/revoke:
    parameters:
      - $ref: '#/components/parameters/KeyId'
    post:
      tags: [Admin Keys]
      operationId: revokeApiKey
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '204':
          description: Отозван (идемпотентно)
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
        '404': { $ref: '#/components/responses/NotFound' }
  /admin/v1/settings:
    get:
      tags: [Admin Settings]
      operationId: getSettings
      x-internal: true
      security:
        - sessionCookie: []
      responses:
        '200':
          description: Настройки владельца
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Settings' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
    patch:
      tags: [Admin Settings]
      operationId: updateSettings
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/UpdateSettingsBody' }
      responses:
        '200':
          description: Обновлено
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Settings' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
  /admin/v1/settings/telegram/test:
    post:
      tags: [Admin Settings]
      operationId: sendTelegramTest
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - $ref: '#/components/parameters/XRequestedWith'
      responses:
        '200':
          description: Результат отправки (ошибка Telegram — sent=false, не 5xx)
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TelegramTestResult' }
        '400': { $ref: '#/components/responses/BadRequest' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
        '403': { $ref: '#/components/responses/Forbidden' }
  /admin/v1/system/status:
    get:
      tags: [Admin System]
      operationId: getSystemStatus
      x-internal: true
      security:
        - sessionCookie: []
      responses:
        '200':
          description: Статус системы
          content:
            application/json:
              schema: { $ref: '#/components/schemas/SystemStatus' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
  /admin/v1/deletion-requests:
    get:
      tags: [Admin System]
      operationId: listDeletionRequests
      x-internal: true
      security:
        - sessionCookie: []
      parameters:
        - name: app_id
          in: query
          required: false
          schema: { type: string, format: uuid }
      responses:
        '200':
          description: Последние 200 запросов на удаление
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeletionRequestList' }
        '401': { $ref: '#/components/responses/Unauthenticated' }
components:
  securitySchemes:
    bearerApiKey:
      type: http
      scheme: bearer
      bearerFormat: 'sk_ + 40 base62'
      description: Read API ключ. Scope операции указан в x-required-scope (read | delete).
    sessionCookie:
      type: apiKey
      in: cookie
      name: am_session
      description: Сессия дашборда после passkey-входа. Изменяющие запросы требуют X-Requested-With.
  parameters:
    AppId:
      name: app_id
      in: path
      required: true
      schema: { type: string, format: uuid }
    KeyId:
      name: key_id
      in: path
      required: true
      schema: { type: string, format: uuid }
    SetupToken:
      name: token
      in: path
      required: true
      schema: { type: string, pattern: '^[A-Za-z0-9_-]{43}$' }
    From:
      name: from
      in: query
      required: true
      schema: { type: string, format: date }
    To:
      name: to
      in: query
      required: true
      schema: { type: string, format: date }
    Compare:
      name: compare
      in: query
      schema: { type: string, enum: [previous_period, none], default: previous_period }
    EnvironmentQ:
      name: environment
      in: query
      schema: { $ref: '#/components/schemas/Environment' }
    UnitQ:
      name: unit
      in: query
      schema: { $ref: '#/components/schemas/Unit' }
    IncludeArchived:
      name: include_archived
      in: query
      schema: { type: boolean, default: false }
    FilterPlatform:
      name: platform
      in: query
      style: form
      explode: true
      schema:
        type: array
        items: { $ref: '#/components/schemas/Platform' }
    FilterAppVersion:
      name: app_version
      in: query
      style: form
      explode: true
      schema:
        type: array
        items: { type: string, maxLength: 64 }
    FilterOsName:
      name: os_name
      in: query
      style: form
      explode: true
      schema:
        type: array
        items: { type: string, maxLength: 32 }
    FilterCountry:
      name: country
      in: query
      style: form
      explode: true
      schema:
        type: array
        items: { type: string, pattern: '^[A-Z]{2}$' }
    XRequestedWith:
      name: X-Requested-With
      in: header
      required: true
      description: CSRF-защита cookie-запросов
      schema: { type: string, enum: [appmetrica-web] }
  headers:
    RetryAfter:
      schema: { type: integer, minimum: 1 }
    SetSessionCookie:
      description: am_session=<token>; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=2592000
      schema: { type: string }
    ClearSessionCookie:
      description: am_session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax
      schema: { type: string }
  responses:
    BadRequest:
      description: invalid_request | range_too_large
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    Unauthenticated:
      description: unauthenticated
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    Forbidden:
      description: insufficient_scope | csrf_check_failed
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    NotFound:
      description: app_not_found | profile_not_found | not_found
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    Conflict:
      description: last_passkey | app_name_taken | confirm_name_mismatch
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    Gone:
      description: setup_token_expired
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    WebAuthnFailed:
      description: webauthn_verification_failed
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    RateLimited:
      description: rate_limited
      headers:
        Retry-After: { $ref: '#/components/headers/RetryAfter' }
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    StorageUnavailable:
      description: storage_unavailable
      headers:
        Retry-After: { $ref: '#/components/headers/RetryAfter' }
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
  schemas:
    Platform:
      type: string
      enum: [ios, android, web, macos, windows, linux, bot, server, other]
    Environment:
      type: string
      enum: [production, development]
      default: production
    Unit:
      type: string
      enum: [persons, installs]
      default: persons
    AppStatus:
      type: string
      enum: [active, archived]
    Granularity:
      type: string
      enum: [hour, day, week, month]
      default: day
    TimeSeriesMetric:
      type: string
      enum: [active, new, sessions, avg_duration_ms]
      default: active
    BreakdownDimension:
      type: string
      enum: [platform, os_name, os_version, app_version, device_model, device_type, country, city, locale]
    BreakdownMetric:
      type: string
      enum: [active, new, sessions]
      default: active
    CohortGranularity:
      type: string
      enum: [day, week]
      default: day
    DeletionSubjectType:
      type: string
      enum: [install, user, app]
    DeletionStatus:
      type: string
      enum: [pending, running, done, failed]
    DeletionSource:
      type: string
      enum: [api, dashboard]
    ApiKeyScope:
      type: string
      enum: [read, delete]
    AlertKey:
      type: string
      enum: [disk_free, insert_errors, backup_stale, ingest_down, api_down, clickhouse_down]
    JobName:
      type: string
      enum: [close_day, digest, system_check, deletion, session_cleanup]
    JobStatus:
      type: string
      enum: [running, ok, failed]
    App:
      type: object
      required: [id, name, platform, timezone, status, created_at, updated_at, archived_at]
      properties:
        id: { type: string, format: uuid }
        name: { type: string, minLength: 1, maxLength: 64 }
        platform: { $ref: '#/components/schemas/Platform' }
        timezone: { type: string, example: Europe/Moscow }
        status: { $ref: '#/components/schemas/AppStatus' }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
        archived_at: { type: string, format: date-time, nullable: true }
    AppSummary:
      type: object
      required: [id, name, platform, status]
      properties:
        id: { type: string, format: uuid }
        name: { type: string }
        platform: { $ref: '#/components/schemas/Platform' }
        status: { $ref: '#/components/schemas/AppStatus' }
    AppList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: '#/components/schemas/App' }
    SparklinePoint:
      type: object
      required: [day, active]
      properties:
        day: { type: string, format: date }
        active: { type: integer, format: int64 }
    PortfolioRow:
      type: object
      required: [app, active_today, active_date, active_same_day_prev_week, active_delta_pct, new_date, sessions_date, retention_d1, retention_d7, sparkline]
      properties:
        app: { $ref: '#/components/schemas/AppSummary' }
        active_today: { type: integer, format: int64 }
        active_date: { type: integer, format: int64 }
        active_same_day_prev_week: { type: integer, format: int64 }
        active_delta_pct: { type: number, nullable: true }
        new_date: { type: integer, format: int64 }
        sessions_date: { type: integer, format: int64 }
        retention_d1: { type: number, minimum: 0, maximum: 1, nullable: true }
        retention_d7: { type: number, minimum: 0, maximum: 1, nullable: true }
        sparkline:
          type: array
          minItems: 30
          maxItems: 30
          items: { $ref: '#/components/schemas/SparklinePoint' }
    Portfolio:
      type: object
      required: [date, environment, unit, rows]
      properties:
        date: { type: string, format: date }
        environment: { $ref: '#/components/schemas/Environment' }
        unit: { $ref: '#/components/schemas/Unit' }
        rows:
          type: array
          items: { $ref: '#/components/schemas/PortfolioRow' }
    AudienceDelta:
      type: object
      required: [active, new, sessions, avg_duration_ms, median_duration_ms]
      properties:
        active: { type: number, nullable: true }
        new: { type: number, nullable: true }
        sessions: { type: number, nullable: true }
        avg_duration_ms: { type: number, nullable: true }
        median_duration_ms: { type: number, nullable: true }
    AudienceCompare:
      type: object
      required: [from, to, active, new, sessions, avg_duration_ms, median_duration_ms, delta_pct]
      properties:
        from: { type: string, format: date }
        to: { type: string, format: date }
        active: { type: integer, format: int64 }
        new: { type: integer, format: int64 }
        sessions: { type: integer, format: int64 }
        avg_duration_ms: { type: number, nullable: true }
        median_duration_ms: { type: number, nullable: true }
        delta_pct: { $ref: '#/components/schemas/AudienceDelta' }
    AudienceSummary:
      type: object
      required: [from, to, active, new, sessions, avg_duration_ms, median_duration_ms]
      properties:
        from: { type: string, format: date }
        to: { type: string, format: date }
        active: { type: integer, format: int64 }
        new: { type: integer, format: int64 }
        sessions: { type: integer, format: int64 }
        avg_duration_ms: { type: number, nullable: true }
        median_duration_ms: { type: number, nullable: true }
        compare: { $ref: '#/components/schemas/AudienceCompare' }
    TimeSeriesPoint:
      type: object
      required: [ts, value]
      properties:
        ts: { type: string, format: date-time, description: Начало корзины с offset TZ приложения }
        value: { type: number, nullable: true }
    TimeSeries:
      type: object
      required: [metric, granularity, points]
      properties:
        metric: { $ref: '#/components/schemas/TimeSeriesMetric' }
        granularity: { $ref: '#/components/schemas/Granularity' }
        points:
          type: array
          items: { $ref: '#/components/schemas/TimeSeriesPoint' }
        compare_points:
          type: array
          items: { $ref: '#/components/schemas/TimeSeriesPoint' }
    RollingActivePoint:
      type: object
      required: [day, dau, wau, mau]
      properties:
        day: { type: string, format: date }
        dau: { type: integer, format: int64 }
        wau: { type: integer, format: int64 }
        mau: { type: integer, format: int64 }
    RollingActive:
      type: object
      required: [points]
      properties:
        points:
          type: array
          items: { $ref: '#/components/schemas/RollingActivePoint' }
    BreakdownRow:
      type: object
      required: [value, count, share]
      properties:
        value: { type: string, description: Пустая строка — значение не определено }
        count: { type: integer, format: int64 }
        share: { type: number, minimum: 0, maximum: 1 }
    Breakdown:
      type: object
      required: [dimension, metric, total, rows]
      properties:
        dimension: { $ref: '#/components/schemas/BreakdownDimension' }
        metric: { $ref: '#/components/schemas/BreakdownMetric' }
        total: { type: integer, format: int64 }
        rows:
          type: array
          items: { $ref: '#/components/schemas/BreakdownRow' }
    RetentionCohort:
      type: object
      required: [cohort_start, size, values]
      properties:
        cohort_start: { type: string, format: date }
        size: { type: integer, format: int64 }
        values:
          type: array
          description: Длина periods+1 (D0..Dperiods); null — период ещё не наступил
          items: { type: number, minimum: 0, maximum: 1, nullable: true }
    RetentionMatrix:
      type: object
      required: [cohort_granularity, periods, cohorts, average]
      properties:
        cohort_granularity: { $ref: '#/components/schemas/CohortGranularity' }
        periods: { type: integer }
        cohorts:
          type: array
          items: { $ref: '#/components/schemas/RetentionCohort' }
        average:
          type: array
          items: { type: number, minimum: 0, maximum: 1, nullable: true }
    RealtimeMinute:
      type: object
      required: [minute, sessions_started]
      properties:
        minute: { type: string, format: date-time }
        sessions_started: { type: integer, format: int64 }
    RecentEvent:
      type: object
      required: [insert_id, event_time, received_at, name, environment, platform, app_version, os_name, os_version, device_model, country, city, install_id, person_key]
      properties:
        insert_id: { type: string, format: uuid, description: 'Стабильный ключ строки ленты (React key, подсветка новых строк)' }
        event_time: { type: string, format: date-time }
        received_at: { type: string, format: date-time }
        name: { type: string, enum: [session_start, session_end] }
        environment: { $ref: '#/components/schemas/Environment' }
        platform: { type: string }
        app_version: { type: string }
        os_name: { type: string }
        os_version: { type: string }
        device_model: { type: string }
        country: { type: string }
        city: { type: string }
        install_id: { type: string, format: uuid }
        person_key: { type: string }
    Realtime:
      type: object
      required: [active_now, per_minute, recent_events, generated_at]
      properties:
        active_now: { type: integer, format: int64 }
        per_minute:
          type: array
          minItems: 30
          maxItems: 30
          items: { $ref: '#/components/schemas/RealtimeMinute' }
        recent_events:
          type: array
          maxItems: 50
          items: { $ref: '#/components/schemas/RecentEvent' }
        generated_at: { type: string, format: date-time }
    ProfileSummary:
      type: object
      required: [person_key, user_id, matched_by, installs_count, first_seen, last_seen]
      properties:
        person_key: { type: string }
        user_id: { type: string, nullable: true }
        matched_by: { type: string, enum: [install_id, user_id] }
        installs_count: { type: integer }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
    ProfileSearchResult:
      type: object
      required: [items]
      properties:
        items:
          type: array
          maxItems: 20
          items: { $ref: '#/components/schemas/ProfileSummary' }
    ProfileInstall:
      type: object
      required: [install_id, platform, os_name, os_version, app_version, device_model, device_type, locale, country, city, first_seen, last_seen]
      properties:
        install_id: { type: string, format: uuid }
        platform: { type: string }
        os_name: { type: string }
        os_version: { type: string }
        app_version: { type: string }
        device_model: { type: string }
        device_type: { type: string }
        locale: { type: string }
        country: { type: string }
        city: { type: string }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
    ProfileSession:
      type: object
      required: [session_id, install_id, started_at, ended_at, duration_ms, app_version, platform, environment, country, city]
      properties:
        session_id: { type: string, format: uuid }
        install_id: { type: string, format: uuid }
        started_at: { type: string, format: date-time }
        ended_at: { type: string, format: date-time, nullable: true }
        duration_ms: { type: integer, format: int64, nullable: true }
        app_version: { type: string }
        platform: { type: string }
        environment: { $ref: '#/components/schemas/Environment' }
        country: { type: string }
        city: { type: string }
    Profile:
      type: object
      required: [person_key, user_id, first_seen, last_seen, installs, sessions, next_before]
      properties:
        person_key: { type: string }
        user_id: { type: string, nullable: true }
        first_seen: { type: string, format: date-time }
        last_seen: { type: string, format: date-time }
        installs:
          type: array
          items: { $ref: '#/components/schemas/ProfileInstall' }
        sessions:
          type: array
          items: { $ref: '#/components/schemas/ProfileSession' }
        next_before: { type: string, format: date-time, nullable: true }
    IntegrationHour:
      type: object
      required: [hour, accepted, rejected]
      properties:
        hour: { type: string, format: date-time }
        accepted: { type: integer, format: int64 }
        rejected: { type: integer, format: int64 }
    IntegrationReason:
      type: object
      required: [code, count, last_seen, example_payload, example_message]
      properties:
        code: { type: string, description: RejectionCode или batch-код ingest }
        count: { type: integer, format: int64 }
        last_seen: { type: string, format: date-time }
        example_payload: { type: string, maxLength: 4096 }
        example_message: { type: string }
    IntegrationVersion:
      type: object
      required: [app_version, accepted, rejected, last_seen]
      properties:
        app_version: { type: string }
        accepted: { type: integer, format: int64 }
        rejected: { type: integer, format: int64 }
        last_seen: { type: string, format: date-time }
    IntegrationHealth:
      type: object
      required: [hours, first_event_at, accepted_total, rejected_total, hourly, reasons, versions]
      properties:
        hours: { type: integer }
        first_event_at: { type: string, format: date-time, nullable: true }
        accepted_total: { type: integer, format: int64 }
        rejected_total: { type: integer, format: int64 }
        hourly:
          type: array
          items: { $ref: '#/components/schemas/IntegrationHour' }
        reasons:
          type: array
          items: { $ref: '#/components/schemas/IntegrationReason' }
        versions:
          type: array
          items: { $ref: '#/components/schemas/IntegrationVersion' }
    CreateDeletionRequestBody:
      type: object
      additionalProperties: false
      required: [subject_type, subject_value]
      properties:
        subject_type: { type: string, enum: [install, user] }
        subject_value: { type: string, minLength: 1, maxLength: 128 }
    DeletionRequest:
      type: object
      required: [id, app_id, subject_type, subject_value, status, source, requested_at, started_at, completed_at, error]
      properties:
        id: { type: string, format: uuid }
        app_id: { type: string, format: uuid }
        subject_type: { $ref: '#/components/schemas/DeletionSubjectType' }
        subject_value: { type: string }
        status: { $ref: '#/components/schemas/DeletionStatus' }
        source: { $ref: '#/components/schemas/DeletionSource' }
        requested_at: { type: string, format: date-time }
        started_at: { type: string, format: date-time, nullable: true }
        completed_at: { type: string, format: date-time, nullable: true }
        error: { type: string, nullable: true }
    DeletionRequestList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          maxItems: 200
          items: { $ref: '#/components/schemas/DeletionRequest' }
    AuthState:
      type: object
      required: [authenticated, setup_required]
      properties:
        authenticated: { type: boolean }
        setup_required: { type: boolean }
    WebAuthnCeremonyOptions:
      type: object
      required: [ceremony_id, options]
      properties:
        ceremony_id: { type: string, format: uuid }
        options:
          type: object
          additionalProperties: true
          description: |
            JSON от go-webauthn v0.18.2: {"publicKey": PublicKeyCredentialCreationOptionsJSON |
            PublicKeyCredentialRequestOptionsJSON, "mediation"?: "conditional"}. Во фронт передаётся
            options.publicKey как optionsJSON для @simplewebauthn/browser 14.0.0.
    FinishLoginBody:
      type: object
      required: [ceremony_id, credential]
      properties:
        ceremony_id: { type: string, format: uuid }
        credential:
          type: object
          additionalProperties: true
          description: AuthenticationResponseJSON из startAuthentication()
    FinishRegistrationBody:
      type: object
      required: [ceremony_id, credential, name]
      properties:
        ceremony_id: { type: string, format: uuid }
        credential:
          type: object
          additionalProperties: true
          description: RegistrationResponseJSON из startRegistration()
        name: { type: string, minLength: 1, maxLength: 64 }
    Passkey:
      type: object
      required: [id, name, created_at, last_used_at, backup_eligible, backup_state, transports]
      properties:
        id: { type: string, format: uuid }
        name: { type: string }
        created_at: { type: string, format: date-time }
        last_used_at: { type: string, format: date-time, nullable: true }
        backup_eligible: { type: boolean }
        backup_state: { type: boolean }
        transports:
          type: array
          items: { type: string, enum: [usb, nfc, ble, smart-card, hybrid, internal] }
    PasskeyList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: '#/components/schemas/Passkey' }
    RenamePasskeyBody:
      type: object
      additionalProperties: false
      required: [name]
      properties:
        name: { type: string, minLength: 1, maxLength: 64 }
    CreateAppBody:
      type: object
      additionalProperties: false
      required: [name, platform, timezone]
      properties:
        name: { type: string, minLength: 1, maxLength: 64 }
        platform: { $ref: '#/components/schemas/Platform' }
        timezone: { type: string, description: IANA, example: Europe/Moscow }
    UpdateAppBody:
      type: object
      additionalProperties: false
      minProperties: 1
      properties:
        name: { type: string, minLength: 1, maxLength: 64 }
        timezone: { type: string }
    DeleteAppBody:
      type: object
      additionalProperties: false
      required: [confirm_name]
      properties:
        confirm_name: { type: string }
    WriteKey:
      type: object
      required: [id, app_id, key, label, created_at, revoked_at, last_used_at]
      properties:
        id: { type: string, format: uuid }
        app_id: { type: string, format: uuid }
        key: { type: string, pattern: '^pk_[A-Za-z0-9]{32}$' }
        label: { type: string, maxLength: 64 }
        created_at: { type: string, format: date-time }
        revoked_at: { type: string, format: date-time, nullable: true }
        last_used_at: { type: string, format: date-time, nullable: true }
    WriteKeyList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: '#/components/schemas/WriteKey' }
    CreateWriteKeyBody:
      type: object
      additionalProperties: false
      properties:
        label: { type: string, maxLength: 64, default: '' }
    CreateAppResult:
      type: object
      required: [app, write_key]
      properties:
        app: { $ref: '#/components/schemas/App' }
        write_key: { $ref: '#/components/schemas/WriteKey' }
    ApiKey:
      type: object
      required: [id, key_prefix, label, scopes, app_id, created_at, revoked_at, last_used_at]
      properties:
        id: { type: string, format: uuid }
        key_prefix: { type: string, pattern: '^sk_[A-Za-z0-9]{8}$' }
        label: { type: string }
        scopes:
          type: array
          items: { $ref: '#/components/schemas/ApiKeyScope' }
        app_id: { type: string, format: uuid, nullable: true }
        created_at: { type: string, format: date-time }
        revoked_at: { type: string, format: date-time, nullable: true }
        last_used_at: { type: string, format: date-time, nullable: true }
    ApiKeyList:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items: { $ref: '#/components/schemas/ApiKey' }
    CreateApiKeyBody:
      type: object
      additionalProperties: false
      required: [label, scopes]
      properties:
        label: { type: string, minLength: 1, maxLength: 64 }
        scopes:
          type: array
          minItems: 1
          maxItems: 2
          uniqueItems: true
          items: { $ref: '#/components/schemas/ApiKeyScope' }
        app_id: { type: string, format: uuid, nullable: true }
    CreateApiKeyResult:
      type: object
      required: [api_key, secret]
      properties:
        api_key: { $ref: '#/components/schemas/ApiKey' }
        secret: { type: string, pattern: '^sk_[A-Za-z0-9]{40}$' }
    Settings:
      type: object
      required: [owner_timezone, telegram_chat_id, digest_enabled, digest_time, alerts_enabled, updated_at]
      properties:
        owner_timezone: { type: string }
        telegram_chat_id: { type: string, nullable: true }
        digest_enabled: { type: boolean }
        digest_time: { type: string, pattern: '^([01][0-9]|2[0-3]):[0-5][0-9]$' }
        alerts_enabled: { type: boolean }
        updated_at: { type: string, format: date-time }
    UpdateSettingsBody:
      type: object
      additionalProperties: false
      minProperties: 1
      properties:
        owner_timezone: { type: string }
        telegram_chat_id: { type: string, pattern: '^-?[0-9]{1,20}$', nullable: true }
        digest_enabled: { type: boolean }
        digest_time: { type: string, pattern: '^([01][0-9]|2[0-3]):[0-5][0-9]$' }
        alerts_enabled: { type: boolean }
    TelegramTestResult:
      type: object
      required: [sent]
      properties:
        sent: { type: boolean }
        telegram_message_id: { type: integer, format: int64 }
        error: { type: string }
    AlertState:
      type: object
      required: [key, firing, first_fired_at, last_notified_at, last_value]
      properties:
        key: { $ref: '#/components/schemas/AlertKey' }
        firing: { type: boolean }
        first_fired_at: { type: string, format: date-time, nullable: true }
        last_notified_at: { type: string, format: date-time, nullable: true }
        last_value: { type: string }
    JobRun:
      type: object
      required: [id, job, started_at, finished_at, status, details]
      properties:
        id: { type: string, format: uuid }
        job: { $ref: '#/components/schemas/JobName' }
        started_at: { type: string, format: date-time }
        finished_at: { type: string, format: date-time, nullable: true }
        status: { $ref: '#/components/schemas/JobStatus' }
        details:
          type: object
          additionalProperties: true
    SystemStatus:
      type: object
      required: [clickhouse, postgres, backup, alerts, jobs]
      properties:
        clickhouse:
          type: object
          required: [ok, disk_free_pct, events_rows]
          properties:
            ok: { type: boolean }
            disk_free_pct: { type: number, nullable: true }
            events_rows: { type: integer, format: int64, nullable: true }
        postgres:
          type: object
          required: [ok]
          properties:
            ok: { type: boolean }
        backup:
          type: object
          required: [last_backup_at, last_backup_name]
          properties:
            last_backup_at: { type: string, format: date-time, nullable: true }
            last_backup_name: { type: string, nullable: true }
        alerts:
          type: array
          items: { $ref: '#/components/schemas/AlertState' }
        jobs:
          type: array
          items: { $ref: '#/components/schemas/JobRun' }
    HealthStatus:
      type: object
      required: [status]
      properties:
        status: { type: string, enum: [ok] }
    ReadyStatus:
      type: object
      required: [status, checks]
      properties:
        status: { type: string, enum: [ready, not_ready] }
        checks:
          type: object
          additionalProperties: { type: string, enum: [ok, fail] }
    Error:
      type: object
      required: [error]
      properties:
        error:
          type: object
          required: [code, message, request_id]
          properties:
            code: { type: string }
            message: { type: string }
            request_id: { type: string }
            details:
              type: object
              additionalProperties: true
```

### 9.2. tRPC router (если используем tRPC)

> **N/A для tRPC.** Причина: бэкенд на Go, контракт — OpenAPI (§5). Вместо tRPC-роутера ниже — готовые конфиги кодогенерации, которые и образуют «типизированный роутер» по обе стороны.

`backend/oapi-api.cfg.yaml`:
```yaml
package: api
output: internal/gen/api/api.gen.go
generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true
output-options:
  skip-prune: false
  nullable-type: true
```

`backend/oapi-ingest.cfg.yaml` (только модели; хендлер пишется вручную):
```yaml
package: ingestapi
output: internal/gen/ingestapi/ingest.gen.go
generate:
  models: true
  embedded-spec: true
```

Генерация (`go tool` — oapi-codegen пиннится через `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0`):
```bash
cd backend
go tool oapi-codegen -config oapi-api.cfg.yaml ../contracts/openapi/api.yaml
go tool oapi-codegen -config oapi-ingest.cfg.yaml ../contracts/openapi/ingest.yaml
```

Сборка роутера `api` (stdlib `net/http.ServeMux` + strict handler; middleware — цепочка `func(http.Handler) http.Handler`):
```go
// backend/internal/api/router.go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/httpx"
)

func NewRouter(s *Server, mw httpx.Middlewares) http.Handler {
	mux := http.NewServeMux()
	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httpx.WriteInvalidRequest,
		ResponseErrorHandlerFunc: httpx.WriteInternalError,
	})
	gen.HandlerFromMux(strict, mux)
	return httpx.Chain(mux,
		middleware.RequestID,
		middleware.Recoverer,
		mw.AccessLog,       // slog JSON, без IP
		mw.Authenticate,    // Bearer sk_ или cookie am_session → ctx principal
		mw.RequireScope,    // x-required-scope из embedded spec
		mw.CSRF,            // X-Requested-With для cookie-мутаций
		mw.RateLimit,       // 10 rps sk_, 30 rps session
		mw.ValidateRequest, // oapi-codegen/nethttp-middleware OapiRequestValidator
	)
}
```

`web/orval.config.ts` (идентичен `00-bootstrap/web/orval.config.ts`; сгенерированный код — в `web/src/lib/api/gen/**`, рукописный mutator `web/src/lib/api/fetcher.ts` лежит вне `gen/`, чтобы `clean: true` его не стирал):
```typescript
import { defineConfig } from "orval";

// Генерация: `pnpm gen:api` (или `make gen-web`). Вывод в src/lib/api/gen/** коммитится и не редактируется руками.
// src/lib/api/fetcher.ts — рукописный mutator (cookie-сессия, базовый URL, конверт ошибок), лежит вне gen/.
export default defineConfig({
  api: {
    input: { target: "../contracts/openapi/api.yaml" },
    output: {
      mode: "tags-split",
      target: "src/lib/api/gen/endpoints",
      schemas: "src/lib/api/gen/model",
      client: "react-query",
      httpClient: "fetch",
      mock: true,
      clean: true,
      formatter: "biome",
      override: {
        mutator: { path: "src/lib/api/fetcher.ts", name: "apiFetch" },
        query: { useQuery: true, useSuspenseQuery: true },
      },
    },
  },
  apiZod: {
    input: { target: "../contracts/openapi/api.yaml" },
    output: {
      mode: "tags-split",
      target: "src/lib/api/gen/zod",
      client: "zod",
      fileExtension: ".zod.ts",
      clean: true,
      formatter: "biome",
    },
  },
});
```

`web/src/lib/api/fetcher.ts` (mutator):
```typescript
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId: string,
  ) {
    super(message);
  }
}

const MUTATING = new Set(['POST', 'PATCH', 'PUT', 'DELETE']);

export async function apiFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers);
  if (MUTATING.has(method)) headers.set('X-Requested-With', 'appmetrica-web');
  const res = await fetch(`/api${url}`, { ...init, headers, credentials: 'include' });
  if (res.status === 401 && typeof window !== 'undefined') {
    window.location.assign(`/login?next=${encodeURIComponent(window.location.pathname + window.location.search)}`);
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: { code: string; message: string; request_id: string } } | null;
    throw new ApiError(res.status, body?.error?.code ?? 'internal_error', body?.error?.message ?? res.statusText, body?.error?.request_id ?? '');
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
```

### 9.3. Error codes catalogue

| HTTP | Code | UX message (RU) | Когда |
|---|---|---|---|
| 400 | `invalid_json` | Тело запроса не является корректным JSON. | Ingest: JSON не парсится |
| 400 | `invalid_batch` | Батч не соответствует контракту. | Ingest: нет `sent_at`, `events` не 1..500, `app_key` ≠ заголовку |
| 400 | `unsupported_content_encoding` | Поддерживается только gzip. | Ingest: `Content-Encoding` не gzip/identity |
| 400 | `invalid_request` | Проверьте параметры запроса. | API: валидация параметров/тела (details.field) |
| 400 | `range_too_large` | Слишком большой период. | API: > 366 дней или > 7 дней для `hour` |
| 401 | `missing_app_key` | Не передан ключ приложения. | Ingest: нет `X-App-Key` и `app_key` |
| 401 | `invalid_app_key` | Ключ не найден или отозван. | Ingest: неизвестный/отозванный `pk_` |
| 401 | `unauthenticated` | Нужен вход. | API: нет/неверный `sk_`, нет/истекла cookie |
| 403 | `app_archived` | Приложение в архиве. | Ingest: `apps.status=archived` |
| 403 | `insufficient_scope` | У ключа нет нужного права. | API: scope ключа не содержит `x-required-scope` |
| 403 | `csrf_check_failed` | Запрос отклонён защитой. | API: cookie-мутация без `X-Requested-With` |
| 404 | `app_not_found` | Приложение не найдено. | API: нет приложения или оно вне `app_id` ключа |
| 404 | `profile_not_found` | Профиль не найден. | API: у `person_key` нет событий |
| 404 | `not_found` | Не найдено. | API: ceremony, passkey, ключ, deletion request, setup-токен |
| 409 | `last_passkey` | Нельзя удалить последний passkey. | Admin: удаление единственного passkey |
| 409 | `app_name_taken` | Приложение с таким именем уже есть. | Admin: дубль `lower(name)` |
| 409 | `confirm_name_mismatch` | Имя для подтверждения не совпадает. | Admin: `deleteApp` |
| 410 | `setup_token_expired` | Ссылка устарела. Создайте новую командой на сервере. | Admin: токен использован или старше 15 мин |
| 413 | `payload_too_large` | Батч слишком большой. | Ingest: > 1 048 576 байт после распаковки |
| 415 | `unsupported_media_type` | Неверный Content-Type. | Ingest: не `application/json`/`text/plain` |
| 422 | `webauthn_verification_failed` | Не удалось проверить passkey. | Admin: подпись/origin/challenge/дубль credential |
| 429 | `app_rate_limited` | Слишком много событий. | Ingest: > 2000 событий/с на приложение |
| 429 | `rate_limited` | Слишком часто. Попробуйте позже. | API: лимит ключа/сессии/входа |
| 500 | `internal_error` | Что-то пошло не так. | Необработанная ошибка (паника перехвачена Recoverer) |
| 503 | `storage_unavailable` | Хранилище временно недоступно. | ClickHouse/Postgres не отвечает или таймаут |
| 200* | `missing_field`, `invalid_field`, `unknown_event_name`, `event_too_old`, `user_id_looks_like_pii`, `duration_required`, `install_rate_limited` | — (только в `rejected[]`) | Ingest: отклонение отдельного события |
| 200* | `timestamp_in_future_clamped`, `duplicate_insert_id_in_batch`, `geo_ip_invalid` | — (только в `warnings[]`) | Ingest: событие принято с поправкой |

---

## Changelog

| Date | Version | Changes |
|---|---|---|
| 2026-10-13 | 1.0 | Initial API contracts: ingest.yaml (POST /v1/events, health), api.yaml (Read API /v1, Admin API /admin/v1), test vectors, codegen configs |
