"use client";

import { useState } from "react";
import type { Webhook } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Field } from "@/components/field";
import { CopyField } from "@/components/copy-button";
import { errorMessage, isApiError } from "@/lib/api";
import { useWebhookMutations } from "@/lib/queries";
import { cn } from "@/lib/utils";

export const EVENTS: { id: string; label: string; description: string }[] = [
  { id: "message.received", label: "Message received", description: "An SMS arrived on a phone with forwarding on." },
  { id: "message.sent", label: "Message sent", description: "The phone's radio accepted an outgoing message." },
  { id: "message.delivered", label: "Message delivered", description: "The carrier confirmed delivery." },
  { id: "message.failed", label: "Message failed", description: "Sending failed on the phone or was rejected." },
  { id: "message.unknown", label: "No result", description: "No report arrived within the wait window." },
  { id: "device.online", label: "Device online", description: "A phone checked in after being offline." },
  { id: "device.offline", label: "Device offline", description: "A phone stopped checking in." },
];

const DEFAULT_EVENTS = ["message.received", "message.sent", "message.delivered", "message.failed"];

function WebhookForm({ existing, onClose }: { existing: Webhook | null; onClose: () => void }) {
  const { create, update } = useWebhookMutations();
  const [name, setName] = useState(existing?.name ?? "");
  const [url, setUrl] = useState(existing?.url ?? "");
  const [events, setEvents] = useState<string[]>(existing?.events ?? DEFAULT_EVENTS);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [secret, setSecret] = useState<string | null>(null);

  const toggle = (id: string) => setEvents((cur) => (cur.includes(id) ? cur.filter((e) => e !== id) : [...cur, id]));
  const pending = create.isPending || update.isPending;

  const submit = () => {
    setErrors({});
    const onError = (e: unknown) => {
      if (isApiError(e)) setErrors(e.fieldMessages());
    };
    if (existing) {
      update.mutate({ id: existing.id, name: name.trim() || undefined, url: url.trim(), events }, { onSuccess: onClose, onError });
    } else {
      create.mutate({ name: name.trim() || undefined, url: url.trim(), events }, { onSuccess: (res) => setSecret(res.secret), onError });
    }
  };

  const err = create.error ?? update.error;

  if (secret) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>Endpoint added</DialogTitle>
          <DialogDescription>Copy the signing secret now. It is shown once.</DialogDescription>
        </DialogHeader>
        <CopyField value={secret} secret />
        <Alert>
          <AlertTitle>Verify every delivery</AlertTitle>
          <AlertDescription>
            Each request carries an <code className="font-mono">X-Simhook-Signature</code> header of the form <code className="font-mono">t=&lt;unix&gt;,v1=&lt;hex&gt;</code>. Compute
            HMAC-SHA256 with this secret over <code className="font-mono">&lt;t&gt;.&lt;raw body&gt;</code> and compare with v1. Reject timestamps older than five minutes.
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </>
    );
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>{existing ? "Edit endpoint" : "Add an endpoint"}</DialogTitle>
        <DialogDescription>We POST a JSON event to this URL and retry failures for a day.</DialogDescription>
      </DialogHeader>
      <div className="grid gap-4">
        <Field label="Name (optional)" htmlFor="wh-name" error={errors.name}>
          <Input id="wh-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={64} placeholder="Production" />
        </Field>
        <Field label="URL" htmlFor="wh-url" error={errors.url} hint="HTTPS, reachable from the internet.">
          <Input id="wh-url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/hooks/simhook" className="font-mono" />
        </Field>
        <div className="grid gap-1.5">
          <p className="text-sm font-medium">Events</p>
          <div className="grid gap-1">
            {EVENTS.map((ev) => {
              const on = events.includes(ev.id);
              return (
                <button
                  key={ev.id}
                  type="button"
                  onClick={() => toggle(ev.id)}
                  className={cn("flex items-start gap-3 rounded-md border px-3 py-2 text-left text-sm transition-colors", on ? "border-primary/50 bg-primary/5" : "hover:bg-muted")}
                >
                  <span className={cn("mt-0.5 grid size-4 shrink-0 place-items-center rounded border text-[10px]", on && "border-primary bg-primary text-primary-foreground")}>
                    {on ? "✓" : ""}
                  </span>
                  <span>
                    <span className="font-mono text-xs">{ev.id}</span>
                    <span className="block text-muted-foreground">{ev.description}</span>
                  </span>
                </button>
              );
            })}
          </div>
          {errors.events ? <p className="text-sm text-destructive">{errors.events}</p> : null}
        </div>
        {err && Object.keys(errors).length === 0 ? <p className="text-sm text-destructive">{errorMessage(err)}</p> : null}
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button onClick={submit} disabled={pending || !url.trim() || events.length === 0}>
          {pending ? "Saving…" : existing ? "Save" : "Add endpoint"}
        </Button>
      </DialogFooter>
    </>
  );
}

export function WebhookDialog({ open, onOpenChange, existing }: { open: boolean; onOpenChange: (o: boolean) => void; existing?: Webhook | null }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open ? <WebhookForm key={existing?.id ?? "new"} existing={existing ?? null} onClose={() => onOpenChange(false)} /> : null}
      </DialogContent>
    </Dialog>
  );
}
