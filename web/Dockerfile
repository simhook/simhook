# syntax=docker/dockerfile:1.7
# Build context: the repository root, because the dashboard depends on the
# workspace package packages/contracts.
#   docker build -f web/Dockerfile --build-arg NEXT_PUBLIC_API_URL=https://api.example.com -t simhook/web .

FROM node:22-alpine AS base
RUN corepack enable && corepack prepare pnpm@10.28.2 --activate
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY web/package.json web/
COPY packages/contracts/package.json packages/contracts/
COPY packages/sdk/package.json packages/sdk/
COPY packages/mcp/package.json packages/mcp/
RUN --mount=type=cache,id=pnpm-store,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile --filter web...

FROM deps AS build
COPY packages/contracts packages/contracts
COPY web web
# Baked into the client bundle at build time. Must be the public API origin.
ARG NEXT_PUBLIC_API_URL=https://api.simhook.dev
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL NEXT_TELEMETRY_DISABLED=1
RUN pnpm --filter web build

FROM node:22-alpine AS runner
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
WORKDIR /app
RUN adduser -D -H -u 10001 web
COPY --from=build --chown=web:web /app/web/.next/standalone ./
COPY --from=build --chown=web:web /app/web/.next/static ./web/.next/static
COPY --from=build --chown=web:web /app/web/public ./web/public
USER web
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3000/login >/dev/null || exit 1
CMD ["node", "web/server.js"]
