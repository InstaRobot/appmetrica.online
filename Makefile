# appmetrica.online — единая точка входа для разработки.
# Все цели запускаются из корня репозитория. Справка: `make help`.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# Пути относительные: абсолютный $(CURDIR) ломает рецепты, если в пути к репозиторию есть пробел.
ROOT := .
BACKEND := $(ROOT)/backend
RECONCILE := $(ROOT)/scripts/reconcile
WEB := $(ROOT)/web
COMPOSE := docker compose -f $(ROOT)/docker-compose.dev.yml --env-file $(ROOT)/.env
SQLC_IMAGE := sqlc/sqlc:1.31.1
K6_IMAGE := grafana/k6:2.3.0
PERF_REPORTS := $(BACKEND)/perf/reports
COVER_PKGS := ingest reports auth identity deletion
COVER_MIN := 80

# Подключаем .env (KEY=value), чтобы go run и pnpm видели переменные.
ifneq (,$(wildcard $(ROOT)/.env))
include $(ROOT)/.env
export
endif

.PHONY: help
help: ## Показать список целей
	@# Цвет только в терминале: в пайпе (`make help | grep`) строки начинаются с имени цели.
	@if [ -t 1 ]; then c=$$'\033[36m'; r=$$'\033[0m'; else c=; r=; fi; \
	grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk -v c="$$c" -v r="$$r" 'BEGIN {FS = ":.*?## "}; {printf "  %s%-20s%s %s\n", c, $$1, r, $$2}'

# ---------------------------------------------------------------------------
# Локальное окружение
# ---------------------------------------------------------------------------

.PHONY: env
env: ## Создать .env из .env.example (если ещё нет)
	@test -f $(ROOT)/.env || cp $(ROOT)/.env.example $(ROOT)/.env
	@echo ".env готов"

.PHONY: dev-infra
dev-infra: env ## Поднять ClickHouse 26.8 и Postgres 18.6 в Docker и дождаться healthcheck
	$(COMPOSE) up -d --wait

.PHONY: dev-down
dev-down: ## Остановить инфраструктуру (данные сохраняются)
	$(COMPOSE) down

.PHONY: dev-reset
dev-reset: ## Остановить инфраструктуру и удалить все локальные данные
	$(COMPOSE) down -v

.PHONY: dev
dev: dev-infra migrate-up geo-db ## Всё сразу: инфраструктура + миграции + ingest, api, worker, web (Ctrl+C останавливает процессы)
	$(MAKE) -j4 --no-print-directory run-ingest run-api run-worker run-web

.PHONY: run-ingest
run-ingest: ## Запустить ingest на :8081 (метрики :9091)
	cd $(BACKEND) && METRICS_LISTEN_ADDR=:9091 CLICKHOUSE_USER=appmetrica_rw CLICKHOUSE_PASSWORD=$(CLICKHOUSE_RW_PASSWORD) go run ./cmd/appmetrica ingest

.PHONY: run-ingest-perf
run-ingest-perf: ## ingest для make perf-ingest: лимит приложения 20 000 событий/с (500 rps × 10 событий одним ключом), LOG_LEVEL=info
	$(MAKE) --no-print-directory run-ingest RATE_APP_EVENTS_PER_SEC=20000 RATE_APP_BURST=40000 LOG_LEVEL=info

.PHONY: run-api
run-api: ## Запустить api на :8080 (метрики :9092), пользователь ClickHouse appmetrica_ro
	cd $(BACKEND) && METRICS_LISTEN_ADDR=:9092 MIGRATE_ON_START=false CLICKHOUSE_USER=appmetrica_ro CLICKHOUSE_PASSWORD=$(CLICKHOUSE_RO_PASSWORD) go run ./cmd/appmetrica api

.PHONY: run-api-e2e
run-api-e2e: ## api для make e2e: APP_ENV=e2e открывает /internal/e2e/* (сбрасывает владельца и passkey в базе POSTGRES_URL!)
	$(MAKE) --no-print-directory run-api APP_ENV=e2e

