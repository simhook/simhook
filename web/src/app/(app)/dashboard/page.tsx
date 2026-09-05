"use client";

import Link from "next/link";
import { ArrowRight, Check, Circle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { OnlineDot, StatusBadge } from "@/components/status-badge";
import { formatCount, limitLabel, messageStatusLabel, relativeTime, truncate } from "@/lib/format";
import { useApiKeys, useDevices, useMe, useMessages, useStats, useWebhooks } from "@/lib/queries";
import { cn } from "@/lib/utils";

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-3xl tabular-nums">{value}</CardTitle>
      </CardHeader>
      {hint ? <CardContent className="pt-0 text-sm text-muted-foreground">{hint}</CardContent> : null}
    </Card>
  );
}

function UsageBar({ used, limit }: { used: number; limit: number }) {
  if (limit < 0) return <p className="text-sm text-muted-foreground">{formatCount(used)} sent · unlimited</p>;
  const pct = Math.min(100, Math.round((used / Math.max(1, limit)) * 100));
  return (
    <div>
      <div className="mb-1 flex justify-between text-sm">
        <span>{formatCount(used)} of {formatCount(limit)}</span>
        <span className="text-muted-foreground">{pct}%</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-muted">
        <div className={cn("h-full rounded-full", pct >= 90 ? "bg-destructive" : "bg-primary")} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export default function DashboardPage() {
  const me = useMe();
  const stats = useStats();
  const devices = useDevices(15_000);
  const keys = useApiKeys();
  const hooks = useWebhooks();
  const recent = useMessages({}, 8);

  const user = me.data?.user;
  const limits = me.data?.limits;
  const usage = me.data?.usage;
  const deviceList = devices.data?.data ?? [];
  const steps = [
    { done: !!user?.email_verified_at, label: "Verify your email", href: "/verify-email" },
    { done: deviceList.length > 0, label: "Pair an Android phone", href: "/devices" },
    { done: (keys.data?.data.length ?? 0) > 0, label: "Create an API key", href: "/api-keys" },
    { done: (stats.data?.sent ?? 0) > 0, label: "Send your first message", href: "/messages" },
    { done: (hooks.data?.data.length ?? 0) > 0, label: "Add a webhook to receive events", href: "/webhooks" },
  ];
  const remaining = steps.filter((s) => !s.done).length;
  const recentMessages = recent.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <>
      <PageHeader title={`Hello${user?.name ? `, ${user.name.split(" ")[0]}` : ""}`} description="Here is how your gateway is doing." />

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Messages sent" value={formatCount(stats.data?.sent)} />
        <Stat label="Messages received" value={formatCount(stats.data?.received)} />
        <Stat
          label="Devices"
          value={formatCount(stats.data?.devices)}
          hint={`${deviceList.filter((d) => d.online).length} online · ${limits ? limitLabel(limits.device_limit) : "…"} allowed`}
        />
      </div>

      <div className="mt-6 grid gap-6 lg:grid-cols-3">
        <div className="grid gap-6 lg:col-span-2">
          {remaining > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle>Get started</CardTitle>
                <CardDescription>{remaining} of {steps.length} steps left.</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-1">
                {steps.map((s) => (
                  <Link
                    key={s.label}
                    href={s.href}
                    className={cn("flex items-center gap-3 rounded-md px-2 py-2 text-sm hover:bg-muted", s.done && "text-muted-foreground line-through")}
                  >
                    {s.done ? <Check className="size-4 text-emerald-600" /> : <Circle className="size-4 text-muted-foreground/50" />}
                    <span className="flex-1">{s.label}</span>
                    {!s.done ? <ArrowRight className="size-4 text-muted-foreground" /> : null}
                  </Link>
                ))}
              </CardContent>
            </Card>
          ) : null}

          <Card>
            <CardHeader className="flex-row items-center justify-between">
              <div>
                <CardTitle>Recent messages</CardTitle>
                <CardDescription>Both directions, newest first.</CardDescription>
              </div>
              <Button variant="outline" size="sm" nativeButton={false} render={<Link href="/messages" />}>
                View all
              </Button>
            </CardHeader>
            <CardContent>
              {recentMessages.length === 0 ? (
                <p className="text-sm text-muted-foreground">Nothing yet. Pair a phone and send your first message.</p>
              ) : (
                <ul className="divide-y">
                  {recentMessages.map((m) => (
                    <li key={m.id} className="flex items-center gap-3 py-2 text-sm">
                      <span className="w-14 shrink-0 text-xs uppercase text-muted-foreground">{m.direction === "outbound" ? "to" : "from"}</span>
                      <span className="w-36 shrink-0 truncate font-mono text-xs">{m.direction === "outbound" ? m.recipient : m.sender}</span>
                      <span className="min-w-0 flex-1 truncate text-muted-foreground">{truncate(m.body, 80)}</span>
                      <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} />
                      <span className="w-24 shrink-0 text-right text-xs text-muted-foreground">{relativeTime(m.created_at)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>

        <div className="grid gap-6">
          <Card>
            <CardHeader>
              <CardTitle>Plan usage</CardTitle>
              <CardDescription>{limits ? `${limits.plan_name} plan` : "…"}</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4">
              {limits && usage ? (
                <>
                  <div>
                    <p className="mb-1 text-sm font-medium">Today</p>
                    <UsageBar used={usage.sent_today} limit={limits.daily_limit} />
                  </div>
                  <div>
                    <p className="mb-1 text-sm font-medium">This month</p>
                    <UsageBar used={usage.sent_this_month} limit={limits.monthly_limit} />
                  </div>
                  <p className="text-xs text-muted-foreground">Up to {limitLabel(limits.batch_limit)} recipients per send. Received messages are not counted.</p>
                </>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex-row items-center justify-between">
              <CardTitle>Devices</CardTitle>
              <Button variant="outline" size="sm" nativeButton={false} render={<Link href="/devices" />}>
                Manage
              </Button>
            </CardHeader>
            <CardContent>
              {deviceList.length === 0 ? (
                <p className="text-sm text-muted-foreground">No phone paired yet.</p>
              ) : (
                <ul className="grid gap-2">
                  {deviceList.map((d) => (
                    <li key={d.id} className="flex items-center justify-between text-sm">
                      <Link href={`/devices/${d.id}`} className="truncate font-medium hover:underline">
                        {d.name}
                      </Link>
                      <OnlineDot online={d.online} />
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  );
}
