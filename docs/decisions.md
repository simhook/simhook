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

## 012. Release signing, distribution, and in-app updates

**Date:** 2026-09-05
**Decision:** One release key, generated once and kept outside the repository, signs every build; Gradle reads it from a gitignored properties file or environment variables and otherwise produces an unsigned APK, so CI can build the release variant without the key. Releases are GitHub releases tagged `android-v<version>` carrying the APK, a copy named `simhook.apk`, a checksum file, and `android.json`, a manifest with the version code, download URL, and SHA-256. `simhook.dev/download/android.json` and `/download/simhook.apk` are stable addresses that redirect to the latest release, so the hosting can move without touching installed apps.

The app polls the manifest twice a day and on opening, offers a newer build on its home screen, downloads it through the system download manager, verifies the hash, and hands the file to the package installer, which enforces that the signature matches. A `min_supported_version_code` in the manifest marks builds the server no longer supports so the app can say the update is required.

**Why:** Sideloaded apps get no store updates, and a gateway that nobody opens must still learn about fixes. Polling a tiny JSON file costs nothing and needs no server code; verifying the hash and letting Android check the signature means a compromised download host cannot push a foreign build. Version and manifest are produced by one script from the same Gradle values, so the APK and what it advertises cannot disagree.

## 013. Public repository, AGPL-3.0 for the service, MIT for the clients

**Date:** 2026-09-05
**Decision:** The repository is public. The API, dashboard, Android app, and deployment files are licensed under AGPL-3.0-only; the SDK, MCP server, and contracts package under MIT. Releases of the Android app are GitHub releases on this repository. Security reports go to security@simhook.dev (see SECURITY.md).

**Why:** The app asks for SMS permissions on people's phones, so being able to read exactly what it does is part of the offer, and release files can only be downloaded from a public repository. AGPL keeps self-hosting free while requiring anyone who runs a modified version as a service to publish their changes, which is the protection the hosted product needs. The client packages are how developers find the service, and a client library with strings attached does not get installed.

## 014. One plain design for the site and the dashboard

**Date:** 2026-09-05
**Decision:** simhook.dev and app.simhook.dev share one visual system: a single column, black text on white, Instrument Sans for words and Geist Mono for anything a machine wrote, hairline rules instead of boxes, no cards, no shadows, no gradients, no illustrations, no icons in navigation. Status is a dot next to a word. Each page has at most one filled black button; every other action is a text link. The site is light only. The site is a static Astro build served by a small Caddy; the dashboard keeps its Next.js stack and carries the look in its tokens and primitives.

**Why:** Three conventional directions were rendered and rejected as interchangeable with every other developer tool. The product is a plain thing: a phone, a request, a receipt. A design that shows the request, the receipt, and the real app, and otherwise stays out of the way, says that better than any hero. It is also the cheapest to keep consistent: the same five colours and two typefaces cover the site, the docs, and every dashboard page.

## 015. Sessions: the API is the only judge, a flag cookie is the only hint

**Date:** 2026-09-05
**Decision:** A dashboard session is a random token stored as a hash, carried in an httpOnly, host-only cookie on the API. Next to it the API sets `simhook_signed_in=1`, a readable cookie on the parent domain (`SIMHOOK_COOKIE_DOMAIN`, `simhook.dev` in production) that carries no secret and only ever means "probably signed in". Nothing but the API writes either cookie: sign-in issues both, sign-out clears both, and any request that finds a cookie with no live session behind it clears both, so a stale flag corrects itself on the next request. The site reads the flag to paint its bar before the first frame; the dashboard's proxy reads it to send a signed-out visitor to sign-in before a page loads, remembering where they were going. Neither trusts it further: the page asks the API, which answers for certain, and a visitor bounced on the flag alone is never bounced again once the API says they are signed in (the sign-in page explains instead of looping).

Sessions slide. One ends after 30 days unused, use pushes the end out, and it ends for good 180 days after sign-in whatever happens. Users see every session (browser, address, signed in, last seen) and can end any of them; changing the password ends every other one, a password reset ends all. Cookie-authenticated writes are checked against the allowed origins (the dashboard, the site, the API) using `Origin`, `Referer`, and `Sec-Fetch-Site`, so a foreign page cannot act with the cookie; API keys and device tokens are exempt because browsers never add them on their own.

**Why:** The first version had the site ask the API on every page and remember the answer in local storage, and the dashboard learn it was signed out only after a failed request. That meant a flash of the wrong bar, a flash of an empty dashboard, a browser that could not sign in again while it held a dead cookie, and three fixes in one evening. A cookie the API keeps in step is read for free, is right on the first frame, and has one source of truth. A sliding session with a cap keeps a daily user signed in without keeping a forgotten laptop signed in forever, and seeing and ending sessions is what a user expects from a page that can send texts from their phone.

**Rules out:** The dashboard or the site writing cookies; anything that trusts the flag for more than routing; a session that never ends.
