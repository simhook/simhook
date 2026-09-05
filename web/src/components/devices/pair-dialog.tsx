"use client";

import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { QRCodeSVG } from "qrcode.react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { CopyButton } from "@/components/copy-button";
import { textLink } from "@/components/page-header";
import { errorMessage } from "@/lib/api";
import { keys, usePairingCode, usePairingCodeMutation } from "@/lib/queries";

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
  const qc = useQueryClient();
  const createPairingCode = usePairingCodeMutation();
  const code = createPairingCode.data;
  // The code itself says when a phone has used it; nothing is inferred
  // from the device list.
  const status = usePairingCode(code?.id);
  const secondsLeft = useCountdown(code?.expires_at);
  const paired = status.data?.consumed ? status.data.device : null;

  useEffect(() => {
    createPairingCode.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!paired) return;
    qc.invalidateQueries({ queryKey: keys.devices });
    qc.invalidateQueries({ queryKey: keys.stats });
  }, [paired, qc]);

  const minutes = useMemo(() => `${Math.floor(secondsLeft / 60)}:${String(secondsLeft % 60).padStart(2, "0")}`, [secondsLeft]);

  if (paired) {
    return (
      <div className="grid gap-3 py-2">
        <p className="inline-flex items-center gap-2 font-medium">
          <span className="size-[7px] rounded-full bg-ok" />
          {paired.name} is paired
        </p>
        <p className="text-sm text-muted-foreground">Sends that name no phone now go out from it.</p>
        <div>
          <Button onClick={onDone}>Done</Button>
        </div>
      </div>
    );
  }
  if (createPairingCode.isError) {
    return (
      <div className="grid gap-3 py-2">
        <p className="text-sm text-destructive">{errorMessage(createPairingCode.error)}</p>
        <div>
          <button type="button" className={textLink} onClick={() => createPairingCode.mutate()}>
            Try again
          </button>
        </div>
      </div>
    );
  }
  if (!code) {
    return <p className="py-6 text-center text-sm text-muted-foreground">Creating a code…</p>;
  }
  return (
    <div className="grid gap-4">
      <div className="mx-auto border p-3">
        <QRCodeSVG value={code.pair_url} size={208} level="M" />
      </div>
      <div className="text-center">
        <p className="font-mono text-3xl tracking-[0.2em]">{code.code}</p>
        <p className="mt-1 text-sm text-muted-foreground">{secondsLeft > 0 ? `Expires in ${minutes}` : "This code expired"}</p>
      </div>
      <div className="flex justify-center gap-5">
        <CopyButton value={code.code} label="Copy code" />
        <button type="button" className={textLink} onClick={() => createPairingCode.mutate()} disabled={createPairingCode.isPending}>
          New code
        </button>
      </div>
      <p className="text-center text-xs text-muted-foreground">
        {status.isError ? errorMessage(status.error) : "Waiting for the phone…"}
      </p>
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
