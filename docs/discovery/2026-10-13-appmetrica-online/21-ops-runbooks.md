# Ops Runbooks: appmetrica.online

**Tier**: шаблон Tier 3/4, применён к Tier 1–2 продукту. Документ включён в пакет, потому что у владельца нет экспертизы
ClickHouse, а продукт держит один сервер с данными, которые нельзя потерять.
**Consumer**: владелец (solo) во время инцидентов, провижининга и плановых работ; AI-агент при подготовке ops-тикетов.

> Окружение: Hetzner AX102-1 (FSN1), Ubuntu 24.04 на RAID1, Tailscale, Dokploy ≥ v0.30.8 (Traefik v3.6, Docker Swarm),
> Applications `appmetrica-ingest` / `appmetrica-api` / `appmetrica-worker` / `appmetrica-web` из GHCR, compose `clickhouse-stack`
> (ClickHouse 26.8.25.14 + clickhouse-backup 2.8.1), Dokploy managed PostgreSQL 18.6, Hetzner Object Storage (fsn1).
> Имена, порты и переменные окружения взяты из канонического якоря; архитектура — в [03-architecture.md](./03-architecture.md),
> риски — в [09-risks-and-mitigations.md](./09-risks-and-mitigations.md), dev-процессы — в [12-dev-runbook.md](./12-dev-runbook.md).

---

## 1. SLI / SLO / SLA

### 1.1. Key SLIs (measured indicators)

| SLI | Measurement | Tool |
|---|---|---|
| Доступность ingest | доля ответов `POST /v1/events` не 5xx от всех запросов за окно | Prometheus-метрики ingest (`:9090/metrics`), читаются worker-ом |
| Латентность ingest | P50 / P95 / P99 времени ответа `POST /v1/events`, включая ожидание async insert | гистограмма ingest на `:9090/metrics` |
| Доля ошибок вставки | ошибки вставки в ClickHouse / все вставки за 5 минут | job `system_check` (алерт `insert_errors`) |
| Свежесть данных | время от ответа 200 до появления события в `GET /v1/apps/{app_id}/realtime` | E2E-смоук после деплоя + ручная проверка S-07 |
| Латентность отчётов | P95 `GET /v1/apps/{app_id}/audience/*`, `/retention`, `/v1/portfolio` | гистограмма api на `:9090/metrics` |
| Выполнение задач | доля успешных `job_runs` по `job` за 24 ч | S-12 «Система» / `SELECT job, status, count(*) FROM job_runs …` |
| Свежесть бэкапа | возраст последнего удалённого бэкапа ClickHouse | `GET http://clickhouse-backup:7171/backup/list/remote` (алерт `backup_stale`) |
| Свободный диск | `free_space / total_space` из `system.disks` | job `system_check` (алерт `disk_free`) |

### 1.2. SLOs (internal targets)

| Service | SLI | SLO target | Window |
|---|---|---|---|
| appmetrica-ingest | Доступность (NFR-002) | 99.9 % | 30 дней скользящее |
| appmetrica-ingest | Латентность P99 (NFR-001) | ≤ 500 мс при 500 rps | 30 дней скользящее |
| appmetrica-api | Отчёты P95 на диапазоне 90 дней (NFR-003) | ≤ 1.5 с | 7 дней скользящее |
| appmetrica-api | Портфель P95 (NFR-003) | ≤ 1 с | 7 дней скользящее |
| Realtime | Свежесть события (NFR-004) | ≤ 10 с | проверка после каждого деплоя |
| Бэкапы ClickHouse | Возраст последнего удалённого бэкапа | ≤ 26 ч | непрерывно |
| Worker | `system_check` выполнен | каждые 5 мин, пропуск ≤ 2 подряд | 24 ч |
| Данные | Потери подтверждённых событий (NFR-005) | 0 | всегда; RPO 24 ч только для катастрофы |

### 1.3. SLAs (external commitments to customers)

> **N/A для этого проекта.** Причина: внутренний инструмент одного владельца, внешних клиентов и договорных
> обязательств нет. Роль SLA играют SLO из 1.2 и критерий успеха NFR-010 (сверка с AppMetrica).

### 1.4. Error budget
- SLO 99.9 % для ingest даёт **43,2 минуты** недоступности в месяц.
- Простой ingest не теряет данные: клиенты держат офлайн-очередь до 7 дней и ретраят 5xx и сетевые ошибки.
  Бюджет защищает свежесть отчётов, а не сохранность данных.
- Израсходовано больше 50 % бюджета за месяц → до конца месяца деплои ingest только с фиксами надёжности.
- Израсходовано больше 100 % → разбор (post-mortem из раздела 9) и тикет на устранение причины до любых новых фич.

## 2. Monitoring & alerting

### 2.1. Alert routing
- **P0 (приём остановлен или под угрозой данные)**: Telegram-бот worker-а (`TELEGRAM_BOT_TOKEN`, чат из `settings.telegram_chat_id`)
  + healthchecks.io (dead-man, уведомление в Telegram и на email владельца).
- **P1 (деградация)**: Telegram-бот worker-а, повтор раз в 6 часов, пока алерт горит.
- **P2 (информация о деплоях и бэкапах)**: Dokploy notifications → Telegram (App Deploy, App Build Error, Database Backup,
  Docker Cleanup, Dokploy Restart).
- **P3 (без алерта)**: экран S-12 «Система» (`GET /admin/v1/system/status`) и вкладка «Интеграция» приложения.

### 2.2. Key alerts

| Alert (`alert_states.key`) | Condition | Severity |
|---|---|---|
| `ingest_down` | `GET INGEST_HEALTH_URL` не отвечает 200 два цикла `system_check` подряд (10 мин) | P0 |
| `clickhouse_down` | ping ClickHouse из worker не проходит два цикла подряд | P0 |
| `disk_free` | свободно меньше 20 % (`DISK_FREE_ALERT_PCT`) на диске ClickHouse | P0 |
| healthchecks.io «down» | нет ping от worker дольше 15 минут (period 5 мин + grace 10 мин) | P0 |
| `insert_errors` | доля ошибок вставки в CH больше 1 % за 5 минут | P1 |
| `api_down` | `GET API_HEALTH_URL` не отвечает 200 два цикла подряд | P1 |
| `backup_stale` | последний удалённый бэкап ClickHouse старше 26 часов | P1 |
| Dokploy «Database Backup» failed | бэкап Postgres не создан | P1 |
| Сертификат | Traefik не продлил Let's Encrypt (видно по ошибке TLS в `ingest_down`) | P1 |

