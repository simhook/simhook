import type { Batch, BatchDetail, Device, Message } from "@simhook/sdk";

/** Renders a timestamp as `YYYY-MM-DD HH:MM:SS UTC`, or `-` when absent. */
export function when(value: string | null | undefined): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${date.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

/** Collapses whitespace and truncates for compact output. */
export function oneLine(text: string, max = 240): string {
  const collapsed = text.replace(/\s+/g, " ").trim();
  return collapsed.length > max ? `${collapsed.slice(0, max - 3)}...` : collapsed;
}

/** Shows -1 style "no limit" values as the word. */
export function limit(value: number): string {
  return value < 0 ? "unlimited" : String(value);
}

export function messageLine(m: Message): string {
  const party = m.direction === "inbound" ? `from ${m.sender ?? "?"}` : `to ${m.recipient ?? "?"}`;
  const problem = m.error_message ? ` (${m.error_code ?? "error"}: ${m.error_message})` : "";
  return `- ${when(m.created_at)} | ${m.direction} ${party} | ${m.status}${problem} | id ${m.id}\n  ${oneLine(m.body)}`;
}

export function batchSummary(b: Batch): string {
  const inFlight = Math.max(0, b.recipient_count - (b.sent_count + b.delivered_count + b.failed_count + b.unknown_count));
  const counts = [
    `${b.delivered_count} delivered`,
    `${b.sent_count} sent`,
    `${b.failed_count} failed`,
    `${b.unknown_count} unknown`,
    `${inFlight} in progress`,
  ].join(", ");
  let summary = `Send ${b.id} is ${b.status}: ${b.recipient_count} recipient(s); ${counts}.`;
  if (b.scheduled_at) summary += ` Scheduled for ${when(b.scheduled_at)}.`;
  if (b.error) summary += ` Error: ${b.error}`;
  return summary;
}

export function batchDetailText(detail: BatchDetail): string {
  const lines = [batchSummary(detail.batch), `Text: ${oneLine(detail.batch.body)}`];
  for (const m of detail.messages) {
    lines.push(`- ${m.recipient ?? "?"}: ${m.status}${m.error_message ? ` (${m.error_message})` : ""} | message id ${m.id}`);
  }
  return lines.join("\n");
}

export function deviceLine(d: Device): string {
  const sims = d.sims.length > 0 ? d.sims.map((s) => `${s.carrier ?? s.display_name ?? "SIM"} (subscription id ${s.subscription_id})`).join(", ") : "no SIM details";
  const flags = [d.online ? "online" : "offline", d.is_default ? "default" : null, d.enabled ? null : "disabled", d.receive_enabled ? null : "not forwarding incoming"]
    .filter((f): f is string => f !== null)
    .join(", ");
  const battery = typeof d.telemetry.battery_percent === "number" ? `; battery ${d.telemetry.battery_percent}%` : "";
  return `- ${d.name} (id ${d.id}): ${flags}; ${sims}${battery}; last seen ${when(d.last_heartbeat_at)}`;
}
