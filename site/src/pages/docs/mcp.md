---
layout: ../../layouts/Doc.astro
title: MCP server
description: "Let Claude, Cursor, or any MCP client send and read texts through your phone."
---

`@simhook/mcp` is an [MCP](https://modelcontextprotocol.io) server over stdio. Add it to a client and the agent gets eight tools for texting through your phone, with the same limits and the same signed audit trail as everything else on the account.

## Setup

1. Pair a phone and create an API key with the `send`, `read`, and `devices` scopes.
2. Add the server to your client.

Claude Desktop, Cursor, and most others (`claude_desktop_config.json`, `.cursor/mcp.json`, and so on):

```json
{
  "mcpServers": {
    "simhook": {
      "command": "npx",
      "args": ["-y", "@simhook/mcp"],
      "env": { "SIMHOOK_API_KEY": "sh_live_..." }
    }
  }
}
```

Claude Code:

```sh
claude mcp add simhook -e SIMHOOK_API_KEY=sh_live_... -- npx -y @simhook/mcp
```

Self-hosting? Add `SIMHOOK_BASE_URL` with your API origin.

## Tools

| Tool | What it does |
|---|---|
| `send_sms` | Sends a text to one or more numbers and reports the outcome per recipient. Waits up to `wait_seconds` for the phone. |
| `get_send_status` | Follows up on a send that was still pending. |
| `list_messages` | Sent and received messages, filtered by direction, status, phone, send, text, and time. Paginated. |
| `get_message` | One message with its delivery state. |
| `wait_for_incoming_sms` | Blocks until a text arrives, optionally from a given number or containing given text. |
| `list_devices` | Paired phones with online state, SIMs, and battery. |
| `get_account` | Plan, limits, usage, and lifetime totals. |
| `count_sms_segments` | Estimates how many SMS parts a text needs. No API call. |

Every tool returns readable text plus structured content for clients that use it.

## Behaviour worth knowing

- Sends count against the plan. The server tells agents to follow a pending send with `get_send_status` rather than sending again.
- Waiting tools stop at 55 seconds per call to stay under common client timeouts. The reply includes a `since` value to continue waiting with.
- API errors come back as tool errors with the API's error code, so the agent can explain a limit or a bad number instead of failing silently.
- stdout carries the protocol; diagnostics go to stderr.
- A key with only the `read` scope makes a read-only server: the agent can watch for texts but not send any.

## In your own code

```ts
import { createServer } from "@simhook/mcp";
import { Simhook } from "@simhook/sdk";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";

const server = createServer({ client: new Simhook({ apiKey: "sh_live_..." }) });
await server.connect(new StdioServerTransport());
```
