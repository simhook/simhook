"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import { errorMessage } from "@/lib/api";
import { absoluteTime, batchStatusLabel, messageStatusLabel, relativeTime } from "@/lib/format";
import { useBatch, useDevices } from "@/lib/queries";

export default function SendPage() {
  const { id } = useParams<{ id: string }>();
  const devices = useDevices();
  const query = useBatch(id, true);
  const b = query.data?.batch;
  const live = b ? b.status === "queued" || b.status === "processing" : true;
  const batch = useBatch(id, live);
  const data = batch.data ?? query.data;

  if (!data) {
    return query.isError ? <p className="text-destructive">{errorMessage(query.error)}</p> : <Skeleton className="h-64" />;
  }
  const { batch: bt, messages } = data;
  const deviceName = devices.data?.data.find((d) => d.id === bt.device_id)?.name ?? "—";
  const inFlight = bt.recipient_count - (bt.sent_count + bt.delivered_count + bt.failed_count + bt.unknown_count);
  const segs: [string, number, string][] = [
    ["Delivered", bt.delivered_count, "bg-emerald-500"],
    ["Sent", bt.sent_count, "bg-primary"],
    ["In flight", inFlight, "bg-muted-foreground/40"],
    ["No result", bt.unknown_count, "bg-amber-500"],
    ["Failed", bt.failed_count, "bg-destructive"],
  ];

  return (
    <>
      <Button variant="ghost" size="sm" className="mb-3 -ml-2 gap-1 text-muted-foreground" nativeButton={false} render={<Link href="/messages" />}>
        <ArrowLeft className="size-4" />
        Messages
      </Button>
      <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-3 text-2xl font-semibold tracking-tight">
            Send to {bt.recipient_count} recipient{bt.recipient_count === 1 ? "" : "s"}
            <StatusBadge status={bt.status} label={batchStatusLabel[bt.status] ?? bt.status} />
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {absoluteTime(bt.created_at)} · from {deviceName}
            {bt.scheduled_at ? ` · scheduled for ${absoluteTime(bt.scheduled_at)}` : ""}
            {live ? " · updating live" : ""}
          </p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Progress</CardTitle>
            {bt.estimated_completion_at && live ? <CardDescription>Expected to finish {relativeTime(bt.estimated_completion_at)}.</CardDescription> : null}
          </CardHeader>
          <CardContent>
            <div className="flex h-3 overflow-hidden rounded-full bg-muted">
              {segs.map(([label, n, cls]) =>
                n > 0 ? <div key={label} className={cls} style={{ width: `${(n / bt.recipient_count) * 100}%` }} title={`${label}: ${n}`} /> : null,
              )}
            </div>
            <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-sm">
              {segs.map(([label, n, cls]) => (
                <span key={label} className="inline-flex items-center gap-1.5">
                  <span className={`size-2.5 rounded-full ${cls}`} />
                  {label} <span className="tabular-nums text-muted-foreground">{n}</span>
                </span>
              ))}
            </div>
            {bt.error ? <p className="mt-3 text-sm text-destructive">{bt.error}</p> : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Message</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="whitespace-pre-wrap text-sm">{bt.body}</p>
            <p className="mt-3 font-mono text-xs text-muted-foreground">{bt.id}</p>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Recipients</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Number</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Detail</TableHead>
                <TableHead className="text-right">Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {messages.map((m) => (
                <TableRow key={m.id}>
                  <TableCell className="font-mono text-xs">{m.recipient}</TableCell>
                  <TableCell>
                    <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} />
                  </TableCell>
                  <TableCell className="max-w-md text-sm text-muted-foreground">
                    {m.error_message ?? (m.delivered_at ? `Delivered ${relativeTime(m.delivered_at)}` : m.sent_at ? `Sent ${relativeTime(m.sent_at)}` : "")}
                  </TableCell>
                  <TableCell className="text-right text-sm text-muted-foreground">{relativeTime(m.updated_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}
