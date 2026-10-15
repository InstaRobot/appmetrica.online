# appmetrica.online

[English](README.md) | **Русский**

**Живое демо: [appmetrica.online](https://appmetrica.online)** — [демо-режим](https://appmetrica.online/demo) только для
чтения на синтетических данных, без регистрации.

Открытая self-hosted аналитика для мобильных приложений, сайтов, ботов и бэкендов — **без SDK**.
Приложения отправляют события `session_start` / `session_end` обычным HTTP-запросом, а вы получаете портфель всех
приложений, аудиторию (DAU/WAU/MAU, новые, сессии, длительность, разбивки), классические когорты retention, realtime,
профили пользователей и Read API. Пользователи считаются людьми, а не устройствами (`install_id` + необязательный
`user_id`).

- **Без SDK.** Один документированный эндпоинт `POST https://<ingest-домен>/v1/events`: батчи до 500 событий,
  идемпотентность по `insert_id`, офлайн-очередь до 7 дней. Рецепты для Swift, Kotlin, JavaScript, React Native,
  Flutter, Kotlin Multiplatform, Go, curl и ботов — в открытой [документации](https://appmetrica.online/docs).
- **Одно приложение на всех платформах.** Приложение может выходить на iOS, Android, в вебе и не только: у каждой
  платформы свой ключ записи, отчёты считают людей сквозь платформы и переключаются на одну платформу, а
  приложения-дубли можно объединить.
- **Приватность по умолчанию.** IP клиента используется только в памяти для определения страны и города и нигде не
  сохраняется — ни в базах, ни в логах. `user_id`, похожие на e-mail или телефон, отклоняются. Удаление данных по
  GDPR — через API или из дашборда.
- **Вход только по passkey.** Паролей нет, утекать нечему; первый passkey регистрируется по одноразовой ссылке из CLI.
- **Английский и русский.** Дашборд, лендинг и документация на двух языках: английский в корне, русский — на
  [`/ru`](https://appmetrica.online/ru); выбор запоминается на год, при первом визите язык берётся из браузера.
- **Светлая и тёмная темы**, публичный лендинг и [демо-режим](https://appmetrica.online/demo) только для чтения на синтетических данных.
- **Быстро на одном сервере.** ClickHouse для событий, PostgreSQL для метаданных, сервисы на Go, дашборд на Next.js.

> Проект не связан с компанией Яндекс. «AppMetrica» — товарный знак его правообладателя; appmetrica.online —
> независимая открытая альтернатива.

## Содержание

- [Архитектура](#архитектура)
- [Требования](#требования)
- [Установка через Docker Compose](#установка-через-docker-compose)
- [Развёртывание в Dokploy](#развёртывание-в-dokploy)
- [Справочник параметров](#справочник-параметров)
- [Первые события](#первые-события)
- [Платформы и слияние приложений](#платформы-и-слияние-приложений)
- [Эксплуатация](#эксплуатация)
- [Разработка](#разработка)
- [Документация](#документация)
- [Лицензия](#лицензия)

## Архитектура

```
         ваши приложения ──► https://INGEST_DOMAIN/v1/events ─┐
                                                              │        ┌────────────┐
 браузер ──► https://APP_DOMAIN  (дашборд, лендинг, доки)     ├─ Caddy ─┤  ingest    ├──► ClickHouse (события)
 скрипты ──► https://API_DOMAIN/v1 (Read API, ключи sk_)      ┘ (HTTPS) │  api       ├──► PostgreSQL (приложения,
                                                                       │  worker    │     ключи, passkey, настройки)
                                                                       │  web       │
                                                                       └────────────┘
```

В [Dokploy](#развёртывание-в-dokploy) место Caddy занимает встроенный Traefik.

| Сервис | Назначение |
|---|---|
| `caddy` | Обратный прокси на портах 80/443, автоматические HTTPS-сертификаты Let's Encrypt (только стек Docker Compose) |
| `web` | Дашборд на Next.js, публичный лендинг, открытая документация API, демо-режим |
| `api` | API дашборда и публичный Read API; при старте применяет миграции баз |
| `ingest` | Приём событий: валидация, коррекция часов, дедупликация, геолокация, лимиты |
| `worker` | Фоновые задачи: закрытие дней в агрегаты, дайджест и алерты в Telegram, удаления по GDPR, слияние приложений, очистка |
| `clickhouse` | Хранилище событий (сырые события — 25 месяцев, дневные агрегаты — бессрочно) |
| `postgres` | Метаданные: приложения, ключи записи и чтения, passkey, сессии, настройки, заявки на удаление, слияния приложений |
| `clickhouse-backup` | Необязательно: инкрементальные бэкапы ClickHouse в любое S3-совместимое хранилище |

`api`, `ingest` и `worker` — один Go-бинарник (`backend/`), запущенный в разных ролях.

## Требования

- Linux-сервер (x86_64 или arm64) с публичным IP-адресом и [Docker Engine](https://docs.docker.com/engine/install/)
  с плагином Compose v2 (команда `docker compose version` должна работать).
- Ресурсы: **2 vCPU, 4 ГБ RAM, 20 ГБ SSD** хватит для небольшого портфеля; рекомендуется **4 vCPU, 8 ГБ RAM**.
  Первой сборке образов нужно около 2 ГБ свободной памяти и доступ в интернет (модули Go, пакеты npm, гео-база
  DB-IP). Данные растут примерно на 10 ГБ в год на каждые 100 000 активных пользователей в день.
- Три DNS-записи (A и/или AAAA), указывающие на сервер, например:

  | Запись | Назначение |
  |---|---|
  | `analytics.example.com` | дашборд (`APP_DOMAIN`) |
  | `i.analytics.example.com` | приём событий (`INGEST_DOMAIN`) — этот адрес попадёт в ваши приложения |
  | `api.analytics.example.com` | Read API (`API_DOMAIN`) |

- Открытые из интернета входящие порты **80** и **443** (TCP; для HTTP/3 ещё 443/UDP) — они нужны Let's Encrypt для
  выпуска сертификатов. Других портов открывать не нужно.

> Адрес приёма событий вшивается в приложения. Выберите домен, который будет вашим долгие годы.

## Установка через Docker Compose

### 1. Установите Docker

```bash
curl -fsSL https://get.docker.com | sh
docker compose version
```

(Подойдёт любой официальный способ установки, см. [документацию Docker](https://docs.docker.com/engine/install/).)

### 2. Скачайте код

```bash
git clone https://github.com/InstaRobot/appmetrica.online.git
cd appmetrica.online/deploy/selfhost
```

### 3. Создайте конфигурацию

```bash
cp .env.example .env
chmod 600 .env
```

Сгенерируйте все секреты и месяц гео-базы одной командой (GNU sed, то есть на Linux-сервере):

```bash
sed -i \
  -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(openssl rand -hex 24)|" \
  -e "s|^CLICKHOUSE_ADMIN_PASSWORD=.*|CLICKHOUSE_ADMIN_PASSWORD=$(openssl rand -hex 24)|" \
  -e "s|^CLICKHOUSE_RW_PASSWORD=.*|CLICKHOUSE_RW_PASSWORD=$(openssl rand -hex 24)|" \
  -e "s|^CLICKHOUSE_RO_PASSWORD=.*|CLICKHOUSE_RO_PASSWORD=$(openssl rand -hex 24)|" \
  -e "s|^KEY_HASH_PEPPER=.*|KEY_HASH_PEPPER=$(openssl rand -base64 32)|" \
  -e "s|^GEO_MONTH=.*|GEO_MONTH=$(date +%Y-%m)|" \
  .env
```

Затем откройте `.env` и укажите свои домены и e-mail:

```dotenv
APP_DOMAIN=analytics.example.com
INGEST_DOMAIN=i.analytics.example.com
API_DOMAIN=api.analytics.example.com
ACME_EMAIL=you@example.com
```

Остальное — по желанию; каждый параметр описан в [Справочнике параметров](#справочник-параметров) и в комментариях
`.env.example`. Сохраните копию `.env` в менеджере паролей: пароли баз и `KEY_HASH_PEPPER` понадобятся для
восстановления из бэкапа.

### 4. Соберите и запустите

```bash
docker compose up -d --build --wait
docker compose ps
```

Первая сборка занимает 5–15 минут. `--wait` возвращает управление, когда все сервисы здоровы. При старте `api`
применяет миграции Postgres и ClickHouse; `ingest`, `worker` и `web` запускаются после него. Caddy получает
сертификаты автоматически при первом HTTPS-запросе (если домен не открывается — смотрите `docker compose logs caddy`).

Проверьте, что сервисы отвечают:

```bash
curl -fsS https://api.analytics.example.com/health     # {"status":"ok"}
curl -fsS https://i.analytics.example.com/health       # {"status":"ok"}
```

### 5. Зарегистрируйте первый passkey

Паролей нет. Создайте одноразовую ссылку регистрации (действует 15 минут, только для одной регистрации):

```bash
docker compose exec api /app/appmetrica admin passkey-link
```

Откройте напечатанную ссылку `https://APP_DOMAIN/setup/…` в браузере, назовите устройство и подтвердите через Touch ID,
Face ID, Windows Hello или аппаратный ключ. Вход выполнен. Дополнительные passkey (ноутбук, телефон, ключ) добавляются
в **Настройки → Passkeys**. Если потеряете все — выполните эту же команду ещё раз.

### 6. Подключите первое приложение

1. **Портфель → Добавить приложение**: название, одна или несколько платформ, часовой пояс. На каждую платформу
   создаётся свой ключ записи `pk_…`.
2. Вкладка **Интеграция** показывает ключ, готовые сниппеты (curl, Swift, Kotlin, JavaScript, Go) и ждёт первое
   событие.
3. Отправьте тестовый батч (см. [Первые события](#первые-события)) — статус сменится на «Получено», и приложение
   появится в портфеле.

## Развёртывание в Dokploy

[Dokploy](https://dokploy.com) собирает тот же стек из Git и отдаёт его через свой Traefik, поэтому Caddy не нужен.
Так работает публичный экземпляр [appmetrica.online](https://appmetrica.online). Compose-файл —
[`deploy/dokploy/docker-compose.yml`](deploy/dokploy/docker-compose.yml); проверено на Dokploy v0.30.8.

[Требования](#требования) те же: сервер с установленным Dokploy, три DNS-записи на него и открытые порты 80/443.

### 1. Создайте Compose-сервис

**Project → Create Service → Compose**, тип **Docker Compose**. Вкладка **General**:

| Поле | Значение |
|---|---|
| Provider | GitHub (или Git): этот репозиторий или ваш форк |
| Branch | `main` |
| Compose Path | `./deploy/dokploy/docker-compose.yml` |
| Trigger Type | On Push (автодеплой) |
| Watch Paths | `backend/**`, `web/**`, `deploy/**`, чтобы правки README не запускали пересборку |

Нажмите **Save**.

### 2. Заполните переменные

Во вкладке **Environment** задайте параметры из [Справочника параметров](#справочник-параметров), кроме
`ACME_EMAIL`, `HTTP_PORT` и `HTTPS_PORT`: сертификаты и порты берёт на себя Traefik. Переключатель
**Create Environment File** оставьте включённым: Dokploy пишет переменные в `.env` рядом с compose-файлом.

Сгенерируйте блок на своём компьютере, замените домены и вставьте:

```bash
cat <<EOF
APP_DOMAIN=analytics.example.com
INGEST_DOMAIN=i.analytics.example.com
API_DOMAIN=api.analytics.example.com
GEO_MONTH=$(date +%Y-%m)
POSTGRES_PASSWORD=$(openssl rand -hex 24)
CLICKHOUSE_ADMIN_PASSWORD=$(openssl rand -hex 24)
CLICKHOUSE_RW_PASSWORD=$(openssl rand -hex 24)
CLICKHOUSE_RO_PASSWORD=$(openssl rand -hex 24)
KEY_HASH_PEPPER=$(openssl rand -base64 32)
EOF
```

Нажмите **Save** и сохраните копию в менеджере паролей. Пароли баз применяются при первом старте на пустых томах;
чтобы сменить их потом, нужен `ALTER USER`, а не новое значение здесь.

### 3. Добавьте домены

Во вкладке **Domains** добавьте три домена с включённым HTTPS и провайдером сертификатов Let's Encrypt. Если
список сервисов пуст, нажмите кнопку обновления рядом с **Service Name**: Dokploy прочитает сервисы из репозитория.

| Host | Service | Container port |
|---|---|---|
| `APP_DOMAIN` | `appmetrica-web` | `3000` |
| `INGEST_DOMAIN` | `appmetrica-ingest` | `8081` |
| `API_DOMAIN` | `appmetrica-api` | `8080` |

У сервисов префикс `appmetrica-`, потому что Dokploy подключает каждый сервис с доменом к общей сети
`dokploy-network`, где короткое имя вроде `api` может указать на контейнер другого проекта.

### 4. Запустите деплой

Нажмите **Deploy**. Первая сборка занимает 5–15 минут: Go-бинарник, дашборд на Next.js и база DB-IP собираются
на сервере.

> Пока идёт деплой, не открывайте **Preview Compose** и не обновляйте список сервисов в окне домена. Оба действия
> заново клонируют репозиторий в каталог, из которого идёт сборка, и она падает с
> `getwd: no such file or directory`. В этом случае просто запустите деплой ещё раз.

Проверьте готовность сервисов:

```bash
curl -fsS https://api.analytics.example.com/ready    # {"checks":{"clickhouse":"ok","postgres":"ok"},"status":"ready"}
curl -fsS https://i.analytics.example.com/ready
```

### 5. Зарегистрируйте первый passkey

Образ backend собран на distroless и не содержит shell, поэтому **Open Terminal** в Dokploy к нему не подключится.
Выполните команду на сервере по SSH:

```bash
docker exec $(docker ps -qf name=appmetrica-api) /app/appmetrica admin passkey-link
```

Дальше — [Подключите первое приложение](#6-подключите-первое-приложение).

### Эксплуатация в Dokploy

- **Обновления.** С автодеплоем каждый пуш, затрагивающий Watch Paths, пересобирает стек; без него нажмите
  **Deploy**. Автодеплой не ждёт CI. Миграции применяет `api` при старте.
- **Гео-база.** Поставьте в `GEO_MONTH` текущий месяц во вкладке **Environment** и запустите деплой.
- **Логи.** Вкладка **Logs** сервиса или `docker logs` на сервере.
- **Бэкап Postgres.** Во вкладке **Backups** добавьте бэкап PostgreSQL сервиса `appmetrica-postgres` (база и
  пользователь `appmetrica`) в S3-хранилище из **Settings → S3 Destinations**.
- **Бэкап ClickHouse.** Во вкладке **Environment** задайте `COMPOSE_PROFILES=backup`,
  `BACKUP_API_URL=http://appmetrica-clickhouse-backup:7171` и переменные `S3_*`, затем запустите деплой.
- **Потерян доступ.** Ещё раз выполните команду `passkey-link` из шага 5.

Альтернативный конвейер, в котором GitHub Actions собирает образы в GHCR и выкатывает их через API Dokploy по
Tailscale (`.github/workflows/deploy.yml`), описан в [`deploy/dokploy/README.md`](deploy/dokploy/README.md).

## Справочник параметров

Все параметры задаются в `deploy/selfhost/.env` и читаются `docker-compose.yml`. После изменения выполните
`docker compose up -d`, а для параметров с пометкой *при сборке* — `docker compose up -d --build`. В Dokploy те же
параметры задаются во вкладке **Environment** и применяются кнопкой **Deploy**.

### Обязательные

| Параметр | Пример | Описание | Как получить |
|---|---|---|---|
| `APP_DOMAIN` | `analytics.example.com` | Домен дашборда, лендинга и документации. Это же relying party для passkey (WebAuthn): ключи привязаны к домену, поэтому после его смены passkey придётся зарегистрировать заново | — |
| `INGEST_DOMAIN` | `i.analytics.example.com` | Домен приёма событий, который используют ваши приложения. *При сборке* — для сниппетов в дашборде | — |
| `API_DOMAIN` | `api.analytics.example.com` | Домен Read API. *При сборке* — для документации в дашборде | — |
| `ACME_EMAIL` | `you@example.com` | E-mail для Let's Encrypt (уведомления об истечении и инцидентах). В Dokploy не используется | — |
| `POSTGRES_PASSWORD` | 48 hex-символов | Пароль пользователя `appmetrica` в Postgres | `openssl rand -hex 24` |
| `CLICKHOUSE_ADMIN_PASSWORD` | 48 hex-символов | ClickHouse `appmetrica_admin`: миграции и бэкапы | `openssl rand -hex 24` |
| `CLICKHOUSE_RW_PASSWORD` | 48 hex-символов | ClickHouse `appmetrica_rw`: используют ingest и worker | `openssl rand -hex 24` |
| `CLICKHOUSE_RO_PASSWORD` | 48 hex-символов | ClickHouse `appmetrica_ro`: пользователь api только для чтения | `openssl rand -hex 24` |
| `KEY_HASH_PEPPER` | base64, ≥ 32 байт | HMAC-«перец» для ключей Read API (`sk_…` хранятся только как хэши). Смена делает недействительными все ключи `sk_` | `openssl rand -base64 32` |
| `GEO_MONTH` | `2026-10` | Месяц базы [DB-IP City Lite](https://db-ip.com/db/download/ip-to-city-lite), которая скачивается при сборке backend. *При сборке*. Если в начале месяца сборка падает с 404 — укажите предыдущий месяц | `date +%Y-%m` |

Для Postgres используйте hex-пароли (`openssl rand -hex`): пароль попадает в URL подключения и не должен содержать
`/`, `@` или `:`.

### Уведомления и мониторинг (необязательно)

| Параметр | По умолчанию | Описание |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | пусто | Токен от [@BotFather](https://t.me/BotFather). Включает ежедневный дайджест портфеля и системные алерты (место на диске, ошибки вставки, устаревший бэкап, упавшие сервисы, невыполненные удаления и слияния приложений). ID чата, время дайджеста и переключатель алертов задаются в **Настройки → Telegram**. Пусто — сообщения пишутся в лог worker |
| `HEALTHCHECKS_PING_URL` | пусто | Адрес [healthchecks.io](https://healthchecks.io) (или совместимого сервиса). Worker пингует его каждые 5 минут; тишина означает, что сервер или worker не работают |
| `DISK_FREE_ALERT_PCT` | `20` | Алерт, когда свободного места на томе ClickHouse меньше этого процента (1–99) |

### Бэкапы ClickHouse в S3 (необязательно)

Подойдёт любое S3-совместимое хранилище (AWS S3, Hetzner Object Storage, Cloudflare R2, MinIO, Backblaze B2).

| Параметр | По умолчанию | Описание |
|---|---|---|
| `COMPOSE_PROFILES` | пусто | `backup` — запустить сервис `clickhouse-backup` |
| `BACKUP_API_URL` | пусто | `http://clickhouse-backup:7171` (в Dokploy — `http://appmetrica-clickhouse-backup:7171`): worker проверяет свежесть бэкапа, а **Настройки → Система** показывает последний бэкап |
| `S3_ENDPOINT` | — | Адрес хранилища, например `https://s3.eu-central-1.amazonaws.com` |
| `S3_BUCKET` | — | Существующий бакет для бэкапов (внутри него — путь `clickhouse/`) |
| `S3_REGION` | — | Регион бакета |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | — | Ключ доступа с правами чтения и записи в бакет |
| `BACKUPS_TO_KEEP_REMOTE` | `35` | Сколько бэкапов хранить (≈ 5 недель ежедневных инкрементов + еженедельные полные) |
| `WATCH_INTERVAL` | `24h` | Интервал инкрементальных бэкапов |
| `FULL_INTERVAL` | `168h` | Интервал полных бэкапов |

### Тонкая настройка (необязательно)

| Параметр | По умолчанию | Описание |
|---|---|---|
| `WEBAUTHN_RP_NAME` | `appmetrica.online` | Название, которое браузер показывает при регистрации passkey |
| `HTTP_PORT`, `HTTPS_PORT` | `80`, `443` | Порты Caddy на хосте (только стек Docker Compose). Для Let's Encrypt порты 80 и 443 должны быть доступны из интернета |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` или `error` (JSON-логи в stdout) |
| `SESSION_TTL` | `720h` | Максимальный срок сессии дашборда |
| `SESSION_IDLE_TTL` | `168h` | Срок сессии без активности |
| `API_PUBLIC_BEARER_ENABLED` | `true` | `false` отключает публичный Read API (ключи `sk_`); дашборд продолжает работать |
| `INGEST_MAX_BODY_BYTES` | `1048576` | Максимальный размер распакованного тела запроса, байт |
| `INGEST_MAX_EVENTS` | `500` | Максимум событий в батче |
| `INGEST_LATE_WINDOW` | `168h` | События старше отклоняются (`event_too_old`) |
| `INGEST_FUTURE_TOLERANCE` | `10m` | Допустимое опережение часов клиента после коррекции |
| `INGEST_ACCEPT_DEVELOPMENT` | `true` | `false` — отбрасывать события с `environment=development` |
| `RATE_APP_EVENTS_PER_SEC`, `RATE_APP_BURST` | `2000`, `4000` | Лимит на приложение; сверх него батч получает HTTP 429 с `Retry-After` |
| `RATE_INSTALL_PER_MIN`, `RATE_INSTALL_BURST` | `100`, `1000` | Лимит на установку; лишние события отклоняются с повторяемым кодом `install_rate_limited` |
| `KEY_CACHE_REFRESH` | `15s` | Как часто ingest перечитывает ключи записи (задержка, после которой отозванный `pk_` перестаёт работать) |
| `WORKER_JOBS_ENABLED` | все задачи | Через запятую: `system_check`, `digest`, `close_day`, `deletion`, `session_cleanup`, `app_merge`. Без `app_merge` [слияние приложений](#платформы-и-слияние-приложений) ждёт в `pending` |
| `MERGE_KEYS_WAIT` | — | Только e2e-стенд (`APP_ENV=e2e`, иначе ошибка запуска): сколько слияние ждёт перезагрузки write-ключей в ingest вместо 17 минут; в CI `0` |

## Первые события

У каждого приложения свой ключ записи `pk_…` (дашборд → приложение → **Интеграция**). Ключи записи публичны по
замыслу: они вшиваются в приложения и позволяют только отправлять события.

```bash
export AM_PK=pk_ваш_ключ_записи
curl -sS -X POST "https://i.analytics.example.com/v1/events" \
  -H "Content-Type: application/json" \
  -H "X-App-Key: $AM_PK" \
  -d "{
    \"sent_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
    \"context\": { \"platform\": \"ios\", \"app_version\": \"1.0.0\" },
    \"events\": [{
      \"insert_id\": \"$(uuidgen | tr A-Z a-z)\",
      \"name\": \"session_start\",
      \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
      \"install_id\": \"$(uuidgen | tr A-Z a-z)\",
      \"session_id\": \"$(uuidgen | tr A-Z a-z)\"
    }]
  }"
# → {"accepted":1,"rejected":[],"server_time":"…","warnings":[]}
```

Полный справочник API (приём событий и Read API), рецепты сессий и офлайн-очереди для каждой платформы и политика
ошибок и повторов — в открытой документации по адресу `https://APP_DOMAIN/docs`, вход не нужен; живая копия —
[appmetrica.online/docs](https://appmetrica.online/docs). OpenAPI-контракты лежат в
[`contracts/openapi/`](contracts/openapi/).

## Платформы и слияние приложений

Приложение — один продукт на одной или нескольких платформах: `ios`, `android`, `web`, `macos`, `windows`, `linux`,
`bot`, `server`, `other`. Платформы выбираются при создании и меняются в **Настройки → Платформы**; убранная
платформа сохраняет свои события и ключи.

Платформа события определяется по порядку:

1. `context.platform` события или батча;
2. платформа ключа записи. Ключи, созданные вместе с приложением, привязаны к своей платформе; в
   **Настройки → Ключи записи** новый ключ получает одну платформу или «Любая платформа»;
3. единственная платформа приложения, если она одна;
4. иначе `other`, и в ответе для события приходит предупреждение `platform_missing`.

Отчёты считают людей сквозь платформы: один и тот же `user_id` на iOS и в вебе — один человек. Переключатель
платформ над отчётами сужает их до одной платформы, а график аудитории умеет рисовать линию на каждую платформу.

**Слияние приложений.** Если iOS и Android подключены как два отдельных приложения, объедините их: откройте
приложение, которое останется, **Настройки → Опасная зона → Объединить…**, выберите второе приложение и введите его
название. Job worker `app_merge` переносит ключи записи (выпущенные версии приложений продолжают с ними работать),
ждёт 17 минут, пока все инстансы приёма событий перечитают ключи, копирует события, удаляет приложение-источник и
пересчитывает дни слияния. Баннер над приложением показывает ход; если шаг упал, баннер показывает ошибку, а
**Повторить** продолжает с этого шага. Пока идёт слияние, ни одно из двух приложений нельзя архивировать или удалить.

Слияние не отменяется. Перед ним сделайте бэкап Postgres и ClickHouse (см. [Эксплуатация](#эксплуатация)) и
проверьте, что на диске ClickHouse хватит места на копию событий источника. Job должен быть в `WORKER_JOBS_ENABLED`
(по умолчанию он там есть); иначе слияние ждёт в `pending`.

## Эксплуатация

Все команды выполняются из `deploy/selfhost` (стек Docker Compose). Для Dokploy см.
[Эксплуатация в Dokploy](#эксплуатация-в-dokploy).

**Логи и состояние**

```bash
docker compose ps
docker compose logs -f --tail=100 api ingest worker
```

**Обновление до новой версии**

```bash
git pull
docker compose up -d --build --wait     # миграции применяет api автоматически
```

**Обновление гео-базы** (раз в месяц): поставьте в `GEO_MONTH` текущий месяц (`date +%Y-%m`) и выполните
`docker compose up -d --build --wait`.

**Бэкап Postgres** (приложения, ключи, passkey, настройки — данных мало, делайте ежедневно, например из cron):

```bash
docker compose exec -T postgres pg_dump -U appmetrica -d appmetrica -Fc > appmetrica-$(date +%F).dump
```

Восстановление в пустую базу:

```bash
docker compose exec -T postgres pg_restore -U appmetrica -d appmetrica --clean --if-exists < appmetrica-2026-10-14.dump
```

**Бэкап ClickHouse**: включите профиль `backup` (см. [Бэкапы ClickHouse в S3](#бэкапы-clickhouse-в-s3-необязательно)),
дальше бэкапами управляет CLI `clickhouse-backup`:

```bash
docker compose exec clickhouse-backup clickhouse-backup list remote
docker compose exec clickhouse-backup clickhouse-backup restore_remote <имя-бэкапа>
```

**Потерян доступ** (не осталось ни одного passkey): `docker compose exec api /app/appmetrica admin passkey-link`
печатает новую одноразовую ссылку регистрации.

**Проверки здоровья**: `https://API_DOMAIN/health`, `https://INGEST_DOMAIN/health`, `https://APP_DOMAIN/api/health`.

**Остановка и удаление**

```bash
docker compose down        # остановить, данные сохраняются
docker compose down -v     # остановить и УДАЛИТЬ все тома с данными
```

**Безопасность**: держите `.env` закрытым (`chmod 600`), открывайте наружу только порты 80/443, обновляйте Docker и
систему, храните бэкапы вне сервера и отзывайте ненужные ключи Read API `sk_` (**Настройки → API-ключи**).

## Разработка

Нужны Go 1.27.2, Node 24.21.0, pnpm 12.11.2, Docker; для `make check` — `golangci-lint` 2.14, для перф-тестов и
скриптов деплоя — `jq`.

```bash
make env          # .env из .env.example с рабочими dev-значениями
make install      # модули Go и зависимости pnpm
make dev-infra    # ClickHouse и Postgres в Docker
make dev          # миграции + ingest :8081, api :8080, worker :8082, web :3000
make seed         # демо-данные: 4 приложения (одно на iOS, Android и Web), 35 дней событий
make passkey-link # одноразовая ссылка регистрации passkey для http://localhost:3000
```

Перед коммитом — `make check` (проверка кодогенерации, линтеры, unit-тесты Go, web и скриптов). Интеграционные
тесты — `make test-integration` (нужен Docker). E2E — `make e2e`. Проект ведётся по TDD и Conventional Commits; правила разработки —
в [CONTRIBUTING.md](CONTRIBUTING.md).

Структура репозитория:

- `backend/` — Go-модуль, один бинарник `appmetrica` (`ingest`, `api`, `worker`, `migrate`, `admin`, `healthcheck`).
- `web/` — дашборд на Next.js 16, лендинг, демо-режим и документация API (`web/content/docs/`).
- `contracts/openapi/` — контракты OpenAPI 3.0; из них генерируется код Go и TypeScript.
- `deploy/selfhost/` — стек Docker Compose для установки на свой сервер; `deploy/dokploy/` — тот же стек для Dokploy
  и конвейер GitHub Actions → GHCR → Dokploy; `deploy/clickhouse/` — конфигурация ClickHouse.
- `scripts/` — скрипты CI и деплоя; `scripts/reconcile` — сверка цифр с выгрузками AppMetrica.
- `docs/discovery/` — продуктовая и техническая спецификация.

## Документация

- Требования, архитектура, модель данных, API-контракты, UX:
  [`docs/discovery/2026-10-13-appmetrica-online/`](docs/discovery/2026-10-13-appmetrica-online/)
- Команды разработки: [12-dev-runbook.md](docs/discovery/2026-10-13-appmetrica-online/12-dev-runbook.md)
- Эксплуатация: [21-ops-runbooks.md](docs/discovery/2026-10-13-appmetrica-online/21-ops-runbooks.md)
- Документация API для интеграций: открыта по адресу `/docs` любой установки
  ([appmetrica.online/docs](https://appmetrica.online/docs)), исходники — [`web/content/docs/`](web/content/docs/)

## Лицензия

[GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0). appmetrica.online можно свободно использовать, изменять
и разворачивать у себя; если вы запускаете изменённую версию как сетевой сервис для других, вы обязаны опубликовать
свои изменения под той же лицензией.

Геолокация использует базу [IP to City Lite](https://db-ip.com) от DB-IP под лицензией
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/); обязательная атрибуция есть в футере дашборда.
