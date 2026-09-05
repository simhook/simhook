# Security

simhook relays SMS through people's own phones. A flaw here can expose message contents or let someone send texts from a SIM they do not own, so reports are welcome and taken seriously.

## Reporting

Write to **security@simhook.dev**. Please do not open a public issue for anything that could be exploited before it is fixed.

Include the component (API, dashboard, Android app, SDK, MCP server, deployment), steps to reproduce, and the impact you believe it has. Encryption is not required.

## What to expect

- An acknowledgement within three working days.
- A fix or a mitigation with a timeline, and a note when it ships.
- Credit in the release notes if you want it. There is no bug bounty.

## Scope

In scope: everything in this repository and the hosted service at simhook.dev.

Out of scope: carrier behaviour, findings that need a rooted phone or physical access to it, denial of service by sheer volume, and automated scanner output without a demonstrated impact.

## Supported versions

The latest release of each component. Fixes ship as new releases; installed phone apps offer the update themselves.
