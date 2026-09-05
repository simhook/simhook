"use client";

import { useDeferredValue, useState } from "react";
import Link from "next/link";
import type { Message } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState, LoadError, PageHeader, textLink } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { SendDialog } from "@/components/messages/send-dialog";
import { absoluteTime, batchStatusLabel, messageStatusLabel, relativeTime, truncate } from "@/lib/format";
import { useBatches, useDevices, useMessages, type MessageFilter } from "@/lib/queries";

function Time({ iso }: { iso: string }) {
  return (
    <span className="whitespace-nowrap text-muted-foreground" title={absoluteTime(iso)}>
      {relativeTime(iso)}
    </span>
  );
}

/** The cell that opens a row: a real button, so a keyboard reaches it. */
function RowButton({ children, onClick, className = "" }: { children: React.ReactNode; onClick: () => void; className?: string }) {
  return (
    <button
      type="button"
      className={`text-left underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground ${className}`}
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
    >
      {children}
    </button>
  );
}

function MessageDetail({ message, deviceName, onClose }: { message: Message | null; deviceName?: string; onClose: () => void }) {
  const m = message;
  return (
    <Dialog open={!!m} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        {m ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                {m.direction === "outbound" ? "Sent to" : "Received from"} <span className="font-mono">{m.direction === "outbound" ? m.recipient : m.sender}</span>
              </DialogTitle>
              <DialogDescription>
                <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} /> {deviceName ? `, ${deviceName}` : ""}
              </DialogDescription>
            </DialogHeader>
            <p className="whitespace-pre-wrap border-l-2 border-border pl-4 text-sm">{m.body}</p>
            {m.error_message ? (
              <p className="text-sm text-destructive">
                {m.error_code ? <span className="font-mono">{m.error_code}: </span> : null}
                {m.error_message}
              </p>
            ) : null}
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              {[
                ["Created", m.created_at],
                ["On the phone", m.dispatched_at],
                ["Sent", m.sent_at],
                ["Delivered", m.delivered_at],
                ["Failed", m.failed_at],
                ["Received", m.received_at],
              ]
                .filter(([, v]) => v)
                .map(([k, v]) => (
                  <div key={k as string} className="contents">
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd>{absoluteTime(v as string)}</dd>
                  </div>
                ))}
              {m.batch_id ? (
                <div className="contents">
                  <dt className="text-muted-foreground">Send</dt>
                  <dd>
                    <Link href={`/sends/${m.batch_id}`} className="underline decoration-underline underline-offset-4 hover:decoration-foreground">
                      View the whole send
                    </Link>
                  </dd>
                </div>
              ) : null}
              <div className="contents">
                <dt className="text-muted-foreground">Message id</dt>
                <dd className="font-mono text-xs">{m.id}</dd>
              </div>
            </dl>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

const DIRECTIONS = { all: "All directions", outbound: "Sent", inbound: "Received" };

function MessagesTab({ deviceNames, onSend }: { deviceNames: Map<string, string>; onSend: () => void }) {
  const [direction, setDirection] = useState("all");
  const [status, setStatus] = useState("all");
  const [device, setDevice] = useState("all");
  const [search, setSearch] = useState("");
  const q = useDeferredValue(search);
  const [selected, setSelected] = useState<Message | null>(null);

  const filter: MessageFilter = {
    direction: direction === "all" ? undefined : (direction as MessageFilter["direction"]),
    status: status === "all" ? undefined : status,
    device_ids: device === "all" ? undefined : device,
    q: q.trim() || undefined,
  };
  const messages = useMessages(filter);
  const rows = messages.data?.pages.flatMap((p) => p.data) ?? [];
  const filtered = !!search || status !== "all" || direction !== "all" || device !== "all";

  return (
    <>
      <div className="mb-4 flex flex-wrap gap-2">
        <Select value={direction} onValueChange={(v) => setDirection(v ?? "all")} items={DIRECTIONS}>
          <SelectTrigger className="w-36" aria-label="Direction">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All directions</SelectItem>
            <SelectItem value="outbound">Sent</SelectItem>
            <SelectItem value="inbound">Received</SelectItem>
          </SelectContent>
        </Select>
        <Select value={status} onValueChange={(v) => setStatus(v ?? "all")} items={{ all: "Any status", ...messageStatusLabel }}>
          <SelectTrigger className="w-40" aria-label="Status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Any status</SelectItem>
            {Object.entries(messageStatusLabel).map(([k, v]) => (
              <SelectItem key={k} value={k}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={device} onValueChange={(v) => setDevice(v ?? "all")} items={{ all: "All phones", ...Object.fromEntries(deviceNames) }}>
          <SelectTrigger className="w-44" aria-label="Phone">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All phones</SelectItem>
            {[...deviceNames.entries()].map(([id, name]) => (
              <SelectItem key={id} value={id}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input placeholder="Search text or number" aria-label="Search" value={search} onChange={(e) => setSearch(e.target.value)} className="w-56" />
      </div>

      {messages.isPending ? (
        <Skeleton className="h-64" />
      ) : messages.isError ? (
        <LoadError error={messages.error} retry={() => messages.refetch()} />
      ) : rows.length === 0 ? (
        <EmptyState
          title={filtered ? "No messages match" : "No messages yet"}
          description={filtered ? "Try a broader filter." : "Send your first message to see it here."}
          action={
            <button type="button" className={textLink} onClick={onSend}>
              Send a message
            </button>
          }
        />
      ) : (
        <>
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead>Number</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Phone</TableHead>
                  <TableHead className="text-right">When</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((m) => (
                  <TableRow key={m.id} className="cursor-pointer hover:bg-secondary" onClick={() => setSelected(m)}>
                    <TableCell className="font-mono text-xs text-muted-foreground">{m.direction === "outbound" ? "to" : "from"}</TableCell>
                    <TableCell className="font-mono text-xs">
                      <RowButton onClick={() => setSelected(m)}>{m.direction === "outbound" ? m.recipient : m.sender}</RowButton>
                    </TableCell>
                    <TableCell className="max-w-md truncate">{truncate(m.body, 90)}</TableCell>
                    <TableCell>
                      <StatusBadge status={m.status} label={messageStatusLabel[m.status] ?? m.status} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">{deviceNames.get(m.device_id) ?? ""}</TableCell>
                    <TableCell className="text-right">
                      <Time iso={m.created_at} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {messages.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <button type="button" className={textLink} onClick={() => messages.fetchNextPage()} disabled={messages.isFetchingNextPage}>
                {messages.isFetchingNextPage ? "Loading…" : "Load more"}
              </button>
            </div>
          ) : null}
        </>
      )}
      <MessageDetail message={selected} deviceName={selected ? deviceNames.get(selected.device_id) : undefined} onClose={() => setSelected(null)} />
    </>
  );
}

function SendsTab({ deviceNames }: { deviceNames: Map<string, string> }) {
  const batches = useBatches();
  const rows = batches.data?.pages.flatMap((p) => p.data) ?? [];
  if (batches.isPending) return <Skeleton className="h-64" />;
  if (batches.isError) return <LoadError error={batches.error} retry={() => batches.refetch()} />;
  if (rows.length === 0) return <EmptyState title="No sends yet" description="Every API call or dashboard send shows up here with its progress." />;
  return (
    <>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Recipients</TableHead>
              <TableHead>Message</TableHead>
              <TableHead>Progress</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Phone</TableHead>
              <TableHead className="text-right">When</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((b) => (
              <TableRow key={b.id}>
                <TableCell>
                  <Link href={`/sends/${b.id}`} className="font-medium hover:underline">
                    {b.recipient_count} recipient{b.recipient_count === 1 ? "" : "s"}
                  </Link>
                  <p className="font-mono text-xs text-muted-foreground">{b.recipient_preview}</p>
                </TableCell>
                <TableCell className="max-w-xs truncate">{truncate(b.body, 60)}</TableCell>
                <TableCell className="whitespace-nowrap text-sm tabular-nums">
                  {b.delivered_count} delivered, {b.sent_count} sent, {b.failed_count} failed
                </TableCell>
                <TableCell>
                  <StatusBadge status={b.status} label={batchStatusLabel[b.status] ?? b.status} />
                </TableCell>
                <TableCell className="text-muted-foreground">{deviceNames.get(b.device_id) ?? ""}</TableCell>
                <TableCell className="text-right">
                  <Time iso={b.created_at} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {batches.hasNextPage ? (
        <div className="mt-3 flex justify-center">
          <button type="button" className={textLink} onClick={() => batches.fetchNextPage()} disabled={batches.isFetchingNextPage}>
            {batches.isFetchingNextPage ? "Loading…" : "Load more"}
          </button>
        </div>
      ) : null}
    </>
  );
}

export default function MessagesPage() {
  const devices = useDevices();
  const [sending, setSending] = useState(false);
  const deviceNames = new Map((devices.data?.data ?? []).map((d) => [d.id, d.name]));
  return (
    <>
      <PageHeader
        title="Messages"
        description="Everything sent and received across your phones."
        actions={<Button onClick={() => setSending(true)}>Send a message</Button>}
      />
      <SendDialog open={sending} onOpenChange={setSending} />
      <Tabs defaultValue="messages">
        <TabsList className="mb-4">
          <TabsTrigger value="messages">Messages</TabsTrigger>
          <TabsTrigger value="sends">Sends</TabsTrigger>
        </TabsList>
        <TabsContent value="messages">
          <MessagesTab deviceNames={deviceNames} onSend={() => setSending(true)} />
        </TabsContent>
        <TabsContent value="sends">
          <SendsTab deviceNames={deviceNames} />
        </TabsContent>
      </Tabs>
    </>
  );
}
