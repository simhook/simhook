import type { HttpClient } from "./http";
import type {
  AccountInfo,
  Batch,
  BatchDetail,
  CreateWebhookParams,
  Delivery,
  Device,
  ListBatchesParams,
  ListDeliveriesParams,
  ListMessagesParams,
  Message,
  Page,
  PairingCode,
  RequestOptions,
  SendMessageParams,
  SendResult,
  Stats,
  UpdateDeviceParams,
  UpdateWebhookParams,
  WaitForBatchOptions,
  Webhook,
  WebhookWithSecret,
} from "./types";
import { constructWebhookEvent, verifyWebhookSignature } from "./webhooks";

function iso(value: string | Date | undefined): string | undefined {
  return value instanceof Date ? value.toISOString() : value;
}

function id(value: string): string {
  return encodeURIComponent(value);
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason);
      return;
    }
    let timer: ReturnType<typeof setTimeout>;
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal?.reason);
    };
    timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

const ACTIVE_BATCH = new Set<Batch["status"]>(["queued", "processing"]);

export class Messages {
  constructor(private readonly http: HttpClient) {}

  /**
   * Queues one text to one or more numbers. Acceptance is not delivery:
   * follow the batch with `batches.waitUntilDone` or subscribe to webhooks.
   * Every recipient counts as one message against the plan.
   */
  send(params: SendMessageParams, options?: RequestOptions): Promise<SendResult> {
    const { to, scheduled_at, ...rest } = params;
    return this.http.request<SendResult>({
      method: "POST",
      path: "/v1/messages",
      body: { ...rest, to: Array.isArray(to) ? to : [to], scheduled_at: iso(scheduled_at) },
      options,
    });
  }

  /** Sent and received messages, newest first unless `order` is `asc`. */
  list(params: ListMessagesParams = {}, options?: RequestOptions): Promise<Page<Message>> {
    return this.http.request<Page<Message>>({
      method: "GET",
      path: "/v1/messages",
      query: { ...params, from: iso(params.from), to: iso(params.to) },
      options,
    });
  }

  /** Walks every page of `list` for you. */
  async *iterate(params: ListMessagesParams = {}, options?: RequestOptions): AsyncGenerator<Message, void, undefined> {
    let cursor = params.cursor;
    do {
      const page = await this.list({ ...params, cursor }, options);
      for (const message of page.data) yield message;
      cursor = page.next_cursor;
    } while (cursor);
  }

  async get(messageId: string, options?: RequestOptions): Promise<Message> {
    const res = await this.http.request<{ message: Message }>({ method: "GET", path: `/v1/messages/${id(messageId)}`, options });
    return res.message;
  }
}

export class Batches {
  constructor(private readonly http: HttpClient) {}

  /** Every send on the account, newest first, with per-status counts. */
  list(params: ListBatchesParams = {}, options?: RequestOptions): Promise<Page<Batch>> {
    return this.http.request<Page<Batch>>({ method: "GET", path: "/v1/batches", query: { ...params }, options });
  }

  async *iterate(params: ListBatchesParams = {}, options?: RequestOptions): AsyncGenerator<Batch, void, undefined> {
    let cursor = params.cursor;
    do {
      const page = await this.list({ ...params, cursor }, options);
      for (const batch of page.data) yield batch;
      cursor = page.next_cursor;
    } while (cursor);
  }

  /** The send plus one message per recipient. */
  get(batchId: string, options?: RequestOptions): Promise<BatchDetail> {
    return this.http.request<BatchDetail>({ method: "GET", path: `/v1/batches/${id(batchId)}`, options });
  }

  /**
   * Polls a send until it leaves `queued`/`processing`, then returns it.
   * On timeout the latest state is returned instead of an error, so check
   * `batch.status` if you need to know.
   */
  async waitUntilDone(batchId: string, options: WaitForBatchOptions = {}): Promise<BatchDetail> {
    const interval = Math.max(1, options.intervalMs ?? 2_000);
    const deadline = Date.now() + (options.timeoutMs ?? 120_000);
    for (;;) {
      const detail = await this.get(batchId, { signal: options.signal });
      const remaining = deadline - Date.now();
      if (!ACTIVE_BATCH.has(detail.batch.status) || remaining <= 0) return detail;
      await delay(Math.min(interval, remaining), options.signal);
    }
  }
}

export class Devices {
  constructor(private readonly http: HttpClient) {}

  /** Paired phones, with `online` computed from the last heartbeat. */
  async list(options?: RequestOptions): Promise<Device[]> {
    const res = await this.http.request<{ data: Device[] }>({ method: "GET", path: "/v1/devices", options });
    return res.data;
  }

  async get(deviceId: string, options?: RequestOptions): Promise<Device> {
    const res = await this.http.request<{ device: Device }>({ method: "GET", path: `/v1/devices/${id(deviceId)}`, options });
    return res.device;
  }

