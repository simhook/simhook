"use client";

import { useState } from "react";
import { MoreHorizontal, Plus } from "lucide-react";
import { toast } from "sonner";
import type { Delivery, Webhook } from "@simhook/contracts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CopyField } from "@/components/copy-button";
import { EmptyState, PageHeader } from "@/components/page-header";
import { StatusBadge } from "@/components/status-badge";
import { EVENTS, WebhookDialog } from "@/components/webhooks/webhook-dialog";
import { errorMessage } from "@/lib/api";
import { absoluteTime, deliveryStatusLabel, formatCount, relativeTime } from "@/lib/format";
import { useDeliveries, useWebhookMutations, useWebhooks, type DeliveryFilter } from "@/lib/queries";

function EndpointsTab({ onAdd, onEdit }: { onAdd: () => void; onEdit: (w: Webhook) => void }) {
  const hooks = useWebhooks();
  const { update, rotateSecret, test, remove } = useWebhookMutations();
  const [rotated, setRotated] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<Webhook | null>(null);
  const list = hooks.data?.data ?? [];

  if (hooks.isPending) return <Skeleton className="h-40" />;
  if (list.length === 0) {
    return (
      <EmptyState
        title="No endpoints yet"
        description="Add a URL to get an HTTP POST for every message event, signed so you can trust it."
        action={<Button onClick={onAdd}>Add an endpoint</Button>}
      />
    );
  }

  return (
    <>
      <div className="grid gap-3">
        {list.map((w) => (
          <Card key={w.id}>
            <CardContent className="grid gap-3 py-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-medium">{w.name || "Unnamed endpoint"}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{w.url}</p>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={w.enabled}
                    disabled={update.isPending}
                    onCheckedChange={(v) => update.mutate({ id: w.id, enabled: v }, { onError: (e) => toast.error(errorMessage(e)) })}
                    aria-label="Enabled"
                  />
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button variant="ghost" size="icon" aria-label="Actions" />}>
                      <MoreHorizontal className="size-4" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => onEdit(w)}>Edit</DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() =>
                          test.mutate(w.id, { onSuccess: () => toast.success("Test event queued. Check the deliveries tab."), onError: (e) => toast.error(errorMessage(e)) })
                        }
                      >
                        Send test event
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => rotateSecret.mutate(w.id, { onSuccess: (r) => setRotated(r.secret), onError: (e) => toast.error(errorMessage(e)) })}>
                        Rotate secret
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => setDeleting(w)}>
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
              <div className="flex flex-wrap gap-1">
                {w.events.map((e) => (
                  <Badge key={e} variant="outline" className="font-mono text-[11px]">
                    {e}
                  </Badge>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                {formatCount(w.success_count)} delivered · {formatCount(w.failure_count)} failed
                {w.last_success_at ? ` · last success ${relativeTime(w.last_success_at)}` : ""}
                {w.last_failure_at ? ` · last failure ${relativeTime(w.last_failure_at)}` : ""}
              </p>
              {!w.enabled && w.disabled_reason ? <p className="text-sm text-amber-700 dark:text-amber-300">{w.disabled_reason}</p> : null}
            </CardContent>
          </Card>
        ))}
      </div>

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
            <DialogDescription>Deliveries stop immediately. Its history stays in the deliveries tab.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
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
        <Select value={webhook} onValueChange={(v) => setWebhook(v ?? "all")}>
          <SelectTrigger className="w-48">
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
        <Select value={status} onValueChange={(v) => setStatus(v ?? "all")}>
          <SelectTrigger className="w-36">
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
        <Select value={event} onValueChange={(v) => setEvent(v ?? "all")}>
          <SelectTrigger className="w-48">
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
      ) : rows.length === 0 ? (
        <EmptyState title="No deliveries yet" description="Events appear here as they are sent to your endpoints." />
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border">
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
                  <TableRow key={d.id} className="cursor-pointer" onClick={() => setSelected(d)}>
                    <TableCell className="font-mono text-xs">{d.event}</TableCell>
                    <TableCell className="max-w-xs truncate">{names.get(d.webhook_id) ?? d.url}</TableCell>
                    <TableCell>
                      <StatusBadge status={d.status} label={deliveryStatusLabel[d.status] ?? d.status} />
                    </TableCell>
                    <TableCell className="tabular-nums">{d.http_status ?? "—"}</TableCell>
                    <TableCell className="tabular-nums">
                      {d.attempt_count}
                      {d.next_attempt_at && d.status === "retrying" ? <span className="text-muted-foreground"> · next {relativeTime(d.next_attempt_at)}</span> : null}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-right text-muted-foreground">{relativeTime(d.created_at)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {deliveries.hasNextPage ? (
            <div className="mt-3 flex justify-center">
              <Button variant="outline" onClick={() => deliveries.fetchNextPage()} disabled={deliveries.isFetchingNextPage}>
                Load more
              </Button>
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
                  {absoluteTime(selected.created_at)} · {deliveryStatusLabel[selected.status] ?? selected.status}
                  {selected.http_status ? ` · HTTP ${selected.http_status}` : ""} · {selected.attempt_count} attempt{selected.attempt_count === 1 ? "" : "s"}
                </DialogDescription>
              </DialogHeader>
              {selected.error ? <p className="text-sm text-destructive">{selected.error}</p> : null}
              <p className="text-sm font-medium">Payload</p>
              <pre className="max-h-72 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs">{JSON.stringify(selected.payload, null, 2)}</pre>
              {selected.response_excerpt ? (
                <>
                  <p className="text-sm font-medium">Response</p>
                  <pre className="max-h-40 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs">{selected.response_excerpt}</pre>
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
        actions={
          <Button onClick={() => setDialog({ open: true, existing: null })} className="gap-1.5">
            <Plus className="size-4" />
            Add endpoint
          </Button>
        }
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
