"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import type { Plan } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { LoadError, PageHeader, textLink } from "@/components/page-header";
import { useAccount, useSession } from "@/components/session-provider";
import { errorMessage } from "@/lib/api";
import { absoluteDate, formatCount, limitLabel, priceLabel } from "@/lib/format";
import { useBilling, useBillingMutations, useCheckoutState, usePlans, type BillingStatus } from "@/lib/queries";

type Interval = "month" | "year";
type Sub = NonNullable<BillingStatus["subscription"]>;

const closedCopy: Record<string, string> = {
  closed: "Paid plans open soon. Every account is on Free until then.",
  region: "Paid plans are not offered in your country.",
  verify_email: "Verify your email address to choose a paid plan.",
};

/** "$12 a month" or "$120 a year". */
function priceWords(cents: number, interval: Interval) {
  return `${priceLabel(cents)} ${interval === "year" ? "a year" : "a month"}`;
}

function planPrice(p: Plan, interval: Interval) {
  return interval === "year" ? p.yearly_price_cents : p.monthly_price_cents;
}

/** Price per month at an interval: the measure of whether a change is up or down. */
function perMonth(p: Plan, interval: Interval) {
  return interval === "year" ? p.yearly_price_cents / 12 : p.monthly_price_cents;
}

function asInterval(v: string | undefined): Interval {
  return v === "year" ? "year" : "month";
}

function isLive(status: string | undefined) {
  return status === "active" || status === "trialing" || status === "past_due";
}

/** "10,000 messages a month, 3 phones, 500 recipients per send." */
function planFacts(p: Plan) {
  const phones = p.device_limit === 1 ? "phone" : "phones";
  return `${limitLabel(p.monthly_limit)} messages a month, ${limitLabel(p.device_limit)} ${phones}, ${limitLabel(p.batch_limit)} recipients per send.`;
}

/** Waits for a checkout the browser came back from, then says so. */
function CheckoutReturn({ id, onDone }: { id: string; onDone: () => void }) {
  const state = useCheckoutState(id);
  const session = useSession();
  const announced = useRef(false);
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setSlow(true), 45_000);
    return () => clearTimeout(t);
  }, []);
  useEffect(() => {
    const d = state.data;
    if (!d || announced.current) return;
    if (d.applied) {
      announced.current = true;
      toast.success("Your plan is active.");
      void session.refresh();
      onDone();
    } else if (d.status === "expired" || d.status === "failed") {
      announced.current = true;
      toast.error(d.status === "expired" ? "The checkout expired before it was paid." : "The payment did not go through.");
      onDone();
    }
  }, [state.data, session, onDone]);
  if (state.isError) {
    return (
      <p className="mb-6 border-l-2 border-destructive pl-4 text-sm">
        <span className="text-destructive">{errorMessage(state.error)}</span>{" "}
        <button type="button" className="underline" onClick={onDone}>
          Dismiss
        </button>
      </p>
    );
  }
  return (
    <p className="mb-6 border-l-2 border-foreground pl-4 text-sm" aria-busy={!slow}>
      <span className="font-medium">{slow ? "Still confirming." : "Confirming your payment."}</span>{" "}
      {slow ? "The plan updates on its own once Polar reports the payment." : "This takes a few seconds."}
      {slow ? (
        <>
          {" "}
          <button type="button" className="underline" onClick={onDone}>
            Dismiss
          </button>
        </>
      ) : null}
    </p>
  );
}

/** One sentence that says what the account has and what happens next. */
function statusSentence(sub: Sub | null, plans: Plan[]): string {
  if (!sub) return "Free.";
  const interval = asInterval(sub.interval);
  const head = sub.managed ? `${sub.plan_name}, ${priceWords(sub.price_cents, interval)}.` : `${sub.plan_name}, set up by hand.`;
  const when = absoluteDate(sub.current_period_end);
  if (!sub.managed) return head;
  if (sub.status === "past_due") return `${head} The last payment failed and Polar is retrying your card.`;
  if (sub.cancel_at_period_end) return `${head} Ends on ${when || "the period end"}. After that, Free.`;
  if (sub.pending) {
    const target = plans.find((p) => p.id === sub.pending?.plan_id);
    const pendingInterval = asInterval(sub.pending.interval);
    const price = target ? `, ${priceWords(planPrice(target, pendingInterval), pendingInterval)}` : "";
    return `${head} Until ${absoluteDate(sub.pending.at)}; from then, ${sub.pending.plan_name}${price}.`;
  }
  return when ? `${head} Renews on ${when}.` : head;
}

