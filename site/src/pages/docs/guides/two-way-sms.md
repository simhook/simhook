---
layout: ../../../layouts/Doc.astro
title: Two-way SMS
headTitle: "Two-way SMS from your own number: auto-replies"
description: "Answer texts to your business number from code: a webhook handler that replies to keywords from the same phone and SIM, a STOP list you keep, and pacing."
updated: 2026-09-06
---

A shop, a clinic, a small team: people text the number they already have for you, and you want some of those texts answered at once and all of them in one place. simhook forwards each incoming text to your code as a webhook, and your code replies through the same phone and SIM, so the whole conversation happens on one number. No short code, no keyword registration, no rented number.

This guide builds a handler that answers two keywords, honours STOP, and leaves everything else to a person.

## How a conversation flows

1. Someone texts your number. The phone receives it and reports it, and the account gets a `message.received` event carrying `sender`, `body`, `device_id`, and `sim_subscription_id`.
2. Your handler decides what to answer and calls `POST /v1/messages` with `to` set to the sender and the same `device_id` and `sim_subscription_id`, so the reply leaves from the SIM the text came in on.
3. The phone sends it; `message.sent` and `message.delivered` follow as the carrier reports.

## Set up

Pair a phone as in the [quickstart](/docs); forwarding is on for every newly paired phone. Create an API key with the `send` scope and a webhook subscribed to `message.received`, and keep its secret. The handler below reads both from `SIMHOOK_API_KEY` and `SIMHOOK_WEBHOOK_SECRET`.

## The handler

It runs anywhere with a fetch-style `Request`: a Next.js route, a Worker, Bun, Deno. Express works the same way with a raw body; the [SDK page](/docs/sdk) shows it.

```ts
import { Simhook, SimhookError, SimhookSignatureError, constructWebhookEvent } from "@simhook/sdk";

const simhook = new Simhook({ apiKey: process.env.SIMHOOK_API_KEY });
const stopped = new Set<string>(); // stand-in for a table in your database

const answers: Record<string, string> = {
  HOURS: "Open Monday to Saturday, 9:00 to 18:00. Closed on Sunday.",
  ADDRESS: "12 Bridge Street, behind the market. Parking in the yard.",
};
const fallback = "Thanks, a person will reply during opening hours. Text HOURS or ADDRESS for a quick answer, STOP to opt out.";

export async function POST(request: Request) {
  const event = await constructWebhookEvent({
    payload: await request.text(),
    signature: request.headers.get("x-simhook-signature"),
    secret: process.env.SIMHOOK_WEBHOOK_SECRET!,
  }).catch((err) => {
    if (err instanceof SimhookSignatureError) return null;
    throw err;
  });
  if (!event) return new Response(null, { status: 401 });
  if (event.event !== "message.received" || !event.data.sender) return new Response(null, { status: 204 });

  const sender = event.data.sender;
  const { body, device_id, sim_subscription_id } = event.data;
  const keyword = body.trim().split(/\s+/)[0]?.toUpperCase() ?? "";
  let reply = answers[keyword] ?? fallback;
  if (keyword === "STOP") {
    stopped.add(sender);
    reply = "You will not get more texts from this number.";
  } else if (stopped.has(sender)) {
    return new Response(null, { status: 204 });
  }

  try {
    // Same phone and SIM the text arrived on, so the answer comes from the number they wrote to.
    await simhook.messages.send({ to: sender, body: reply, device_id, sim_subscription_id: sim_subscription_id ?? undefined });
  } catch (err) {
    if (!(err instanceof SimhookError && err.isPlanLimit)) throw err;
    console.warn("reply not sent:", err.code); // 204 anyway: a retry tomorrow would be worse than silence
  }
  return new Response(null, { status: 204 });
}
```

Answer within 30 seconds. The send call returns as soon as the server has queued the reply, so this handler does. A delivery that gets no `2xx` is retried for two days with the same `X-Simhook-Delivery` header, so if you do anything slower than one send, record the delivery id first.

## STOP is yours to keep

simhook has no opt-out registry. A STOP arrives like any other text, is forwarded, and is not acted on; the `Set` above is only a stand-in for a table in your database. Check that table before every send to a number, including reminders and anything sent from other code, and keep it for good. The [terms](/terms) put it plainly: when someone asks you to stop, stop. Most countries regulate who you may text and when, and that stays with you.

## Pacing and limits

The phone pauses after each text, five seconds by default, and works through its queue in order. Change the delay per phone in the dashboard or with `PATCH /v1/devices/{id}` and `send_delay_seconds`. A busy hour queues rather than fails, and `estimated_completion_at` on a send says when it will be done.

A consumer SIM is for a person's messages. Replies to people who wrote to you are exactly that; a broadcast to everyone who ever texted you is not, and carriers watch for it.

Replies count against the plan, one per recipient; received texts are free and uncounted. Free is 30 messages a day and 500 a month, so a shop that answers 15 texts a day fits and one that answers 50 does not. Over the limit the API answers `429` with `plan_limit_daily`, which the SDK exposes as `err.isPlanLimit`; the handler above logs it and moves on rather than letting the delivery retry. The [pricing page](/pricing) has the paid plans.

## The dashboard is the conversation view

The dashboard's Messages page lists both directions newest first, with the number on each row. Search for a number and you see one conversation: what they wrote, what your code answered, and whether it was delivered. When a text needs a person, reply from there with the send dialog, and it appears in the same list. The delivery log under Webhooks shows every call to your handler and what it answered, which is where to look first when a reply did not go out.
