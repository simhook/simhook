"use client";

import { useEffect, useMemo, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Check, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { CopyButton } from "@/components/copy-button";
import { errorMessage } from "@/lib/api";
import { useDeviceMutations, useDevices } from "@/lib/queries";

function useCountdown(until: string | undefined) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!until) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [until]);
  return until ? Math.max(0, Math.round((new Date(until).getTime() - now) / 1000)) : 0;
}

function PairFlow({ onDone }: { onDone: () => void }) {
  const { createPairingCode } = useDeviceMutations();
  const devices = useDevices(3_000);
  // Snapshot the devices that existed when the dialog opened; a device
  // outside that set (or created after opening) means the phone paired.
  const [openedAt] = useState(() => Date.now());
  const [snapshot] = useState(() => (devices.data ? new Set(devices.data.data.map((d) => d.id)) : null));
  const code = createPairingCode.data;
  const secondsLeft = useCountdown(code?.expires_at);

  useEffect(() => {
    createPairingCode.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const paired = devices.data?.data.find((d) => (snapshot ? !snapshot.has(d.id) : new Date(d.created_at).getTime() > openedAt));
  const minutes = useMemo(() => `${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, "0")}`, [secondsLeft]);

  if (paired) {
    return (
      <div className="flex flex-col items-center gap-3 py-6 text-center">
        <span className="grid size-12 place-items-center rounded-full bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
          <Check className="size-6" />
        </span>
        <p className="font-medium">{paired.name} is paired</p>
        <p className="text-sm text-muted-foreground">Sends that name no device now go out from it.</p>
        <Button onClick={onDone}>Done</Button>
      </div>
    );
  }
  if (createPairingCode.isError) {
    return (
      <div className="grid gap-3 py-4">
        <p className="text-sm text-destructive">{errorMessage(createPairingCode.error)}</p>
        <Button variant="outline" onClick={() => createPairingCode.mutate()}>
          Try again
        </Button>
      </div>
    );
  }
  if (!code) {
    return <p className="py-6 text-center text-sm text-muted-foreground">Creating a code…</p>;
  }
  return (
    <div className="grid gap-4">
      <div className="mx-auto rounded-lg border bg-white p-3">
        <QRCodeSVG value={code.pair_url} size={208} level="M" />
      </div>
      <div className="text-center">
        <p className="font-mono text-3xl tracking-[0.2em]">{code.code}</p>
        <p className="mt-1 text-sm text-muted-foreground">{secondsLeft > 0 ? `Expires in ${minutes}` : "This code expired"}</p>
      </div>
      <div className="flex justify-center gap-2">
        <CopyButton value={code.code} label="Copy code" />
        <Button variant="outline" size="sm" className="gap-1.5" onClick={() => createPairingCode.mutate()} disabled={createPairingCode.isPending}>
          <RefreshCw className="size-3.5" />
          New code
        </Button>
      </div>
      <p className="text-center text-xs text-muted-foreground">Waiting for the phone…</p>
    </div>
  );
}

export function PairDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Pair a phone</DialogTitle>
          <DialogDescription>
            Install the simhook app on an Android phone, then scan this code or type it in. It works once and expires in ten minutes.
          </DialogDescription>
        </DialogHeader>
        {open ? <PairFlow onDone={() => onOpenChange(false)} /> : null}
      </DialogContent>
    </Dialog>
  );
}
