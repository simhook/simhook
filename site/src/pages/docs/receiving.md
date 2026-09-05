---
layout: ../../layouts/Doc.astro
title: Receiving
description: "How texts that reach the phone reach you: forwarding, the inbound message, webhooks versus polling, and what Android does with one-time codes."
---

Every text the phone receives becomes a message on your account with `direction: "inbound"`, and goes out to your webhooks as `message.received`. Nothing else is needed: no keywords, no short codes, no carrier setup.

## Forwarding

Forwarding is on for every newly paired phone. Turn it off per phone in the app or in the dashboard (`receive_enabled`) when a phone should only send. The change reaches the phone within seconds.

The app reads the text as it arrives and reports it at once; it does not scan the phone's message history, and it does not stop the text from landing in the phone's messaging app.

## What arrives

An inbound message carries the sender, the text, when the phone received it, and which SIM received it:

```json
{
  "id": "01a071d8-1c5b-7a4d-8496-2129a0e8fb6e",
  "device_id": "01a071d7-86c0-73f2-b1be-f5e3a59411b0",
  "direction": "inbound",
  "status": "received",
  "sender": "+15550002222",
  "body": "Merhaba, siparişim ne zaman gelir?",
  "sim_subscription_id": 1,
  "received_at": "2026-09-05T13:53:02Z",
  "created_at": "2026-09-05T13:53:03.579Z"
}
```

Long texts arrive already joined. The phone fingerprints each text, so a text delivered twice by the network is reported once.

## Webhooks or polling

Subscribe a webhook to `message.received` and you get every text within a second or two of the phone receiving it, signed, with retries. [Webhooks](/docs/webhooks) covers verification.

Without an endpoint, poll `GET /v1/messages?direction=inbound&order=asc&from=<last created_at>` and remember the last `created_at` you saw. The [SDK](/docs/sdk) and the [MCP server](/docs/mcp) both have a wait-for-a-text helper built on this.

## Replying

A reply is a normal send to the sender's number, from the same phone and SIM if you want the conversation to stay on one number:

```sh
curl https://api.simhook.dev/v1/messages \
  -H "X-Api-Key: sh_..." \
  -H "Content-Type: application/json" \
  -d '{"to": ["+15550002222"], "body": "Yarın 14:00 ile 16:00 arası.", "device_id": "01a071d7-…", "sim_subscription_id": 1}'
```

## One-time codes

Verification codes are the most common thing people forward, and Android has rules about them.

- The simhook app targets Android 16 on purpose. Android 17 withholds any text that looks like a one-time code from apps targeting 17 for three hours unless they are the phone's default SMS app. At target 16 the text arrives normally, which is verified on Android 17 devices.
- Texts in the WebOTP or SMS Retriever format (a line like `@example.com #482913`) are delayed by three hours on Android 16 QPR2 and newer regardless of target. Ordinary codes are unaffected. If you control the sender, do not use that format for texts you want forwarded.

## Opt-outs and the law

Messages people send you are theirs; keep them only as long as you need them. If you send marketing, honour replies like STOP yourself: simhook forwards the reply, it does not act on it. Most countries regulate who you may text and when; that responsibility stays with you.
