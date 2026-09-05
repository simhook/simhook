"use client";

import { useState } from "react";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
        title="Phones"
        description="Phones paired with this account. Sends go out from the default phone unless a request names one."
        actions={<Button onClick={() => setPairing(true)}>Pair a phone</Button>}
      />
      <PairDialog open={pairing} onOpenChange={setPairing} />

      {devices.isPending ? (
        <div className="grid gap-3">
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      ) : list.length === 0 ? (
        <EmptyState
          title="No phone paired yet"
          description="Install the app on an Android phone with a SIM, then pair it with a code from here."
          action={<Button onClick={() => setPairing(true)}>Pair a phone</Button>}
        />
      ) : (
        <ul className="border-t">
          {list.map((d) => (
            <li key={d.id} className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b py-3.5">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Link href={`/devices/${d.id}`} className="font-medium hover:underline">
                    {d.name}
                  </Link>
                  {d.is_default ? <Badge variant="secondary">default</Badge> : null}
                  {!d.enabled ? <Badge variant="outline">disabled</Badge> : null}
                  {d.push_token_invalidated_at ? <Badge variant="destructive">needs the app opened</Badge> : null}
                </div>
                <p className="text-sm text-muted-foreground">
                  {[d.brand, d.model].filter(Boolean).join(" ")}
                  {d.os_version ? `, Android ${d.os_version}` : ""}, last check-in {relativeTime(d.last_heartbeat_at)}
                </p>
              </div>
              <p className="font-mono text-xs text-muted-foreground tabular-nums">
                {formatCount(d.sent_count)} sent, {formatCount(d.received_count)} received
              </p>
              <OnlineDot online={d.online} />
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