  /** Renames a phone or changes its sending behaviour. Only the fields given change. */
  async update(deviceId: string, params: UpdateDeviceParams, options?: RequestOptions): Promise<Device> {
    const res = await this.http.request<{ device: Device }>({ method: "PATCH", path: `/v1/devices/${id(deviceId)}`, body: params, options });
    return res.device;
  }

  /** Makes this phone the one used when a send names no device. */
  async setDefault(deviceId: string, options?: RequestOptions): Promise<Device> {
    const res = await this.http.request<{ device: Device }>({ method: "POST", path: `/v1/devices/${id(deviceId)}/default`, options });
    return res.device;
  }

  /** Removes the phone and revokes its token. Its messages stay. */
  unpair(deviceId: string, options?: RequestOptions): Promise<void> {
    return this.http.request<void>({ method: "DELETE", path: `/v1/devices/${id(deviceId)}`, options });
  }

  /** A one-time code, valid for ten minutes, to pair a phone. Show `pair_url` as a QR code or `code` as text. */
  createPairingCode(options?: RequestOptions): Promise<PairingCode> {
    return this.http.request<PairingCode>({ method: "POST", path: "/v1/devices/pairing-codes", options });
  }
}

export class Webhooks {
  constructor(private readonly http: HttpClient) {}

  async list(options?: RequestOptions): Promise<Webhook[]> {
    const res = await this.http.request<{ data: Webhook[] }>({ method: "GET", path: "/v1/webhooks", options });
    return res.data;
  }

  async get(webhookId: string, options?: RequestOptions): Promise<Webhook> {
    const res = await this.http.request<{ webhook: Webhook }>({ method: "GET", path: `/v1/webhooks/${id(webhookId)}`, options });
    return res.webhook;
  }

  /** Subscribes a URL to events. The returned secret is shown once. */
  create(params: CreateWebhookParams, options?: RequestOptions): Promise<WebhookWithSecret> {
    return this.http.request<WebhookWithSecret>({ method: "POST", path: "/v1/webhooks", body: params, options });
  }

  async update(webhookId: string, params: UpdateWebhookParams, options?: RequestOptions): Promise<Webhook> {
    const res = await this.http.request<{ webhook: Webhook }>({ method: "PATCH", path: `/v1/webhooks/${id(webhookId)}`, body: params, options });
    return res.webhook;
  }

  /** Issues a new signing secret. The old one stops working immediately. */
  async rotateSecret(webhookId: string, options?: RequestOptions): Promise<string> {
    const res = await this.http.request<{ secret: string }>({ method: "POST", path: `/v1/webhooks/${id(webhookId)}/rotate-secret`, options });
    return res.secret;
  }

  /** Queues a `ping` delivery so you can check your endpoint and signature handling. */
  async test(webhookId: string, options?: RequestOptions): Promise<Delivery> {
    const res = await this.http.request<{ delivery: Delivery }>({ method: "POST", path: `/v1/webhooks/${id(webhookId)}/test`, options });
    return res.delivery;
  }

  delete(webhookId: string, options?: RequestOptions): Promise<void> {
    return this.http.request<void>({ method: "DELETE", path: `/v1/webhooks/${id(webhookId)}`, options });
  }

  /** Delivery attempts across all webhooks, newest first. */
  listDeliveries(params: ListDeliveriesParams = {}, options?: RequestOptions): Promise<Page<Delivery>> {
    return this.http.request<Page<Delivery>>({
      method: "GET",
      path: "/v1/webhooks/deliveries",
      query: { ...params, from: iso(params.from), to: iso(params.to) },
      options,
    });
  }

  async getDelivery(deliveryId: string, options?: RequestOptions): Promise<Delivery> {
    const res = await this.http.request<{ delivery: Delivery }>({ method: "GET", path: `/v1/webhooks/deliveries/${id(deliveryId)}`, options });
    return res.delivery;
  }

  /** Same as the top-level `verifyWebhookSignature`. */
  readonly verifySignature: typeof verifyWebhookSignature = verifyWebhookSignature;

  /** Same as the top-level `constructWebhookEvent`. */
  readonly constructEvent: typeof constructWebhookEvent = constructWebhookEvent;
}

export class Account {
  constructor(private readonly http: HttpClient) {}

  /** The account behind the key, with its plan limits and current usage. */
  me(options?: RequestOptions): Promise<AccountInfo> {
    return this.http.request<AccountInfo>({ method: "GET", path: "/v1/auth/me", options });
  }

  /** Lifetime totals. */
  stats(options?: RequestOptions): Promise<Stats> {
    return this.http.request<Stats>({ method: "GET", path: "/v1/stats", options });
  }
}
