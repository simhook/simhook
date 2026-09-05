---
layout: ../layouts/Page.astro
title: Privacy
description: "What simhook stores, why, for how long, and who else sees it."
---

simhook relays text messages through your own phone. To do that it has to hold some of your data. This page says what, in plain words. Last updated 5 September 2026.

## What we store

- **Your account.** The email address you sign up with and a hash of your password. Never the password itself.
- **Your messages.** The text, the numbers involved, the timestamps, and the delivery states of every message sent or received through simhook, so you can see them in the dashboard and read them through the API. This includes the content of texts the phone receives while forwarding is on.
- **Your phones.** The model, Android version, app version, SIM details (carrier, country, subscription id, never the phone number of the SIM unless the phone reports one), and at each check-in the battery level, network type, and a few similar figures. They are shown in the dashboard and used to decide whether a phone is online.
- **Your integrations.** Webhook URLs and their signing secrets (stored encrypted), API keys (stored hashed), and the log of every webhook delivery, including what your server answered.
- **Server logs.** Request logs with IP addresses, kept for a short time for debugging and abuse prevention.

## What we do not do

- No analytics or tracking on this site, in the dashboard, or in the app. No advertising. The one third-party script is Cloudflare Turnstile on the sign-in, sign-up, and password reset forms, which tells people from bots; it may set a cookie of its own to do that.
- No reading of your messages by people, except when you ask for help with a specific one, or when abuse is reported.
- No selling or sharing of your data. It is used to run the service and for nothing else.

## Who else sees data

- **Hetzner** hosts the servers.
- **Resend** sends the emails the service needs: verification, password reset, and notices about your account.
- **Google Firebase Cloud Messaging** delivers pushes to your phone. A push carries only a nudge to check in, never message content.
- **Google** is involved if you sign in with Google: it tells us your Google account id, email address, name, and picture, and we keep the id to recognise you next time. It learns that you signed in to simhook and nothing else.
- **Cloudflare** sits in front of the site and the API and sees the traffic in transit, and runs the bot check on the sign-in forms.

## How long

Messages and delivery logs stay until you delete them or your account. Server logs are dropped after a few days. Everything about an account is deleted when the account is deleted; write to security@simhook.dev to have that done and it will be, within a week.

## Your side of it

The messages people send to your phone are theirs as much as yours. Keep what you need, delete the rest, and follow the law where you and they are.

## Self-hosting

None of the above applies to a simhook you run yourself: the data stays on your server and this page is not about it.

## Questions

security@simhook.dev.
