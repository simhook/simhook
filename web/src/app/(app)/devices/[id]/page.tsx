"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";
import type { Device } from "@simhook/contracts";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Field } from "@/components/field";
import { LoadError, textLink } from "@/components/page-header";
import { OnlineDot } from "@/components/status-badge";
import { errorMessage } from "@/lib/api";
import { absoluteTime, formatCount, relativeTime } from "@/lib/format";
import { useDevice, useDeviceMutations } from "@/lib/queries";

type Telemetry = {
  battery_percent?: number;
  charging?: boolean;
  network?: string;
  uptime_ms?: number;
  timezone?: string;
  locale?: string;
  storage_free_bytes?: number;
  keep_alive?: boolean;
  outbox_pending?: number;
};

type Sim = { subscription_id: number; slot: number; carrier?: string; display_name?: string; country?: string };

const INTERVALS = [15, 20, 30, 60, 120, 240];

function gb(bytes?: number) {
  return bytes === undefined ? "" : `${(bytes / 1e9).toFixed(1)} GB free`;
}

function uptime(ms?: number) {
  if (!ms) return "";
  const h = Math.floor(ms / 3600000);
  return h >= 48 ? `${Math.floor(h / 24)} days` : `${h} h`;
}

/** The editable fields, initialized from the server record it is keyed on. */
function SettingsFields({ d, sims, busy, onSave }: { d: Device; sims: Sim[]; busy: boolean; onSave: (body: Record<string, unknown>) => void }) {
  const [name, setName] = useState(d.name);
  const [delay, setDelay] = useState(String(d.send_delay_seconds));
  const [interval, setInterval_] = useState(String(d.heartbeat_interval_minutes));
  const [sim, setSim] = useState(d.preferred_sim_subscription_id == null ? "default" : String(d.preferred_sim_subscription_id));
  // A value set through the API that is not one of ours is still shown.
  const intervals = INTERVALS.includes(d.heartbeat_interval_minutes) ? INTERVALS : [...INTERVALS, d.heartbeat_interval_minutes].sort((a, b) => a - b);

  const dirty =
    name.trim() !== d.name ||
    Number(delay) !== d.send_delay_seconds ||
    Number(interval) !== d.heartbeat_interval_minutes ||
    (sim === "default" ? d.preferred_sim_subscription_id != null : Number(sim) !== d.preferred_sim_subscription_id);

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name" htmlFor="name">
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} maxLength={64} />
        </Field>
        <Field label="Delay between sends (seconds)" htmlFor="delay" hint="A pause after each message keeps the carrier from flagging the SIM.">
          <Input id="delay" type="number" min={0} max={3600} value={delay} onChange={(e) => setDelay(e.target.value)} />
        </Field>
        <Field label="Check-in interval" htmlFor="interval">
          <Select value={interval} onValueChange={(v) => setInterval_(v ?? "20")}>
            <SelectTrigger id="interval">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {intervals.map((m) => (
                <SelectItem key={m} value={String(m)}>
                  Every {m} minutes
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field label="Preferred SIM" htmlFor="sim" hint="Used when a send does not name a SIM.">
          <Select value={sim} onValueChange={(v) => setSim(v ?? "default")}>
            <SelectTrigger id="sim">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="default">Phone default</SelectItem>
              {sims.map((s) => (
                <SelectItem key={s.subscription_id} value={String(s.subscription_id)}>
                  {s.display_name || s.carrier || `SIM ${s.slot + 1}`}, id {s.subscription_id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>
      <div>
        <Button
          disabled={!dirty || busy}
          onClick={() =>
            onSave({
              name: name.trim() || undefined,
              send_delay_seconds: Number(delay),
              heartbeat_interval_minutes: Number(interval),
              ...(sim === "default" ? { clear_preferred_sim: true } : { preferred_sim_subscription_id: Number(sim) }),
            })
          }
        >
          Save changes
        </Button>
      </div>
    </>
  );
}

export default function DevicePage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const device = useDevice(id);
  const { update, setDefault, unpair } = useDeviceMutations();
  const d = device.data?.device;
  const [confirmUnpair, setConfirmUnpair] = useState(false);

  if (device.isPending) {
    return (
      <div className="mt-12 grid gap-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40" />
      </div>
    );
  }
  if (device.isError || !d) {
    return (
      <div className="mt-12">
        {device.isError ? <LoadError error={device.error} retry={() => device.refetch()} /> : <p className="text-sm">No such phone.</p>}
        <Link href="/devices" className={`${textLink} mt-4 inline-block`}>
          Back to phones
        </Link>
      </div>
    );
  }

  const telemetry = (d.telemetry ?? {}) as Telemetry;
  const sims = (Array.isArray(d.sims) ? d.sims : []) as Sim[];
  const busy = update.isPending;
  const about = [[d.brand, d.model].filter(Boolean).join(" "), d.os_version ? `Android ${d.os_version}` : "", d.app_version_name ? `app ${d.app_version_name}` : ""].filter(Boolean);

  const save = (body: Omit<Parameters<typeof update.mutate>[0], "id">) =>
    update.mutate(
      { id: d.id, ...body },
      {
        onSuccess: () => toast.success("Saved. The phone is told at once."),
        onError: (e) => toast.error(errorMessage(e)),
      },
    );

  return (
    <>
      <Link href="/devices" className={`${textLink} mt-8 inline-block text-muted-foreground`}>
        Back to phones
      </Link>
      <div className="mb-6 mt-4 flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-baseline gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">{d.name}</h1>
            {d.is_default ? <span className="font-mono text-[11px] text-muted-foreground">default</span> : null}
            {!d.enabled ? <span className="font-mono text-[11px] text-muted-foreground">disabled</span> : null}
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            <OnlineDot online={d.online} />
            {[`, last check-in ${relativeTime(d.last_heartbeat_at)}`, ...about.map((a) => `, ${a}`)].join("")}
          </p>
        </div>
        <div className="flex items-center gap-5">
          {!d.is_default ? (
            <button
              type="button"
              className={textLink}
              onClick={() => setDefault.mutate(d.id, { onSuccess: () => toast.success("This is now the default phone."), onError: (e) => toast.error(errorMessage(e)) })}
              disabled={setDefault.isPending}
            >
              Make default
            </button>
          ) : null}
          <Button variant="destructive" onClick={() => setConfirmUnpair(true)}>
            Unpair
          </Button>
        </div>
      </div>

      {d.push_token_invalidated_at ? (
        <Alert variant="destructive" className="mb-6">
          <AlertTitle>The phone cannot be reached by push.</AlertTitle>
          <AlertDescription>{d.push_token_invalid_reason ?? "Its push registration is no longer valid."} Open the app on the phone to reconnect it.</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>settings</CardTitle>
            <CardDescription>Switches apply at once. The other fields save together.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-5">
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium">Enabled</p>
                <p className="text-sm text-muted-foreground">When off, nothing is sent from this phone; it still forwards what it receives.</p>
              </div>
              <Switch checked={d.enabled} disabled={busy} onCheckedChange={(v) => save({ enabled: v })} aria-label="Enabled" />
            </div>
            <div className="flex items-center justify-between gap-4">
              <div>
                <p className="text-sm font-medium">Forward incoming SMS</p>
                <p className="text-sm text-muted-foreground">Messages this phone receives are stored and sent to your webhooks.</p>
              </div>
              <Switch checked={d.receive_enabled} disabled={busy} onCheckedChange={(v) => save({ receive_enabled: v })} aria-label="Forward incoming SMS" />
            </div>
            <SettingsFields key={d.updated_at} d={d} sims={sims} busy={busy} onSave={save} />
          </CardContent>
        </Card>

        <div className="grid gap-6">
          <Card>
            <CardHeader>
              <CardTitle>activity</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-3 text-sm">
              <div>
                <p className="text-muted-foreground">Sent</p>
                <p className="text-xl tabular-nums">{formatCount(d.sent_count)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Received</p>
                <p className="text-xl tabular-nums">{formatCount(d.received_count)}</p>
              </div>
              <div className="col-span-2 text-muted-foreground">Paired {absoluteTime(d.created_at)}</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>phone state</CardTitle>
              <CardDescription>From the last check-in.</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-1.5 text-sm">
              {telemetry.battery_percent !== undefined ? (
                <p>
                  Battery {telemetry.battery_percent}%{telemetry.charging ? ", charging" : ""}
                </p>
              ) : null}
              {telemetry.network ? <p>Network: {telemetry.network}</p> : null}
              {telemetry.storage_free_bytes !== undefined ? <p>Storage: {gb(telemetry.storage_free_bytes)}</p> : null}
              {telemetry.uptime_ms ? <p>Uptime: {uptime(telemetry.uptime_ms)}</p> : null}
              {telemetry.timezone ? <p>Time zone: {telemetry.timezone}</p> : null}
              {telemetry.outbox_pending ? <p>{telemetry.outbox_pending} waiting in the phone&apos;s queue</p> : null}
              {telemetry.keep_alive ? <p>Keep-alive notification on</p> : null}
              {Object.keys(telemetry).length === 0 ? <p className="text-muted-foreground">No check-in yet.</p> : null}
            </CardContent>
          </Card>
        </div>
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>sim cards</CardTitle>
          <CardDescription>Pass a SIM id as sim_subscription_id in a send to use that SIM.</CardDescription>
        </CardHeader>
        <CardContent>
          {sims.length === 0 ? (
            <p className="text-sm text-muted-foreground">No SIM reported. The phone needs the phone-state permission to list them.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Slot</TableHead>
                  <TableHead>Carrier</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Country</TableHead>
                  <TableHead className="text-right">SIM id</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sims.map((s) => (
                  <TableRow key={s.subscription_id}>
                    <TableCell>{s.slot + 1}</TableCell>
                    <TableCell>{s.carrier ?? ""}</TableCell>
                    <TableCell>{s.display_name ?? ""}</TableCell>
                    <TableCell className="uppercase">{s.country ?? ""}</TableCell>
                    <TableCell className="text-right font-mono">{s.subscription_id}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Dialog open={confirmUnpair} onOpenChange={setConfirmUnpair}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Unpair {d.name}?</DialogTitle>
            <DialogDescription>The phone stops receiving sends and its credentials are revoked. Message history stays.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmUnpair(false)}>
              Cancel
            </Button>
            <Button
              disabled={unpair.isPending}
              onClick={() =>
                unpair.mutate(d.id, {
                  onSuccess: () => {
                    toast.success("Phone unpaired.");
                    router.replace("/devices");
                  },
                  onError: (e) => toast.error(errorMessage(e)),
                })
              }
            >
              Unpair
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
