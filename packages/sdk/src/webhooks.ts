import { SimhookError, SimhookSignatureError } from "./errors";
import type { WebhookEvent } from "./types";

/** Header carrying `t=<unix seconds>,v1=<hex HMAC-SHA256>`. */
export const SIGNATURE_HEADER = "X-Simhook-Signature";
/** Header carrying the event name, for example `message.received`. */
export const EVENT_HEADER = "X-Simhook-Event";
/** Header carrying the delivery id. Use it to de-duplicate retries. */
export const DELIVERY_HEADER = "X-Simhook-Delivery";
/** Signatures older than this many seconds are rejected by default. */
export const DEFAULT_TOLERANCE_SECONDS = 300;

export type WebhookPayload = string | Uint8Array | ArrayBuffer;

export interface VerifyWebhookParams {
  /** The raw request body, exactly as received. Do not re-serialize parsed JSON. */
  payload: WebhookPayload;
  /** Value of the `X-Simhook-Signature` header. */
  signature: string | null | undefined;
  /** The signing secret shown when the webhook was created or its secret rotated. */
  secret: string;
  /** Maximum signature age in seconds. Default 300. */
  tolerance?: number;
  /** Current time as unix seconds. Defaults to now; useful in tests. */
  now?: number;
}

const encoder = new TextEncoder();

function toBytes(payload: WebhookPayload): Uint8Array {
  if (typeof payload === "string") return encoder.encode(payload);
  if (payload instanceof Uint8Array) return payload;
  return new Uint8Array(payload);
}

function subtle(): SubtleCrypto {
  const s = globalThis.crypto?.subtle;
  if (!s) {
    throw new SimhookError("Web Crypto is not available in this runtime.", { status: 0, code: "crypto_unavailable" });
  }
  return s;
}

async function hmacHex(secret: string, parts: Uint8Array[]): Promise<string> {
  const key = await subtle().importKey("raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const data = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let offset = 0;
  for (const p of parts) {
    data.set(p, offset);
    offset += p.length;
  }
  const mac = new Uint8Array(await subtle().sign("HMAC", key, data));
  let hex = "";
  for (const b of mac) hex += b.toString(16).padStart(2, "0");
  return hex;
}

function constantTimeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

/** Splits a signature header into its timestamp and signatures. Returns null when malformed. */
export function parseSignatureHeader(header: string): { timestamp: number; signatures: string[] } | null {
  let timestamp: number | undefined;
  const signatures: string[] = [];
  for (const part of header.split(",")) {
    const eq = part.indexOf("=");
    if (eq < 0) return null;
    const key = part.slice(0, eq).trim();
    const value = part.slice(eq + 1).trim();
    if (key === "t") {
      if (!/^\d+$/.test(value)) return null;
      timestamp = Number(value);
    } else if (key === "v1") {
      signatures.push(value);
    }
  }
  if (timestamp === undefined || timestamp === 0 || signatures.length === 0) return null;
  return { timestamp, signatures };
}

/**
 * Produces the value simhook puts in `X-Simhook-Signature` for a payload.
 * Handy for testing your own webhook handler.
 */
export async function signWebhookPayload(secret: string, payload: WebhookPayload, timestamp = Math.floor(Date.now() / 1000)): Promise<string> {
  const hex = await hmacHex(secret, [encoder.encode(`${timestamp}.`), toBytes(payload)]);
  return `t=${timestamp},v1=${hex}`;
}

/** True when the signature matches the payload and is within tolerance. */
export async function verifyWebhookSignature(params: VerifyWebhookParams): Promise<boolean> {
  const parsed = params.signature ? parseSignatureHeader(params.signature) : null;
  if (!parsed) return false;
  const tolerance = params.tolerance ?? DEFAULT_TOLERANCE_SECONDS;
  const now = params.now ?? Math.floor(Date.now() / 1000);
  if (Math.abs(now - parsed.timestamp) > tolerance) return false;
  const expected = await hmacHex(params.secret, [encoder.encode(`${parsed.timestamp}.`), toBytes(params.payload)]);
  return parsed.signatures.some((s) => constantTimeEqual(s, expected));
}

/**
 * Verifies the signature and parses the payload into a typed event.
 * Throws `SimhookSignatureError` when the signature is missing, wrong, or stale.
 */
export async function constructWebhookEvent(params: VerifyWebhookParams): Promise<WebhookEvent> {
  if (!(await verifyWebhookSignature(params))) {
    throw new SimhookSignatureError("Webhook signature is missing, invalid, or too old.");
  }
  const text = typeof params.payload === "string" ? params.payload : new TextDecoder().decode(toBytes(params.payload));
  try {
    return JSON.parse(text) as WebhookEvent;
  } catch (err) {
    throw new SimhookError("Webhook payload is not valid JSON.", { status: 0, code: "invalid_payload", cause: err });
  }
}
