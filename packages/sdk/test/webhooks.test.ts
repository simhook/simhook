import { createHmac } from "node:crypto";
import { describe, expect, it } from "vitest";
import { SimhookError, SimhookSignatureError, constructWebhookEvent, parseSignatureHeader, signWebhookPayload, verifyWebhookSignature } from "../src/index";

const secret = "whs_test_secret_value";
const payload = JSON.stringify({ id: "evt_1", event: "message.received", created_at: "2026-09-05T00:00:00Z", data: { id: "m1", body: "hello" } });
const ts = 1_800_000_000;
const reference = `t=${ts},v1=${createHmac("sha256", secret).update(`${ts}.${payload}`).digest("hex")}`;

describe("webhook signatures", () => {
  it("signs the same way the server does", async () => {
    expect(await signWebhookPayload(secret, payload, ts)).toBe(reference);
  });

  it("accepts a valid signature within tolerance", async () => {
    expect(await verifyWebhookSignature({ payload, signature: reference, secret, now: ts + 100 })).toBe(true);
    expect(await verifyWebhookSignature({ payload, signature: reference, secret, now: ts - 100 })).toBe(true);
  });

  it("accepts bytes as the payload", async () => {
    const bytes = new TextEncoder().encode(payload);
    expect(await verifyWebhookSignature({ payload: bytes, signature: reference, secret, now: ts })).toBe(true);
    expect(await verifyWebhookSignature({ payload: bytes.buffer as ArrayBuffer, signature: reference, secret, now: ts })).toBe(true);
  });

  it("rejects stale or future signatures", async () => {
    expect(await verifyWebhookSignature({ payload, signature: reference, secret, now: ts + 301 })).toBe(false);
    expect(await verifyWebhookSignature({ payload, signature: reference, secret, now: ts - 301 })).toBe(false);
    expect(await verifyWebhookSignature({ payload, signature: reference, secret, now: ts + 301, tolerance: 600 })).toBe(true);
  });

  it("rejects a wrong secret or a tampered payload", async () => {
    expect(await verifyWebhookSignature({ payload, signature: reference, secret: "other", now: ts })).toBe(false);
    expect(await verifyWebhookSignature({ payload: payload + " ", signature: reference, secret, now: ts })).toBe(false);
  });

  it("rejects malformed headers", async () => {
    for (const signature of [null, undefined, "", "v1=abc", `t=abc,v1=abc`, `t=${ts}`, "garbage", `t=${ts},v1=`]) {
      expect(await verifyWebhookSignature({ payload, signature, secret, now: ts })).toBe(false);
    }
  });

  it("parses headers", () => {
    expect(parseSignatureHeader("t=1,v1=aa,v1=bb")).toEqual({ timestamp: 1, signatures: ["aa", "bb"] });
    expect(parseSignatureHeader("t=0,v1=aa")).toBeNull();
    expect(parseSignatureHeader("nope")).toBeNull();
  });

  it("constructs a typed event", async () => {
    const event = await constructWebhookEvent({ payload, signature: reference, secret, now: ts });
    expect(event.event).toBe("message.received");
    if (event.event === "message.received") expect(event.data.body).toBe("hello");
  });

  it("throws on a bad signature", async () => {
    await expect(constructWebhookEvent({ payload, signature: reference, secret: "other", now: ts })).rejects.toBeInstanceOf(SimhookSignatureError);
  });

  it("throws on a payload that is not json", async () => {
    const broken = "{";
    const signature = await signWebhookPayload(secret, broken, ts);
    const err = (await constructWebhookEvent({ payload: broken, signature, secret, now: ts }).catch((e: unknown) => e)) as SimhookError;
    expect(err).toBeInstanceOf(SimhookError);
    expect(err.code).toBe("invalid_payload");
  });
});
