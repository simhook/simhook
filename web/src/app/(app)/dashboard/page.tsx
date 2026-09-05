"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { LoadError, PageHeader } from "@/components/page-header";
import { useAccount } from "@/components/session-provider";
import { OnlineDot, StatusBadge } from "@/components/status-badge";
import { formatCount, limitLabel, messageStatusLabel, relativeTime, truncate } from "@/lib/format";
import { useApiKeys, useDevices, useMessages, useStats, useWebhooks } from "@/lib/queries";
import { cn } from "@/lib/utils";

const link = "text-sm underline decoration-underline underline-offset-4 hover:decoration-foreground";

function Stat({ label, value, hint }: { label: string; value: number | undefined; hint?: string }) {
  return (
    <div>
      <p className="text-[26px] font-medium leading-none tabular-nums tracking-tight">{value === undefined ? "…" : formatCount(value)}</p>
      <p className="mt-2 font-mono text-xs text-muted-foreground">{label}</p>
      {hint ? <p className="mt-1 text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}

function UsageLine({ label, used, limit }: { label: string; used: number; limit: number }) {
  const unlimited = limit < 0;
  const pct = unlimited ? 0 : Math.min(100, Math.round((used / Math.max(1, limit)) * 100));
  return (
    <div>
      <div className="flex justify-between text-sm">
        <span>{label}</span>
        <span className="tabular-nums text-muted-foreground">
          {formatCount(used)} {unlimited ? "sent, unlimited" : `of ${formatCount(limit)}`}
        </span>
      </div>
      {!unlimited ? (
        <div className="mt-1.5 h-px w-full bg-border">
          <div className={cn("h-px", pct >= 90 ? "bg-destructive" : "bg-foreground")} style={{ width: `${pct}%` }} />
        </div>
      ) : null}
    </div>
  );
}

function Section({ title, aside, children }: { title: string; aside?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="mb-12">
      <div className="mb-2 flex items-baseline justify-between gap-4">
        <h2 className="font-mono text-xs tracking-wide text-muted-foreground">{title}</h2>
        {aside}
      </div>
      {children}
    </section>
  );
}

export default function DashboardPage() {
  const { user, limits, usage } = useAccount();
  const stats = useStats();
  const devices = useDevices(15_000);
  const keys = useApiKeys();
  const hooks = useWebhooks();
  const recent = useMessages({}, 8);

  const deviceList = devices.data?.data ?? [];
  const online = deviceList.filter((d) => d.online);
  const hookList = hooks.data?.data ?? [];
  const steps = [
    { done: !!user.email_verified_at, label: "Verify your email", href: "/verify-email" },
    { done: deviceList.length > 0, label: "Pair an Android phone", href: "/devices" },
    { done: (keys.data?.data.length ?? 0) > 0, label: "Create an API key", href: "/api-keys" },
    { done: (stats.data?.sent ?? 0) > 0, label: "Send your first message", href: "/messages" },
    { done: hookList.length > 0, label: "Add a webhook to receive events", href: "/webhooks" },
  ];
  const remaining = steps.filter((s) => !s.done).length;
  const recentMessages = recent.data?.pages.flatMap((p) => p.data).slice(0, 8) ?? [];

  const status = (() => {
    if (!devices.data) return "";
    if (deviceList.length === 0) return "No phone is paired yet, so nothing can be sent.";
    const first = online[0] ?? deviceList[0];
    const parts = [
      online.length === 0
        ? `${first.name} is offline; last seen ${relativeTime(first.last_heartbeat_at)}.`
        : online.length === 1
          ? `${first.name} is online.`
          : `${online.length} phones are online.`,
    ];
    parts.push(`${formatCount(usage.sent_today)} sent today.`);
    return parts.join(" ");
  })();

  return (
    <>
      <PageHeader title="Overview" description={status} />

      <div className="mb-12 grid grid-cols-2 gap-8 sm:grid-cols-4">
        <Stat label="sent" value={stats.data?.sent} />
        <Stat label="received" value={stats.data?.received} />
        <Stat label="phones" value={stats.data?.devices} hint={`${online.length} online, ${limitLabel(limits.device_limit)} allowed`} />
        <Stat label="webhooks" value={hooks.data ? hookList.length : undefined} hint={hookList.some((h) => !h.enabled) ? "one is paused" : undefined} />
      </div>
      {stats.isError ? <LoadError error={stats.error} retry={() => stats.refetch()} /> : null}

      {remaining > 0 ? (
        <Section title={`get started, ${remaining} of ${steps.length} left`}>
          <ul className="border-t">
            {steps.map((s) => (
              <li key={s.label} className="border-b">
                <Link
                  href={s.href}
                  className={cn("flex items-center gap-3 py-2.5 text-sm", s.done ? "text-muted-foreground line-through" : "hover:underline")}
                >
                  <span className={cn("size-[7px] rounded-full", s.done ? "bg-ok" : "bg-dot-off")} />
                  {s.label}
                </Link>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      <Section
        title="recent messages"
        aside={
          <Link href="/messages" className={link}>
            All messages
          </Link>
        }
      >
        {recent.isError ? (
          <LoadError error={recent.error} retry={() => recent.refetch()} />
        ) : recentMessages.length === 0 ? (
          <p className="border-y py-6 text-sm text-muted-foreground">
            {recent.isPending ? "Loading…" : "Nothing yet. Pair a phone and send your first message."}
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <tbody>
                {recentMessages.map((m) => (
                  <tr key={m.id} className="border-b">
                    <td className="w-10 py-2.5 pr-3 font-mono text-xs text-muted-foreground">{m.direction === "outbound" ? "to" : "from"}</td>
                    <td className="py-2.5 pr-4 font-mono text-xs whitespace-nowrap">{m.direction === "outbound" ? m.recipient : m.sender}</td>
                    <td className="max-w-[360px] truncate py-2.5 pr-4 text-muted-foreground">{truncate(m.body, 80)}</td>
                    <td className="py-2.5 pr-4 whitespace-nowrap">
                      <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} />
                    </td>
                    <td className="py-2.5 text-right font-mono text-xs whitespace-nowrap text-muted-foreground" title={new Date(m.created_at).toLocaleString()}>
                      {relativeTime(m.created_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <div className="grid gap-x-12 md:grid-cols-2">
        <Section
          title="phones"
          aside={
            <Link href="/devices" className={link}>
              Manage
            </Link>
          }
        >
          {devices.isError ? (
            <LoadError error={devices.error} retry={() => devices.refetch()} />
          ) : deviceList.length === 0 ? (
            <div className="border-y py-6 text-sm">
              <p className="text-muted-foreground">{devices.isPending ? "Loading…" : "No phone paired yet."}</p>
              {!devices.isPending ? (
                <Button className="mt-3" nativeButton={false} render={<Link href="/devices" />}>
                  Pair a phone
                </Button>
              ) : null}
            </div>
          ) : (
            <table className="w-full text-sm">
              <tbody>
                {deviceList.map((d) => (
                  <tr key={d.id} className="border-b">
                    <td className="py-2.5 pr-4">
                      <Link href={`/devices/${d.id}`} className="hover:underline">
                        {d.name}
                      </Link>
                      {d.is_default ? <span className="ml-2 font-mono text-[11px] text-muted-foreground">default</span> : null}
                    </td>
                    <td className="py-2.5 pr-4 text-muted-foreground">{relativeTime(d.last_heartbeat_at)}</td>
                    <td className="py-2.5 text-right">
                      <OnlineDot online={d.online} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Section>

        <Section title={`${limits.plan_name.toLowerCase()} plan`}>
          <div className="grid gap-4 border-t pt-4">
            <UsageLine label="Today" used={usage.sent_today} limit={limits.daily_limit} />
            <UsageLine label="This month" used={usage.sent_this_month} limit={limits.monthly_limit} />
            <p className="text-xs text-muted-foreground">
              Up to {limitLabel(limits.batch_limit)} recipients per send. Received messages are not counted.{" "}
              <Link href="/settings" className="underline decoration-underline underline-offset-4 hover:decoration-foreground">
                Plans
              </Link>
            </p>
          </div>
        </Section>
      </div>
    </>
  );
}
