"use client";

import { useState } from "react";
import { toast } from "sonner";
import type { Delivery, Webhook } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CopyField } from "@/components/copy-button";
import { EmptyState, LoadError, PageHeader, textLink } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { EVENTS, WebhookDialog } from "@/components/webhooks/webhook-dialog";
import { errorMessage } from "@/lib/api";
import { absoluteTime, deliveryStatusLabel, formatCount, relativeTime } from "@/lib/format";
import { useDeliveries, useWebhookMutations, useWebhooks, type DeliveryFilter } from "@/lib/queries";

/** Row actions are words with a dot between them. */
function Actions({ children }: { children: React.ReactNode }) {
  return <span className="flex flex-wrap items-baseline gap-x-2 text-sm [&>button]:underline [&>button]:decoration-underline [&>button]:underline-offset-4 [&>button:hover]:decoration-foreground [&>button:disabled]:opacity-50">{children}</span>;
}

const Dot = () => <span className="text-muted-foreground">·</span>;

function EndpointsTab({ onAdd, onEdit }: { onAdd: () => void; onEdit: (w: Webhook) => void }) {
  const hooks = useWebhooks();
  const { update, rotateSecret, test, remove } = useWebhookMutations();
  const [rotated, setRotated] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<Webhook | null>(null);
  const list = hooks.data?.data ?? [];

  if (hooks.isPending) return <Skeleton className="h-40" />;
  if (hooks.isError) return <LoadError error={hooks.error} retry={() => hooks.refetch()} />;
  if (list.length === 0) {
    return (
      <EmptyState
        title="No endpoints yet"
        description="Add a URL to get an HTTP POST for every message event, signed so you can trust it."
        action={
          <button type="button" className={textLink} onClick={onAdd}>
            Add an endpoint
          </button>
        }
      />
    );
  }

  return (
    <>
      <ul className="border-t">
        {list.map((w) => (
          <li key={w.id} className="grid gap-2 border-b py-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="font-medium">{w.name || "Unnamed endpoint"}</p>
                <p className="truncate font-mono text-xs text-muted-foreground">{w.url}</p>
              </div>
              <label className="flex items-center gap-2 text-sm text-muted-foreground">
                {w.enabled ? "On" : "Off"}
                <Switch
                  checked={w.enabled}
                  disabled={update.isPending}
                  onCheckedChange={(v) => update.mutate({ id: w.id, enabled: v }, { onError: (e) => toast.error(errorMessage(e)) })}
                  aria-label="Enabled"
                />
              </label>
            </div>
            <p className="font-mono text-[11px] text-muted-foreground">{w.events.join(", ")}</p>
            <p className="text-xs text-muted-foreground">
              {formatCount(w.success_count)} delivered, {formatCount(w.failure_count)} failed
              {w.last_success_at ? `, last success ${relativeTime(w.last_success_at)}` : ""}
              {w.last_failure_at ? `, last failure ${relativeTime(w.last_failure_at)}` : ""}
            </p>
            {!w.enabled && w.disabled_reason ? <p className="text-sm text-warn">{w.disabled_reason}</p> : null}
            <Actions>
              <button type="button" onClick={() => onEdit(w)}>
                Edit
              </button>
              <Dot />
              <button
                type="button"
                disabled={test.isPending}
                onClick={() => test.mutate(w.id, { onSuccess: () => toast.success("Test event queued. See the deliveries tab."), onError: (e) => toast.error(errorMessage(e)) })}
              >
                Send a test event
              </button>
              <Dot />
              <button
                type="button"
                disabled={rotateSecret.isPending}
                onClick={() => rotateSecret.mutate(w.id, { onSuccess: (r) => setRotated(r.secret), onError: (e) => toast.error(errorMessage(e)) })}
              >
                Rotate secret
              </button>
              <Dot />
              <button type="button" className="text-destructive" onClick={() => setDeleting(w)}>
                Delete
              </button>
            </Actions>
          </li>
        ))}
      </ul>

      <Dialog open={!!rotated} onOpenChange={(o) => !o && setRotated(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New signing secret</DialogTitle>
            <DialogDescription>Deliveries from now on are signed with it. It is shown once.</DialogDescription>
          </DialogHeader>
          {rotated ? <CopyField value={rotated} secret /> : null}
          <DialogFooter>
            <Button onClick={() => setRotated(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete this endpoint?</DialogTitle>
            <DialogDescription>Deliveries stop at once. Its history stays in the deliveries tab.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              disabled={remove.isPending}
              onClick={() => deleting && remove.mutate(deleting.id, { onSuccess: () => setDeleting(null), onError: (e) => toast.error(errorMessage(e)) })}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function DeliveriesTab() {
  const hooks = useWebhooks();
  const [webhook, setWebhook] = useState("all");
  const [status, setStatus] = useState("all");
  const [event, setEvent] = useState("all");
  const [selected, setSelected] = useState<Delivery | null>(null);
  const filter: DeliveryFilter = {
    webhook_id: webhook === "all" ? undefined : webhook,
    status: status === "all" ? undefined : status,
    event: event === "all" ? undefined : event,
  };
  const deliveries = useDeliveries(filter);
  const rows = deliveries.data?.pages.flatMap((p) => p.data) ?? [];
  const names = new Map((hooks.data?.data ?? []).map((w) => [w.id, w.name || w.url]));

  return (
    <>
      <div className="mb-4 flex flex-wrap gap-2">
        <Select value={webhook} onValueChange={(v) => setWebhook(v ?? "all")} items={{ all: "All endpoints", ...Object.fromEntries(names) }}>
          <SelectTrigger className="w-48" aria-label="Endpoint">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All endpoints</SelectItem>
            {(hooks.data?.data ?? []).map((w) => (
              <SelectItem key={w.id} value={w.id}>
                {w.name || w.url}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={status} onValueChange={(v) => setStatus(v ?? "all")} items={{ all: "Any status", ...deliveryStatusLabel }}>
          <SelectTrigger className="w-36" aria-label="Status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Any status</SelectItem>
            {Object.entries(deliveryStatusLabel).map(([k, v]) => (
              <SelectItem key={k} value={k}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={event} onValueChange={(v) => setEvent(v ?? "all")} items={{ all: "Any event", ...Object.fromEntries([...EVENTS.map((e) => e.id), "ping"].map((e) => [e, e])) }}>
          <SelectTrigger className="w-48" aria-label="Event">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Any event</SelectItem>
            {[...EVENTS.map((e) => e.id), "ping"].map((e) => (
              <SelectItem key={e} value={e}>
                {e}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {deliveries.isPending ? (
        <Skeleton className="h-48" />
      ) : deliveries.isError ? (
        <LoadError error={deliveries.error} retry={() => deliveries.refetch()} />
      ) : rows.length === 0 ? (
        <EmptyState title="No deliveries yet" description="Events appear here as they are sent to your endpoints." />
      ) : (
        <>
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Event</TableHead>
                  <TableHead>Endpoint</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>HTTP</TableHead>
                  <TableHead>Attempts</TableHead>
                  <TableHead className="text-right">When</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((d) => (
                  <TableRow key={d.id} className="cursor-pointer hover:bg-secondary" onClick={() => setSelected(d)}>
                    <TableCell className="font-mono text-xs">
                      <button
                        type="button"
                        className="underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
                        onClick={(e) => {
                          e.stopPropagation();
                          setSelected(d);
                        }}
                      >
                        {d.event}
                      </button>
                    </TableCell>
                    <TableCell className="max-w-xs truncate">{names.get(d.webhook_id) ?? d.url}</TableCell>
                    <TableCell>
                      <StatusBadge status={d.status} label={deliveryStatusLabel[d.status] ?? d.status} />
                    </TableCell>
                    <TableCell className="tabular-nums">{d.http_status ?? ""}</TableCell>
                    <TableCell className="tabular-nums">
                      {d.attempt_count}
                      {d.next_attempt_at && d.status === "retrying" ? <span className="text-muted-foreground">, next {relativeTime(d.next_attempt_at)}</span> : null}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-muted-foreground" title={absoluteTime(d.created_at)}>
                      {relativeTime(d.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {deliveries.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <button type="button" className={textLink} onClick={() => deliveries.fetchNextPage()} disabled={deliveries.isFetchingNextPage}>
                {deliveries.isFetchingNextPage ? "Loading…" : "Load more"}
              </button>
            </div>
          ) : null}
        </>
      )}
      <Dialog open={!!selected} onOpenChange={(o) => !o && setSelected(null)}>
        <DialogContent className="sm:max-w-2xl">
          {selected ? (
            <>
              <DialogHeader>
                <DialogTitle className="font-mono text-base">{selected.event}</DialogTitle>
                <DialogDescription>
                  {absoluteTime(selected.created_at)}, {deliveryStatusLabel[selected.status] ?? selected.status}
                  {selected.http_status ? `, HTTP ${selected.http_status}` : ""}, {selected.attempt_count} attempt{selected.attempt_count === 1 ? "" : "s"}
                </DialogDescription>
              </DialogHeader>
              {selected.error ? <p className="text-sm text-destructive">{selected.error}</p> : null}
              <p className="font-mono text-xs text-muted-foreground">payload</p>
              <pre className="max-h-72 overflow-auto border bg-secondary p-3 font-mono text-xs">{JSON.stringify(selected.payload, null, 2)}</pre>
              {selected.response_excerpt ? (
                <>
                  <p className="font-mono text-xs text-muted-foreground">response</p>
                  <pre className="max-h-40 overflow-auto border bg-secondary p-3 font-mono text-xs">{selected.response_excerpt}</pre>
                </>
              ) : null}
            </>
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  );
}

export default function WebhooksPage() {
  const [dialog, setDialog] = useState<{ open: boolean; existing: Webhook | null }>({ open: false, existing: null });
  return (
    <>
      <PageHeader
        title="Webhooks"
        description="Get an HTTP request the moment a message is received, sent, delivered, or fails."
        actions={<Button onClick={() => setDialog({ open: true, existing: null })}>Add an endpoint</Button>}
      />
      <WebhookDialog open={dialog.open} existing={dialog.existing} onOpenChange={(o) => setDialog((d) => ({ ...d, open: o }))} />
      <Tabs defaultValue="endpoints">
        <TabsList className="mb-4">
          <TabsTrigger value="endpoints">Endpoints</TabsTrigger>
          <TabsTrigger value="deliveries">Deliveries</TabsTrigger>
        </TabsList>
        <TabsContent value="endpoints">
          <EndpointsTab onAdd={() => setDialog({ open: true, existing: null })} onEdit={(w) => setDialog({ open: true, existing: w })} />
        </TabsContent>
        <TabsContent value="deliveries">
          <DeliveriesTab />
        </TabsContent>
      </Tabs>
    </>
  );
}
