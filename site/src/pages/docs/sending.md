---
layout: ../../layouts/Doc.astro
title: Sending
description: "The send request, the send record, the life of a message, pacing, long texts, scheduling, and plan limits."
---

A send is one text to one or more recipients, sent from one phone. `POST /v1/messages` creates it; the phone does the rest.

## The request

| Field | Type | Notes |
|---|---|---|
| `to` | string[] | Recipient numbers, ideally E.164 such as `+14155550123`. One to 5,000. The plan caps how many per send. |
| `body` | string | The text, 1 to 1,600 characters. Long texts go out as concatenated SMS. |
| `device_id` | string | Which phone. Defaults to the account's default phone, else the most recently online one. |
| `sim_subscription_id` | integer | Which SIM on a dual-SIM phone. Unknown ids fall back to the phone's preferred SIM. |
| `scheduled_at` | timestamp | Send later, up to seven days ahead. |

```sh
curl https://api.simhook.dev/v1/messages \
  -H "X-Api-Key: sh_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "to": ["+15550001111", "+15550002222"],
    "body": "Doors open at 19:00. Reply STOP to opt out.",
    "device_id": "01a071d7-86c0-73f2-b1be-f5e3a59411b0"
  }'
```

The answer is `202` with a `batch`: the send record. Its counters (`dispatched_count`, `sent_count`, `delivered_count`, `failed_count`, `unknown_count`) move as the phone reports. Its `status` goes from `queued` to `processing` while messages go out, and ends as `completed` when every recipient was reached, `partial` when some failed, `failed` when all did, or `unknown` when reports never came. `GET /v1/batches/{id}` returns it at any time; `GET /v1/messages?batch_id=…` lists the individual messages.

## The life of a message

| Status | Meaning |
|---|---|
| `queued` | Accepted by the server, waiting for the phone. |
| `dispatched` | The phone has it and will send it at its pace. |
| `sent` | The carrier accepted it from the phone. |
| `delivered` | The carrier confirmed it reached the recipient's handset. |
| `failed` | The phone could not send it, or the carrier reported a failure. `error_code` and `error_message` say why. |
| `unknown` | No report arrived within the stale window. Rare; usually the phone lost power or signal mid-send. |

`sent` is what most SMS APIs call success. `delivered` is stronger: it is the carrier's delivery receipt, and some carriers and some recipients never produce one. Treat `sent` as done and `delivered` as a bonus, or wait for `delivered` when it matters.

Timestamps for each step are on the message: `dispatched_at`, `sent_at`, `delivered_at`, `failed_at`.

## Which phone, which SIM

Each account has a default phone; the dashboard sets it, or `POST /v1/devices/{id}/default`. A send without `device_id` goes to the default phone if it is online, otherwise to the phone that checked in most recently. A send to a phone that is offline waits in its queue until it comes back.

A phone lists its SIMs with their `subscription_id`. Pass one as `sim_subscription_id` to send from that SIM; leave it out to use the phone's preferred SIM, which you set per phone.

## Pacing

Carriers watch for machine-like sending. Every phone has a send delay, five seconds by default, and it pauses that long after each message. Change it per phone in the dashboard or with `PATCH /v1/devices/{id}` (`send_delay_seconds`, 0 to 3600). A send to 100 recipients at a five-second delay takes about eight minutes; the send record's `estimated_completion_at` tells you when to expect it to finish.

## Long texts

A single SMS holds 160 characters when the text fits the GSM-7 alphabet, and 70 when it needs UCS-2 (most non-Latin scripts, and emoji). Longer texts are split into parts of 153 or 67 characters and stitched together by the receiving phone. A part costs the carrier plan one SMS, but counts once against the simhook plan. The SDK has `countSegments()` to estimate the split before sending.

## Scheduling

Pass `scheduled_at` to hold a send until then. It counts against the plan when it goes out, not when it is created. Scheduled sends appear in the dashboard with their time and can be followed like any other.

## Limits and errors

Plans limit messages per day and per month, phones per account, and recipients per send. A request over a limit is refused with `429` and a code that says which limit: `plan_limit_daily`, `plan_limit_monthly`, or `plan_limit_batch`. `GET /v1/stats` reports the account's usage against its plan.

Validation problems come back as `422` with a `validation_failed` code and a per-field list:

```json
{
  "status": 422,
  "code": "validation_failed",
  "message": "The request has invalid fields.",
  "errors": [{ "field": "body.to[0]", "message": "not a phone number" }]
}
```

## Listing messages

`GET /v1/messages` returns messages newest first, fifty at a time. Filters: `direction` (`outbound` or `inbound`), `status`, `batch_id`, `device_ids` (comma separated), `q` (matches text, recipient, and sender), and `from` / `to` bounds on `created_at`. Page with `cursor` from the previous page's `next_cursor`.

To walk forward through new messages, ask for `order=asc` with a `from` bound and remember the last `created_at` you saw. Webhooks are the better option when you can expose an endpoint.
