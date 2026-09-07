"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { LoadError, textLink } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { absoluteTime, batchStatusLabel, messageStatusLabel, relativeTime } from "@/lib/format";
import { batchLive, useBatch, useDevices, useMessages } from "@/lib/queries";

export default function SendPage() {
  const { id } = useParams<{ id: string }>();
  const devices = useDevices();
  // The counters, asked again every few seconds only while the send is
  // moving; the recipients come a page at a time and refresh more slowly,
  // so a send to thousands is not downloaded whole every few seconds.
  const batch = useBatch(id);
  const live = batchLive(batch.data?.batch.status);
  const messages = useMessages({ batch_id: id }, 100, live);

  if (batch.isPending) return <Skeleton className="mt-12 h-64" />;
  if (batch.isError) {
    return (
      <div className="mt-12">
        <LoadError error={batch.error} retry={() => batch.refetch()} />
        <Link href="/messages" className={`${textLink} mt-4 inline-block`}>
          Back to messages
        </Link>
      </div>
    );
  }
  const bt = batch.data.batch;
  const rows = messages.data?.pages.flatMap((p) => p.data) ?? [];
  const deviceName = devices.data?.data.find((d) => d.id === bt.device_id)?.name ?? "";
  const inFlight = bt.recipient_count - (bt.sent_count + bt.delivered_count + bt.failed_count + bt.unknown_count);
  const segs: [string, number, string][] = [
    ["Delivered", bt.delivered_count, "bg-ok"],
    ["Sent", bt.sent_count, "bg-foreground"],
    ["In flight", inFlight, "bg-dot-off"],
    ["No result", bt.unknown_count, "bg-warn"],
    ["Failed", bt.failed_count, "bg-destructive"],
  ];

  return (
    <>
      <Link href="/messages" className={`${textLink} mt-8 inline-block text-muted-foreground`}>
        Back to messages
      </Link>
      <div className="mb-6 mt-4 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex flex-wrap items-baseline gap-3 text-2xl font-semibold tracking-tight">
            Send to {bt.recipient_count} recipient{bt.recipient_count === 1 ? "" : "s"}
            <StatusBadge status={bt.status} label={batchStatusLabel[bt.status] ?? bt.status} />
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {absoluteTime(bt.created_at)}
            {deviceName ? `, from ${deviceName}` : ""}
            {bt.scheduled_at ? `, scheduled for ${absoluteTime(bt.scheduled_at)}` : ""}
            {live ? ", updating live" : ""}
          </p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>progress</CardTitle>
            {bt.estimated_completion_at && live ? <CardDescription>Expected to finish {relativeTime(bt.estimated_completion_at)}.</CardDescription> : null}
          </CardHeader>
          <CardContent>
            <div className="flex h-px w-full bg-border">
              {segs.map(([label, n, cls]) =>
                n > 0 ? <div key={label} className={`h-px ${cls}`} style={{ width: `${(n / bt.recipient_count) * 100}%` }} title={`${label}: ${n}`} /> : null,
              )}
            </div>
            <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-sm">
              {segs.map(([label, n, cls]) => (
                <span key={label} className="inline-flex items-center gap-2">
                  <span className={`size-[7px] rounded-full ${cls}`} />
                  {label} <span className="tabular-nums text-muted-foreground">{n}</span>
                </span>
              ))}
            </div>
            {bt.error ? <p className="mt-3 text-sm text-destructive">{bt.error}</p> : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>message</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="whitespace-pre-wrap break-words text-sm">{bt.body}</p>
            <p className="mt-3 truncate font-mono text-xs text-muted-foreground" title={bt.id}>
              {bt.id}
            </p>
          </CardContent>
        </Card>
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>recipients</CardTitle>
          {bt.recipient_count > rows.length && !messages.isPending ? <CardDescription>The latest {rows.length} of {bt.recipient_count}.</CardDescription> : null}
        </CardHeader>
        <CardContent>
          {messages.isPending ? (
            <Skeleton className="h-24" />
          ) : messages.isError ? (
            <LoadError error={messages.error} retry={() => messages.refetch()} />
          ) : (
            <>
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
                  {rows.map((m) => (
                    <TableRow key={m.id}>
                      <TableCell className="font-mono text-xs">{m.recipient}</TableCell>
                      <TableCell>
                        <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} />
                      </TableCell>
                      <TableCell className="max-w-md text-sm text-muted-foreground">
                        {m.error_message ?? (m.delivered_at ? `Delivered ${relativeTime(m.delivered_at)}` : m.sent_at ? `Sent ${relativeTime(m.sent_at)}` : "")}
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground" title={absoluteTime(m.updated_at)}>
                        {relativeTime(m.updated_at)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              {messages.hasNextPage ? (
                <div className="mt-3 flex justify-center">
                  <button type="button" className={textLink} onClick={() => messages.fetchNextPage()} disabled={messages.isFetchingNextPage}>
                    {messages.isFetchingNextPage ? "Loading…" : "Load more"}
                  </button>
                </div>
              ) : null}
            </>
          )}
        </CardContent>
      </Card>
    </>
  );
}