type Change = { plan: Plan; interval: Interval };

/** What a change is called and what it does, for the button and the confirmation. */
function describe(sub: Sub | null, plans: Plan[], change: Change) {
  const { plan, interval } = change;
  const current = sub ? plans.find((p) => p.id === sub.plan_id) : undefined;
  const currentInterval = asInterval(sub?.interval);
  const samePlan = !!sub && sub.plan_id === plan.id;
  const up = !sub || !current || perMonth(plan, interval) >= perMonth(current, currentInterval);
  const label = !sub ? "Upgrade" : samePlan ? (interval === "year" ? "Switch to yearly" : "Switch to monthly") : up ? "Upgrade" : "Downgrade";
  const title = !sub ? `Upgrade to ${plan.name}` : samePlan ? `Switch to ${interval === "year" ? "yearly" : "monthly"} billing` : `${label} to ${plan.name}`;
  const when = absoluteDate(sub?.current_period_end) || "the end of the current period";
  const currentName = current?.name ?? "your current plan";
  const price = priceWords(planPrice(plan, interval), interval);
  let timing: string;
  if (samePlan && up) timing = `Starts now. The unused part of your year is credited toward it.`;
  else if (samePlan) timing = `Starts on ${when}, when the current period ends. You keep paying monthly until then.`;
  else if (up) timing = `Starts now. You pay the difference for the rest of this period today, then ${price} from ${when}.`;
  else timing = `Starts on ${when}, when your ${currentName} period ends. You keep ${currentName} until then. Nothing is refunded.`;
  return { label, title, lines: [`${plan.name}: ${planFacts(plan)}`, `${price}.`, timing] };
}

