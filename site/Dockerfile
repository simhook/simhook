# syntax=docker/dockerfile:1.7
# Build context: the repository root, because the site reads the OpenAPI
# document from the workspace package packages/contracts.
#   docker build -f site/Dockerfile -t simhook/site .
# The result is a Caddy image serving static files on port 80. The API and
# dashboard addresses are baked in at build time; leave the args empty to get
# simhook.dev's.

FROM node:22-alpine AS base
RUN corepack enable && corepack prepare pnpm@10.28.2 --activate
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY site/package.json site/
COPY web/package.json web/
COPY packages/contracts/package.json packages/contracts/
COPY packages/sdk/package.json packages/sdk/
COPY packages/mcp/package.json packages/mcp/
RUN --mount=type=cache,id=pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile --filter site...

FROM deps AS build
ARG PUBLIC_API_URL
ARG PUBLIC_APP_URL
ENV PUBLIC_API_URL=$PUBLIC_API_URL PUBLIC_APP_URL=$PUBLIC_APP_URL
COPY packages/contracts packages/contracts
COPY site site
# The build fetches the two typefaces from fontsource once; the cache keeps
# later builds offline-safe.
RUN --mount=type=cache,id=astro-fonts,target=/app/site/node_modules/.astro/fonts \
    pnpm --filter site build

FROM caddy:2-alpine AS runner
COPY site/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/site/dist /srv
EXPOSE 80
