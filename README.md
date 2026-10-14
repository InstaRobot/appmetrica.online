# appmetrica.online

**English** | [Русский](README.ru.md)

**Live demo: [appmetrica.online](https://appmetrica.online)** — open the [read-only demo](https://appmetrica.online/demo)
on a synthetic dataset, no sign-up needed.

Open-source, self-hosted analytics for mobile apps, websites, bots and backends — **without an SDK**.
Your apps send `session_start` / `session_end` events with a plain HTTP request; you get a portfolio of all
apps, audience (DAU/WAU/MAU, new users, sessions, duration, breakdowns), classic retention cohorts, realtime,
user profiles and a Read API. Users are counted as people, not devices (`install_id` + optional `user_id`).

- **No SDK.** One documented endpoint: `POST https://<ingest-domain>/v1/events`, batches of up to 500 events,
  idempotent by `insert_id`, offline queues up to 7 days. Recipes for Swift, Kotlin, JavaScript, Go, curl and bots
  are in the public [documentation](https://appmetrica.online/docs).
- **One app, many platforms.** An app can ship on iOS, Android, the web and more: each platform gets its own write
  key, reports count people across platforms with a platform switcher on top, and duplicate apps can be merged.
- **Privacy by design.** The client IP is used only in memory for a country/city lookup and is never stored —
  not in the databases, not in logs. `user_id` values that look like an e-mail or phone number are rejected.
  GDPR deletion by API or from the dashboard.
- **Passkey-only sign-in.** No passwords to leak; the first passkey is registered with a one-time link from the CLI.
- **Light and dark themes**, a public landing page and a read-only [demo mode](https://appmetrica.online/demo) on a synthetic dataset.
- **Fast on one server.** ClickHouse for events, PostgreSQL for metadata, Go services, Next.js dashboard.

> Not affiliated with Yandex. "AppMetrica" is a trademark of its owner; this project is an independent
> open-source alternative.

## Contents

- [Architecture](#architecture)
- [Requirements](#requirements)
- [Installation with Docker Compose](#installation-with-docker-compose)
- [Deploying with Dokploy](#deploying-with-dokploy)
- [Configuration reference](#configuration-reference)
- [Sending your first events](#sending-your-first-events)
- [Platforms and merging apps](#platforms-and-merging-apps)
- [Operations](#operations)
- [Development](#development)
- [Documentation](#documentation)
- [License](#license)

## Architecture

```
            your apps ──► https://INGEST_DOMAIN/v1/events ─┐
                                                           │        ┌────────────┐
 browser ──► https://APP_DOMAIN  (dashboard, landing, docs)├─ Caddy ─┤  ingest    ├──► ClickHouse (events)
 scripts ──► https://API_DOMAIN/v1 (Read API, sk_ keys)    ┘ (HTTPS) │  api       ├──► PostgreSQL (apps, keys,
                                                                    │  worker    │     passkeys, settings)
                                                                    │  web       │
                                                                    └────────────┘
```

Under [Dokploy](#deploying-with-dokploy), Dokploy's Traefik takes Caddy's place.

| Service | Role |
|---|---|
| `caddy` | Reverse proxy on ports 80/443, automatic HTTPS certificates from Let's Encrypt (Docker Compose stack only) |
| `web` | Next.js dashboard, public landing page, public API documentation, demo mode |
| `api` | Dashboard API and public Read API; applies database migrations on start |
| `ingest` | Event ingestion: validation, clock correction, deduplication, geo lookup, rate limits |
| `worker` | Background jobs: closing days into aggregates, Telegram digest and alerts, GDPR deletions, cleanup |
| `clickhouse` | Event storage (raw events kept 25 months, daily aggregates forever) |
| `postgres` | Metadata: apps, write/read keys, passkeys, sessions, settings, deletion requests |
| `clickhouse-backup` | Optional: incremental ClickHouse backups to any S3-compatible storage |

`api`, `ingest` and `worker` are one Go binary (`backend/`) started with different roles.

## Requirements

- A Linux server (x86_64 or arm64) with a public IP address and [Docker Engine](https://docs.docker.com/engine/install/)
  with the Compose v2 plugin (`docker compose version` must work).
- Resources: **2 vCPU, 4 GB RAM, 20 GB SSD** is enough for a small portfolio; **4 vCPU, 8 GB RAM** is recommended.
  The first build of the images needs about 2 GB of free RAM and internet access (Go modules, npm packages,
  the DB-IP geo database). Storage grows by roughly 10 GB per year per 100 000 daily active users.
- Three DNS records (A and/or AAAA) pointing to the server, for example:

  | Record | Purpose |
  |---|---|
  | `analytics.example.com` | dashboard (`APP_DOMAIN`) |
  | `i.analytics.example.com` | event ingestion (`INGEST_DOMAIN`) — this URL goes into your apps |
  | `api.analytics.example.com` | Read API (`API_DOMAIN`) |

- Inbound ports **80** and **443** (TCP; also 443/UDP for HTTP/3) open to the internet — Let's Encrypt needs them
  to issue certificates. No other port has to be exposed.

> The ingest URL is compiled into your apps. Pick a domain you will keep for years.

## Installation with Docker Compose

### 1. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
docker compose version
```

(Any official installation method works; see the [Docker docs](https://docs.docker.com/engine/install/).)

### 2. Get the code

```bash
git clone https://github.com/InstaRobot/appmetrica.online.git
cd appmetrica.online/deploy/selfhost
```

### 3. Create the configuration

```bash
cp .env.example .env
chmod 600 .env
```

Generate all secrets and the geo database month in one go (GNU sed, i.e. on the Linux server):

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

Then open `.env` and set your domains and e-mail:

```dotenv
APP_DOMAIN=analytics.example.com
INGEST_DOMAIN=i.analytics.example.com
API_DOMAIN=api.analytics.example.com
ACME_EMAIL=you@example.com
```

Everything else is optional; every variable is described in [Configuration reference](#configuration-reference)
and in comments inside `.env.example`. Keep a copy of `.env` in a password manager: the database passwords
and `KEY_HASH_PEPPER` are needed to restore a backup.

### 4. Build and start

```bash
docker compose up -d --build --wait
docker compose ps
```

The first build takes 5–15 minutes. `--wait` returns when every service is healthy. On start, `api` applies the
Postgres and ClickHouse migrations; `ingest`, `worker` and `web` start after it. Caddy requests the certificates
automatically on the first HTTPS request (check `docker compose logs caddy` if a domain is not ready).

Check that the services answer:

```bash
curl -fsS https://api.analytics.example.com/health     # {"status":"ok"}
curl -fsS https://i.analytics.example.com/health       # {"status":"ok"}
```

### 5. Register the first passkey

There are no passwords. Create a one-time sign-up link (valid for 15 minutes, single use):

```bash
docker compose exec api /app/appmetrica admin passkey-link
```

Open the printed `https://APP_DOMAIN/setup/…` link in the browser, name the device and confirm with Touch ID,
Face ID, Windows Hello or a security key. You are signed in. Add more passkeys (laptop, phone, hardware key) in
**Settings → Passkeys**. If you ever lose all of them, run the same command again.

### 6. Connect your first app

1. **Portfolio → Add app**: name, one or more platforms, time zone. A write key `pk_…` is created for each
   platform.
2. The **Integration** tab shows the key, ready-to-copy snippets (curl, Swift, Kotlin, JavaScript, Go) and waits for
   the first event.
3. Send a test batch (see [Sending your first events](#sending-your-first-events)) — the status turns to
   "received" and the app appears in the portfolio.

## Deploying with Dokploy

[Dokploy](https://dokploy.com) builds the same stack from Git and serves it through its own Traefik, so there is
no Caddy. The public instance at [appmetrica.online](https://appmetrica.online) runs this way. The compose file is
[`deploy/dokploy/docker-compose.yml`](deploy/dokploy/docker-compose.yml); tested with Dokploy v0.30.8.

The [requirements](#requirements) are the same: a server with Dokploy installed, three DNS records pointing to it
and ports 80/443 open.

### 1. Create the Compose service

**Project → Create Service → Compose**, type **Docker Compose**. On the **General** tab:

| Field | Value |
|---|---|
| Provider | GitHub (or Git): this repository or your fork |
| Branch | `main` |
| Compose Path | `./deploy/dokploy/docker-compose.yml` |
| Trigger Type | On Push (autodeploy) |
| Watch Paths | `backend/**`, `web/**`, `deploy/**`, so README changes do not trigger a rebuild |

Click **Save**.

### 2. Fill in the environment

On the **Environment** tab set the variables from [Configuration reference](#configuration-reference), except
`ACME_EMAIL`, `HTTP_PORT` and `HTTPS_PORT`: Traefik takes care of certificates and ports. Keep
**Create Environment File** on, so Dokploy writes the variables to `.env` next to the compose file.

Generate the block on your machine, replace the domains and paste it:

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

Click **Save** and keep a copy in a password manager. The database passwords are applied on the first start with
empty volumes; changing them later needs `ALTER USER`, not just a new value here.

### 3. Add the domains

On the **Domains** tab add three domains with HTTPS on and the Let's Encrypt certificate provider. If the
service list is empty, press the refresh button next to **Service Name**: Dokploy reads the services from the
repository.

| Host | Service | Container port |
|---|---|---|
| `APP_DOMAIN` | `appmetrica-web` | `3000` |
| `INGEST_DOMAIN` | `appmetrica-ingest` | `8081` |
| `API_DOMAIN` | `appmetrica-api` | `8080` |

Services carry the `appmetrica-` prefix because Dokploy attaches every service with a domain to the shared
`dokploy-network`, where a short name like `api` could resolve to another project's container.

### 4. Deploy

Click **Deploy**. The first build takes 5–15 minutes: the Go binary, the Next.js dashboard and the DB-IP database
are built on the server.

> Do not open **Preview Compose** or refresh the service list in the domain dialog while a deployment is running.
> Both re-clone the repository into the directory the build runs from, and the build fails with
> `getwd: no such file or directory`. If that happens, deploy again.

Check that the services are ready:

```bash
curl -fsS https://api.analytics.example.com/ready    # {"checks":{"clickhouse":"ok","postgres":"ok"},"status":"ready"}
curl -fsS https://i.analytics.example.com/ready
```

### 5. Register the first passkey

The backend image is distroless and has no shell, so Dokploy's **Open Terminal** cannot attach to it. Run the
command on the server over SSH:

```bash
docker exec $(docker ps -qf name=appmetrica-api) /app/appmetrica admin passkey-link
```

Then continue with [Connect your first app](#6-connect-your-first-app).

### Operations under Dokploy

- **Updates.** With autodeploy on, every push that touches the watched paths rebuilds the stack; otherwise click
  **Deploy**. Autodeploy does not wait for CI. `api` applies the migrations on start.
- **Geo database.** Set `GEO_MONTH` to the current month on the **Environment** tab and deploy.
- **Logs.** The **Logs** tab of the service, or `docker logs` on the server.
- **Postgres backups.** On the **Backups** tab add a PostgreSQL backup of the `appmetrica-postgres` service
  (database and user `appmetrica`) to an S3 destination from **Settings → S3 Destinations**.
- **ClickHouse backups.** On the **Environment** tab set `COMPOSE_PROFILES=backup`,
  `BACKUP_API_URL=http://appmetrica-clickhouse-backup:7171` and the `S3_*` variables, then deploy.
- **Lost access.** Run the `passkey-link` command from step 5 again.

An alternative pipeline, where GitHub Actions builds images into GHCR and rolls them out through the Dokploy API
over Tailscale (`.github/workflows/deploy.yml`), is described in [`deploy/dokploy/README.md`](deploy/dokploy/README.md)
(in Russian).

## Configuration reference

All variables live in `deploy/selfhost/.env`. Values are read by `docker-compose.yml`; changing them requires
`docker compose up -d` (and `--build` for variables marked *build time*). Under Dokploy the same variables are set
on the **Environment** tab and applied with **Deploy**.

### Required

| Variable | Example | Description | How to generate |
|---|---|---|---|
| `APP_DOMAIN` | `analytics.example.com` | Dashboard, landing and docs domain. Also the passkey (WebAuthn) relying party: passkeys are bound to it, so changing it later requires registering passkeys again | — |
| `INGEST_DOMAIN` | `i.analytics.example.com` | Event ingestion domain used by your apps. *Build time* for the dashboard snippets | — |
| `API_DOMAIN` | `api.analytics.example.com` | Read API domain. *Build time* for the dashboard docs | — |
| `ACME_EMAIL` | `you@example.com` | E-mail for Let's Encrypt (expiry and incident notices). Not used under Dokploy | — |
| `POSTGRES_PASSWORD` | 48 hex chars | Password of the `appmetrica` Postgres user | `openssl rand -hex 24` |
| `CLICKHOUSE_ADMIN_PASSWORD` | 48 hex chars | ClickHouse `appmetrica_admin`: migrations and backups | `openssl rand -hex 24` |
| `CLICKHOUSE_RW_PASSWORD` | 48 hex chars | ClickHouse `appmetrica_rw`: used by ingest and worker | `openssl rand -hex 24` |
| `CLICKHOUSE_RO_PASSWORD` | 48 hex chars | ClickHouse `appmetrica_ro`: read-only user of api | `openssl rand -hex 24` |
| `KEY_HASH_PEPPER` | base64, ≥ 32 bytes | HMAC pepper for Read API keys (`sk_…` are stored only as hashes). Changing it invalidates every `sk_` key | `openssl rand -base64 32` |
| `GEO_MONTH` | `2026-10` | Month of the [DB-IP City Lite](https://db-ip.com/db/download/ip-to-city-lite) database downloaded during the backend build. *Build time*. If the build fails with 404 early in the month, use the previous month | `date +%Y-%m` |

Use hex passwords (`openssl rand -hex`) for Postgres: they go into a connection URL and must not contain `/`, `@`
or `:`.

### Notifications and monitoring (optional)

| Variable | Default | Description |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | empty | Token from [@BotFather](https://t.me/BotFather). Enables the daily portfolio digest and system alerts (disk space, insert errors, stale backups, services down). The chat ID, digest time and alert switch are set in **Settings → Telegram**. Empty — messages are written to the worker log |
| `HEALTHCHECKS_PING_URL` | empty | [healthchecks.io](https://healthchecks.io) (or compatible) URL. The worker pings it every 5 minutes; silence means the server or the worker is down |
| `DISK_FREE_ALERT_PCT` | `20` | Alert when free space on the ClickHouse volume drops below this percentage (1–99) |

### ClickHouse backups to S3 (optional)

Any S3-compatible storage works (AWS S3, Hetzner Object Storage, Cloudflare R2, MinIO, Backblaze B2).

| Variable | Default | Description |
|---|---|---|
| `COMPOSE_PROFILES` | empty | Set to `backup` to start the `clickhouse-backup` service |
| `BACKUP_API_URL` | empty | Set to `http://clickhouse-backup:7171` (under Dokploy `http://appmetrica-clickhouse-backup:7171`) so the worker checks backup freshness and **Settings → System** shows the last backup |
| `S3_ENDPOINT` | — | Storage endpoint, e.g. `https://s3.eu-central-1.amazonaws.com` |
| `S3_BUCKET` | — | Existing bucket for the backups (path `clickhouse/` inside it) |
| `S3_REGION` | — | Bucket region |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | — | Access key with read/write permissions to the bucket |
| `BACKUPS_TO_KEEP_REMOTE` | `35` | Number of remote backups to keep (≈ 5 weeks of daily incrementals + weekly full) |
| `WATCH_INTERVAL` | `24h` | Interval between incremental backups |
| `FULL_INTERVAL` | `168h` | Interval between full backups |

### Tuning (optional)

| Variable | Default | Description |
|---|---|---|
| `WEBAUTHN_RP_NAME` | `appmetrica.online` | Name the browser shows when registering a passkey |
| `HTTP_PORT`, `HTTPS_PORT` | `80`, `443` | Host ports of Caddy (Docker Compose stack only). Let's Encrypt needs 80 and 443 reachable from the internet |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` (JSON logs to stdout) |
| `SESSION_TTL` | `720h` | Absolute lifetime of a dashboard session |
| `SESSION_IDLE_TTL` | `168h` | Idle timeout of a dashboard session |
| `API_PUBLIC_BEARER_ENABLED` | `true` | `false` disables the public Read API (`sk_` keys); the dashboard keeps working |
| `INGEST_MAX_BODY_BYTES` | `1048576` | Maximum decompressed request body, bytes |
| `INGEST_MAX_EVENTS` | `500` | Maximum events per batch |
| `INGEST_LATE_WINDOW` | `168h` | Events older than this are rejected (`event_too_old`) |
| `INGEST_FUTURE_TOLERANCE` | `10m` | Allowed client clock skew into the future after correction |
| `INGEST_ACCEPT_DEVELOPMENT` | `true` | `false` drops events with `environment=development` |
| `RATE_APP_EVENTS_PER_SEC`, `RATE_APP_BURST` | `2000`, `4000` | Per-app rate limit; above it the batch gets HTTP 429 with `Retry-After` |
| `RATE_INSTALL_PER_MIN`, `RATE_INSTALL_BURST` | `100`, `1000` | Per-install limit; extra events are rejected as retryable `install_rate_limited` |
| `KEY_CACHE_REFRESH` | `15s` | How often ingest reloads write keys (delay before a revoked `pk_` key stops working) |
| `WORKER_JOBS_ENABLED` | all jobs | Comma-separated list of `system_check`, `digest`, `close_day`, `deletion`, `session_cleanup`, `app_merge`. A list without `app_merge` leaves [app merges](#platforms-and-merging-apps) waiting in `pending` |
| `MERGE_KEYS_WAIT` | — | E2E stand only (`APP_ENV=e2e`, rejected otherwise): how long a merge waits for ingest to reload write keys instead of 17 minutes; `0` in CI |

## Sending your first events

Every app has its own write key `pk_…` (dashboard → app → **Integration**). Write keys are public by design — they
are embedded into apps and only allow sending events.

```bash
export AM_PK=pk_your_write_key
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

The full API reference (ingestion and Read API), session and offline-queue recipes for each platform and the
error/retry policy are in the public documentation at `https://APP_DOMAIN/docs`, no sign-in needed; the live copy
is at [appmetrica.online/docs](https://appmetrica.online/docs). The OpenAPI contracts are in
[`contracts/openapi/`](contracts/openapi/).

## Platforms and merging apps

An app is one product on one or more platforms: `ios`, `android`, `web`, `macos`, `windows`, `linux`, `bot`,
`server`, `other`. Choose them when you create the app and change them in **Settings → Platforms**; removing a
platform keeps its events and keys.

Each event gets its platform in this order:

1. `context.platform` of the event or of the batch;
2. the platform of the write key. Keys created with the app are bound to their platform; in **Settings → Write keys**
   a new key gets one platform or "Any platform";
3. the only platform of the app, if it has exactly one;
4. otherwise `other`, and the response carries a `platform_missing` warning for the event.

Reports count people across platforms: the same `user_id` on iOS and on the web is one person. The platform switcher
above the reports narrows them to one platform, and the audience chart can draw a line per platform.

**Merging apps.** If iOS and Android were connected as two separate apps, merge them: open the app that stays,
**Settings → Danger zone → Merge…**, pick the other app and type its name. The worker job `app_merge` moves the write
keys (released app versions keep sending with them), waits 17 minutes until every ingest instance reloads the keys,
copies the events, deletes the source app and recalculates the merged days. A banner above the app shows the
progress; if a step fails, the banner shows the error and **Retry** continues from that step. While a merge is
running, neither app can be archived or deleted.

A merge cannot be undone. Before merging, back up Postgres and ClickHouse (see [Operations](#operations)) and make
sure the ClickHouse disk has free space for a copy of the source app's events. The job must be listed in
`WORKER_JOBS_ENABLED` (it is by default); otherwise the merge waits in `pending`.

## Operations

All commands run from `deploy/selfhost` (Docker Compose stack). Under Dokploy see
[Operations under Dokploy](#operations-under-dokploy).

**Logs and status**

```bash
docker compose ps
docker compose logs -f --tail=100 api ingest worker
```

**Update to a new version**

```bash
git pull
docker compose up -d --build --wait     # migrations are applied automatically by api
```

**Refresh the geo database** (monthly): set `GEO_MONTH` to the current month (`date +%Y-%m`) and run
`docker compose up -d --build --wait`.

**Back up Postgres** (apps, keys, passkeys, settings — small, back it up daily, e.g. from cron):

```bash
docker compose exec -T postgres pg_dump -U appmetrica -d appmetrica -Fc > appmetrica-$(date +%F).dump
```

Restore into an empty database:

```bash
docker compose exec -T postgres pg_restore -U appmetrica -d appmetrica --clean --if-exists < appmetrica-2026-10-14.dump
```

**Back up ClickHouse**: enable the `backup` profile (see [ClickHouse backups to S3](#clickhouse-backups-to-s3-optional)),
then manage backups through the `clickhouse-backup` CLI:

```bash
docker compose exec clickhouse-backup clickhouse-backup list remote
docker compose exec clickhouse-backup clickhouse-backup restore_remote <backup-name>
```

**Lost access** (all passkeys gone): `docker compose exec api /app/appmetrica admin passkey-link` prints a new
one-time sign-up link.

**Health endpoints**: `https://API_DOMAIN/health`, `https://INGEST_DOMAIN/health`, `https://APP_DOMAIN/api/health`.

**Stop / remove**

```bash
docker compose down        # stop, keep data
docker compose down -v     # stop and DELETE all data volumes
```

**Security checklist**: keep `.env` private (`chmod 600`), expose only ports 80/443, keep Docker and the host
updated, store backups outside the server, and rotate `sk_` Read API keys you no longer use (**Settings → API keys**).

## Development

Requirements: Go 1.27.2, Node 24.21.0, pnpm 12.11.2, Docker; `golangci-lint` 2.14 for `make check`, `jq` for perf
and deploy scripts.

```bash
make env          # .env from .env.example with working dev values
make install      # Go modules and pnpm dependencies
make dev-infra    # ClickHouse and Postgres in Docker
make dev          # migrations + ingest :8081, api :8080, worker :8082, web :3000
make seed         # demo data: 4 apps (one on iOS, Android and Web), 35 days of events
make passkey-link # one-time passkey sign-up link for http://localhost:3000
```

Before committing: `make check` (code generation check, linters, Go/web/script unit tests). Integration tests:
`make test-integration` (Docker). E2E: `make e2e`. The project follows TDD and Conventional Commits; development rules (in Russian) are in
[CONTRIBUTING.md](CONTRIBUTING.md).

Repository layout:

- `backend/` — Go module, one binary `appmetrica` (`ingest`, `api`, `worker`, `migrate`, `admin`, `healthcheck`).
- `web/` — Next.js 16 dashboard, landing page, demo mode and API documentation (`web/content/docs/`).
- `contracts/openapi/` — OpenAPI 3.0 contracts; Go and TypeScript code is generated from them.
- `deploy/selfhost/` — Docker Compose stack for self-hosting; `deploy/dokploy/` — the same stack for Dokploy and
  the GitHub Actions → GHCR → Dokploy pipeline; `deploy/clickhouse/` — ClickHouse configuration.
- `scripts/` — CI and deploy scripts; `scripts/reconcile` — reconciliation of numbers with AppMetrica exports.
- `docs/discovery/` — product and technical specification (in Russian).

## Documentation

- Product requirements, architecture, data model, API contracts, UX:
  [`docs/discovery/2026-10-13-appmetrica-online/`](docs/discovery/2026-10-13-appmetrica-online/) (Russian)
- Developer runbook: [12-dev-runbook.md](docs/discovery/2026-10-13-appmetrica-online/12-dev-runbook.md)
- Operations runbooks: [21-ops-runbooks.md](docs/discovery/2026-10-13-appmetrica-online/21-ops-runbooks.md)
- API documentation for integrations: public, at `/docs` of every installation
  ([appmetrica.online/docs](https://appmetrica.online/docs)), sources in [`web/content/docs/`](web/content/docs/)

## License

[GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0). You may use, modify and self-host appmetrica.online
freely; if you run a modified version as a network service for others, you must publish your changes under the
same license.

Geolocation uses the [IP to City Lite](https://db-ip.com) database by DB-IP, licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/); the dashboard footer carries the required attribution.
