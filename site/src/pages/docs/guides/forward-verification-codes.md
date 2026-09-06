---
layout: ../../../layouts/Doc.astro
title: Forward verification codes
headTitle: "Forward SMS verification codes to your app by webhook"
description: "Get one-time codes texted to a number you own into your code within seconds: a webhook on message.received, a regex, and two alternatives."
updated: 2026-09-06
---

A service texts a code to your number, and your code needs it: a sign-in you are automating, a test suite that registers accounts, an agent that has to finish a login. With simhook the phone that owns the number forwards each text to a webhook within a second or two of receiving it, and picking out the code is one regular expression.

This is for numbers you own and control. Forwarding someone else's codes is account takeover, whatever the reason, and it is against the [terms](/terms).

## 1. Pair a phone

Follow the [quickstart](/docs): an account, the [app](/download) on an Android phone with the SIM in it, the SMS permission granted, and the phone paired from the dashboard. Forwarding is on for every newly paired phone. Send yourself a text and check that it appears under Messages in the dashboard before going further.

## 2. Subscribe a webhook to message.received

Every text the phone receives becomes a `message.received` event. Subscribe an `https` endpoint with an API key that has the `webhooks` scope:

```sh
curl https://api.simhook.dev/v1/webhooks \
  -H "X-Api-Key: sh_..." \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/hooks/codes", "events": ["message.received"]}'
```

The response includes `secret`, shown once. Keep it where your handler can read it; below it is `SIMHOOK_WEBHOOK_SECRET`.

## 3. Pick the code out

Deliveries are signed, and the SDK verifies them. This handler runs on anything with a fetch-style `Request`: a Next.js route, a Worker, Bun, Deno. It ignores every text that is not from the sender you expect, and every text without a four-to-eight-digit number in it.

```ts
import { constructWebhookEvent, SimhookSignatureError } from "@simhook/sdk";

const SENDERS = new Set(["+12125550148"]); // the service that texts you codes, as the dashboard shows it
const CODE = /\b\d{4,8}\b/;

export async function POST(request: Request) {
  try {
    const event = await constructWebhookEvent({
      payload: await request.text(),
      signature: request.headers.get("x-simhook-signature"),
      secret: process.env.SIMHOOK_WEBHOOK_SECRET!,
    });
    if (event.event === "message.received" && SENDERS.has(event.data.sender ?? "")) {
      const code = event.data.body.match(CODE)?.[0];
      if (code) await useCode(code); // yours: finish the sign-in, post it to chat, store it
    }
    return new Response(null, { status: 204 });
  } catch (err) {
    if (err instanceof SimhookSignatureError) return new Response(null, { status: 401 });
    throw err;
  }
}
```

Three things to know. The sender of a code is often a short code or a name rather than a full number; send yourself one and copy what the dashboard shows as the sender. Answer within 30 seconds and do slow work after answering, or the delivery is retried. Retries carry the same `X-Simhook-Delivery` header, so if a code must be used exactly once, remember the delivery ids you have handled.

## Without an endpoint: poll

If nothing of yours is reachable from the internet, walk forward through new inbound messages with `messages.list`. `from` is an inclusive bound on `created_at`, so keep the ids of the last page to skip what you have already handled.

```ts
import { Simhook } from "@simhook/sdk";

const simhook = new Simhook({ apiKey: process.env.SIMHOOK_API_KEY });
let from = new Date();
let handled = new Set<string>();

for (;;) {
  const page = await simhook.messages.list({ direction: "inbound", order: "asc", from, limit: 100 });
  for (const m of page.data) {
    if (handled.has(m.id) || m.sender !== "+12125550148") continue;
    const code = m.body.match(/\b\d{4,8}\b/)?.[0];
    if (code) console.log("code", code);
  }
  if (page.data.length > 0) {
    from = new Date(page.data[page.data.length - 1].created_at);
    handled = new Set(page.data.map((m) => m.id));
  }
  await new Promise((r) => setTimeout(r, 3000));
}
```

Reads retry on their own, and the key only needs the `read` scope.

## From an agent

The [MCP server](/docs/mcp) has `wait_for_incoming_sms`. It takes `from_number`, `contains`, `since`, and `timeout_seconds` (up to 55, 45 by default), and returns the first matching text or, after the timeout, a `since` value to call again with. An agent asked to sign in somewhere calls it as soon as it has requested the code:

```
wait_for_incoming_sms({ from_number: "+12125550148", contains: "code", timeout_seconds: 55 })
```

A key with only the `read` scope makes that server read-only, which is the right key for a job that should never send anything.

## What Android does with codes

Android decides whether the app sees the text at all.

- Android 17 withholds any text that looks like a one-time code from apps targeting Android 17 for three hours, unless the app is the phone's default SMS app. The simhook app targets Android 16 on purpose, so ordinary codes arrive normally; this is verified on Android 17 devices.
- Texts in the WebOTP or SMS Retriever format, a line like `@example.com #482913`, are delayed three hours on Android 16 QPR2 and newer whatever the app targets. If you control the sender, do not use that format for texts you want forwarded.

The app reports each text as it arrives and does not scan the phone's message history, and it does not stop the text from landing in the phone's messaging app.

## Only for numbers you own

The phone, the SIM, and the account the code is for must be yours or your company's. Codes are secrets: keep the endpoint on `https`, verify every signature, and delete forwarded texts when you no longer need them. [Receiving](/docs/receiving) has the rest of what arrives with a text.