### 2.3. Действия по каждому алерту worker

**`ingest_down`**
1. В Dokploy (через Tailscale) открыть `appmetrica-ingest` → Logs. Ищем panic, ошибку подключения к CH, ошибку загрузки mmdb.
2. `docker service ps appmetrica-ingest --no-trunc` на сервере: смотрим, не крутится ли рестарт-луп.
3. Если сломал последний деплой: Deployments → Rollback на предыдущий образ.
4. Если `clickhouse_down` горит одновременно — идти в раздел 7.2.
5. Проверка: `curl -s -o /dev/null -w '%{http_code}' https://i.appmetrica.online/health` возвращает 200.

**`clickhouse_down`** → раздел 7.2 «ClickHouse не отвечает».

**`disk_free`** → раздел 7.3 «Полный или почти полный диск».

**`insert_errors`**
1. Логи `appmetrica-ingest`: тип ошибки (`TOO_MANY_PARTS`, `MEMORY_LIMIT_EXCEEDED`, timeout).
2. `TOO_MANY_PARTS`: выполнить запрос «активные парты» из 6.1. Если частей много, проверить, что слияния идут (`system.merges`).
   Временно снизить нагрузку нельзя: клиенты ретраят сами, ничего не делаем, пока слияния не догонят.
3. `MEMORY_LIMIT_EXCEEDED`: найти тяжёлый запрос в `system.processes` и убить `KILL QUERY WHERE query_id = '…'`.
4. Если ошибки не уходят 30 минут — откат последнего деплоя ingest или ClickHouse.

**`api_down`**
1. Логи `appmetrica-api`. Частая причина — упавшая миграция при старте (`MIGRATE_ON_START=true`).
2. Ошибка миграции: откатить образ api на предыдущий; миграцию чинить forward-fix PR-ом (раздел 5.4).
3. Ingest при этом работает, данные не теряются; страдает только дашборд.

**`backup_stale`**
1. `docker logs --since 26h $(docker ps -qf name=clickhouse-backup)` — ошибка S3 (ключи, endpoint) или CH.
2. `docker exec $(docker ps -qf name=clickhouse-backup) clickhouse-backup list remote` — какой бэкап последний.
3. Ручной запуск: `docker exec $(docker ps -qf name=clickhouse-backup) clickhouse-backup create_remote --diff-from-remote=<последний> manual-$(date +%Y%m%d)`.
4. Если ошибка в ключах S3 — раздел 7.5 «Ротация ключей S3».

## 3. On-call rotation

- **Primary**: владелец, единственный оператор. Ротации нет.
- **Secondary**: нет. Компенсация — офлайн-очередь клиентов (7 дней) и автоматический откат Swarm при провале healthcheck.
- **Escalation chain**: владелец → Hetzner support (железо, сеть) → Dokploy GitHub issues (баги панели).
- **Handoff**: не применяется. Состояние инцидентов живёт в `docs/incidents/` и `alert_states`.

### 3.1. On-call responsibilities
- P0: реакция в течение 4 часов в бодрствующее время (08:00–23:00 по `settings.owner_timezone`). Ночные P0 разбираются утром:
  клиентская очередь держит данные 7 дней.
- P1: реакция в течение 24 часов.
- Mitigation прежде root cause: сначала откат или перезапуск, расследование потом.
- Post-mortem в `docs/incidents/YYYY-MM-DD-<slug>.md` для каждого P0 и для P1 с потерей больше 30 минут ingest.

### 3.2. On-call tools
- Ноутбук с Tailscale (доступ к SSH сервера и панели Dokploy `http://<tailscale-имя-сервера>:3000`).
- Telegram (алерты worker, Dokploy, healthchecks.io).
- Доступ в Hetzner Robot (firewall, rescue, KVM) и Hetzner Console (Object Storage).
- Этот документ и `deploy/dokploy/README.md`.
- Аппаратный ключ или телефон с passkey для дашборда.

## 4. Incident response playbook

### 4.1. When alert fires

```
1. ПОДТВЕРДИТЬ: открыть Telegram-алерт, отметить время начала в заметке инцидента.
2. ОЦЕНИТЬ ЗОНУ: ingest (данные копятся у клиентов) / api или web (страдает только дашборд) / хранилище (CH, PG, диск).
3. ПРОВЕРИТЬ ОЧЕВИДНОЕ: был ли деплой за последний час (Dokploy → Deployments), не обновлялся ли Dokploy.
4. СМЯГЧИТЬ: откат образа, перезапуск сервиса, освобождение диска — по разделам 2.3 и 7.
5. ПРОВЕРИТЬ: /health зелёный, тестовый curl-батч принят (раздел 5.3), алерт перешёл в «восстановлено».
6. ЗАПИСАТЬ: создать docs/incidents/YYYY-MM-DD-<slug>.md по шаблону раздела 9.
7. ИСПРАВИТЬ ПРИЧИНУ: тикет с меткой incident, ссылка в post-mortem.
```

### 4.2. Mitigation toolkit

**Rollback**:
- Приложения: Dokploy → Application → Deployments → Rollback на предыдущий образ, или из CI
  `POST $DOKPLOY_URL/api/application.saveDockerProvider` с прошлым тегом + `POST $DOKPLOY_URL/api/application.deploy`.
- Автоматический откат: Swarm Update Config `{"Parallelism":1,"Delay":10000000000,"FailureAction":"rollback","Order":"start-first"}`
  откатывает сам, если новый контейнер не проходит healthcheck.
- Миграции БД: **не откатываются**. Только forward-fix новой миграцией (expand/contract, раздел 5.4).

**Feature flag disable**:
- Отдельного сервиса флагов нет (решение для solo-инструмента). Аналоги «выключателя»:
  архивировать приложение (`POST /admin/v1/apps/{app_id}/archive` → ingest отвечает 403 `app_archived`) или
  отозвать конкретный write-ключ (≤ 15 с до вступления в силу).