.PHONY: run-worker
run-worker: ## Запустить worker на :8082 (метрики :9093)
	cd $(BACKEND) && METRICS_LISTEN_ADDR=:9093 CLICKHOUSE_USER=appmetrica_rw CLICKHOUSE_PASSWORD=$(CLICKHOUSE_RW_PASSWORD) go run ./cmd/appmetrica worker

.PHONY: run-web
run-web: ## Запустить Next.js dev-сервер на :3000
	cd $(WEB) && pnpm dev

.PHONY: install
install: ## Установить зависимости (Go-модули и pnpm)
	cd $(BACKEND) && go mod download
	cd $(WEB) && pnpm install --frozen-lockfile

.PHONY: geo-db
geo-db: ## Скачать DB-IP City Lite (текущий месяц, иначе предыдущий) в backend/.geo/
	@mkdir -p $(BACKEND)/.geo
	@if [ -f $(BACKEND)/.geo/dbip-city-lite.mmdb ]; then echo "geo-db: уже скачана"; else \
	  m=$$(date -u +%Y-%m); \
	  if ! curl -fsSI "https://download.db-ip.com/free/dbip-city-lite-$$m.mmdb.gz" >/dev/null; then \
	    m=$$(date -u -v-1m +%Y-%m 2>/dev/null || date -u -d '1 month ago' +%Y-%m); fi; \
	  echo "geo-db: скачиваю $$m"; \
	  curl -fsSL "https://download.db-ip.com/free/dbip-city-lite-$$m.mmdb.gz" | gunzip > $(BACKEND)/.geo/dbip-city-lite.mmdb; \
	fi

# ---------------------------------------------------------------------------
# Миграции, сиды, администрирование
# ---------------------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Применить миграции goose к Postgres и ClickHouse
	cd $(BACKEND) && go run ./cmd/appmetrica migrate up

.PHONY: migrate-status
migrate-status: ## Показать статус миграций Postgres и ClickHouse
	cd $(BACKEND) && go run ./cmd/appmetrica migrate status

.PHONY: seed
seed: ## Залить демо-данные: 3 приложения, write-ключ DEV_WRITE_KEY, 35 дней событий в ClickHouse
	cd $(BACKEND) && CLICKHOUSE_USER=appmetrica_rw CLICKHOUSE_PASSWORD=$(CLICKHOUSE_RW_PASSWORD) go run ./cmd/seed --apps 3 --days 35 --daily-installs 400 --write-key $(DEV_WRITE_KEY)

.PHONY: passkey-link
passkey-link: ## Напечатать одноразовую ссылку регистрации passkey (15 минут)
	cd $(BACKEND) && go run ./cmd/appmetrica admin passkey-link

.PHONY: send-test-batch
send-test-batch: ## Отправить тестовый батч (session_start + session_end) в локальный ingest
	@now=$$(date -u +%Y-%m-%dT%H:%M:%S.000Z); \
	install=$$(uuidgen | tr 'A-Z' 'a-z'); session=$$(uuidgen | tr 'A-Z' 'a-z'); \
	curl -sS -X POST http://localhost:8081/v1/events \
	  -H "Content-Type: application/json" -H "X-App-Key: $(DEV_WRITE_KEY)" \
	  -d "{\"sent_at\":\"$$now\",\"context\":{\"platform\":\"ios\",\"os_name\":\"iOS\",\"os_version\":\"26.1\",\"app_version\":\"1.0.0\",\"device_model\":\"iPhone17,1\",\"device_type\":\"phone\",\"locale\":\"ru-RU\",\"environment\":\"development\",\"ip\":\"95.31.18.119\"},\"events\":[{\"insert_id\":\"$$(uuidgen | tr 'A-Z' 'a-z')\",\"name\":\"session_start\",\"timestamp\":\"$$now\",\"install_id\":\"$$install\",\"session_id\":\"$$session\"},{\"insert_id\":\"$$(uuidgen | tr 'A-Z' 'a-z')\",\"name\":\"session_end\",\"timestamp\":\"$$now\",\"install_id\":\"$$install\",\"session_id\":\"$$session\",\"duration_ms\":42000}]}" ; echo

# ---------------------------------------------------------------------------
# Кодогенерация
# ---------------------------------------------------------------------------

.PHONY: gen
gen: gen-go-api gen-sqlc gen-web ## Перегенерировать весь код из contracts/ и queries/

