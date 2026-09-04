# Decisions

Short architecture decision records. Newest at the bottom. Each one says what we chose, why, and what it rules out.

## 001. Product name: simhook

**Date:** 2026-09-04
**Decision:** The product is called **simhook**. SIM plus webhook.

| Surface | Value |
|---|---|
| Domains | simhook.dev (primary), simhook.io (redirect) |
| GitHub org | github.com/simhook |
| Android application id | `dev.simhook.app` |
| Kotlin package | `dev.simhook.app` |
| npm SDK | `@simhook/sdk` |
| npm MCP server | `@simhook/mcp` |
| Go module | `github.com/simhook/simhook` |
| Env var prefix | `SIMHOOK_` |
| API key header | `X-Api-Key` |
| Webhook signature header | `X-Simhook-Signature` |

## 002. Backend in Go, PostgreSQL only

**Date:** 2026-09-04
**Decision:** The API, job workers, and scheduled tasks are one Go program backed by PostgreSQL. No Redis, no document store.

**Why:**
- One static binary is the simplest thing a self-hoster can run.
- The workload is thousands of idle network calls (push notifications, webhook deliveries). Goroutines make that cheap.
- Users, devices, batches, messages, subscriptions, and webhooks are all one-to-many with joins on every screen. That is relational data.
- A Postgres-backed job queue enqueues the dispatch job **in the same transaction** that inserts the message rows. A message can never exist without its job, and a job can never point at a message that failed to write.

**Stack:**
- Go, current stable.
- HTTP: `chi` router with `huma` for typed handlers, request validation at the edge, and a generated OpenAPI 3.1 document. That document is the contract the dashboard, SDK, and Android DTOs are generated from.
- DB: `pgx` with typed row mapping. Queries are plain SQL next to the code that runs them. No code generation step.
- Migrations: `goose`, embedded in the binary.
- Jobs: `river`. Periodic jobs replace cron.
- Push: Firebase Cloud Messaging behind a `DevicePush` interface, with a logging no-op implementation for development.
- Credentials: sessions, API keys, device tokens, and one-time codes are random tokens stored as SHA-256 hashes. Passwords use Argon2id. Webhook signing secrets are AES-GCM encrypted with the server key.

**Rules out:** Any send path that bypasses the queue.

## 003. Web dashboard on Next.js

**Date:** 2026-09-04
**Decision:** Next.js app router, React 19, Tailwind 4, shadcn/Radix, TanStack Query. The dashboard is a thin client of the API and authenticates with an httpOnly session cookie issued by the API. The Next.js server never holds credentials or talks to the database.

## 004. Android app: Kotlin and Compose only

**Date:** 2026-09-04
**Decision:** A single Kotlin + Jetpack Compose app, package `dev.simhook.app`, minSdk 26. No XML views.

**Design points:**
- Pairing exchanges a short-lived code (shown as a QR in the dashboard) for a **device token** that can only call device endpoints: heartbeat, report received, report status, refresh push token. A developer's API key never touches the phone. Revoking a device revokes its token and nothing else.
- Tokens live in Keystore-backed encrypted storage.
- Outbound messages go into a Room-backed outbox drained by a foreground service at the configured rate, so the queue survives process death and the UI can show it truthfully.
- Each received SMS gets a fingerprint on the phone that the server treats as an idempotency key.
- Real release keystore from day one. No analytics or crash reporting SDKs by default.
- Distribution is a signed APK from GitHub Releases plus a small update manifest the app polls.

## 005. Monorepo layout

**Date:** 2026-09-04

```
simhook/
  api/          Go module: server, workers, migrations
  web/          Next.js dashboard
  android/      Kotlin app
  packages/
    contracts/  OpenAPI spec (generated from the API) + generated TS types
    sdk/        @simhook/sdk
    mcp/        @simhook/mcp
  docs/         this folder
  deploy/       docker compose, reverse proxy, service units
```

pnpm workspaces for the JS packages. Go and Gradle are their own toolchains.

## 006. Build order

**Date:** 2026-09-04
**Decision:** API first, then the Android app, then the dashboard, then SDK and MCP. Billing is its own phase after the core loop works end to end on a real phone.

**Why:** Everything is a client of the API. The phone app carries the most platform risk and needs time on real hardware, so it comes before the dashboard.

## 007. Message lifecycle

**Date:** 2026-09-04
**Decision:** Outbound messages move through `queued`, `dispatched`, `sent`, `delivered`, with `failed` and `unknown` as terminal states. Inbound messages are `received`. Transitions are enforced server-side; an out-of-order report is recorded in the log but does not move the state backwards.

| From | To | Trigger |
|---|---|---|
| queued | dispatched | push accepted by the push provider |
| queued | failed | push rejected, or no valid push token |
| dispatched | sent | phone reports the carrier accepted it |
| dispatched | failed | phone reports a send error |
| sent | delivered | phone reports a delivery receipt |
| sent | failed | phone reports a delivery failure |
| queued, dispatched | unknown | no report within the stale window |

Batch counters are incremented atomically on each transition, never recomputed by scanning.
