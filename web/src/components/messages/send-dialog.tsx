"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "@/components/field";
import { errorMessage, isApiError } from "@/lib/api";
import { useDevices, useSendMessage } from "@/lib/queries";

/** Rough segment count: GSM-7 gives 160 (153 concatenated), anything else 70 (67). */
function segments(body: string): { count: number; unicode: boolean } {
  const unicode = /[^\x00-\x7F]/.test(body);
  const single = unicode ? 70 : 160;
  const multi = unicode ? 67 : 153;
  if (body.length === 0) return { count: 0, unicode };
  return { count: body.length <= single ? 1 : Math.ceil(body.length / multi), unicode };
}

function parseRecipients(raw: string): string[] {
  return raw
    .split(/[\n,;]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function SendDialog({ open, onOpenChange, initialTo = "" }: { open: boolean; onOpenChange: (open: boolean) => void; initialTo?: string }) {
  const router = useRouter();
  const devices = useDevices();
  const send = useSendMessage();
  const [to, setTo] = useState(initialTo);
  const [body, setBody] = useState("");
  const [device, setDevice] = useState("auto");
  const [scheduled, setScheduled] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const recipients = useMemo(() => parseRecipients(to), [to]);
  const seg = segments(body);
  const list = devices.data?.data ?? [];
  const enabledDevices = list.filter((d) => d.enabled);

  const reset = () => {
    setTo(initialTo);
    setBody("");
    setDevice("auto");
    setScheduled("");
    setFieldErrors({});
    send.reset();
  };

  const submit = () => {
    setFieldErrors({});
    send.mutate(
      {
        to: recipients,
        body,
        device_id: device === "auto" ? undefined : device,
        scheduled_at: scheduled ? new Date(scheduled).toISOString() : undefined,
      },
      {
        onSuccess: (res) => {
          toast.success(`Queued ${res.batch.recipient_count} message${res.batch.recipient_count === 1 ? "" : "s"}.`, {
            action: { label: "Follow", onClick: () => router.push(`/sends/${res.batch.id}`) },
          });
          onOpenChange(false);
          reset();
        },
        onError: (e) => {
          if (isApiError(e)) setFieldErrors(e.fieldMessages());
        },
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) reset();
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Send a message</DialogTitle>
          <DialogDescription>Each recipient counts as one message against your plan.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Field
            label="Recipients"
            htmlFor="to"
            error={fieldErrors.to}
            hint={`${recipients.length} number${recipients.length === 1 ? "" : "s"}. One per line, or separated by commas.`}
          >
            <Textarea id="to" rows={3} placeholder={"+14155550123\n+447700900123"} value={to} onChange={(e) => setTo(e.target.value)} className="font-mono" />
          </Field>
          <Field
            label="Message"
            htmlFor="body"
            error={fieldErrors.body}
            hint={`${body.length} characters · ${seg.count} segment${seg.count === 1 ? "" : "s"}${seg.unicode ? " (non-Latin characters shorten segments)" : ""}`}
          >
            <Textarea id="body" rows={5} maxLength={1600} value={body} onChange={(e) => setBody(e.target.value)} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Send from" htmlFor="device">
              <Select
                value={device}
                onValueChange={(v) => setDevice(v ?? "auto")}
                items={{ auto: "Default phone", ...Object.fromEntries(enabledDevices.map((d) => [d.id, d.name + (d.online ? "" : " (offline)")])) }}
              >
                <SelectTrigger id="device">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">Default phone</SelectItem>
                  {enabledDevices.map((d) => (
                    <SelectItem key={d.id} value={d.id}>
                      {d.name}
                      {d.online ? "" : " (offline)"}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Schedule (optional)" htmlFor="when" error={fieldErrors.scheduled_at}>
              <Input id="when" type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
            </Field>
          </div>
          {enabledDevices.length === 0 && !devices.isPending ? (
            <p className="text-sm text-destructive">
              No enabled phone.{" "}
              <Link href="/devices" className="underline">
                Pair a phone
              </Link>{" "}
              first.
            </p>
          ) : null}
          {send.isError && Object.keys(fieldErrors).length === 0 ? <p className="text-sm text-destructive">{errorMessage(send.error)}</p> : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={send.isPending || recipients.length === 0 || body.trim().length === 0}>
            {send.isPending ? "Queuing…" : recipients.length > 1 ? `Send to ${recipients.length}` : "Send"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
