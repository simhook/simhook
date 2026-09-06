# simhook

Turn an Android phone into an SMS API. Send and receive text messages from your code through your own SIM, with a REST API, webhooks, and a dashboard. No per-message fees.

## Layout

```
api/         Go service: HTTP API, job workers, scheduled tasks, migrations
web/         Next.js dashboard
site/        simhook.dev: landing page, docs, download page (Astro, static)
android/     Kotlin + Jetpack Compose phone app
packages/
  contracts/ OpenAPI spec generated from the API, plus generated TypeScript types
  sdk/       @simhook/sdk, the TypeScript client
  mcp/       @simhook/mcp, an MCP server for AI agents
deploy/      docker compose, reverse proxy, service units
docs/        decisions
```

## Development

Prerequisites: Go, Node 20+, pnpm, Docker (for Postgres), Android Studio for the phone app.

```
docker compose -f deploy/docker-compose.dev.yaml up -d
cp api/.env.example api/.env      # fill SIMHOOK_SECRET_KEY
cd api && go run ./cmd/simhook migrate up && go run ./cmd/simhook serve
```

The Android app builds with the Gradle wrapper. Push needs a Firebase config at `android/app/src/debug/google-services.json` (the development project; release builds use `src/release/`); without it the app still builds and runs, but the server cannot wake the phone.

```
cd android && ./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Signed releases, the update manifest installed apps poll, and the publishing script are described in [android/RELEASING.md](android/RELEASING.md).

The JavaScript packages share one pnpm workspace:

```
pnpm install
pnpm build      # dashboard, site, SDK, and MCP server
pnpm test       # SDK and MCP server test suites
pnpm contracts  # regenerate TypeScript types after changing the API
pnpm brand      # redraw icons, share images, and Android drawables from the mark
```

## Using the API

- REST: authenticate with an API key in the `X-Api-Key` header. The OpenAPI document lives at `packages/contracts/openapi.json`.
- JavaScript/TypeScript: `npm install @simhook/sdk`. See `packages/sdk/README.md`.
- AI agents: `npx -y @simhook/mcp` with `SIMHOOK_API_KEY` set. See `packages/mcp/README.md`.

## License

The API, dashboard, Android app, and deployment files are under the [GNU AGPL v3](LICENSE): use it, self-host it, change it, and if you run a modified version as a service, publish your changes. The client packages under `packages/` (`@simhook/sdk`, `@simhook/mcp`, `@simhook/contracts`) are [MIT](packages/sdk/LICENSE) so they fit into any project.

Security reports: see [SECURITY.md](SECURITY.md).

## Deploying

One host, Docker Compose, Cloudflare in front. The runbook is `deploy/README.md`; the same files work for self-hosting. CI builds container images for the API, the dashboard, and the site on every push to main.

See `docs/decisions.md` for the why behind the stack.
