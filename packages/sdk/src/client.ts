import { SimhookError } from "./errors";
import { HttpClient } from "./http";
import { Account, Batches, Devices, Messages, Webhooks } from "./resources";
import type { Plan, RequestOptions } from "./types";
import { VERSION } from "./version";

export const DEFAULT_BASE_URL = "https://api.simhook.dev";

export interface SimhookOptions {
  /** API key from the dashboard, starting with `sh_`. Falls back to the `SIMHOOK_API_KEY` environment variable. */
  apiKey?: string;
  /** API origin. Falls back to `SIMHOOK_BASE_URL`, then to https://api.simhook.dev. Point it at your own host when self-hosting. */
  baseUrl?: string;
  /** A fetch implementation, for proxies or tests. Defaults to the global fetch. */
  fetch?: typeof fetch;
  /** Per-request timeout in milliseconds. Default 30000. */
  timeoutMs?: number;
  /** Retries for reads that fail with 429, 5xx, or a network error. Default 2. Writes are never retried. */
  maxRetries?: number;
}

function readEnv(name: string): string | undefined {
  try {
    const env = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env;
    const value = env?.[name];
    return value ? value : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Entry point of the SDK.
 *
 * ```ts
 * const simhook = new Simhook({ apiKey: "sh_..." });
 * const { batch } = await simhook.messages.send({ to: "+14155550123", body: "Your code is 4921" });
 * ```
 */
export class Simhook {
  readonly messages: Messages;
  readonly batches: Batches;
  readonly devices: Devices;
  readonly webhooks: Webhooks;
  readonly account: Account;
  /** The API origin this client talks to. */
  readonly baseUrl: string;
  private readonly http: HttpClient;

  constructor(options: SimhookOptions = {}) {
    const apiKey = options.apiKey ?? readEnv("SIMHOOK_API_KEY");
    if (!apiKey) {
      throw new SimhookError("An API key is required. Pass { apiKey } or set SIMHOOK_API_KEY.", { status: 0, code: "missing_api_key" });
    }
    const fetchImpl = options.fetch ?? globalThis.fetch;
    if (typeof fetchImpl !== "function") {
      throw new SimhookError("No fetch implementation found. Pass { fetch } or run on Node 20 or newer.", { status: 0, code: "fetch_unavailable" });
    }
    this.baseUrl = (options.baseUrl ?? readEnv("SIMHOOK_BASE_URL") ?? DEFAULT_BASE_URL).replace(/\/+$/, "");
    this.http = new HttpClient({
      apiKey,
      baseUrl: this.baseUrl,
      // Wrapped so a bare global fetch keeps the receiver it expects.
      fetch: (input, init) => fetchImpl(input, init),
      timeoutMs: options.timeoutMs ?? 30_000,
      maxRetries: options.maxRetries ?? 2,
      userAgent: `simhook-sdk-js/${VERSION}`,
    });
    this.messages = new Messages(this.http);
    this.batches = new Batches(this.http);
    this.devices = new Devices(this.http);
    this.webhooks = new Webhooks(this.http);
    this.account = new Account(this.http);
  }

  /** Plans and their limits. Needs no key. */
  async plans(options?: RequestOptions): Promise<Plan[]> {
    const res = await this.http.request<{ data: Plan[] }>({ method: "GET", path: "/v1/plans", options });
    return res.data;
  }
}
