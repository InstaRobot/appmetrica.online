# syntax=docker/dockerfile:1
# Образ appmetrica-web (Next.js 16.4 standalone).
# Сборка (CI): docker build --build-arg API_INTERNAL_URL=http://appmetrica-api:8080 \
#   --build-arg NEXT_PUBLIC_INGEST_URL=https://i.appmetrica.online \
#   --build-arg NEXT_PUBLIC_API_URL=https://api.appmetrica.online -t ghcr.io/<owner>/appmetrica-web:<sha> web

ARG NODE_IMAGE=node:24.21.0-alpine

FROM ${NODE_IMAGE} AS deps
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@12.11.2 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml source.config.ts ./
COPY patches ./patches
COPY content ./content
RUN --mount=type=cache,id=pnpm-store,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile

FROM deps AS build
COPY . .
ARG API_INTERNAL_URL=http://appmetrica-api:8080
ARG NEXT_PUBLIC_INGEST_URL=https://i.appmetrica.online
ARG NEXT_PUBLIC_API_URL=https://api.appmetrica.online
ENV API_INTERNAL_URL=${API_INTERNAL_URL} \
    NEXT_PUBLIC_INGEST_URL=${NEXT_PUBLIC_INGEST_URL} \
    NEXT_PUBLIC_API_URL=${NEXT_PUBLIC_API_URL} \
    NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM ${NODE_IMAGE} AS run
WORKDIR /app
# API_INTERNAL_URL читается и во время работы (server components, /docs): ENV стадии build в образ не попадает.
ARG API_INTERNAL_URL=http://appmetrica-api:8080
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0 \
    API_INTERNAL_URL=${API_INTERNAL_URL}
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:3000/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", "server.js"]
