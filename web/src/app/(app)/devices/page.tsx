"use client";

import { useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, LoadError, PageHeader, textLink } from "@/components/page-header";
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
      ) : devices.isError ? (
        <LoadError error={devices.error} retry={() => devices.refetch()} />
      ) : list.length === 0 ? (
        <EmptyState
          title="No phone paired yet"
          description="Install the app on an Android phone with a SIM, then pair it with a code from here."
          action={
            <button type="button" className={textLink} onClick={() => setPairing(true)}>
              Pair a phone
            </button>
          }
        />
      ) : (
        <ul className="border-t">
          {list.map((d) => {
            const about = [[d.brand, d.model].filter(Boolean).join(" "), d.os_version ? `Android ${d.os_version}` : ""].filter(Boolean);
            return (
              <li key={d.id} className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b py-3.5">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-3">
                    <Link href={`/devices/${d.id}`} className="font-medium hover:underline">
                      {d.name}
                    </Link>
                    {d.is_default ? <span className="font-mono text-[11px] text-muted-foreground">default</span> : null}
                    {!d.enabled ? <span className="font-mono text-[11px] text-muted-foreground">disabled</span> : null}
                    {d.push_token_invalidated_at ? <span className="font-mono text-[11px] text-destructive">needs the app opened</span> : null}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {[...about, `last check-in ${relativeTime(d.last_heartbeat_at)}`].join(", ")}
                  </p>
                </div>
                <p className="font-mono text-xs text-muted-foreground tabular-nums">
                  {formatCount(d.sent_count)} sent, {formatCount(d.received_count)} received
                </p>
                <OnlineDot online={d.online} />
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
