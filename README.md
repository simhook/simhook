# simhook

Turn an Android phone into an SMS API. Send and receive text messages from your code through your own SIM, with a REST API, webhooks, and a dashboard. No per-message fees.

## Layout

```
api/         Go service: HTTP API, job workers, scheduled tasks, migrations
web/         Next.js dashboard
android/     Kotlin + Jetpack Compose phone app
packages/
  contracts/ OpenAPI spec generated from the API, plus generated TypeScript types
  sdk/       @simhook/sdk
  mcp/       @simhook/mcp
deploy/      docker compose, reverse proxy, service units
docs/        architecture study, decisions
```

## Development

Prerequisites: Go, Node 20+, pnpm, Docker (for Postgres), Android Studio for the phone app.

```
docker compose -f deploy/docker-compose.dev.yaml up -d
cp api/.env.example api/.env      # fill SIMHOOK_SECRET_KEY
cd api && go run ./cmd/simhook migrate up && go run ./cmd/simhook serve
```

The Android app builds with the Gradle wrapper. Push needs a Firebase config at `android/app/google-services.json`; without it the app still builds and runs, but the server cannot wake the phone.

```
cd android && ./gradlew assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

See `docs/decisions.md` for the why behind the stack.
