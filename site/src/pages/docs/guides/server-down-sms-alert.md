---
layout: ../../../layouts/Doc.astro
title: SMS alert when a server is down
headTitle: "SMS alert when your server is down, from cron"
description: "A shell script that checks a URL from cron or a systemd timer and texts you once when it fails and once when it recovers, through your own SIM."
updated: 2026-09-06
---

Email gets filtered and chat apps get muted; a text on the phone in your pocket does not. This guide sets up a check that runs every minute and texts you once when a URL stops answering and once when it is back, sent through your own SIM by a phone on your desk. It needs nothing but `curl`, a state file, and one API call.

Run it from a machine other than the one you are watching. A check that lives on the server it checks goes down with it.

## What you need

- A simhook account with a paired phone, per the [quickstart](/docs). Keep that phone on power and, if it is strict about background apps, exempt from battery optimisation; the [app](/download) offers the setting.
- An API key with the `send` scope, kept in a file only root can read.
- A machine with `curl` and either cron or systemd.

## The script

`POST /v1/messages` takes `to`, a list of numbers, and `body`, the text. The state file remembers the last result, so the script texts on a change and stays silent otherwise.

```sh
#!/bin/sh
# check.sh: texts once when URL stops answering and once when it is back.
set -u
. /etc/simhook-alert.env          # SIMHOOK_API_KEY=sh_...  (chmod 600)

URL="https://example.com/healthz"
TO="+14155550123"
STATE="/var/tmp/simhook-check.state"

text() {
  curl -sS -o /dev/null -w '%{http_code}\n' https://api.simhook.dev/v1/messages \
    -H "X-Api-Key: $SIMHOOK_API_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"to\": [\"$TO\"], \"body\": \"$1\"}"
}

if curl -fsS --max-time 10 -o /dev/null "$URL"; then now=up; else now=down; fi
was=$(cat "$STATE" 2>/dev/null || echo up)
[ "$now" = "$was" ] && exit 0

echo "$now" > "$STATE"
stamp=$(date -u +%H:%M)
if [ "$now" = down ]; then
  text "DOWN $URL is not answering ($stamp UTC)"
else
  text "UP $URL is answering again ($stamp UTC)"
fi
```

`curl -f` treats any `4xx` or `5xx` as a failure, and `--max-time 10` treats a hang as one. The API answers `202`, and the send goes to the account's default phone, which picks it up within a second or two when it is online; `sent` and `delivered` follow as the carrier reports. The printed status code is the script's only output, so the log stays short: `202` means sent, `429` a plan limit, anything else is worth a look.

```sh
install -m 755 check.sh /usr/local/bin/check.sh
printf 'SIMHOOK_API_KEY=sh_...\n' > /etc/simhook-alert.env && chmod 600 /etc/simhook-alert.env
/usr/local/bin/check.sh            # run it once by hand
```

## Every minute with cron

```
* * * * * /usr/local/bin/check.sh >>/var/log/simhook-check.log 2>&1
```

`crontab -e` as a user that can read the env file. Nothing is printed on a quiet minute, so the log only grows when something changes.

## Or a systemd timer

Two units, then `systemctl enable --now simhook-check.timer`. `journalctl -u simhook-check` shows every run.

```ini
# /etc/systemd/system/simhook-check.service
[Unit]
Description=Text me when the site goes down or comes back

[Service]
Type=oneshot
ExecStart=/usr/local/bin/check.sh
```

```ini
# /etc/systemd/system/simhook-check.timer
[Unit]
Description=Run the site check every minute

[Timer]
OnBootSec=1min
OnUnitActiveSec=1min
AccuracySec=5s

[Install]
WantedBy=timers.target
```

## Know when the phone itself is off

The alert is only as good as the phone sending it. The phone checks in every 20 minutes and whenever the server pushes to it; when it misses its check-ins, the account gets a `device.offline` event, and `device.online` when it is back. Subscribe a webhook to both, with a key that has the `webhooks` scope, and point it at something other than the server you are watching:

```sh
curl https://api.simhook.dev/v1/webhooks \
  -H "X-Api-Key: sh_..." \
  -H "Content-Type: application/json" \
  -d '{"url": "https://hooks.example.net/simhook", "events": ["device.offline", "device.online", "message.failed"]}'
```

The delivery's `data` is the phone: its `name`, `id`, and `last_heartbeat_at`. Add `message.failed` so a text the phone could not send is not lost quietly; its `error_code` says why. A send to a phone that is offline waits in its queue for up to a day and goes out when the phone comes back, so a text that arrives late points at the phone, not the script. [Webhooks](/docs/webhooks) covers signatures and retries.

## Limits

Free is 30 messages a day and 500 a month. Over that the API refuses with `429` and `plan_limit_daily` or `plan_limit_monthly`, and the script prints the `429`. The state file is what keeps the count down: a site that flaps sends two texts per flap, never one a minute, and a site that is down for a day sends one. Every number in `to` is one message, so texting a team of three costs three a change. If the site is slow rather than down, lengthen `--max-time` before you shorten anything else. The [pricing page](/pricing) has the other plans.
