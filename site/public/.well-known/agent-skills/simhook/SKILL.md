---
name: simhook
description: Send and receive SMS through a simhook account, where an Android phone's own SIM sends and receives behind a REST API at api.simhook.dev. Use when a task must text a phone number, follow a send to delivery, wait for an SMS reply or a verification code sent to a number the user owns, read message history, verify a simhook webhook, or set up @simhook/sdk or @simhook/mcp. Needs SIMHOOK_API_KEY.
license: MIT
metadata:
  homepage: https://simhook.dev
  source: https://github.com/simhook/simhook
---

# simhook

simhook turns an Android phone into an SMS API. The phone's SIM does the sending and receiving; the API at `https://api.simhook.dev` queues, paces, records, and reports. One message is one recipient. Sends count against the account's plan (Free: 30 a day, 500 a month); received texts are free. The documentation is available as Markdown from https://simhook.dev/llms.txt.

## Ground rules

- Authenticate with an API key from the dashboard: header `X-Api-Key: sh_...` (or `Authorization: Bearer sh_...`). Keys carry scopes: `send`, `read`, `devices`, `webhooks`. A `403` means the key lacks the scope the call needs: tell the user, do not retry.
- Text only numbers the user may text, one recipient per conversation, never bulk marketing from a consumer SIM. Forward verification codes only for numbers the user owns.
- A send is accepted (`202`) before it is sent. Never send again because a send is still pending: follow it instead.
- Never put the API key in browser code. It belongs on a server or in the agent's environment.
- Self-hosted simhook: use the user's own base URL. The SDK and the MCP server read `SIMHOOK_BASE_URL`.

## Send a text

```sh
curl https://api.simhook.dev/v1/messages \
  -H "X-Api-Key: $SIMHOOK_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to": ["+14155550123"], "body": "Your table is ready."}'
```

Body fields: `to` (one number or a list, with country code), `body`, and optionally `device_id`, `sim_subscription_id`, `scheduled_at` (RFC 3339). The answer is `202` with a `batch`, the send record: its `id`, `status`, and per-state counts.

With the SDK (Node 20+, Bun, Deno, edge runtimes; no dependencies):

```ts
import { Simhook } from "@simhook/sdk";

const simhook = new Simhook({ apiKey: process.env.SIMHOOK_API_KEY });
const { batch } = await simhook.messages.send({ to: "+14155550123", body: "Your table is ready." });
const result = await simhook.batches.waitUntilDone(batch.id); // polls with backoff until every message settles
console.log(result.batch.status, result.messages[0]?.status);
```

Follow a send with `GET /v1/batches/{id}`; list its messages with `GET /v1/messages?batch_id={id}`. A message moves `queued` (accepted), `dispatched` (on the phone), `sent` (the carrier accepted it), `delivered` (the carrier confirmed it). It ends in `failed` (with `error_code`) or `unknown` (no report arrived). Some carriers never confirm delivery, so `sent` can be the last word.

Pacing: a phone sends one text every `send_delay_seconds` (default 5), so 100 recipients take about eight minutes; the send reports `estimated_completion_at`. A phone that is offline holds its queue for up to a day. A long text is split by the phone and counts once.

## Wait for a reply or a code

Prefer a webhook when the user has an HTTPS endpoint. Poll otherwise. From an MCP client, use `wait_for_incoming_sms`.

Webhook: `POST /v1/webhooks` with `url` and `events` (for example `["message.received"]`); the answer includes `secret`, shown once. Every delivery carries `X-Simhook-Event`, `X-Simhook-Delivery` (the same id on every retry of that delivery) and `X-Simhook-Signature: t=<unix seconds>,v1=<hex>`, where hex is HMAC-SHA256 over `<t>.<raw request body>` keyed with the secret. Verify against the raw bytes, reject timestamps more than five minutes old, answer any `2xx` within 30 seconds, and do slow work after answering. Failed deliveries are retried for about two days.

```ts
import { constructWebhookEvent, SIGNATURE_HEADER, SimhookSignatureError } from "@simhook/sdk";

const event = await constructWebhookEvent({
  payload: rawBody, // the request body as received, not re-serialised
  signature: request.headers.get(SIGNATURE_HEADER),
  secret: process.env.SIMHOOK_WEBHOOK_SECRET,
}); // throws SimhookSignatureError on a bad signature
if (event.event === "message.received") handle(event.data.sender, event.data.body);
```

Events: `message.received`, `message.sent`, `message.delivered`, `message.failed`, `message.unknown`, `device.online`, `device.offline`, `ping`.

Polling: walk forward through inbound messages. `from` is an inclusive bound on `created_at`, so remember the ids already handled.

```ts
const page = await simhook.messages.list({ direction: "inbound", order: "asc", from: since, limit: 100 });
```

List filters: `direction`, `status`, `device_ids`, `batch_id`, `q` (matches text, recipient, and sender), `from`, `to`, `order`, `cursor`, `limit` (up to 100). `simhook.messages.iterate(filters)` walks the pages. One message: `GET /v1/messages/{id}`.

## Phones

`GET /v1/devices` (`simhook.devices.list()`) lists the paired phones with `online`, their SIMs, and battery. Sends go to the account's default phone unless `device_id` is given; on a dual-SIM phone, `sim_subscription_id` picks the SIM. Pairing needs a person holding the phone: `simhook.devices.createPairingCode()` returns `code`, `pair_url` (show it as a link or a QR code) and `expires_at`. Never report a phone as paired unless the device list shows it.

## MCP server

`npx -y @simhook/mcp` with `SIMHOOK_API_KEY` in its environment gives any MCP client eight tools: `send_sms`, `get_send_status`, `list_messages`, `get_message`, `wait_for_incoming_sms` (`from_number`, `contains`, `since`, `timeout_seconds` up to 55; call it again with the `since` it returns), `list_devices`, `get_account`, `count_sms_segments`. A key with only the `read` scope makes it read-only. Setup for Claude, Cursor, and others: https://simhook.dev/docs/mcp.md

## Errors and limits

Every error is JSON with a stable `code`. The SDK throws `SimhookError` with `status`, `code`, `fieldMessages()` for validation problems (`validation_failed`, for example `body.to[0]: not a phone number`), and `isPlanLimit` for `plan_limit_daily` and `plan_limit_monthly` (HTTP `429`). Reads retry on `429`, `5xx`, and network failures; writes never retry, so a send is never duplicated. Plan, limits, and usage: the MCP tool `get_account`, or the dashboard's Settings page.

## Reference

- API reference: https://simhook.dev/docs/api (OpenAPI document: https://api.simhook.dev/openapi.json)
- Sending: https://simhook.dev/docs/sending.md
- Receiving: https://simhook.dev/docs/receiving.md
- Webhooks: https://simhook.dev/docs/webhooks.md
- SDK: https://simhook.dev/docs/sdk.md
- Guides: https://simhook.dev/docs/guides/forward-verification-codes.md, https://simhook.dev/docs/guides/server-down-sms-alert.md, https://simhook.dev/docs/guides/two-way-sms.md
