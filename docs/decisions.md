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

## 008. SDK and MCP server

**Date:** 2026-09-05
**Decision:** `@simhook/sdk` is a hand-written, dependency-free TypeScript client over `fetch` and Web Crypto, so the same build runs on Node 20+, edge runtimes, and in browsers. Its types are bundled from the OpenAPI contract at build time; the published package has no workspace dependency. Request and response fields keep the API's snake_case names: the SDK is a typed transport, not a translation layer. Reads retry on transient failures; writes never retry, because a duplicated send is worse than a failed one.

`@simhook/mcp` wraps the SDK with the official MCP TypeScript SDK over stdio and exposes a small tool set for agent workflows: send, follow a send, list and fetch messages, wait for an incoming message, list phones, account usage, and segment counting. Long waits stop below common client request timeouts and hand back a `since` value so the agent can continue, instead of holding a request open.


## 009. Phone app targets API 36

**Date:** 2026-09-05
**Decision:** The Android app compiles against the newest SDK but keeps `targetSdk = 36` until it can act as the phone's default SMS app.

**Why:** Android 17 withholds any incoming SMS that looks like a one-time code from apps targeting API 37 or higher for three hours: the received-SMS broadcast is not delivered and SMS provider queries are filtered, unless the app holds the default SMS role or a similar system role. Verified on the Android 17 emulator: at target 37 a text reading "your code is 8888" never reached the app; at target 36 it was forwarded within a second. Relaying verification codes is a core use of the product, and the app ships as a direct APK download, so no store deadline forces the target up.

Texts in the WebOTP or SMS Retriever formats (`@domain #code`) are delayed the same way on Android 16 QPR2 and newer regardless of target. That format is rare outside app-specific flows and is documented as a limitation. The way out is an optional mode in which the app takes the default SMS role; that is a later phase.

## 010. Deployment: one host, containers, Cloudflare in front

**Date:** 2026-09-05
**Decision:** Production runs on a single VPS with Docker Compose: Postgres, the API, the dashboard, Caddy, and a nightly dump. Cloudflare proxies the domain. Caddy obtains certificates through Cloudflare's DNS API, so the proxy stays on and the origin never needs to answer HTTP challenges. CI builds and publishes the images to GitHub's registry; the compose file can also build them on the host, which keeps self-hosting identical to production.

**Why:** The workload is small. The phones do the heavy lifting; the server routes pushes and stores rows. One host with off-site backups is cheaper and easier to reason about than managed services, and nothing in the design prevents splitting it later.

Behind the proxy the API trusts forwarded client addresses only when `SIMHOOK_TRUST_PROXY` is set, so a directly exposed instance cannot be fooled about who is calling.

## 011. Forwarding on by default, and phones follow dashboard changes at once

**Date:** 2026-09-05
**Decision:** A newly paired phone forwards incoming SMS by default. Any change to a phone made through the dashboard or the API (settings, default phone, unpairing) sends the phone the same check-in push the presence sweep uses, so it reloads its settings within seconds instead of at its next scheduled heartbeat.

**Why:** In the first production test the phone forwarded nothing because the switch defaulted to off, and the fix made in the dashboard took a heartbeat interval to arrive. Receiving is a headline feature and the user grants the SMS permission during setup, so off-by-default was only a trap. Reusing the check-in push means no new app code and no new message type.