.PHONY: gen-go-api
gen-go-api: ## oapi-codegen: contracts/openapi → backend/internal/gen
	cd $(BACKEND) && go tool oapi-codegen -config oapi-ingest.cfg.yaml ../contracts/openapi/ingest.yaml
	cd $(BACKEND) && go tool oapi-codegen -config oapi-api.cfg.yaml ../contracts/openapi/api.yaml

.PHONY: gen-sqlc
gen-sqlc: ## sqlc (в Docker): queries/postgres + migrations/postgres → internal/pgstore/sqlc
	docker run --rm -u $$(id -u):$$(id -g) -v "$$(pwd)/backend":/src -w /src $(SQLC_IMAGE) generate

.PHONY: gen-web
gen-web: ## orval: contracts/openapi/api.yaml → web/src/lib/api/gen; копии спек для API reference (S-13)
	cd $(WEB) && pnpm gen:api
	@# Docker-образ web собирается из web/ без contracts/: Fumadocs читает спеки из этих копий.
	mkdir -p $(WEB)/src/lib/api/gen/openapi
	cp $(ROOT)/contracts/openapi/ingest.yaml $(ROOT)/contracts/openapi/api.yaml $(WEB)/src/lib/api/gen/openapi/

.PHONY: gen-check
gen-check: gen ## Проверить, что сгенерированный код закоммичен и актуален
	@# git diff не видит новых неотслеживаемых файлов — их ловит git ls-files --others.
	git diff --exit-code -- backend/internal/gen backend/internal/pgstore/sqlc web/src/lib/api/gen
	@untracked=$$(git ls-files --others --exclude-standard -- backend/internal/gen backend/internal/pgstore/sqlc web/src/lib/api/gen); \
	if [ -n "$$untracked" ]; then echo "gen-check: сгенерированный код не закоммичен:"; echo "$$untracked"; exit 1; fi

.PHONY: reconcile
reconcile: ## Сверка с AppMetrica (NFR-010): make reconcile ARGS="-csv am.csv -api https://api.appmetrica.online/v1 -app <id>", ключ sk_ в AM_SK
	@bin=$$(mktemp -d)/reconcile && go -C $(RECONCILE) build -o "$$bin" . && "$$bin" $(ARGS)

# ---------------------------------------------------------------------------
# Тесты и качество
# ---------------------------------------------------------------------------

.PHONY: test
test: test-go test-web test-scripts ## Unit-тесты Go, web и CI-скриптов

.PHONY: test-go
test-go: ## Unit-тесты Go (без Docker)
	cd $(BACKEND) && go test -race -count=1 ./...

.PHONY: test-integration
test-integration: ## Интеграционные тесты Go с testcontainers (нужен Docker)
	cd $(BACKEND) && go test -race -count=1 -tags=integration ./...

.PHONY: coverage
coverage: ## Покрытие Go (unit + integration, -race) и гейт scripts/coverage-gate.sh (≥80% для ключевых пакетов)
	cd $(BACKEND) && go test -race -count=1 -tags=integration -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage.out ./...
	COVER_PKGS="$(COVER_PKGS)" COVER_MIN=$(COVER_MIN) bash $(ROOT)/scripts/coverage-gate.sh $(BACKEND)/coverage.out

.PHONY: fuzz
fuzz: ## Fuzz-тесты Go по FUZZTIME (по умолчанию 30s): FuzzLooksLikePII
	cd $(BACKEND) && go test -run '^$$' -fuzz '^FuzzLooksLikePII$$' -fuzztime $(or $(FUZZTIME),30s) ./internal/ingest/

.PHONY: test-scripts
test-scripts: ## Тесты CI-скриптов (coverage-gate, check-tdd-order, perf-compare), деплоя (deploy-plan, dokploy-deploy, geo-month) и сверки (scripts/reconcile)
	bash $(ROOT)/scripts/test-ci-gates.sh
	bash $(ROOT)/scripts/test-deploy.sh
	cd $(RECONCILE) && go test -race -count=1 ./...

.PHONY: tdd-order
tdd-order: ## Проверить порядок коммитов TDD относительно BASE (по умолчанию origin/main)
	bash $(ROOT)/scripts/check-tdd-order.sh $(or $(BASE),origin/main)

