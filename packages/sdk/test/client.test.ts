import { afterEach, describe, expect, it, vi } from "vitest";
import { Simhook, SimhookError } from "../src/index";

interface Call {
  url: URL;
  init: RequestInit;
}

function fakeFetch(handler: (call: Call, n: number) => Response | Promise<Response>) {
  const calls: Call[] = [];
  const impl = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const call = { url: new URL(input instanceof Request ? input.url : String(input)), init: init ?? {} };
    calls.push(call);
    return handler(call, calls.length);
  });
  return { calls, fetch: impl as unknown as typeof fetch };
}

function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", ...headers } });
}

function client(fetchImpl: typeof fetch, extra: ConstructorParameters<typeof Simhook>[0] = {}) {
  return new Simhook({ apiKey: "sh_test", baseUrl: "https://api.example.test/", fetch: fetchImpl, ...extra });
}

describe("Simhook client", () => {
  afterEach(() => {
    delete process.env.SIMHOOK_API_KEY;
    delete process.env.SIMHOOK_BASE_URL;
  });

  it("requires an api key", () => {
    expect(() => new Simhook({ apiKey: "" })).toThrow(SimhookError);
  });

  it("reads the key and base url from the environment", async () => {
    process.env.SIMHOOK_API_KEY = "sh_env";
    process.env.SIMHOOK_BASE_URL = "https://self.hosted.test/";
    const f = fakeFetch(() => json({ data: [] }));
    const simhook = new Simhook({ fetch: f.fetch });
    await simhook.devices.list();
    expect(simhook.baseUrl).toBe("https://self.hosted.test");
    expect(new Headers(f.calls[0]!.init.headers).get("x-api-key")).toBe("sh_env");
  });

  it("sends with the key header and a json body", async () => {
    const f = fakeFetch(() => json({ batch: { id: "b1", status: "queued" }, message_ids: ["m1"] }, 202));
    const res = await client(f.fetch).messages.send({ to: "+15550001111", body: "hi", scheduled_at: new Date("2026-01-02T03:04:05Z") });
    expect(res.message_ids).toEqual(["m1"]);
    const call = f.calls[0]!;
    expect(call.url.toString()).toBe("https://api.example.test/v1/messages");
    expect(call.init.method).toBe("POST");
    const headers = new Headers(call.init.headers);
    expect(headers.get("x-api-key")).toBe("sh_test");
    expect(headers.get("content-type")).toBe("application/json");
    expect(headers.get("user-agent")).toMatch(/^simhook-sdk-js\/\d/);
    expect(JSON.parse(String(call.init.body))).toEqual({ to: ["+15550001111"], body: "hi", scheduled_at: "2026-01-02T03:04:05.000Z" });
  });

  it("builds list queries", async () => {
    const f = fakeFetch(() => json({ data: [] }));
    await client(f.fetch).messages.list({ device_ids: ["d1", "d2"], direction: "inbound", from: new Date(0), limit: 10, q: "" });
    const url = f.calls[0]!.url;
    expect(url.pathname).toBe("/v1/messages");
    expect(url.searchParams.get("device_ids")).toBe("d1,d2");
    expect(url.searchParams.get("direction")).toBe("inbound");
    expect(url.searchParams.get("from")).toBe("1970-01-01T00:00:00.000Z");
    expect(url.searchParams.get("limit")).toBe("10");
    expect(url.searchParams.has("q")).toBe(false);
  });

  it("maps api errors and does not retry writes", async () => {
    const f = fakeFetch(() =>
      json({ status: 422, code: "validation_failed", message: "The request has invalid fields.", errors: [{ field: "body.to[0]", message: "must be a phone number" }] }, 422),
    );
    const err = await client(f.fetch)
      .messages.send({ to: "nope", body: "x" })
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(SimhookError);
    const e = err as SimhookError;
    expect(e.status).toBe(422);
    expect(e.code).toBe("validation_failed");
    expect(e.isValidation).toBe(true);
    expect(e.fieldMessages()).toEqual({ "body.to[0]": "must be a phone number" });
    expect(f.calls).toHaveLength(1);
  });

  it("flags plan limits", async () => {
    const f = fakeFetch(() => json({ status: 429, code: "plan_limit_daily", message: "Daily limit reached." }, 429));
    const err = (await client(f.fetch)
      .messages.send({ to: "+15550001111", body: "x" })
      .catch((e: unknown) => e)) as SimhookError;
    expect(err.isPlanLimit).toBe(true);
    expect(err.isRateLimited).toBe(true);
    expect(f.calls).toHaveLength(1);
  });

  it("wraps non-json failures", async () => {
    const f = fakeFetch(() => new Response("<html>bad gateway</html>", { status: 502 }));
    const err = (await client(f.fetch, { maxRetries: 0 })
      .devices.list()
      .catch((e: unknown) => e)) as SimhookError;
    expect(err.status).toBe(502);
    expect(err.code).toBe("internal_error");
  });

  it("retries reads on 503", async () => {
    const f = fakeFetch((_, n) => (n === 1 ? json({ status: 503, code: "unavailable", message: "starting" }, 503, { "retry-after": "0" }) : json({ data: [{ id: "d1" }] })));
    const devices = await client(f.fetch).devices.list();
    expect(devices).toHaveLength(1);
    expect(f.calls).toHaveLength(2);
  });

  it("times out", async () => {
    const f = fakeFetch(
      (call) =>
        new Promise<Response>((_, reject) => {
          call.init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        }),
    );
    const err = (await client(f.fetch, { timeoutMs: 20, maxRetries: 0 })
      .devices.list()
      .catch((e: unknown) => e)) as SimhookError;
    expect(err).toBeInstanceOf(SimhookError);
    expect(err.code).toBe("timeout");
  });

  it("passes caller aborts through untouched", async () => {
    const f = fakeFetch((call) => {
      if (call.init.signal?.aborted) return Promise.reject(new DOMException("aborted", "AbortError"));
      return json({ data: [] });
    });
    const controller = new AbortController();
    controller.abort();
    const err = (await client(f.fetch)
      .devices.list({ signal: controller.signal })
      .catch((e: unknown) => e)) as Error;
    expect(err).not.toBeInstanceOf(SimhookError);
    expect(err.name).toBe("AbortError");
  });

  it("iterates pages", async () => {
    const f = fakeFetch((call) => (call.url.searchParams.get("cursor") ? json({ data: [{ id: "m2" }] }) : json({ data: [{ id: "m1" }], next_cursor: "c1" })));
    const ids: string[] = [];
    for await (const m of client(f.fetch).messages.iterate({ limit: 1 })) ids.push(m.id);
    expect(ids).toEqual(["m1", "m2"]);
    expect(f.calls[1]!.url.searchParams.get("cursor")).toBe("c1");
  });

  it("waits for a send to finish", async () => {
    const f = fakeFetch((_, n) => json({ batch: { id: "b1", status: n < 3 ? "processing" : "completed" }, messages: [] }));
    const detail = await client(f.fetch).batches.waitUntilDone("b1", { intervalMs: 1 });
    expect(detail.batch.status).toBe("completed");
    expect(f.calls).toHaveLength(3);
  });

  it("returns the latest state when waiting times out", async () => {
    const f = fakeFetch(() => json({ batch: { id: "b1", status: "processing" }, messages: [] }));
    const detail = await client(f.fetch).batches.waitUntilDone("b1", { intervalMs: 1, timeoutMs: 5 });
    expect(detail.batch.status).toBe("processing");
  });

  it("unwraps single resources and accepts empty responses", async () => {
    const f = fakeFetch((call) => (call.init.method === "DELETE" ? new Response(null, { status: 204 }) : json({ device: { id: "d1", name: "Pixel" } })));
    const simhook = client(f.fetch);
    const device = await simhook.devices.get("d1");
    expect(device.name).toBe("Pixel");
    await expect(simhook.devices.unpair("d1")).resolves.toBeUndefined();
    expect(f.calls[1]!.url.pathname).toBe("/v1/devices/d1");
  });

  it("escapes ids in paths", async () => {
    const f = fakeFetch(() => json({ message: { id: "x" } }));
    await client(f.fetch).messages.get("a/b c");
    expect(f.calls[0]!.url.pathname).toBe("/v1/messages/a%2Fb%20c");
  });
});
