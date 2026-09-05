import type { FieldError } from "./types";

/**
 * Every failure the SDK raises is a SimhookError. For HTTP failures,
 * `status` and `code` mirror the API's error body. For local failures
 * (timeouts, unreachable host, bad signature) `status` is 0 and `code`
 * names the problem.
 */
export class SimhookError extends Error {
  /** HTTP status code, or 0 when the request never got a response. */
  readonly status: number;
  /** Stable machine-readable code, for example `validation_failed`, `plan_limit_daily`, `timeout`. */
  readonly code: string;
  /** Per-field problems for validation errors. Empty otherwise. */
  readonly errors: FieldError[];

  constructor(message: string, details: { status: number; code: string; errors?: FieldError[]; cause?: unknown }) {
    super(message, details.cause === undefined ? undefined : { cause: details.cause });
    this.name = "SimhookError";
    this.status = details.status;
    this.code = details.code;
    this.errors = details.errors ?? [];
  }

  /** The request was refused because a plan limit was reached (daily, monthly, per send, devices, or webhooks). */
  get isPlanLimit(): boolean {
    return this.code.startsWith("plan_limit_");
  }

  get isRateLimited(): boolean {
    return this.status === 429;
  }

  get isUnauthenticated(): boolean {
    return this.status === 401;
  }

  get isNotFound(): boolean {
    return this.status === 404;
  }

  get isValidation(): boolean {
    return this.code === "validation_failed";
  }

  /** Field problems keyed by field path, for example `{ "body.to[0]": "..." }`. */
  fieldMessages(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const e of this.errors) {
      out[e.field ?? ""] = e.message;
    }
    return out;
  }
}

/** Raised by `constructWebhookEvent` when a webhook signature does not check out. */
export class SimhookSignatureError extends SimhookError {
  constructor(message: string) {
    super(message, { status: 0, code: "invalid_signature" });
    this.name = "SimhookSignatureError";
  }
}