- Дайджест и алерты выключаются в S-12 (`digest_enabled=false`, `alerts_enabled=false`).

**Rate limiting**:
- Снизить `RATE_APP_EVENTS_PER_SEC` / `RATE_APP_BURST` или `RATE_INSTALL_PER_MIN` / `RATE_INSTALL_BURST` в env `appmetrica-ingest`
  и передеплоить (rolling, без простоя).
- Сбросить нагрузку от мусорного трафика с неверными ключами не нужно: 401 отдаётся до разбора событий и обращения к CH.

**Scaling**:
- Вертикально: сервер AX102-1 имеет 16 ядер и 128 ГБ, ClickHouse ограничен конфигом `deploy/clickhouse/config.d`.
- Горизонтально ingest: увеличить Replicas в Dokploy до 2 (лимиты rate станут поделены между репликами, TD-001).

**Traffic shifting**:
- Временный прокси перед `i.appmetrica.online` (CDN с WAF): смена A-записи на прокси, origin — IP сервера.
  Используется только при DDoS (раздел 7.6).

### 4.3. Status page

> **N/A для этого проекта.** Причина: потребитель статуса — один владелец. Внутренний статус — экран S-12 «Система»
> (`GET /admin/v1/system/status`: ClickHouse, Postgres, последний бэкап, горящие алерты, последние `job_runs`).

## 5. Deployment runbook

### 5.1. Pre-deploy checklist
- [ ] PR смёржен в `main`, CI зелёный (lint, unit, интеграционные тесты на testcontainers, Playwright smoke).
- [ ] Миграции PG и CH проверены на expand/contract-совместимость: старый код работает с новой схемой.
- [ ] В миграции ClickHouse нет `ALTER … MODIFY COLUMN` вместе с другими изменениями (мутация — отдельным релизом вне пика).
- [ ] Свежий бэкап ClickHouse не старше 26 часов (S-12 «Система»).
- [ ] Окно деплоя: будни 10:00–19:00 по `settings.owner_timezone`; ClickHouse и Dokploy не обновляются в пятницу.

### 5.2. Deploy
- `deploy.yml` на push в `main`: сборка образов `ghcr.io/<owner>/appmetrica-backend:<sha>` и `ghcr.io/<owner>/appmetrica-web:<sha>` →
  `tailscale/github-action` (вход в tailnet) → `application.saveDockerProvider` (новый тег) → `application.deploy` для
  `appmetrica-api` (первым, применяет миграции), затем `appmetrica-ingest`, `appmetrica-worker`, `appmetrica-web`.
- Swarm rolling update `start-first`: новая реплика стартует, проходит healthcheck, старая останавливается.
- Healthcheck Go-сервисов в Dokploy (Advanced → Cluster Settings → Swarm Settings, время в наносекундах):
  `{"Test":["CMD","/app/appmetrica","healthcheck","--url","http://127.0.0.1:8081/health"],"Interval":10000000000,"Timeout":3000000000,"StartPeriod":20000000000,"Retries":3}`
  (порт 8080 для api, 8082 для worker).
- Canary не применяется: одна реплика каждого сервиса.

### 5.3. Post-deploy verification
- [ ] `curl -s https://i.appmetrica.online/health` и `curl -s https://api.appmetrica.online/health` возвращают 200.
- [ ] Тестовый батч в служебное приложение `smoke` (environment `development`) принят, `accepted = 2`:
  ```bash
  NOW=$(date -u +%Y-%m-%dT%H:%M:%S.000Z); IID=$(uuidgen); SID=$(uuidgen)
  curl -s https://i.appmetrica.online/v1/events -H "X-App-Key: $SMOKE_PK" -H 'Content-Type: application/json' -d "{
    \"sent_at\":\"$NOW\",\"context\":{\"platform\":\"server\",\"environment\":\"development\"},
    \"events\":[{\"insert_id\":\"$(uuidgen)\",\"name\":\"session_start\",\"timestamp\":\"$NOW\",\"install_id\":\"$IID\",\"session_id\":\"$SID\"},
                {\"insert_id\":\"$(uuidgen)\",\"name\":\"session_end\",\"timestamp\":\"$NOW\",\"install_id\":\"$IID\",\"session_id\":\"$SID\",\"duration_ms\":1000}]}"
  ```
- [ ] Событие видно в S-07 Realtime приложения `smoke` (environment development) не позже чем через 10 секунд.
- [ ] Вход в дашборд по passkey работает, портфель открывается.

### 5.4. Rollback trigger
Любая проверка из 5.3 падает дольше 10 минут → откат образа в Dokploy (Deployments → Rollback).
**Миграции**: если новая миграция уже применена, откатывается только код (схема совместима по правилу expand/contract),
а исправление идёт новой миграцией. Ручной `goose down` на проде запрещён.

### 5.5. Первичный провижининг сервера (выполняется один раз, неделя 2 — E9a)

1. **Заказ**: Hetzner Robot → Dedicated → AX102-1, локация FSN1 (если нет в наличии — HEL1), основной IPv4.
   Решение по ECC — open question OQ-003 в [10-open-questions.md](./10-open-questions.md).
2. **ОС**: Robot → Rescue (Linux 64 bit) → перезагрузка → SSH в rescue → `installimage` с конфигом:
   ```
   DRIVE1 /dev/nvme0n1
   DRIVE2 /dev/nvme1n1
   SWRAID 1
   SWRAIDLEVEL 1
   BOOTLOADER grub
   HOSTNAME appmetrica-prod
   PART swap swap 16G
   PART /boot ext3 1024M
   PART / ext4 all
   IMAGE /root/.oldroot/nfs/install/../images/Ubuntu-2404-noble-amd64-base.tar.gz
   ```
   После перезагрузки: `cat /proc/mdstat` показывает `[UU]` у всех массивов; `apt update && apt -y full-upgrade`;
   `unattended-upgrades` включён; создан пользователь `ops` с sudo и SSH-ключом; `PasswordAuthentication no`, `PermitRootLogin no`.
3. **Tailscale**: `curl -fsSL https://tailscale.com/install.sh | sh && tailscale up --hostname appmetrica-prod`;
   в админке Tailscale отключить key expiry для узла `appmetrica-prod`.