.PHONY: perf-ingest
perf-ingest: ## k6 2.3.0 в Docker: 500 rps × 10 мин на локальный ingest (`make dev-infra migrate-up seed` + run-ingest-perf); PERF_RATE, PERF_DURATION
	@mkdir -p $(PERF_REPORTS)
	@# host-gateway: ingest слушает на хосте; на Linux host.docker.internal иначе не резолвится.
	docker run --rm --add-host=host.docker.internal:host-gateway -u $$(id -u):$$(id -g) \
	  -v "$$(pwd)/backend/perf":/perf -w /perf \
	  -e K6_APP_KEY=$(DEV_WRITE_KEY) -e INGEST_URL=$(or $(INGEST_URL),http://host.docker.internal:8081) \
	  -e PERF_RATE=$(or $(PERF_RATE),500) -e PERF_DURATION=$(or $(PERF_DURATION),10m) \
	  $(K6_IMAGE) run --summary-export reports/latest.json ingest-load.js
	@if [ -f $(PERF_REPORTS)/baseline.json ]; then bash $(ROOT)/scripts/perf-compare.sh $(PERF_REPORTS)/baseline.json $(PERF_REPORTS)/latest.json; \
	else echo "perf-ingest: baseline.json нет — сохраните отчёт: cp $(PERF_REPORTS)/latest.json $(PERF_REPORTS)/baseline.json"; fi

.PHONY: perf-reports
perf-reports: ## Перф-тест отчётов NFR-003 (нужен Docker, ~16 ГБ RAM): 100k DAU × 90 дней, P95 из 20 прогонов → backend/perf/reports/reports-latest.json
	cd $(BACKEND) && go test -count=1 -tags=perf -timeout 60m -v -run 'TestReportsPerf|TestPortfolio_ExplainPrunesPartitions' ./internal/reports/

.PHONY: test-web
test-web: ## Unit-тесты web (Vitest)
	cd $(WEB) && pnpm test

.PHONY: e2e
e2e: ## E2E Playwright (требует запущенных ingest, api с APP_ENV=e2e и worker: `make dev-infra migrate-up geo-db seed` + run-ingest/run-api-e2e/run-worker)
	cd $(WEB) && pnpm build && pnpm e2e

.PHONY: lint
lint: lint-go lint-web ## Линтеры Go и web

.PHONY: lint-go
lint-go: ## golangci-lint v2 (backend и scripts/reconcile с конфигом backend)
	cd $(BACKEND) && golangci-lint run ./...
	cd $(RECONCILE) && golangci-lint run -c ../../backend/.golangci.yml ./...

.PHONY: lint-web
lint-web: ## Biome + проверка типов
	cd $(WEB) && pnpm biome check . && pnpm typecheck

.PHONY: fmt
fmt: ## Форматирование Go (gofumpt + goimports) и web (Biome)
	cd $(BACKEND) && golangci-lint fmt ./...
	cd $(WEB) && pnpm format

.PHONY: check
check: gen-check lint test ## Validation Commands: всё, что нужно прогнать перед коммитом

# ---------------------------------------------------------------------------
# Сборка и отладка
# ---------------------------------------------------------------------------

.PHONY: build
build: ## Собрать Docker-образы backend и web локально
	m=$$(date -u +%Y-%m); docker build --build-arg GEO_MONTH=$$m --build-arg VERSION=dev -t appmetrica-backend:dev $(BACKEND)
	docker build --build-arg API_INTERNAL_URL=http://appmetrica-api:8080 --build-arg NEXT_PUBLIC_INGEST_URL=https://i.appmetrica.online --build-arg NEXT_PUBLIC_API_URL=https://api.appmetrica.online -t appmetrica-web:dev $(WEB)

.PHONY: ch
ch: ## Открыть clickhouse-client к локальной базе appmetrica (админ)
	$(COMPOSE) exec clickhouse clickhouse-client --user appmetrica_admin --password $(CLICKHOUSE_ADMIN_PASSWORD) --database appmetrica

.PHONY: psql
psql: ## Открыть psql к локальной базе appmetrica
	$(COMPOSE) exec postgres psql -U appmetrica -d appmetrica
