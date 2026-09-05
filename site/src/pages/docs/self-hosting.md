---
layout: ../../layouts/Doc.astro
title: Self-hosting
description: "Run the whole of simhook on one small server with Docker Compose, and point the app at it."
---

Everything that runs simhook.dev is in the [repository](https://github.com/simhook/simhook) under the AGPL-3.0, and the production setup is one Compose file: Postgres, the API, the dashboard, this site, Caddy for TLS, and a nightly database dump. A small VPS runs all of it; the phones do the heavy lifting.

## What you need

- A Linux host with 2 CPUs and 4 GB of memory (a Hetzner CX22 or similar) with ports 22, 80, and 443 open.
- A domain on Cloudflare, and an API token with `Zone: DNS: Edit` on it. Caddy uses it to obtain certificates through the DNS challenge, so the proxy can stay on.
- A Firebase project with Cloud Messaging, for pushes to the phones. The service account JSON is the only secret the phones' side needs.
- SMTP credentials from a transactional email provider, for sign-up and password emails.

## Steps

```sh
curl -fsSL https://get.docker.com | sh
git clone https://github.com/simhook/simhook.git /opt/simhook
cd /opt/simhook/deploy
cp .env.example .env          # domains, Cloudflare token, database password
cp api.env.example api.env    # secret key, SMTP
mkdir -p secrets && cp /path/to/firebase-service-account.json secrets/fcm.json
chown 10001:10001 secrets/fcm.json && chmod 400 secrets/fcm.json
docker compose -f docker-compose.prod.yaml up -d --build
```

Create proxied `A` records for the root domain, `www`, `api`, and `app`, set Cloudflare's SSL mode to Full (strict), and open `https://app.<your domain>` to register the first account. The API runs its migrations on start. The full runbook, including updating, backups, and restoring, is in [deploy/README.md](https://github.com/simhook/simhook/blob/main/deploy/README.md).

## Pointing the app at your server

The app itself does not need to change. A pairing link or QR code carries the API address (`simhook://pair?code=…&api=https://api.your-domain`), and your dashboard generates it with your domain. Pair from your dashboard and the phone talks to your server from then on.

Two things are built into the app and worth knowing:

- **Updates.** The app polls `https://simhook.dev/download/android.json` for new releases, which is fine for most self-hosters: you get the same signed builds. To serve your own builds instead, build the app with `-PupdateManifestUrl=https://your-domain/…` and your own signing key; [android/RELEASING.md](https://github.com/simhook/simhook/blob/main/android/RELEASING.md) covers the release process.
- **Push.** Pushes carry no message content, only a nudge to check in. The app and the server must belong to the same Firebase project: the published app is tied to simhook's production project, so a self-hosted server can wake phones only if you build the app with your own project's `google-services.json` in `android/app/src/release/` and give the server that project's service account. Without that, phones still work, but only on their check-in schedule.

## Optional: Google sign-in and a bot check

Both are off until their keys are set in `deploy/api.env`; the sign-in page adapts on its own.

- **Google sign-in.** Create an OAuth client (web application) in Google Cloud with `https://api.<your domain>/v1/auth/google/callback` as the redirect URI, then set `SIMHOOK_GOOGLE_CLIENT_ID` and `SIMHOOK_GOOGLE_CLIENT_SECRET`. The code exchange runs on the API; the dashboard only links to it.
- **Bot check.** Create a Cloudflare Turnstile widget for `app.<your domain>` and set `SIMHOOK_TURNSTILE_SITE_KEY` and `SIMHOOK_TURNSTILE_SECRET_KEY`. Sign-in, sign-up, and password reset then need a token, which the widget obtains without bothering most visitors.

## Keeping it healthy

- `SIMHOOK_SECRET_KEY` encrypts stored webhook secrets. Losing it means every webhook needs a new secret. Keep a copy off the host.
- The API, the dashboard, and the site must share a parent domain (`api.` and `app.` under your root domain, which the Compose file assumes). The API sets a readable signed-in flag cookie on that domain so the site and the dashboard know who is signed in before asking. If your dashboard has to live elsewhere, leave `SIMHOOK_COOKIE_DOMAIN` empty on the API and set `SIMHOOK_SESSION_FLAG=off` on the dashboard.
- The `backup` service writes a dump every day into `deploy/backups`. Copy that directory somewhere else on a schedule; a backup on the same disk is not a backup.
- Update with `git pull` and `docker compose up -d --build`. The `api` image CI publishes to GitHub's registry works anywhere; the `web` and `site` images are built with simhook.dev's addresses baked in, so build those two yourself, which `--build` does.

## The license, briefly

AGPL-3.0 means you can run simhook for yourself or your company without restriction. If you change it and offer the changed version to others as a service, you have to publish your changes under the same license. The client packages (`@simhook/sdk`, `@simhook/mcp`) are MIT.
