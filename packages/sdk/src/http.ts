import { SimhookError } from "./errors";
import type { FieldError, RequestOptions } from "./types";

export type QueryValue = string | number | boolean | Date | string[] | null | undefined;

export interface HttpConfig {
  apiKey: string;
  baseUrl: string;
  fetch: typeof fetch;
  timeoutMs: number;
  maxRetries: number;
  userAgent: string;
}

export interface HttpRequest {
  method: "GET" | "POST" | "PATCH" | "DELETE";
  path: string;
  query?: Record<string, QueryValue>;
  body?: unknown;
  options?: RequestOptions;
}

// Statuses worth a second try for idempotent requests.
const RETRYABLE = new Set([408, 425, 429, 500, 502, 503, 504]);

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function backoff(attempt: number): number {
  return Math.min(8_000, 300 * 2 ** attempt) + Math.floor(Math.random() * 200);
}

function retryAfterMs(res: Response): number | undefined {
  const raw = res.headers.get("Retry-After");
  if (!raw) return undefined;
  const seconds = Number(raw);
  const ms = Number.isFinite(seconds) ? seconds * 1000 : new Date(raw).getTime() - Date.now();
  if (!Number.isFinite(ms) || ms <= 0) return undefined;
  return Math.min(ms, 30_000);
}

function codeForStatus(status: number): string {
  switch (status) {
    case 400:
      return "bad_request";
    case 401:
      return "unauthenticated";
    case 403:
      return "forbidden";
    case 404:
      return "not_found";
    case 409:
      return "conflict";
    case 422:
      return "validation_failed";
    case 429:
      return "rate_limited";
    case 503:
      return "unavailable";
    default:
      return status >= 500 ? "internal_error" : "error";
  }
}

async function errorFromResponse(res: Response): Promise<SimhookError> {
  let text = "";
  try {
    text = await res.text();
  } catch {
    // The body is optional here.
  }
  try {
    const parsed = JSON.parse(text) as { code?: unknown; message?: unknown; errors?: unknown };
    if (typeof parsed.message === "string" && typeof parsed.code === "string") {
      const errors = Array.isArray(parsed.errors) ? (parsed.errors as FieldError[]) : [];
      return new SimhookError(parsed.message, { status: res.status, code: parsed.code, errors });
    }
  } catch {
    // Not JSON; fall through to a generic error.
  }
  return new SimhookError(`The API responded with HTTP ${res.status}.`, { status: res.status, code: codeForStatus(res.status) });
}

/** Links a caller's signal with a timeout. `done` must be called to release the timer. */
function withTimeout(parent: AbortSignal | undefined, ms: number): { signal: AbortSignal; done: () => void } {
  const controller = new AbortController();
  const onAbort = () => controller.abort(parent?.reason);
  if (parent) {
    if (parent.aborted) controller.abort(parent.reason);
    else parent.addEventListener("abort", onAbort, { once: true });
  }
  const timer = ms > 0 ? setTimeout(() => controller.abort(new Error("timeout")), ms) : undefined;
  return {
    signal: controller.signal,
    done: () => {
      if (timer !== undefined) clearTimeout(timer);
      parent?.removeEventListener("abort", onAbort);
    },
  };
}

export class HttpClient {
  constructor(private readonly cfg: HttpConfig) {}

  buildUrl(path: string, query?: Record<string, QueryValue>): string {
    const url = new URL(this.cfg.baseUrl + path);
    for (const [key, value] of Object.entries(query ?? {})) {
      if (value === undefined || value === null || value === "") continue;
      if (value instanceof Date) {
        url.searchParams.set(key, value.toISOString());
      } else if (Array.isArray(value)) {
        if (value.length > 0) url.searchParams.set(key, value.join(","));
      } else {
        url.searchParams.set(key, String(value));
      }
    }
    return url.toString();
  }

  async request<T>(req: HttpRequest): Promise<T> {
    const url = this.buildUrl(req.path, req.query);
    const headers = new Headers({ "X-Api-Key": this.cfg.apiKey, Accept: "application/json" });
    if (this.cfg.userAgent) headers.set("User-Agent", this.cfg.userAgent);
    let body: string | undefined;
    if (req.body !== undefined) {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(req.body);
    }
    const maxRetries = req.options?.maxRetries ?? (req.method === "GET" ? this.cfg.maxRetries : 0);
    const timeoutMs = req.options?.timeoutMs ?? this.cfg.timeoutMs;

    for (let attempt = 0; ; attempt++) {
      const { signal, done } = withTimeout(req.options?.signal, timeoutMs);
      let res: Response;
      try {
        res = await this.cfg.fetch(url, { method: req.method, headers, body, signal });
      } catch (err) {
        done();
        // A cancellation by the caller is theirs to handle.
        if (req.options?.signal?.aborted) throw err;
        const timedOut = signal.aborted;
        if (attempt < maxRetries) {
          await sleep(backoff(attempt));
          continue;
        }
        throw new SimhookError(timedOut ? `The request timed out after ${timeoutMs} ms.` : "Could not reach the simhook API.", {
          status: 0,
          code: timedOut ? "timeout" : "connection_error",
          cause: err,
        });
      }
      try {
        if (res.ok) {
          if (res.status === 204) return undefined as T;
          try {
            return (await res.json()) as T;
          } catch (err) {
            throw new SimhookError("The API returned a response that is not JSON.", { status: res.status, code: "invalid_response", cause: err });
          }
        }
        const error = await errorFromResponse(res);
        if (attempt < maxRetries && RETRYABLE.has(res.status)) {
          await sleep(retryAfterMs(res) ?? backoff(attempt));
          continue;
        }
        throw error;
      } finally {
        done();
      }
    }
  }
}