4. **Firewall (Hetzner Robot → Firewall, stateless, ≤ 10 правил на вход, включить фильтр IPv6)**:

   | # | Протокол | Src port | Dst port | TCP flags | Действие | Назначение |
   |---|---|---|---|---|---|---|
   | 1 | ICMP | — | — | — | accept | ping, PMTU |
   | 2 | TCP | — | 80 | — | accept | HTTP-01 Let's Encrypt, редирект |
   | 3 | TCP | — | 443 | — | accept | HTTPS (web, ingest, api) |
   | 4 | UDP | — | 41641 | — | accept | Tailscale direct |
   | 5 | TCP | — | 32768–65535 | ack | accept | ответы на исходящие TCP |
   | 6 | UDP | 53 | 32768–65535 | — | accept | ответы DNS |
   | 7 | UDP | 123 | 32768–65535 | — | accept | ответы NTP |
   | 8 | UDP | 3478 | 32768–65535 | — | accept | STUN Tailscale |
   | 9 | все | — | — | — | discard | всё остальное (22 и 3000 закрыты снаружи) |

   Правила 6–8 уточняют якорь: без них stateless-firewall отбрасывает ответы DNS/NTP/STUN, и сервер не резолвит имена.
   Проверка снаружи: `nmap -Pn -p 22,80,443,3000,8123,9000 <public-ip>` — открыты только 80 и 443.
5. **Dokploy**: `curl -sSL https://dokploy.com/install.sh | sh`; сразу зайти на `http://appmetrica-prod:3000` через Tailscale,
   создать учётку администратора, включить TOTP 2FA; Settings → отключить автообновление; версия не ниже v0.30.8.
6. **Реестр GHCR**: Dokploy → Registry → Name `ghcr`, Registry URL `ghcr.io`, Username — GitHub-логин владельца,
   Password — classic PAT со scope `read:packages` → Test.
7. **S3**: Hetzner Console → Object Storage → бакеты `appmetrica-ch-backup` и `appmetrica-pg-backup` в fsn1 с versioning;
   ключ доступа S3. Dokploy → Settings → Destinations → S3: endpoint `https://fsn1.your-objectstorage.com`, region `fsn1`, бакет `appmetrica-pg-backup`.
8. **Postgres**: Dokploy → Create Database → PostgreSQL, образ `postgres:18.6-trixie`, БД `appmetrica`, пользователь `appmetrica`,
   внутренний хост из вкладки General → в `POSTGRES_URL` всех Go-сервисов. Backups: destination выше, cron `0 3 * * *`, хранить 30.
9. **ClickHouse stack**: Dokploy → Create Compose → источник Git (этот репозиторий), путь `deploy/clickhouse/compose.yaml`,
   переменные `CLICKHOUSE_*`, `S3_*`, `BACKUPS_TO_KEEP_REMOTE=35`, `WATCH_INTERVAL=24h`, `FULL_INTERVAL=168h`.
   Named volumes `clickhouse-data`, `clickhouse-logs`; сеть `dokploy-network` (external). Образы с тегом и `@sha256`-дайджестом.
10. **Applications**: `appmetrica-api`, `appmetrica-ingest`, `appmetrica-worker` (образ `ghcr.io/<owner>/appmetrica-backend:<sha>`,
    Command — `api` / `ingest` / `worker`) и `appmetrica-web` (`ghcr.io/<owner>/appmetrica-web:<sha>`). При создании App Name задаётся
    ровно как в якоре, без автоматического суффикса: на эти имена ссылаются `API_INTERNAL_URL`, `INGEST_HEALTH_URL`, `API_HEALTH_URL`.
    Env — полный список из `.env.example` (секции Go-общие, ingest, api, worker, web). Swarm Settings — healthcheck и Update Config из 5.2.
