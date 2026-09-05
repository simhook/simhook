---
layout: ../../layouts/Doc.astro
title: Quickstart
description: "From nothing to a delivered text in ten minutes. An account, a phone, a key, one request, one webhook."
---

simhook has three parts: an Android phone with a SIM that does the sending and receiving, a server that queues messages and talks to the phone, and your code, which talks to the server. This page gets all three connected.

## 1. Create an account

Sign up at [app.simhook.dev](https://app.simhook.dev/register) and confirm your email. Every account starts on the Free plan: 30 messages a day, one phone, the whole API.

## 2. Put the app on a phone

Any Android phone from 8.0 up with a SIM in it. [Download the app](/download), open the file, allow the install, then grant the SMS permission when the app asks. The phone does not need to be new or fast; it needs signal and power.

## 3. Pair it

In the dashboard, open **Phones** and choose **Pair a phone**. Scan the QR code with the app, or type the code, or open the link on the phone. Codes last ten minutes.

Once paired, the phone checks in every 20 minutes and whenever the server pushes to it. The dashboard shows it as online, with its SIMs, battery, and network.

## 4. Make an API key

Open **API keys**, create one, and copy it. It is shown once. Keys have scopes: `send`, `read`, `devices`, and `webhooks`. A key with only `send` can send and nothing else.

Pass it in the `X-Api-Key` header. `Authorization: Bearer <key>` works too.

## 5. Send a message

```sh
curl https://api.simhook.dev/v1/messages \
  -H "X-Api-Key: sh_..." \
  -H "Content-Type: application/json" \
  -d '{"to": ["+15550001111"], "body": "Your code is 482913"}'
```

The server answers `202 Accepted` with a **send**, the record that tracks one text to one or more recipients:

```json
{
  "batch": {
    "id": "01a071d8-819e-78f7-ae4f-9f71156d265f",
    "device_id": "01a071d7-86c0-73f2-b1be-f5e3a59411b0",
    "body": "Your code is 482913",
    "recipient_count": 1,
    "status": "queued",
    "dispatched_count": 0,
    "sent_count": 0,
    "delivered_count": 0,
    "failed_count": 0,
    "created_at": "2026-09-05T13:53:28.061Z"
  }
}
```

Acceptance is not delivery. The phone picks the message up within a second or two, sends it, and reports back.

## 6. Follow it

Ask for the send, or for the message itself:

```sh
curl https://api.simhook.dev/v1/batches/01a071d8-819e-78f7-ae4f-9f71156d265f -H "X-Api-Key: sh_..."
curl "https://api.simhook.dev/v1/messages?batch_id=01a071d8-819e-78f7-ae4f-9f71156d265f" -H "X-Api-Key: sh_..."
```

A message moves through `queued`, `dispatched` (on the phone), `sent` (the carrier took it), and `delivered` (the carrier confirmed it reached the handset). It ends in `failed` if the phone could not send it, or `unknown` if no report ever came. [Sending](/docs/sending) has the details.

## 7. Get texts back

Every text the phone receives is forwarded to you. Add a webhook in the dashboard or with the API, subscribe to `message.received`, and keep the secret it shows you:

```sh
curl https://api.simhook.dev/v1/webhooks \
  -H "X-Api-Key: sh_..." \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/sms-events", "events": ["message.received", "message.delivered", "message.failed"]}'
```

Deliveries are signed. [Webhooks](/docs/webhooks) shows how to verify them in a few lines.

## Next

- Use the [TypeScript SDK](/docs/sdk) instead of raw HTTP.
- Let an agent send and read texts through the [MCP server](/docs/mcp).
- Run the whole thing on your own server: [self-hosting](/docs/self-hosting).
- Everything the API can do: the [reference](/docs/api).
