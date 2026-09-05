import type { components } from "@simhook/contracts";

type Schemas = components["schemas"];

export type MessageDirection = "outbound" | "inbound";

/** Outbound messages move queued -> dispatched -> sent -> delivered, or end in failed or unknown. Inbound ones are received. */
export type MessageStatus = "queued" | "dispatched" | "sent" | "delivered" | "failed" | "unknown" | "received";

/** A send is processing until every recipient has an outcome. */
export type BatchStatus = "queued" | "processing" | "completed" | "partial" | "failed" | "unknown";

export type DeliveryStatus = "pending" | "retrying" | "delivered" | "failed";

export type MessageEventName = "message.received" | "message.sent" | "message.delivered" | "message.failed" | "message.unknown";
export type DeviceEventName = "device.online" | "device.offline";

/** Events a webhook can subscribe to. */
export type WebhookEventName = MessageEventName | DeviceEventName;

/** Every event a delivery can carry, including the `ping` sent by a webhook test. */
export type DeliveredEventName = WebhookEventName | "ping";

/** A SIM card reported by the phone. */
export interface SimCard {
  subscription_id: number;
  slot: number;
  carrier?: string | null;
  display_name?: string | null;
  country?: string | null;
}

/** Phone state reported with each heartbeat. Keys may grow over time. */
export interface DeviceTelemetry {
  battery_percent?: number;
  charging?: boolean;
  network?: string;
  locale?: string;
  timezone?: string;
  uptime_ms?: number;
  keep_alive?: boolean;
  outbox_pending?: number;
  storage_free_bytes?: number;
  [key: string]: unknown;
}

export type Message = Omit<Schemas["Message"], "direction" | "status"> & { direction: MessageDirection; status: MessageStatus };
export type Batch = Omit<Schemas["Batch"], "status"> & { status: BatchStatus };
export type Device = Omit<Schemas["Device"], "sims" | "telemetry"> & { sims: SimCard[]; telemetry: DeviceTelemetry };
export type Webhook = Omit<Schemas["Webhook"], "events"> & { events: WebhookEventName[] };
export type Delivery = Omit<Schemas["Delivery"], "status" | "event" | "payload"> & {
  status: DeliveryStatus;
  event: DeliveredEventName;
  /** The JSON body that was posted to the endpoint. */
  payload: WebhookEvent;
};
export type User = Schemas["User"];
export type Plan = Schemas["Plan"];
export type Limits = Schemas["Limits"];
export type Usage = Schemas["UsageView"];
export type Stats = Schemas["Stats"];
export type FieldError = Schemas["FieldError"];
export type PairingCode = Schemas["PairingCodeOutputBody"];

export interface AccountInfo {
  user: User;
  limits: Limits;
  usage: Usage;
}

/** One page of a list. Pass `next_cursor` back as `cursor` to continue; it is absent on the last page. */
export interface Page<T> {
  data: T[];
  next_cursor?: string;
}

/** Per-call overrides. */
export interface RequestOptions {
  /** Cancels the request. The signal's reason is thrown as-is. */
  signal?: AbortSignal;
  /** Overrides the client timeout for this call. */
  timeoutMs?: number;
  /** Overrides the client retry count for this call. Writes default to 0. */
  maxRetries?: number;
}

export interface SendMessageParams {
  /** One number or many. E.164 like `+14155550123` works everywhere; local formats are passed to the phone as-is. */
  to: string | string[];
  /** The text. Up to 1600 characters; long texts go out as concatenated SMS. */
  body: string;
  /** Phone to send from. Defaults to the account's default device, else the most recently online one. */
  device_id?: string;
  /** SIM to use on a multi-SIM phone. Unknown ids fall back to the phone's preferred SIM. */
  sim_subscription_id?: number;
  /** Send later, up to 7 days ahead. */
  scheduled_at?: string | Date;
}

export interface SendResult {
  batch: Batch;
  /** One id per recipient, in the order given after de-duplication. */
  message_ids: string[];
}

export interface ListMessagesParams {
  /** Only these devices. Default: all paired devices. */
  device_ids?: string[];
  direction?: MessageDirection;
  status?: MessageStatus;
  /** Only messages from one send. */
  batch_id?: string;
  /** Matches text, recipient, and sender. */
  q?: string;
  /** Inclusive lower bound on created_at. */
  from?: string | Date;
  /** Exclusive upper bound on created_at. */
  to?: string | Date;
  /** `desc` (default) for newest first; `asc` to walk forward when polling. */
  order?: "desc" | "asc";
  cursor?: string;
  /** Page size, 1 to 100. Default 50. */
  limit?: number;
}

export interface ListBatchesParams {
  cursor?: string;
  limit?: number;
}

export interface BatchDetail {
  batch: Batch;
  /** One message per recipient. */
  messages: Message[];
}

export interface WaitForBatchOptions {
  /** Time between polls in milliseconds. Default 2000. */
  intervalMs?: number;
  /** Give up after this long and return the latest state. Default 120000. */
  timeoutMs?: number;
  signal?: AbortSignal;
}

export type UpdateDeviceParams = Schemas["DevicePatchBody"];

export interface CreateWebhookParams {
  /** HTTPS endpoint that receives POSTs. */
  url: string;
  events: WebhookEventName[];
  name?: string;
}

export interface UpdateWebhookParams {
  url?: string;
  events?: WebhookEventName[];
  name?: string;
  /** Re-enabling clears an automatic pause. */
  enabled?: boolean;
}

export interface WebhookWithSecret {
  webhook: Webhook;
  /** Signing secret. Shown once; store it. */
  secret: string;
}

export interface ListDeliveriesParams {
  webhook_id?: string;
  status?: DeliveryStatus;
  event?: DeliveredEventName;
  from?: string | Date;
  to?: string | Date;
  cursor?: string;
  limit?: number;
}

interface WebhookEventBase {
  /** Unique per event. Retries of the same delivery reuse it. */
  id: string;
  created_at: string;
}

/** The JSON body of a webhook delivery. Narrow on `event` to type `data`. */
export type WebhookEvent =
  | (WebhookEventBase & { event: MessageEventName; data: Message })
  | (WebhookEventBase & { event: DeviceEventName; data: Device })
  | (WebhookEventBase & { event: "ping"; data: { webhook_id: string; message: string } });