11. **Домены (Dokploy → Domains, HTTPS, Let's Encrypt)**: `appmetrica.online` → `appmetrica-web:3000`, `i.appmetrica.online` → `appmetrica-ingest:8081`,
    `api.appmetrica.online` → `appmetrica-api:8080`. DNS: A-записи трёх имён на IPv4 сервера.
12. **Первый вход**: Dokploy → `appmetrica-api` → Terminal → `/app/appmetrica admin passkey-link` → открыть ссылку в браузере,
    зарегистрировать passkey на ноутбуке, затем через S-12 добавить второй (телефон или аппаратный ключ).
13. **Уведомления**: Dokploy → Notifications → Telegram (бот и чат владельца): App Deploy, App Build Error, Database Backup,
    Docker Cleanup, Dokploy Restart. healthchecks.io: check `appmetrica-worker`, period 5 минут, grace 10 минут, интеграции Telegram и email;
    ping URL → `HEALTHCHECKS_PING_URL`.
14. **CI**: GitHub Secrets `DOKPLOY_URL` (`http://appmetrica-prod:3000`), `DOKPLOY_API_KEY` (Dokploy → Settings → Profile → API/CLI),
    `DOKPLOY_APP_ID_INGEST`, `DOKPLOY_APP_ID_API`, `DOKPLOY_APP_ID_WORKER`, `DOKPLOY_APP_ID_WEB` (из `GET /api/project.all`),
    `TS_OAUTH_CLIENT_ID`, `TS_OAUTH_SECRET` (Tailscale OAuth client с тегом `tag:ci`).

### 5.6. Плановые обновления Dokploy и ClickHouse

**Dokploy** (security-релизы — в течение 48 часов, остальные — раз в месяц, через 3–5 дней после выхода):
1. Прочитать release notes и GitHub Security Advisories `Dokploy/dokploy`.
2. Свежий бэкап PG и CH (S-12 «Система»).
3. Dokploy → Settings → Update (или `curl -sSL https://dokploy.com/install.sh | sh -s update`).
4. После обновления: `nmap -Pn -p 3000 <public-ip>` снаружи — порт должен быть закрыт (issue #2661 переоткрывает его в Docker,
   но Robot firewall его отбрасывает). Проверить 5.3.

**ClickHouse patch внутри 26.8 LTS** (раз в месяц, если вышел патч с security-фиксом или фиксом бага, который нас касается):
1. Бэкап: `clickhouse-backup create_remote --diff-from-remote=<последний> pre-upgrade-$(date +%Y%m%d)`.
2. CI: прогнать интеграционные тесты с новым тегом (переменная образа в testcontainers).
3. Поменять тег и дайджест в `deploy/clickhouse/compose.yaml`, PR, merge, Dokploy → `clickhouse-stack` → Deploy.
   На время рестарта (30–60 с) ingest отдаёт 503 `storage_unavailable` с `Retry-After`; клиенты ретраят.
4. Проверить `SELECT version()`, 5.3, алерт `insert_errors` не горит 30 минут.

**Переход на следующий LTS** (26.8 → следующая LTS-ветка): отдельный тикет. Прогон всей тестовой матрицы и golden-фикстур на новом образе,
чтение backward-incompatible changes, учебное восстановление бэкапа 26.8 в новую версию на временном контейнере (раздел 6.3), затем шаги patch-обновления.

## 6. Database runbook

### 6.1. Common operations

**ClickHouse — подключение** (только чтение; запись руками на проде запрещена):
```bash
CH=$(docker ps -qf label=com.docker.compose.service=clickhouse)   # контейнер сервиса clickhouse из clickhouse-stack
docker exec -it "$CH" clickhouse-client --user appmetrica_ro --password "$CLICKHOUSE_RO_PASSWORD" --database appmetrica
```

**Свободное место на дисках**:
```sql
SELECT name, formatReadableSize(free_space) AS free, formatReadableSize(total_space) AS total,
       round(free_space / total_space * 100, 1) AS free_pct
FROM system.disks;
```

**Размер таблиц и число активных партов**:
```sql
SELECT table, sum(rows) AS rows, formatReadableSize(sum(bytes_on_disk)) AS size, count() AS active_parts
FROM system.parts WHERE active AND database = 'appmetrica'
GROUP BY table ORDER BY sum(bytes_on_disk) DESC;
```
Норма: `active_parts` у `events` — сотни; больше 3000 на таблицу означает, что слияния не успевают.

**Идущие слияния и мутации**:
```sql
SELECT table, elapsed, progress, num_parts, formatReadableSize(total_size_bytes_compressed) AS size FROM system.merges;
SELECT table, mutation_id, command, create_time, parts_to_do, is_done, latest_fail_reason
FROM system.mutations WHERE database = 'appmetrica' AND is_done = 0;
```
Зависшая мутация (`latest_fail_reason` не пуст, держится дольше 2 часов) → `KILL MUTATION WHERE mutation_id = '<id>'`,
задача `deletion` получит `failed` и будет повторена (attempts ≤ 5).

**Медленные запросы за сутки**:
```sql
SELECT event_time, query_duration_ms, read_rows, formatReadableSize(memory_usage) AS mem, user, substring(query, 1, 200) AS q
FROM system.query_log
WHERE type = 'QueryFinish' AND event_date >= today() - 1 AND query_duration_ms > 1500
ORDER BY query_duration_ms DESC LIMIT 20;
```

**Проверка, что TTL работает** (самая старая дата должна быть не старше 25 месяцев):
```sql
SELECT min(event_date), max(event_date) FROM appmetrica.events;
SELECT partition, min(min_date), sum(rows) FROM system.parts
WHERE table = 'events' AND active GROUP BY partition ORDER BY partition LIMIT 3;
```
Если старые партиции не удалились — проверить `merge_with_ttl_timeout` и выполнить `ALTER TABLE appmetrica.events MATERIALIZE TTL`
(мутация, вне пика, под пользователем `appmetrica_rw`).

**OPTIMIZE FINAL — только по необходимости**: `OPTIMIZE TABLE appmetrica.events FINAL` переписывает все парты таблицы (сотни ГБ),
занимает часы и требует свободного места размером с таблицу. Метрики не зависят от дедупликации (uniqExact), поэтому OPTIMIZE не нужен.
Допустимо только для одной партиции после массового спама: `OPTIMIZE TABLE appmetrica.events PARTITION 202611 FINAL`.

**Postgres — диагностика**:
```sql
SELECT count(*) FROM pg_stat_activity;
SELECT pid, now() - query_start AS dur, left(query, 120) FROM pg_stat_activity
WHERE state = 'active' AND query_start < now() - interval '1 minute';
SELECT relname, last_autovacuum, n_dead_tup FROM pg_stat_user_tables ORDER BY n_dead_tup DESC LIMIT 10;
SELECT status, count(*) FROM deletion_requests GROUP BY status;
SELECT job, status, max(started_at) FROM job_runs GROUP BY job, status ORDER BY job;
```
**Kill runaway query**: `SELECT pg_terminate_backend(<pid>);`

### 6.2. Failover

> **N/A для этого проекта.** Причина: реплик нет ни у ClickHouse, ни у Postgres (один сервер, TD-004).
> Замена отказавшей БД — восстановление из бэкапа (6.3), полная потеря сервера — раздел 8.

### 6.3. Backup restoration

**ClickHouse — восстановление на проде (`restore_remote`)**:
1. Остановить приём: Dokploy → `appmetrica-ingest` → Replicas 0. Клиенты получают ошибки соединения и копят очередь до 7 дней.
   Остановить `appmetrica-worker` (Replicas 0), чтобы `close_day` и `deletion` не писали в CH.
2. Список бэкапов: `docker exec $(docker ps -qf name=clickhouse-backup) clickhouse-backup list remote`.
   Выбрать последний успешный (инкрементальные бэкапы восстанавливаются цепочкой автоматически).
3. Восстановить схему и данные с удалением текущих таблиц:
   `docker exec $(docker ps -qf name=clickhouse-backup) clickhouse-backup restore_remote --rm <backup_name>`.
4. Проверка:
   ```sql
   SELECT count(), max(received_at) FROM appmetrica.events;
   SELECT name FROM system.tables WHERE database = 'appmetrica' ORDER BY name;
   ```
   Должны присутствовать `events`, `install_first_seen`, `user_first_seen`, `identity_links`, `rejected_events`, `daily_stats`,
   три MV (`install_first_seen_mv`, `user_first_seen_mv`, `identity_links_mv`) и таблица версий goose.
5. Вернуть Replicas 1 для `appmetrica-ingest` и `appmetrica-worker`. Клиенты досылают очереди; события с момента бэкапа
   до остановки, которые клиенты уже удалили из очереди (получили 200), потеряны — это и есть RPO 24 ч.
6. Записать инцидент (раздел 9).

**ClickHouse — ежемесячное учебное восстановление (первая неделя месяца, на том же сервере, без влияния на прод)**:
```bash
docker network create ch-drill
docker volume create ch-drill-data
docker run -d --name ch-drill --network ch-drill -v ch-drill-data:/var/lib/clickhouse \
  -e CLICKHOUSE_USER=drill -e CLICKHOUSE_PASSWORD=drill clickhouse/clickhouse-server:26.8.25.14
docker run --rm --network ch-drill -v ch-drill-data:/var/lib/clickhouse \
  -e CLICKHOUSE_HOST=ch-drill -e CLICKHOUSE_USERNAME=drill -e CLICKHOUSE_PASSWORD=drill \
  -e REMOTE_STORAGE=s3 -e S3_ENDPOINT=https://fsn1.your-objectstorage.com -e S3_REGION=fsn1 \
  -e S3_BUCKET=appmetrica-ch-backup -e S3_ACCESS_KEY="$S3_ACCESS_KEY" -e S3_SECRET_KEY="$S3_SECRET_KEY" \
  altinity/clickhouse-backup:2.8.1 restore_remote <backup_name>
docker exec ch-drill clickhouse-client --user drill --password drill \
  -q "SELECT count() FROM appmetrica.events WHERE event_date < today() - 1"
docker rm -f ch-drill && docker volume rm ch-drill-data && docker network rm ch-drill
```
Сравнить `count()` с продом на тех же датах (расхождение допускается только из-за удалений GDPR после бэкапа).
Время восстановления записать в `docs/ops/restore-log.md`. Если оно больше 2 часов, RTO 4 ч под угрозой: заводим тикет.

**Postgres — восстановление**:
1. Остановить `appmetrica-api` и `appmetrica-worker` (Replicas 0); ingest продолжает работать на кэше ключей
   (последняя загрузка), но новые ключи и приложения не появятся до восстановления.
2. Dokploy → Database → Backups → выбрать бэкап → Restore. Если восстановление из UI недоступно — вручную:
   скачать файл из бакета `appmetrica-pg-backup` (`aws s3 cp s3://appmetrica-pg-backup/<file> . --endpoint-url https://fsn1.your-objectstorage.com`),
   для `.sql.gz` — `gunzip -c <file> | docker exec -i <pg-container> psql -U appmetrica -d appmetrica`,
   для custom-формата — `docker exec -i <pg-container> pg_restore -U appmetrica -d appmetrica --clean --if-exists < <file>`.
3. Проверить `SELECT count(*) FROM apps; SELECT count(*) FROM write_keys WHERE revoked_at IS NULL; SELECT count(*) FROM passkeys;`.
4. Вернуть сервисы; `api` при старте применит миграции, если бэкап старше последней миграции.
5. Ключи, созданные после бэкапа, потеряны: перевыпустить в S-11 и S-12.

### 6.4. Data corruption
1. Остановить запись: `appmetrica-ingest` Replicas 0 (клиенты копят очередь), `appmetrica-worker` Replicas 0.
2. Определить масштаб: какие таблицы, какие `app_id`, диапазон `received_at`, `write_key_id` (для спама).
3. Точечное исправление вместо восстановления, если масштаб ограничен:
   спам через утёкший ключ — `DELETE FROM appmetrica.events WHERE app_id = '<app>' AND write_key_id = '<key>' AND received_at BETWEEN '<from>' AND '<to>'`
   (под `appmetrica_rw`) + отзыв ключа в S-11; затем пересчитать `daily_stats` за затронутые закрытые дни, удалив строки
   `closed_days` и дав `close_day` пересчитать их.
4. Иначе — восстановление ClickHouse по 6.3.
5. Вернуть запись, проверить 5.3, написать post-mortem.

## 7. Service-specific runbooks

### 7.1. Ingest отвечает 5xx
**Symptom**: алерт `ingest_down` или `insert_errors`, в «Интеграции» провал accepted.

**Quick diagnostics**:
1. `curl -sv https://i.appmetrica.online/health` — TLS, Traefik, процесс.
2. Логи `appmetrica-ingest`: 503 `storage_unavailable` → проблема CH (7.2); 500 `internal_error` → баг кода.
3. Был ли деплой за последний час.

**Mitigation**:
- Баг после деплоя → Rollback образа.
- CH недоступен → 7.2; ingest при этом корректно отдаёт 503 с `Retry-After`, данные не теряются.
- Ошибка mmdb (повреждённая гео-база после `geo-refresh.yml`) → откат на предыдущий образ, перезапуск workflow.

### 7.2. ClickHouse не отвечает
**Symptom**: алерты `clickhouse_down`, `ingest_down`, ответы 503.

**Quick diagnostics**:
1. `docker ps -a --filter name=clickhouse` — контейнер жив или в рестарт-лупе.
2. `docker logs --tail 200 <clickhouse-container>` — `Cannot allocate memory`, `No space left on device`, ошибка конфигурации после обновления.
3. `df -h /var/lib/docker` и `free -g`.

**Mitigation**:
- Нет места → 7.3.
- Нет памяти → проверить, что в `deploy/clickhouse/config.d` задан `max_server_memory_usage_to_ram_ratio` (0.6), перезапустить compose.
- Ошибка после обновления образа → вернуть прошлый тег и дайджест в compose, Deploy.
- Повреждение партов (`checksum mismatch` в логах) → ClickHouse переносит битые парты в `detached/`; данные этих партов восстанавливаются из бэкапа (6.3).

### 7.3. Полный или почти полный диск
**Symptom**: алерт `disk_free` (< 20 %), в худшем случае `No space left on device`.

**Quick diagnostics**:
1. `df -h`, `du -sh /var/lib/docker/volumes/*clickhouse-data*`.
2. Запрос размера таблиц (6.1); `SELECT formatReadableSize(sum(bytes_on_disk)) FROM system.detached_parts;`.
3. `du -sh /var/lib/docker/volumes/*clickhouse-data*/_data/shadow` — остатки FREEZE от бэкапов.
4. `docker system df` — образы и build cache.

**Mitigation (по порядку, от безопасного к рискованному)**:
1. `docker image prune -a --filter "until=168h"` и `docker builder prune -af` (образы на сервере не собираются, кэш не нужен).
2. Очистить `shadow/`: `docker exec $(docker ps -qf name=clickhouse-backup) clickhouse-backup clean`.
3. Удалить локальные бэкапы: `clickhouse-backup list local` → `clickhouse-backup delete local <name>`
   (при `backups_to_keep_local: -1` их быть не должно — проверить конфиг).
4. Удалить `detached`-парты, если они старше 7 дней и не нужны: `ALTER TABLE appmetrica.events DROP DETACHED PART '<name>' SETTINGS allow_drop_detached = 1`.
5. Временно сократить TTL `rejected_events` нельзя без миграции; при острой нехватке — `ALTER TABLE appmetrica.rejected_events DROP PARTITION '<YYYY-MM-DD>'` за старые дни.
6. Если место заканчивается из-за роста данных — тикет на увеличение диска (дополнительные NVMe в Robot) или сокращение TTL `events` миграцией.

### 7.4. Потеря доступа к дашборду (все passkey недоступны)
**Symptom**: на `/login` нет подходящего passkey.

**Mitigation**:
1. Tailscale → панель Dokploy → `appmetrica-api` → Terminal (или `docker exec -it $(docker ps -qf name=appmetrica-api) /app/appmetrica admin passkey-link`).
2. Команда печатает `https://appmetrica.online/setup/<token>`, ссылка живёт 15 минут и одноразовая.
3. Зарегистрировать новый passkey, затем в S-12 → Passkeys удалить утерянные.
4. Если нет и Tailscale-доступа → break-glass (7.7).

### 7.5. Ротация ключей и секретов

| Секрет | Когда | Процедура | Последствия |
|---|---|---|---|
| `KEY_HASH_PEPPER` | только при подозрении на утечку вместе с дампом PG | новое значение (32 байта, base64) в env `appmetrica-api` → деплой → **все** `sk_`-ключи перестают проходить проверку (HMAC не совпадает) → выпустить новые `sk_` в S-12, обновить скрипты, отозвать старые | Все интеграции Read API получают 401 до замены ключей. `pk_` не затронуты (хранятся открыто), сессии не затронуты (sha256 без pepper) |
| `sk_`-ключ | утечка, увольнение подрядчика, раз в 12 месяцев | S-12 → API-ключи → создать новый → обновить скрипт → Revoke старого | Отозванный ключ получает 401 `unauthenticated` сразу: api проверяет `sk_` по `api_keys.key_hash` в Postgres на каждый запрос, кэша нет |
| `pk_`-ключ | только при спаме через ключ | S-11 → создать второй ключ → выпустить версии приложений с новым → когда старые версии ушли или спам критичен → Revoke | Отзыв ломает приём от всех версий со старым ключом. Без спама старые ключи не отзываются |
| `TELEGRAM_BOT_TOKEN` | утечка | @BotFather → `/revoke` → новый токен в env `appmetrica-worker` → деплой → S-12 → «Отправить тестовое сообщение» (`POST /admin/v1/settings/telegram/test`). Также обновить токен в Dokploy Notifications | Алерты не приходят между отзывом и деплоем |
| S3-ключи (`S3_ACCESS_KEY`, `S3_SECRET_KEY`) | утечка, раз в 12 месяцев | Hetzner Console → новый ключ → env `clickhouse-stack` + Dokploy Destination → redeploy compose → `clickhouse-backup list remote` работает → удалить старый ключ | Бэкапы не создаются, пока ключ не обновлён (алерт `backup_stale` через 26 ч) |
| Пароли ClickHouse (`appmetrica_rw`, `appmetrica_ro`, backup-пользователь) | утечка | новый `password_sha256_hex` в `deploy/clickhouse/users.d/*.xml` + env сервисов → деплой compose, затем ingest, worker, api | Сервисы со старым паролем получают ошибку авторизации до своего деплоя |
| `DOKPLOY_API_KEY` | утечка, раз в 6 месяцев | Dokploy → Profile → API/CLI → новый ключ → GitHub Secret → удалить старый | CI не деплоит до обновления секрета |

### 7.6. DDoS или поток мусора на ingest
**Symptom**: рост CPU, 429 `app_rate_limited`, всплеск 401 в логах, резкий рост «новых» в одном приложении.

**Quick diagnostics**:
1. Логи ingest: доля 401 (`invalid_app_key`) — мусор без ключа; доля 429 — поток по реальному ключу.
2. «Интеграция» приложения: всплеск по одной `app_version` или по одному ключу.

**Mitigation**:
- Поток без ключа: обрабатывается дёшево (401 до разбора тела), действий не требуется; Hetzner фильтрует объёмные L3/L4-атаки.
- Поток по реальному `pk_`: снизить `RATE_APP_*` (4.2), отозвать скомпрометированный ключ по 7.5, удалить мусор по 6.4.
- L7-флуд, который сервер не выдерживает: временно поставить CDN/WAF-прокси перед `i.appmetrica.online` (смена A-записи),
  а после атаки вернуть прямую запись.

### 7.7. Break-glass: нет доступа через Tailscale
**Symptom**: узел `appmetrica-prod` offline в Tailscale, SSH и панель недоступны.

**Mitigation**:
1. Hetzner Robot → Firewall → временное правило «TCP dst 22, src <мой текущий IP>/32, accept» выше правила discard
   (вступает через 20–30 секунд).
2. `ssh ops@<public-ip>`, `systemctl status tailscaled`, `tailscale up`.
3. Если SSH не отвечает — Robot → Reset или KVM-консоль (заказ в Robot), либо Rescue System для починки.
4. **Удалить временное правило firewall сразу после восстановления Tailscale.**

## 8. Disaster recovery (DR)

### 8.1. Scenarios

| Scenario | RTO | RPO | Action |
|---|---|---|---|
| Отказ одного NVMe | 0 (RAID1 продолжает работу) | 0 | Robot → заявка на замену диска, `mdadm --manage /dev/md<N> --add` после замены |
| Повреждение таблиц ClickHouse | 2 ч | 24 ч | `restore_remote` (6.3) |
| Повреждение Postgres | 1 ч | 24 ч | восстановление PG (6.3) |
| Полная потеря сервера | 4 ч | 24 ч | чек-лист 8.4 |
| Потеря домена `appmetrica.online` | зависит от наличия резервного домена (OQ-002) | 0 при fallback-домене | новые A-записи резервного домена, домены в Dokploy |
| Компрометация сервера | 6 ч | 24 ч | новый сервер по 8.4, ротация всех секретов по 7.5, старый сервер — Rescue и снятие образа для анализа |

### 8.2. DR drills
- **Ежемесячно**: учебное восстановление ClickHouse (6.3) и Postgres на временный контейнер, запись в `docs/ops/restore-log.md`.
- **Раз в полгода**: «бумажный» прогон чек-листа 8.4 с проверкой, что все секреты есть в менеджере паролей владельца,
  а `deploy/dokploy/README.md` актуален.

### 8.3. Business continuity plan (BCP)
- Данные не теряются при простое до 7 дней: клиентская очередь и окно досыла `INGEST_LATE_WINDOW=168h`.
- Все секреты (env Dokploy, S3, Telegram, Dokploy API, Tailscale OAuth, GHCR PAT) хранятся в менеджере паролей владельца,
  не только на сервере.
- Альтернативный канал алертов при недоступности Telegram — email от healthchecks.io.

### 8.4. Чек-лист «полная потеря сервера» (RTO 4 ч)

| Время | Шаг |
|---|---|
| 0:00–0:30 | Заказать новый AX102-1 (FSN1 или HEL1); пока сервер ставится — скачать последние бэкапы (проверить список `appmetrica-ch-backup`, `appmetrica-pg-backup`) |
| 0:30–1:15 | 5.5 шаги 2–5: installimage, пользователь `ops`, Tailscale, firewall (скопировать правила), Dokploy |
| 1:15–1:45 | 5.5 шаги 6–9: GHCR, S3 destination, Postgres (восстановление из бэкапа по 6.3), compose `clickhouse-stack` |
| 1:45–2:45 | ClickHouse `restore_remote` последнего бэкапа (на пустой сервер без `--rm`) |
| 2:45–3:15 | 5.5 шаг 10: Applications с последними тегами из GHCR (тот же env из менеджера паролей) |
| 3:15–3:30 | 5.5 шаг 11: DNS A-записи трёх имён на новый IP (TTL записей заранее 300 с), домены в Dokploy, сертификаты |
| 3:30–4:00 | Проверка 5.3, вход по passkey (passkey привязаны к RP ID `appmetrica.online` и переживают смену сервера), уведомления 5.5 шаг 13, обновление GitHub Secrets с новыми App ID Dokploy |

После восстановления клиенты сами досылают очереди за время простоя; события между последним бэкапом и аварией,
уже подтверждённые сервером, потеряны (RPO 24 ч).

## 9. Post-mortem template

```markdown
# Post-mortem: <короткое название>

**Дата**: YYYY-MM-DD
**Длительность**: X мин (от первого алерта до «восстановлено»)
**Влияние**: задержка данных у N приложений / недоступен дашборд / потеряно M подтверждённых событий
**Severity**: P0 / P1
**Автор**: владелец
**Статус**: Draft / Closed

## Summary
1–2 предложения: что произошло.

## Timeline (время в UTC)
- HH:MM — первый алерт (какой: ingest_down / clickhouse_down / disk_free / healthchecks.io)
- HH:MM — начат разбор
- HH:MM — применено смягчение (откат / перезапуск / очистка диска)
- HH:MM — восстановлено (алерт «восстановлено» пришёл)

## Impact
- Данные: были ли потеряны подтверждённые события (да/нет, оценка).
- Ingest: минуты 5xx; клиенты досылали очередь до HH:MM.
- Дашборд: минуты недоступности.

## Root cause
Техническое объяснение.

## Trigger
Что запустило инцидент (деплой, обновление Dokploy/ClickHouse, рост данных, внешний трафик).

## Resolution
Что сработало.

## Detection
Как узнали (алерт worker / healthchecks.io / Dokploy / заметил сам). Сколько прошло от начала до алерта.

## Action items
- [ ] AI-001 (P0, владелец, YYYY-MM-DD): …
- [ ] AI-002 (P1, владелец, YYYY-MM-DD): …

## Lessons learned
Что сработало, что нет, что сделать иначе.

## Blameless note
Разбор без поиска виноватых: фокус на системе и процессах.
```

## 10. Chaos engineering (if Tier 4)

> **N/A в полном объёме.** Причина: Tier 1–2, один сервер, отдельного staging нет. Вместо chaos-инструментов —
> облегчённые учения на проде раз в квартал в рабочее время, с контролем по разделу 5.3:
> - Остановить `appmetrica-ingest` на 10 минут (Replicas 0): проверить, что пришёл `ingest_down`, а клиенты досылают очередь
>   (рост accepted после возврата на экране «Интеграция»).
> - Остановить `appmetrica-worker` на 20 минут: проверить, что healthchecks.io прислал «down».
> - Ежемесячное восстановление из бэкапа (6.3) — основное учение на устойчивость данных.

---

## 11. Runbook maintenance

- Ревью документа раз в 6 месяцев и после каждого P0/P1 (уроки из post-mortem вносятся сюда).
- После каждого ручного изменения в Dokploy (новая переменная, домен, healthcheck) — обновить `deploy/dokploy/README.md`
  и раздел 5.5 этого документа в том же PR.
- Учения 6.3 и 10 проверяют, что команды из документа работают на текущих версиях (ClickHouse 26.8.x, clickhouse-backup 2.8.1, Dokploy).
- Документ остаётся action-oriented: каждая процедура — команды и проверки, без пересказа архитектуры.

---

Расширять документ новыми runbook-ами по мере роста системы: кастомные события (P1), второй ingest-узел в РФ (OQ-001),
переход на следующую LTS ClickHouse.
