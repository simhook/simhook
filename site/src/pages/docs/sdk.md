---
layout: ../../layouts/Doc.astro
title: SDK
description: "The TypeScript client for the simhook API. No dependencies, typed from the OpenAPI contract, runs anywhere fetch does."
---

`@simhook/sdk` is a thin, typed client over `fetch`. It has no dependencies, so it runs on Node 20 and newer, Bun, Deno, Cloudflare Workers, Vercel Edge, and in browsers. Field names are the API's own, in snake_case.

```sh
npm install @simhook/sdk
```

```ts
import { Simhook } from "@simhook/sdk";

const simhook = new Simhook({ apiKey: process.env.SIMHOOK_API_KEY });
```

`apiKey` falls back to the `SIMHOOK_API_KEY` environment variable, and `baseUrl` to `SIMHOOK_BASE_URL`, then to `https://api.simhook.dev`.

## Sending

```ts
const { batch } = await simhook.messages.send({
  to: "+14155550123",
  body: "Your table is ready.",
});

// Acceptance is not delivery. Follow the send until the phone reports back.
const result = await simhook.batches.waitUntilDone(batch.id);
console.log(result.batch.status, result.messages[0]?.status); // completed delivered
```

`to` takes one number or an array. Add `device_id`, `sim_subscription_id`, or `scheduled_at` as in the [API](/docs/sending). `waitUntilDone` polls with backoff and stops at a timeout you can set.

## Reading

```ts
const page = await simhook.messages.list({ direction: "inbound", limit: 50 });
const next = await simhook.messages.list({ direction: "inbound", cursor: page.next_cursor });

// Or let the SDK walk the pages
for await (const message of simhook.messages.iterate({ direction: "inbound" })) {
  console.log(message.sender, message.body);
}
```

## Phones

```ts
const devices = await simhook.devices.list();
const online = devices.filter((d) => d.online);

await simhook.devices.update(online[0].id, { send_delay_seconds: 3 });
await simhook.devices.setDefault(online[0].id);

// Pair a new phone: show pair_url as a QR code, or code as text
const { code, pair_url, expires_at } = await simhook.devices.createPairingCode();
```

## Webhooks

```ts
const { webhook, secret } = await simhook.webhooks.create({
  url: "https://example.com/hooks/simhook",
  events: ["message.received", "message.delivered", "message.failed"],
});
```

Verify deliveries with the raw request body:

```ts
import { constructWebhookEvent, SIGNATURE_HEADER, SimhookSignatureError } from "@simhook/sdk";

// Express, with a raw body
app.post("/hooks/simhook", express.raw({ type: "*/*" }), async (req, res) => {
  try {
    const event = await constructWebhookEvent({
      payload: req.body,
      signature: req.header(SIGNATURE_HEADER),
      secret: process.env.SIMHOOK_WEBHOOK_SECRET,
    });
    if (event.event === "message.received") console.log("text from", event.data.sender, ":", event.data.body);
    res.sendStatus(204);
  } catch (err) {
    if (err instanceof SimhookSignatureError) return res.sendStatus(401);
    throw err;
  }
});
```

`verifyWebhookSignature` returns a boolean instead of throwing, and `signWebhookPayload` makes a valid header for your own tests.

## Errors

Every failure is a `SimhookError` with `status` (the HTTP status, or 0 when nothing came back), a stable `code`, and per-field `errors` for validation problems.

```ts
import { SimhookError } from "@simhook/sdk";

try {
  await simhook.messages.send({ to: "not a number", body: "hi" });
} catch (err) {
  if (err instanceof SimhookError) {
    err.code;            // "validation_failed"
    err.fieldMessages(); // { "body.to[0]": "not a phone number" }
    err.isPlanLimit;     // true for plan_limit_daily, plan_limit_monthly, ...
  }
}
```

Reads retry twice on `429`, `5xx`, and network failures. Writes never retry, so the SDK never duplicates a send. Pass `{ signal }` to cancel a call, or `{ timeoutMs, maxRetries }` to override the defaults for one call.

## Options

| Option | Default | Notes |
|---|---|---|
| `apiKey` | `SIMHOOK_API_KEY` | Required |
| `baseUrl` | `SIMHOOK_BASE_URL`, then `https://api.simhook.dev` | Your own host when self-hosting |
| `fetch` | global `fetch` | For proxies or tests |
| `timeoutMs` | `30000` | Per request |
| `maxRetries` | `2` | Reads only |

## Segments

```ts
import { countSegments } from "@simhook/sdk";

countSegments("Your code is 4921");
// { encoding: "GSM-7", length: 17, segments: 1, per_segment: 160, remaining: 143 }
```

An estimate following GSM 03.38; the phone performs the real split.
