# Deploying simhook

One Linux host runs everything: Postgres, the API, the dashboard, Caddy for TLS and routing, and a nightly database dump. Cloudflare proxies the domain in front of it. The same files serve self-hosters.

## What you need

- A VPS with 2 vCPUs and 4 GB of RAM running Ubuntu 24.04 (a Hetzner CX22 or similar). Open ports 22, 80, and 443.
- The domain on Cloudflare, with a Cloudflare API token that has `Zone: DNS: Edit` on that zone.
- A Firebase service account JSON for push (the same one used in development).
- SMTP credentials from a transactional email provider (Postmark, Resend, Amazon SES, Mailgun all work).

## First deploy

1. **Install Docker** on the host.

   ```sh
   curl -fsSL https://get.docker.com | sh
   ```

2. **Get the code** onto the host. The repository is private, so use a deploy key or a fine-grained token with read access.

   ```sh
   git clone git@github.com:simhook/simhook.git /opt/simhook
   cd /opt/simhook/deploy
   ```

3. **Configure.**

   ```sh
   cp .env.example .env          # domains, Cloudflare token, database password
   cp api.env.example api.env    # secret key, SMTP, admin email
   mkdir -p secrets && cp /path/to/firebase-service-account.json secrets/fcm.json
   chown 10001:10001 secrets/fcm.json && chmod 400 secrets/fcm.json   # the API runs as uid 10001
   openssl rand -base64 32       # value for SIMHOOK_SECRET_KEY
   openssl rand -hex 24          # value for POSTGRES_PASSWORD
   ```

4. **DNS.** In Cloudflare, create proxied `A` records for `simhook.dev`, `www`, `api`, and `app` pointing at the host. Set SSL/TLS to **Full (strict)** and turn on **Always Use HTTPS**.

5. **Start.** The first run builds the images on the host, which takes a few minutes.

   ```sh
   docker compose -f docker-compose.prod.yaml up -d --build
   docker compose -f docker-compose.prod.yaml logs -f api
   ```

   The API runs its migrations on start. Caddy requests certificates through the Cloudflare DNS challenge, so the proxy can stay on.

6. **Check.**

   ```sh
   curl https://api.simhook.dev/healthz
   ```

   Then open `https://app.simhook.dev`, register, verify the email, pair a phone.

## Shipping images from your machine

When the server cannot reach the repository or the registry (for example while deploy keys are disabled for the organization), build here and push the images over ssh. Set `API_IMAGE=simhook/api:prod` and `WEB_IMAGE=simhook/web:prod` in `.env`, copy the `deploy` directory to the host, then:

```sh
docker build -t simhook/api:prod api
docker build -f web/Dockerfile --build-arg NEXT_PUBLIC_API_URL=https://api.simhook.dev -t simhook/web:prod .
docker build -t simhook/caddy:local deploy/caddy
docker save simhook/api:prod simhook/web:prod simhook/caddy:local | gzip -1 | ssh simhook "gunzip | docker load"
ssh simhook "cd /opt/simhook/deploy && docker compose -f docker-compose.prod.yaml up -d"
```

## Updating

```sh
cd /opt/simhook && git pull
cd deploy && docker compose -f docker-compose.prod.yaml up -d --build
```

A change to `caddy/Caddyfile` alone needs a reload, not a rebuild:

```sh
docker compose -f docker-compose.prod.yaml exec caddy caddy reload --config /etc/caddy/Caddyfile
```

To use the images CI publishes instead of building on the host, log in to GitHub's registry with a token that has `read:packages`, set `API_IMAGE` and `WEB_IMAGE` in `.env`, then:

```sh
docker compose -f docker-compose.prod.yaml pull && docker compose -f docker-compose.prod.yaml up -d
```

## Backups

The `backup` service writes a dump into `./backups` every 24 hours and keeps `BACKUP_KEEP_DAYS` of them. Copy that directory somewhere else on a schedule, for example with `rclone` to object storage. Restore with:

```sh
docker compose -f docker-compose.prod.yaml exec -T postgres pg_restore -U simhook -d simhook --clean --if-exists < backups/simhook-YYYY-MM-DD-HHMM.dump
```

## Notes

- `SIMHOOK_SECRET_KEY` encrypts stored webhook secrets and signs sessions. Losing it means every webhook needs a new secret and everyone signs in again. Keep a copy off the host.
- The dashboard bakes the API origin into its build. Changing `API_DOMAIN` means rebuilding the `web` image.
- The API trusts forwarded client addresses only because compose sets `SIMHOOK_TRUST_PROXY=true`. Do not set that on an instance exposed without a proxy.
- Logs: `docker compose -f docker-compose.prod.yaml logs -f api web caddy`.
- `simhook.dev/download/android.json` and `/download/simhook.apk` redirect to the latest release under `ANDROID_RELEASES_URL` (default `https://github.com/simhook/simhook`). The phone app polls the first address for updates; see `android/RELEASING.md`.
