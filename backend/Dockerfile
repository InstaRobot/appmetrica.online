# syntax=docker/dockerfile:1
# Образ appmetrica-backend: один бинарник для ролей ingest | api | worker | migrate | admin.
# Сборка (CI): docker build --build-arg GEO_MONTH=2026-10 --build-arg VERSION=<git sha> -t ghcr.io/<owner>/appmetrica-backend:<sha> backend

ARG GO_IMAGE=golang:1.27.2-alpine

FROM ${GO_IMAGE} AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/appmetrica ./cmd/appmetrica

# Гео-база DB-IP City Lite (CC BY 4.0) скачивается на этапе сборки; GEO_MONTH = YYYY-MM.
FROM ${GO_IMAGE} AS geo
ARG GEO_MONTH
RUN test -n "${GEO_MONTH}" || (echo "GEO_MONTH (YYYY-MM) is required" && exit 1)
RUN wget -q -O /tmp/dbip.mmdb.gz "https://download.db-ip.com/free/dbip-city-lite-${GEO_MONTH}.mmdb.gz" \
    && gunzip -c /tmp/dbip.mmdb.gz > /dbip-city-lite.mmdb \
    && test "$(wc -c < /dbip-city-lite.mmdb)" -gt 50000000

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/appmetrica /app/appmetrica
COPY --from=geo --chown=nonroot:nonroot /dbip-city-lite.mmdb /app/geo/dbip-city-lite.mmdb
ENV GEOIP_DB_PATH=/app/geo/dbip-city-lite.mmdb
USER nonroot:nonroot
EXPOSE 8080 8081 8082 9090
ENTRYPOINT ["/app/appmetrica"]
CMD ["api"]
