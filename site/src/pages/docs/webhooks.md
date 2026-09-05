---
layout: ../../layouts/Doc.astro
title: Webhooks
description: "Events, the delivery payload, signature verification in three languages, retries, and auto-pause."
---

A webhook is a URL on your server that simhook calls when something happens: a text arrived, a message was delivered, a phone went offline. Each call is signed with a secret only you and simhook know.

## Creating one

In the dashboard under **Webhooks**, or with the API:

```sh
curl https://api.simhook.dev/v1/webhooks \
  -H "X-Api-Key: sh_live_..." \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/sms-events", "events": ["message.received", "message.delivered", "message.failed"]}'
```

The response includes `secret`. It is shown once; store it where your server can read it. `POST /v1/webhooks/{id}/rotate-secret` issues a new one when you need to. The URL must be `https`.

## Events

| Event | When | `data` |
|---|---|---|
| `message.sent` | The carrier accepted an outbound message from the phone. | The message |
| `message.delivered` | The carrier confirmed delivery to the handset. | The message |
| `message.failed` | The phone or the carrier could not deliver it. `error_code` says why. | The message |
| `message.unknown` | No outcome arrived within the stale window. | The message |
| `message.received` | The phone received a text. | The inbound message |
| `device.online` | A phone came back after being offline. | The phone |
| `device.offline` | A phone missed its check-ins. | The phone |
| `ping` | You pressed the test button, or called `POST /v1/webhooks/{id}/test`. | Nothing useful |

## The delivery

Each delivery is a `POST` with a JSON body and three headers:

```http
POST /sms-events HTTP/1.1
Content-Type: application/json
X-Simhook-Event: message.received
X-Simhook-Delivery: 01a071e4-2b1f-7d33-9c6a-2f0b7c1e9a10
X-Simhook-Signature: t=1788618234,v1=9f3c1a2d…

{
  "id": "01a071e4-2b1f-7d33-9c6a-2f0b7c1e9a10",
  "event": "message.received",
  "created_at": "2026-09-05T13:53:03.612Z",
  "data": {
    "id": "01a071d8-1c5b-7a4d-8496-2129a0e8fb6e",
    "direction": "inbound",
    "status": "received",
    "sender": "+15550002222",
    "body": "Merhaba, siparişim ne zaman gelir?",
    "received_at": "2026-09-05T13:53:02Z"
  }
}
```

Answer with any `2xx` within 30 seconds. Do the work afterwards if it takes longer. Retries reuse the same `X-Simhook-Delivery` id, so you can de-duplicate on it.

## Verifying the signature

The header is `t=<unix seconds>,v1=<hex>`. The hex is HMAC-SHA256 over the string `<t>.<raw request body>` with your secret as the key. Verify with the bytes you received; re-serialised JSON will not match. Reject timestamps more than five minutes from now, and compare in constant time.

With the SDK:

```ts
import { constructWebhookEvent, SimhookSignatureError } from "@simhook/sdk";

export async function POST(request: Request) {
  try {
    const event = await constructWebhookEvent({
      payload: await request.text(),
      signature: request.headers.get("x-simhook-signature"),
      secret: process.env.SIMHOOK_WEBHOOK_SECRET!,
    });
    if (event.event === "message.received") console.log(event.data.sender, event.data.body);
    return new Response(null, { status: 204 });
  } catch (err) {
    if (err instanceof SimhookSignatureError) return new Response(null, { status: 401 });
    throw err;
  }
}
```

By hand, in Node:

```js
import { createHmac, timingSafeEqual } from "node:crypto";

function verify(rawBody, header, secret) {
  const parts = Object.fromEntries(header.split(",").map((p) => p.split("=")));
  if (Math.abs(Date.now() / 1000 - Number(parts.t)) > 300) return false;
  const expected = createHmac("sha256", secret).update(`${parts.t}.${rawBody}`).digest("hex");
  return expected.length === parts.v1.length && timingSafeEqual(Buffer.from(expected), Buffer.from(parts.v1));
}
```

In Python:

```python
import hmac, hashlib, time

def verify(raw_body: bytes, header: str, secret: str) -> bool:
    parts = dict(p.split("=", 1) for p in header.split(","))
    if abs(time.time() - int(parts["t"])) > 300:
        return False
    expected = hmac.new(secret.encode(), f"{parts['t']}.".encode() + raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, parts["v1"])
```

## Retries

A delivery that fails (no answer, a network error, or a non-`2xx` status) is retried after 1 minute, then 5 and 15 minutes, then 1, 3, 6, 12, and 24 hours: nine attempts over about two days. A `4xx` answer other than `408` and `429` means the request itself was rejected, so those stop after three attempts.

Every attempt is in the delivery log: `GET /v1/webhooks/deliveries`, or the **Deliveries** tab of a webhook in the dashboard, with the status, the answer, and the time.

## Auto-pause

An endpoint that has failed at least 20 times in the last seven days, with failures making up at least half of its deliveries and no success in the last 24 hours, is paused so it stops burning retries. Paused webhooks show as such in the dashboard; fix the endpoint and turn it back on, or `PATCH /v1/webhooks/{id}` with `enabled: true`. Deliveries missed while paused are not replayed; the messages themselves are still on the account.

## Testing

The **Send test** button, or `POST /v1/webhooks/{id}/test`, sends a `ping` event through the same path as real deliveries, signature included. The SDK's `signWebhookPayload` produces a valid header for your own test suite.
