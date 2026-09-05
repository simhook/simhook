import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { Simhook, SimhookError, countSegments } from "@simhook/sdk";
import { z } from "zod";
import { batchDetailText, deviceLine, limit, messageLine } from "./format";
import { VERSION } from "./version";

export interface CreateServerOptions {
  /** A configured SDK client. Defaults to one built from SIMHOOK_API_KEY and SIMHOOK_BASE_URL. */
  client?: Simhook;
}

const INSTRUCTIONS = [
  "simhook sends and receives SMS through the user's own Android phone.",
  "Use send_sms to text someone and wait_for_incoming_sms to wait for a reply.",
  "Every recipient of a send counts against the user's plan, so never repeat a send just because the outcome is still pending; check get_send_status instead.",
  "list_devices shows which phones are paired and online; a send needs an online phone.",
].join(" ");

const MESSAGE_STATUSES = ["queued", "dispatched", "sent", "delivered", "failed", "unknown", "received"] as const;

const messageShape = z.looseObject({ id: z.string(), direction: z.string(), status: z.string(), body: z.string() });
const batchShape = z.looseObject({ id: z.string(), status: z.string(), recipient_count: z.number() });
const deviceShape = z.looseObject({ id: z.string(), name: z.string(), online: z.boolean() });

function reply(text: string, structured?: Record<string, unknown>): CallToolResult {
  return { content: [{ type: "text", text }], ...(structured ? { structuredContent: structured } : {}) };
}

