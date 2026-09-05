"use client";

import { useState } from "react";
import Link from "next/link";
import { Plus, Smartphone } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, PageHeader } from "@/components/page-header";
import { OnlineDot } from "@/components/status-badge";
import { PairDialog } from "@/components/devices/pair-dialog";
import { formatCount, relativeTime } from "@/lib/format";
import { useDevices } from "@/lib/queries";

export default function DevicesPage() {
  const devices = useDevices(15_000);
  const [pairing, setPairing] = useState(false);
  const list = devices.data?.data ?? [];

  return (
    <>
      <PageHeader
        title="Devices"
        description="Phones paired with this account. Sends go out from the default device unless a request names one."
        actions={
          <Button onClick={() => setPairing(true)} className="gap-1.5">
            <Plus className="size-4" />
            Pair a phone
          </Button>
        }
      />
      <PairDialog open={pairing} onOpenChange={setPairing} />

      {devices.isPending ? (
        <div className="grid gap-3">
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState
          title="No phone paired yet"
          description="Install the app on an Android phone with a SIM, then pair it with a code from here."
          action={<Button onClick={() => setPairing(true)}>Pair a phone</Button>}
        />
      ) : (
        <div className="grid gap-3">
          {list.map((d) => (
            <Card key={d.id} className="transition-colors hover:bg-muted/40">
              <CardContent className="flex flex-wrap items-center gap-4 py-4">
                <span className="grid size-10 shrink-0 place-items-center rounded-md bg-muted">
                  <Smartphone className="size-5 text-muted-foreground" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Link href={`/devices/${d.id}`} className="font-medium hover:underline">
                      {d.name}
                    </Link>
                    {d.is_default ? <Badge variant="secondary">Default</Badge> : null}
                    {!d.enabled ? <Badge variant="outline">Disabled</Badge> : null}
                    {d.push_token_invalidated_at ? <Badge variant="destructive">Needs the app opened</Badge> : null}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {[d.brand, d.model].filter(Boolean).join(" ")}
                    {d.os_version ? ` · Android ${d.os_version}` : ""} · last check-in {relativeTime(d.last_heartbeat_at)}
                  </p>
                </div>
                <div className="flex items-center gap-6 text-sm">
                  <div className="text-right">
                    <p className="tabular-nums">{formatCount(d.sent_count)} sent</p>
                    <p className="tabular-nums text-muted-foreground">{formatCount(d.received_count)} received</p>
                  </div>
                  <OnlineDot online={d.online} />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </>
  );
}
