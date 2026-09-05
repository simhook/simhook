/** Small formatting helpers shared by the screens. */

const rtf = typeof Intl !== "undefined" ? new Intl.RelativeTimeFormat("en", { numeric: "auto" }) : null;

/** "3 minutes ago", "yesterday", "in 2 hours". */
export function relativeTime(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return "never";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "unknown";
  const diff = (t - now) / 1000;
  const abs = Math.abs(diff);
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 31536000],
    ["month", 2592000],
    ["week", 604800],
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ];
  for (const [unit, secs] of units) {
    if (abs >= secs) return rtf ? rtf.format(Math.round(diff / secs), unit) : `${Math.round(abs / secs)} ${unit}s ago`;
  }
  return abs < 10 ? "just now" : rtf ? rtf.format(Math.round(diff), "second") : `${Math.round(abs)}s ago`;
}

export function absoluteTime(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

export function formatCount(n: number | null | undefined): string {
  return new Intl.NumberFormat().format(n ?? 0);
}

/** Plan limits use -1 for unlimited. */
export function limitLabel(n: number): string {
  return n < 0 ? "unlimited" : formatCount(n);
}

export function priceLabel(cents: number): string {
  if (cents === 0) return "Free";
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}

export const messageStatusLabel: Record<string, string> = {
  queued: "Queued",
  dispatched: "On the phone",
  sent: "Sent",
  delivered: "Delivered",
  failed: "Failed",
  unknown: "No result",
  received: "Received",
};

export const batchStatusLabel: Record<string, string> = {
  queued: "Queued",
  processing: "Sending",
  completed: "Completed",
  partial: "Partly failed",
  failed: "Failed",
  unknown: "No result",
};

export const deliveryStatusLabel: Record<string, string> = {
  pending: "Pending",
  retrying: "Retrying",
  delivered: "Delivered",
  failed: "Failed",
};

export function statusTone(status: string): "ok" | "warn" | "bad" | "muted" {
  switch (status) {
    case "delivered":
    case "completed":
    case "received":
      return "ok";
    case "sent":
    case "dispatched":
    case "queued":
    case "processing":
    case "pending":
    case "retrying":
      return "muted";
    case "partial":
    case "unknown":
      return "warn";
    case "failed":
      return "bad";
    default:
      return "muted";
  }
}

export function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}

/** "Chrome on Windows", from a User-Agent string, for the sessions list. */
export function browserName(ua: string | null | undefined): string {
  if (!ua) return "Unknown browser";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /OPR\//.test(ua)
      ? "Opera"
      : /Firefox\//.test(ua)
        ? "Firefox"
        : /Chrome\//.test(ua)
          ? "Chrome"
          : /Safari\//.test(ua)
            ? "Safari"
            : /curl\//.test(ua)
              ? "curl"
              : "Unknown browser";
  const os = /Windows/.test(ua)
    ? "Windows"
    : /iPhone|iPad/.test(ua)
      ? "iOS"
      : /Android/.test(ua)
        ? "Android"
        : /Mac OS X|Macintosh/.test(ua)
          ? "macOS"
          : /CrOS/.test(ua)
            ? "ChromeOS"
            : /Linux/.test(ua)
              ? "Linux"
              : "";
  return os ? `${browser} on ${os}` : browser;
}