function failure(err: unknown): CallToolResult {
  if (err instanceof SimhookError) {
    const fields = Object.entries(err.fieldMessages())
      .map(([field, message]) => `${field}: ${message}`)
      .join("; ");
    return { content: [{ type: "text", text: `simhook error ${err.code}: ${err.message}${fields ? ` (${fields})` : ""}` }], isError: true };
  }
  throw err;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function digits(value: string): string {
  return value.replace(/\D/g, "");
}

function sameNumber(a: string, b: string): boolean {
  const x = digits(a);
  const y = digits(b);
  if (!x || !y) return false;
  return x === y || x.endsWith(y) || y.endsWith(x);
}

const readOnly = { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: true };

/** Builds the MCP server. Connect it to a transport to start serving. */
export function createServer(options: CreateServerOptions = {}): McpServer {
  const simhook = options.client ?? new Simhook();
  const server = new McpServer({ name: "simhook", version: VERSION }, { instructions: INSTRUCTIONS });

  server.registerTool(
    "send_sms",
    {
      title: "Send SMS",
      description:
        "Sends a text message from the user's Android phone to one or more numbers. Returns the outcome per recipient once the phone reports it, or the pending state if it takes longer than wait_seconds.",
      inputSchema: {
        to: z.array(z.string().min(3).max(32)).min(1).max(100).describe("Recipient phone numbers, ideally in E.164 form such as +14155550123."),
        body: z.string().min(1).max(1600).describe("Message text. Over 160 GSM characters (70 for other alphabets) it goes out as several SMS parts."),
        device_id: z.string().optional().describe("Phone to send from. Omit to use the account's default phone."),
        sim_subscription_id: z.number().int().optional().describe("SIM to use on a dual-SIM phone. See list_devices for subscription ids."),
        scheduled_at: z.iso.datetime({ offset: true }).optional().describe("ISO 8601 time to send at, up to 7 days ahead. Omit to send now."),
        wait_seconds: z.number().int().min(0).max(55).default(20).describe("Seconds to wait for the phone to report an outcome. 0 returns as soon as the send is queued."),
      },
      outputSchema: { batch: batchShape, messages: z.array(messageShape) },
      annotations: { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: true },
    },
    async ({ to, body, device_id, sim_subscription_id, scheduled_at, wait_seconds }) => {
      try {
        const { batch } = await simhook.messages.send({ to, body, device_id, sim_subscription_id, scheduled_at });
        const wait = scheduled_at ? 0 : wait_seconds;
        const detail = wait > 0 ? await simhook.batches.waitUntilDone(batch.id, { timeoutMs: wait * 1000, intervalMs: 1500 }) : await simhook.batches.get(batch.id);
        const pending = detail.batch.status === "queued" || detail.batch.status === "processing";
        const note = pending ? `\nNo final outcome yet. Check later with get_send_status (send_id ${batch.id}); do not send again.` : "";
        return reply(batchDetailText(detail) + note, { batch: detail.batch, messages: detail.messages });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "get_send_status",
    {
      title: "Get send status",
      description: "Shows a send (batch) with the status of every recipient. Use it to follow up on a send_sms call that was still pending.",
      inputSchema: { send_id: z.string().describe("The batch id returned by send_sms.") },
      outputSchema: { batch: batchShape, messages: z.array(messageShape) },
      annotations: readOnly,
    },
    async ({ send_id }) => {
      try {
        const detail = await simhook.batches.get(send_id);
        return reply(batchDetailText(detail), { batch: detail.batch, messages: detail.messages });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "list_messages",
    {
      title: "List messages",
      description: "Lists sent and received SMS, newest first by default. Filter by direction, status, phone, send, text, or time range. Follow next_cursor for more.",
      inputSchema: {
        direction: z.enum(["inbound", "outbound"]).optional().describe("inbound = received by the phone, outbound = sent through simhook."),
        status: z.enum(MESSAGE_STATUSES).optional(),
        device_id: z.string().optional().describe("Only messages on this phone."),
        send_id: z.string().optional().describe("Only messages from one send."),
        search: z.string().max(200).optional().describe("Text to match in the body, sender, or recipient."),
        since: z.iso.datetime({ offset: true }).optional().describe("Only messages created at or after this time."),
        until: z.iso.datetime({ offset: true }).optional().describe("Only messages created before this time."),
        order: z.enum(["desc", "asc"]).default("desc").describe("desc for newest first, asc to walk forward in time."),
        limit: z.number().int().min(1).max(100).default(20),
        cursor: z.string().optional().describe("next_cursor from a previous call."),
      },
      outputSchema: { messages: z.array(messageShape), next_cursor: z.string().optional() },
      annotations: readOnly,
    },
    async ({ direction, status, device_id, send_id, search, since, until, order, limit: pageSize, cursor }) => {
      try {
        const page = await simhook.messages.list({
          direction,
          status,
          device_ids: device_id ? [device_id] : undefined,
          batch_id: send_id,
          q: search,
          from: since,
          to: until,
          order,
          limit: pageSize,
          cursor,
        });
        const lines = page.data.length > 0 ? page.data.map(messageLine) : ["No messages match."];
        if (page.next_cursor) lines.push(`More available: call again with cursor ${page.next_cursor}.`);
        return reply(lines.join("\n"), { messages: page.data, next_cursor: page.next_cursor });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "get_message",
    {
      title: "Get message",
      description: "Fetches one message by id with its current delivery state.",
      inputSchema: { message_id: z.string() },
      outputSchema: { message: messageShape },
      annotations: readOnly,
    },
    async ({ message_id }) => {
      try {
        const message = await simhook.messages.get(message_id);
        return reply(messageLine(message), { message });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "wait_for_incoming_sms",
    {
      title: "Wait for incoming SMS",
      description:
        "Waits for a text message to arrive on the user's phone, optionally from a given number or containing given text. Good for one-time codes and replies. Returns when a match arrives or the timeout passes.",
      inputSchema: {
        from_number: z.string().optional().describe("Only accept messages from this number. Digits are compared loosely, so +1 (555) 000-1111 matches +15550001111."),
        contains: z.string().optional().describe("Only accept messages whose text contains this, case-insensitive."),
        since: z.iso.datetime({ offset: true }).optional().describe("Ignore messages received before this time. Defaults to now."),
        timeout_seconds: z.number().int().min(1).max(55).default(45),
      },
      outputSchema: { found: z.boolean(), message: messageShape.optional() },
      annotations: readOnly,
    },
    async ({ from_number, contains, since, timeout_seconds }) => {
      const start = since ? new Date(since) : new Date();
      const deadline = Date.now() + timeout_seconds * 1000;
      const interval = Math.min(3_000, Math.max(500, timeout_seconds * 100));
      const needle = contains?.toLowerCase();
      try {
        for (;;) {
          const page = await simhook.messages.list({ direction: "inbound", from: start, order: "asc", limit: 100 });
          const hit = page.data.find((m) => (!from_number || sameNumber(m.sender ?? "", from_number)) && (!needle || m.body.toLowerCase().includes(needle)));
          if (hit) return reply(`Received:\n${messageLine(hit)}`, { found: true, message: hit });
          const remaining = deadline - Date.now();
          if (remaining <= 0) {
            return reply(`No matching SMS arrived within ${timeout_seconds} seconds. Call again with since=${start.toISOString()} to keep waiting.`, { found: false });
          }
          await sleep(Math.min(interval, remaining));
        }
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "list_devices",
    {
      title: "List devices",
      description: "Lists the Android phones paired to the account with their online state, SIMs, and battery. A send needs an online phone.",
      outputSchema: { devices: z.array(deviceShape) },
      annotations: readOnly,
    },
    async () => {
      try {
        const devices = await simhook.devices.list();
        const text = devices.length > 0 ? devices.map(deviceLine).join("\n") : "No phone is paired. The user has to pair one from the simhook dashboard before anything can be sent.";
        return reply(text, { devices });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "get_account",
    {
      title: "Get account",
      description: "Shows the account behind the API key: plan, limits, usage today and this month, and lifetime totals.",
      outputSchema: {
        email: z.string(),
        plan: z.string(),
        limits: z.looseObject({}),
        usage: z.looseObject({}),
        stats: z.looseObject({}),
      },
      annotations: readOnly,
    },
    async () => {
      try {
        const [me, stats] = await Promise.all([simhook.account.me(), simhook.account.stats()]);
        const text = [
          `Account ${me.user.email} on the ${me.limits.plan_name} plan.`,
          `Today: ${me.usage.sent_today} of ${limit(me.limits.daily_limit)} messages sent. This month: ${me.usage.sent_this_month} of ${limit(me.limits.monthly_limit)}.`,
          `Up to ${limit(me.limits.batch_limit)} recipients per send and ${limit(me.limits.device_limit)} phone(s).`,
          `Lifetime: ${stats.sent} sent, ${stats.received} received, ${stats.devices} phone(s) paired.`,
        ].join("\n");
        return reply(text, { email: me.user.email, plan: me.limits.plan_name, limits: me.limits, usage: me.usage, stats });
      } catch (err) {
        return failure(err);
      }
    },
  );

  server.registerTool(
    "count_sms_segments",
    {
      title: "Count SMS segments",
      description: "Estimates how many SMS parts a text needs and which encoding applies, without sending anything. Useful before sending long or non-Latin text.",
      inputSchema: { text: z.string() },
      outputSchema: { encoding: z.string(), length: z.number(), segments: z.number(), per_segment: z.number(), remaining: z.number() },
      annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
    },
    async ({ text }) => {
      const info = countSegments(text);
      const summary = `${info.segments} SMS part(s) using ${info.encoding}: ${info.length} of ${info.segments * info.per_segment || info.per_segment} units used, ${info.remaining} left before another part is needed.`;
      return reply(summary, { ...info });
    },
  );

  return server;
}
