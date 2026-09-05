# @simhook/sdk

TypeScript client for the [simhook](https://simhook.dev) API: send and receive SMS through your own Android phone.

- No dependencies. Uses `fetch` and Web Crypto, so it runs on Node 20+, Bun, Deno, Cloudflare Workers, and Vercel Edge. It is for servers: an API key in a browser is a key given away.
- Fully typed from the API's OpenAPI contract, including narrowed status and event names.
- Auto-pagination, send polling, timeouts, and retries for reads built in.
- Webhook signature verification and an SMS segment counter.

## Install

```sh
npm install @simhook/sdk
```

## Send a message

```ts
import { Simhook } from "@simhook/sdk";

const simhook = new Simhook({ apiKey: process.env.SIMHOOK_API_KEY });

const { batch } = await simhook.messages.send({
  to: "+14155550123",
  body: "Your table is ready.",
});

// Acceptance is not delivery. Follow the send until the phone reports back.
const result = await simhook.batches.waitUntilDone(batch.id);
console.log(result.batch.status, result.messages[0]?.status); // completed delivered
```

`to` accepts one number or an array. Add `device_id` to pick a phone, `sim_subscription_id` for a dual-SIM phone, or `scheduled_at` to send later.

## Read messages

```ts
// Newest first, one page at a time
const page = await simhook.messages.list({ direction: "inbound", limit: 50 });
const next = await simhook.messages.list({ direction: "inbound", cursor: page.next_cursor });

// Or let the SDK walk the pages
for await (const message of simhook.messages.iterate({ direction: "inbound" })) {
  console.log(message.sender, message.body);
}
```

To poll for new messages, ask for `order: "asc"` with a `from` bound and keep the last `created_at` you saw. Webhooks are the better option when you can expose an endpoint.

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

Create one and keep the secret; it is shown once.

```ts
const { webhook, secret } = await simhook.webhooks.create({
  url: "https://example.com/hooks/simhook",
  events: ["message.received", "message.delivered", "message.failed"],
});
```

Verify deliveries with the raw request body. Re-serialized JSON will not match.

```ts
import { constructWebhookEvent, SIGNATURE_HEADER, SimhookSignatureError } from "@simhook/sdk";

// Node with a raw body (Express: app.post(path, express.raw({ type: "*/*" }), handler))
app.post("/hooks/simhook", express.raw({ type: "*/*" }), async (req, res) => {
  try {
    const event = await constructWebhookEvent({
      payload: req.body,
      signature: req.header(SIGNATURE_HEADER),
      secret: process.env.SIMHOOK_WEBHOOK_SECRET,
    });
    if (event.event === "message.received") {
      console.log("text from", event.data.sender, ":", event.data.body);
    }
    res.sendStatus(204);
  } catch (err) {
    if (err instanceof SimhookSignatureError) return res.sendStatus(401);
    throw err;
  }
});

// Fetch-style runtimes (Workers, Next.js route handlers, Bun, Deno)
export async function POST(request: Request) {
  const event = await constructWebhookEvent({
    payload: await request.text(),
    signature: request.headers.get("x-simhook-signature"),
    secret: process.env.SIMHOOK_WEBHOOK_SECRET!,
  });
  return new Response(null, { status: 204 });
}
```

`verifyWebhookSignature` returns a boolean instead of throwing. `signWebhookPayload` produces a valid header for your own tests. Each delivery carries `X-Simhook-Delivery`; retries reuse the id, so you can de-duplicate on it.

## Errors

Every failure is a `SimhookError` with `status` (HTTP status, or 0 when no response arrived), a stable `code`, and per-field `errors` for validation problems.

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

Reads retry twice on 429 and 5xx responses and on network failures. Writes never retry, so a send is never duplicated by the SDK. Pass `{ signal }` to cancel a call, or `{ timeoutMs, maxRetries }` to override the defaults per call.

## Options

| Option | Default | Notes |
|---|---|---|
| `apiKey` | `SIMHOOK_API_KEY` env | Required |
| `baseUrl` | `SIMHOOK_BASE_URL` env, then `https://api.simhook.dev` | Point at your own host when self-hosting |
| `fetch` | global `fetch` | For proxies or tests |
| `timeoutMs` | `30000` | Per request |
| `maxRetries` | `2` | Reads only |

## Segment counter

```ts
import { countSegments } from "@simhook/sdk";

countSegments("Your code is 4921");
// { encoding: "GSM-7", length: 17, segments: 1, per_segment: 160, remaining: 143 }
```

An estimate following GSM 03.38; the phone performs the real split.
