import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { InMemoryTransport } from "@modelcontextprotocol/sdk/inMemory.js";
import { Simhook } from "@simhook/sdk";
import { describe, expect, it } from "vitest";
import { createServer } from "../src/server";

type Handler = (url: URL, init: RequestInit) => Response;

interface TextBlock {
  type: string;
  text: string;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
}

function textOf(result: unknown): string {
  return ((result as { content?: TextBlock[] }).content ?? []).map((c) => c.text).join("\n");
}

const device = {
  id: "d1",
  name: "Pixel 8",
  online: true,
  is_default: true,
  enabled: true,
  receive_enabled: true,
  sims: [{ subscription_id: 1, slot: 0, carrier: "T-Mobile" }],
  telemetry: { battery_percent: 80 },
  last_heartbeat_at: "2026-09-05T00:00:00Z",
};

function message(over: Record<string, unknown> = {}) {
  return {
    id: "m1",
    batch_id: "b1",
    device_id: "d1",
    direction: "outbound",
    status: "delivered",
    body: "hi",
    recipient: "+15550001111",
    sender: null,
    error_code: null,
    error_message: null,
    created_at: "2026-09-05T00:00:00Z",
    updated_at: "2026-09-05T00:00:05Z",
    ...over,
  };
}

function batch(over: Record<string, unknown> = {}) {
  return {
    id: "b1",
    status: "completed",
    recipient_count: 1,
    dispatched_count: 1,
    sent_count: 0,
    delivered_count: 1,
    failed_count: 0,
    unknown_count: 0,
    body: "hi",
    scheduled_at: null,
    error: null,
    created_at: "2026-09-05T00:00:00Z",
    ...over,
  };
}

async function harness(handler: Handler) {
  const calls: { url: URL; init: RequestInit }[] = [];
  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    calls.push({ url, init: init ?? {} });
    return handler(url, init ?? {});
  }) as unknown as typeof fetch;
  const simhook = new Simhook({ apiKey: "sh_test", baseUrl: "https://api.example.test", fetch: fetchImpl });
  const server = createServer({ client: simhook });
  const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
  await server.connect(serverTransport);
  const client = new Client({ name: "test", version: "0.0.0" });
  await client.connect(clientTransport);
  return {
    client,
    calls,
    close: async () => {
      await client.close();
      await server.close();
    },
  };
}

describe("simhook MCP server", () => {
  it("lists its tools", async () => {
    const h = await harness(() => json({}));
    const { tools } = await h.client.listTools();
    expect(tools.map((t) => t.name).sort()).toEqual([
      "count_sms_segments",
      "get_account",
      "get_message",
      "get_send_status",
      "list_devices",
      "list_messages",
      "send_sms",
      "wait_for_incoming_sms",
    ]);
    await h.close();
  });

  it("sends and reports the outcome", async () => {
    const h = await harness((url, init) => {
      if (init.method === "POST" && url.pathname === "/v1/messages") return json({ batch: batch({ status: "queued" }), message_ids: ["m1"] }, 202);
      if (url.pathname === "/v1/batches/b1") return json({ batch: batch(), messages: [message()] });
      return json({ status: 404, code: "not_found", message: "No such resource." }, 404);
    });
    const res = await h.client.callTool({ name: "send_sms", arguments: { to: ["+15550001111"], body: "hi", wait_seconds: 5 } });
    expect(res.isError).toBeFalsy();
    const sent = h.calls.find((c) => c.init.method === "POST")!;
    expect(JSON.parse(String(sent.init.body))).toEqual({ to: ["+15550001111"], body: "hi" });
    expect((res.structuredContent as { batch: { status: string } }).batch.status).toBe("completed");
    expect(textOf(res)).toContain("completed");
    expect(textOf(res)).toContain("+15550001111: delivered");
    await h.close();
  });

  it("says when a send is still pending", async () => {
    const h = await harness((url, init) => {
      if (init.method === "POST") return json({ batch: batch({ status: "queued" }), message_ids: ["m1"] }, 202);
      if (url.pathname === "/v1/batches/b1") return json({ batch: batch({ status: "processing", delivered_count: 0 }), messages: [message({ status: "dispatched" })] });
      return json({}, 404);
    });
    const res = await h.client.callTool({ name: "send_sms", arguments: { to: ["+15550001111"], body: "hi", wait_seconds: 0 } });
    expect(textOf(res)).toContain("do not send again");
    await h.close();
  });

  it("turns api errors into tool errors", async () => {
    const h = await harness(() => json({ status: 429, code: "plan_limit_daily", message: "Daily limit reached." }, 429));
    const res = await h.client.callTool({ name: "send_sms", arguments: { to: ["+15550001111"], body: "hi", wait_seconds: 0 } });
    expect(res.isError).toBe(true);
    expect(textOf(res)).toContain("plan_limit_daily");
    await h.close();
  });

  it("lists devices", async () => {
    const h = await harness(() => json({ data: [device] }));
    const res = await h.client.callTool({ name: "list_devices", arguments: {} });
    expect(textOf(res)).toContain("Pixel 8");
    expect(textOf(res)).toContain("online, default");
    expect(textOf(res)).toContain("T-Mobile");
    await h.close();
  });

  it("lists messages with filters", async () => {
    const h = await harness(() => json({ data: [message()], next_cursor: "abc" }));
    const res = await h.client.callTool({ name: "list_messages", arguments: { direction: "outbound", device_id: "d1", limit: 5 } });
    const url = h.calls[0]!.url;
    expect(url.searchParams.get("direction")).toBe("outbound");
    expect(url.searchParams.get("device_ids")).toBe("d1");
    expect(url.searchParams.get("limit")).toBe("5");
    expect(textOf(res)).toContain("cursor abc");
    await h.close();
  });

  it("counts segments without touching the api", async () => {
    const h = await harness(() => {
      throw new Error("the api must not be called");
    });
    const res = await h.client.callTool({ name: "count_sms_segments", arguments: { text: "hello" } });
    expect(res.structuredContent).toMatchObject({ segments: 1, encoding: "GSM-7", length: 5 });
    expect(h.calls).toHaveLength(0);
    await h.close();
  });

  it("waits for an incoming message", async () => {
    let polls = 0;
    const h = await harness(() => {
      polls++;
      return json({
        data: polls < 2 ? [] : [message({ id: "m9", direction: "inbound", status: "received", sender: "+1 (555) 000-2222", recipient: null, body: "Your code is 4921" })],
      });
    });
    const res = await h.client.callTool({
      name: "wait_for_incoming_sms",
      arguments: { from_number: "5550002222", contains: "code", timeout_seconds: 10 },
    });
    expect(res.structuredContent).toMatchObject({ found: true, message: { id: "m9" } });
    expect(polls).toBe(2);
    await h.close();
  });

  it("gives up waiting at the timeout", async () => {
    const h = await harness(() => json({ data: [] }));
    const res = await h.client.callTool({ name: "wait_for_incoming_sms", arguments: { timeout_seconds: 1 } });
    expect(res.structuredContent).toMatchObject({ found: false });
    expect(textOf(res)).toContain("since=");
    await h.close();
  });
});
