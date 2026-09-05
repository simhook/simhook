import createClient from "openapi-fetch";
import type { paths } from "@simhook/contracts";

/** Where the API lives. The browser talks to it directly with the session cookie. */
export const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(/\/$/, "");

/** Link to the API's own reference documentation. */
export const API_DOCS_URL = `${API_URL}/docs`;

export const api = createClient<paths>({
  baseUrl: API_URL,
  credentials: "include",
  headers: { Accept: "application/json" },
});

export type FieldError = { field?: string; message: string };

/** The one error shape the API returns, as a thrown exception. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: FieldError[];

  constructor(status: number, code: string, message: string, fields: FieldError[] = []) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
  }

  get isUnauthenticated() {
    return this.status === 401;
  }

  /** A per-field message keyed by the last path segment, e.g. "email" for "body.email". */
  fieldMessages(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const f of this.fields) {
      const key = (f.field ?? "").split(".").pop()?.replace(/\[\d+\]$/, "") ?? "";
      if (key && !out[key]) out[key] = f.message;
    }
    return out;
  }
}

type Envelope<T> = { data?: T; error?: unknown; response: Response };

/** Turns openapi-fetch's result tuple into a value or a thrown ApiError. */
export async function unwrap<T>(p: Promise<Envelope<T>>): Promise<T> {
  let r: Envelope<T>;
  try {
    r = await p;
  } catch {
    throw new ApiError(0, "network", "Could not reach the API. Check your connection and that the service is up.");
  }
  if (r.error !== undefined || !r.response.ok) {
    const body = (r.error ?? {}) as { status?: number; code?: string; message?: string; errors?: FieldError[] };
    throw new ApiError(
      body.status ?? r.response.status,
      body.code ?? `http_${r.response.status}`,
      body.message ?? `Request failed with HTTP ${r.response.status}`,
      body.errors ?? [],
    );
  }
  return r.data as T;
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

export function errorMessage(e: unknown): string {
  if (isApiError(e)) return e.message;
  if (e instanceof Error) return e.message;
  return "Something went wrong.";
}