function ConfirmDialog({
  open,
  title,
  lines,
  confirmLabel,
  keepLabel,
  busy,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  lines: string[];
  confirmLabel: string;
  keepLabel: string;
  busy: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{lines[0]}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-2 text-sm">
          {lines.slice(1).map((l) => (
            <p key={l}>{l}</p>
          ))}
        </div>
        <DialogFooter>
          <button type="button" className={textLink} disabled={busy} onClick={onClose}>
            {keepLabel}
          </button>
          <Button disabled={busy} onClick={onConfirm}>
            {busy ? "One moment" : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function BillingPage() {
  const router = useRouter();
  const params = useSearchParams();
  const checkoutID = params.get("checkout");
  const { limits, usage } = useAccount();
  const billing = useBilling();
  const plans = usePlans();
  const { checkout, change, cancel, resume, portal } = useBillingMutations();
  const [interval, setInterval] = useState<Interval>("month");
  const [confirming, setConfirming] = useState<Change | "cancel" | null>(null);
  const [buying, setBuying] = useState<string | null>(null);

  const status = billing.data;
  const sub = status?.subscription ?? null;
  const managed = !!sub?.managed && isLive(sub.status);
  const list = plans.data?.data ?? [];

  // The period follows the subscription until the reader changes it.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !sub?.interval) return;
    seeded.current = true;
    setInterval(asInterval(sub.interval));
  }, [sub?.interval]);

  const clearCheckout = () => router.replace("/billing");
  const busy = change.isPending || cancel.isPending || resume.isPending;

  const buy = (p: Plan) => {
    setBuying(p.id);
    checkout.mutate(
      { plan_id: p.id, interval },
      {
        onSuccess: (r) => window.location.assign(r.url),
        onError: (e) => {
          toast.error(errorMessage(e));
          setBuying(null);
        },
      },
    );
  };

  const applyChange = (c: Change) =>
    change.mutate(
      { plan_id: c.plan.id, interval: c.interval },
      {
        onSuccess: (r) => {
          const s = r.subscription;
          toast.success(s?.pending ? `${c.plan.name} starts on ${absoluteDate(s.pending.at)}.` : `You are on ${c.plan.name} now.`);
          setConfirming(null);
        },
        onError: (e) => toast.error(errorMessage(e)),
      },
    );

  const undoPending = () =>
    sub &&
    change.mutate(
      { plan_id: sub.plan_id, interval: asInterval(sub.interval) },
      {
        onSuccess: () => toast.success(`You stay on ${sub.plan_name}.`),
        onError: (e) => toast.error(errorMessage(e)),
      },
    );

  const applyCancel = () =>
    cancel.mutate(undefined, {
      onSuccess: (r) => {
        const when = absoluteDate(r.subscription?.current_period_end);
        toast.success(when ? `Your plan ends on ${when}.` : "Your plan ends at the end of this period.");
        setConfirming(null);
      },
      onError: (e) => toast.error(errorMessage(e)),
    });

  const openPortal = () =>
    portal.mutate(undefined, {
      onSuccess: (r) => window.location.assign(r.url),
      onError: (e) => toast.error(errorMessage(e)),
    });

  // What the action column shows for a plan, given the period chosen.
  const action = (p: Plan) => {
    const paid = p.monthly_price_cents > 0;
    const currentRow = sub ? sub.plan_id === p.id && (!sub.managed || asInterval(sub.interval) === interval) : !paid;
    if (currentRow) return <span className="text-muted-foreground">Current plan</span>;
    if (sub?.pending && sub.pending.plan_id === p.id && asInterval(sub.pending.interval) === interval) {
      return <span className="text-muted-foreground">Starts {absoluteDate(sub.pending.at)}</span>;
    }
    if (!paid) return null;
    if (managed) {
      if (sub?.cancel_at_period_end) return null;
      const d = describe(sub, list, { plan: p, interval });
      return (
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setConfirming({ plan: p, interval })}>
          {d.label}
        </Button>
      );
    }
    if (!status?.checkout.available) return null;
    return (
      <Button size="sm" disabled={buying !== null} onClick={() => buy(p)}>
        {buying === p.id ? "One moment" : "Upgrade"}
      </Button>
    );
  };

  const confirm = confirming && confirming !== "cancel" ? describe(sub, list, confirming) : null;
  const freePlan = list.find((p) => p.monthly_price_cents === 0);

  return (
    <>
      <PageHeader title="Billing" description="Your plan, invoices, and payment method." />
      {checkoutID ? <CheckoutReturn id={checkoutID} onDone={clearCheckout} /> : null}
      {billing.isPending || plans.isPending ? (
        <Skeleton className="h-40" />
      ) : billing.isError ? (
        <LoadError error={billing.error} retry={() => billing.refetch()} />
      ) : plans.isError ? (
        <LoadError error={plans.error} retry={() => plans.refetch()} />
      ) : status ? (
        <div className="grid gap-8 text-sm">
          <section className="grid gap-2 border-t border-border pt-4">
            <h2 className="font-mono text-xs tracking-wide text-muted-foreground">current plan</h2>
            <p>{statusSentence(sub, list)}</p>
            <p className="text-muted-foreground">
              {formatCount(usage.sent_this_month)} of {limitLabel(limits.monthly_limit)} messages used this month.
            </p>
            {!sub && !status.checkout.available ? <p className="text-muted-foreground">{closedCopy[status.checkout.reason ?? "closed"] ?? closedCopy.closed}</p> : null}
            <div className="flex flex-wrap items-center gap-5 pt-1">
              {managed && sub?.cancel_at_period_end ? (
                <button
                  type="button"
                  className={textLink}
                  disabled={busy}
                  onClick={() => resume.mutate(undefined, { onSuccess: () => toast.success("Your plan continues."), onError: (e) => toast.error(errorMessage(e)) })}
                >
                  Keep {sub.plan_name}
                </button>
              ) : null}
              {managed && !sub?.cancel_at_period_end && sub?.pending ? (
                <button type="button" className={textLink} disabled={busy} onClick={undoPending}>
                  Stay on {sub.plan_name}
                </button>
              ) : null}
              {managed && !sub?.cancel_at_period_end ? (
                <button type="button" className={textLink} disabled={busy} onClick={() => setConfirming("cancel")}>
                  Cancel plan
                </button>
              ) : null}
              {status.portal_available ? (
                <button type="button" className={textLink} disabled={portal.isPending} onClick={openPortal}>
                  {sub?.status === "past_due" ? "Update payment method" : "Invoices and payment method"}
                </button>
              ) : null}
            </div>
          </section>

          <section className="grid gap-3 border-t border-border pt-4">
            <h2 className="font-mono text-xs tracking-wide text-muted-foreground">plans</h2>
            <fieldset className="flex flex-wrap items-center gap-x-5 gap-y-2">
              <legend className="sr-only">Billing period</legend>
              <span className="text-muted-foreground">Billed</span>
              {(["month", "year"] as const).map((i) => (
                <label key={i} className="flex cursor-pointer items-center gap-2">
                  <input type="radio" name="interval" value={i} checked={interval === i} onChange={() => setInterval(i)} className="size-3.5 accent-foreground" />
                  {i === "month" ? "monthly" : "yearly, two months free"}
                </label>
              ))}
            </fieldset>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Plan</TableHead>
                    <TableHead>Messages a day</TableHead>
                    <TableHead>Messages a month</TableHead>
                    <TableHead>Recipients per send</TableHead>
                    <TableHead>Phones</TableHead>
                    <TableHead>Price</TableHead>
                    <TableHead className="text-right" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {list.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell className="font-medium">{p.name}</TableCell>
                      <TableCell>{p.daily_limit < 0 ? "no cap" : formatCount(p.daily_limit)}</TableCell>
                      <TableCell>{limitLabel(p.monthly_limit)}</TableCell>
                      <TableCell>{limitLabel(p.batch_limit)}</TableCell>
                      <TableCell>{limitLabel(p.device_limit)}</TableCell>
                      <TableCell>{p.monthly_price_cents > 0 ? priceWords(planPrice(p, interval), interval) : "$0"}</TableCell>
                      <TableCell className="text-right">{action(p)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {status.provider === "polar" ? (
              <p className="max-w-[72ch] text-muted-foreground">
                Payments are handled by Polar, our merchant of record; its name is on the card statement, and tax is added at checkout where it applies.
                Upgrades start right away and are charged for the rest of the period. Downgrades and cancellations take effect at the next renewal.
              </p>
            ) : null}
          </section>
        </div>
      ) : null}

      {confirm && confirming && confirming !== "cancel" ? (
        <ConfirmDialog
          open
          title={confirm.title}
          lines={confirm.lines}
          confirmLabel={confirm.title}
          keepLabel={`Keep ${sub?.plan_name ?? "current plan"}`}
          busy={change.isPending}
          onConfirm={() => applyChange(confirming)}
          onClose={() => setConfirming(null)}
        />
      ) : null}
      {confirming === "cancel" && sub ? (
        <ConfirmDialog
          open
          title={`Cancel ${sub.plan_name}?`}
          lines={[
            `${sub.plan_name} stays active until ${absoluteDate(sub.current_period_end) || "the end of the current period"}.`,
            freePlan ? `After that your account is on Free: ${planFacts(freePlan)}` : "After that your account is on Free.",
            "Nothing is refunded for the remaining time.",
          ]}
          confirmLabel="Cancel plan"
          keepLabel={`Keep ${sub.plan_name}`}
          busy={cancel.isPending}
          onConfirm={applyCancel}
          onClose={() => setConfirming(null)}
        />
      ) : null}
    </>
  );
}

export default function Page() {
  return (
    <Suspense fallback={<Skeleton className="mt-12 h-40" />}>
      <BillingPage />
    </Suspense>
  );
}
