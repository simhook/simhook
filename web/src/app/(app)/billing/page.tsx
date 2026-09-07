"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import type { Plan } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { LoadError, PageHeader, textLink } from "@/components/page-header";
import { useAccount, useSession } from "@/components/session-provider";
import { errorMessage } from "@/lib/api";
import { absoluteTime, formatCount, limitLabel, priceLabel } from "@/lib/format";
import { useBilling, useBillingMutations, useCheckoutState, usePlans, type BillingStatus } from "@/lib/queries";

type Interval = "month" | "year";

/** The reasons checkout is unavailable, in the dashboard's words. */
const closedCopy: Record<string, string> = {
  closed: "Paid plans open soon. Every account is on Free until then.",
  region: "Paid plans are not offered in your country.",
  verify_email: "Verify your email address to choose a paid plan.",
};

function intervalWord(interval: string | undefined) {
  return interval === "year" ? "yearly" : "monthly";
}

/** Waits for a checkout the browser came back from, then says so. */
function CheckoutReturn({ id, onDone }: { id: string; onDone: () => void }) {
  const state = useCheckoutState(id);
  const session = useSession();
  const announced = useRef(false);
  // After a while the wait is no longer worth a spinner: the webhook
  // finishes the job on its own, and the reader may go on.
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
  if (slow) {
    return (
      <p className="mb-6 border-l-2 border-foreground pl-4 text-sm">
        <span className="font-medium">Still confirming.</span> The plan updates on its own once Polar reports the payment.{" "}
        <button type="button" className="underline" onClick={onDone}>
          Dismiss
        </button>
      </p>
    );
  }
  return (
    <p className="mb-6 border-l-2 border-foreground pl-4 text-sm" aria-busy="true">
      <span className="font-medium">Confirming your payment.</span> This takes a few seconds.
    </p>
  );
}

function PlanLine({ status }: { status: BillingStatus }) {
  const { limits, usage } = useAccount();
  const sub = status.subscription;
  const used = `${formatCount(usage.sent_this_month)} of ${limitLabel(limits.monthly_limit)} messages used this month.`;
  if (!sub) {
    return <CardDescription>You are on Free. {used}</CardDescription>;
  }
  const when = sub.current_period_end ? absoluteTime(sub.current_period_end) : "";
  let ends: string;
  if (sub.status === "past_due") ends = "The last payment failed; the card is being retried.";
  else if (sub.cancel_at_period_end) ends = when ? `It ends on ${when}.` : "It ends at the end of this period.";
  else if (sub.pending) ends = `It changes to ${sub.pending.plan_name}, ${intervalWord(sub.pending.interval)}, on ${absoluteTime(sub.pending.at)}.`;
  else if (!sub.managed) ends = "It was set up by hand.";
  else ends = when ? `It renews on ${when} for ${priceLabel(sub.price_cents)}.` : "";
  return (
    <CardDescription>
      You are on {sub.plan_name}, {intervalWord(sub.interval)}. {ends} {used}
    </CardDescription>
  );
}

function PlansTable({ status, interval }: { status: BillingStatus; interval: Interval }) {
  const { limits } = useAccount();
  const plans = usePlans();
  const { checkout, change } = useBillingMutations();
  const sub = status.subscription;
  const managed = !!sub?.managed && (sub.status === "active" || sub.status === "trialing" || sub.status === "past_due");
  const [busy, setBusy] = useState<string | null>(null);

  const price = (p: Plan) => (interval === "year" ? p.yearly_price_cents : p.monthly_price_cents);
  const isCurrent = (p: Plan) => p.id === limits.plan_id && (!sub || !sub.managed || sub.interval === interval || p.monthly_price_cents === 0);

  const choose = (p: Plan) => {
    setBusy(p.id);
    if (managed) {
      change.mutate(
        { plan_id: p.id, interval },
        {
          onSuccess: (r) => {
            const s = r.subscription;
            toast.success(s?.pending ? `${p.name} starts on ${absoluteTime(s.pending.at)}.` : `You are on ${p.name} now.`);
          },
          onError: (e) => toast.error(errorMessage(e)),
          onSettled: () => setBusy(null),
        },
      );
      return;
    }
    checkout.mutate(
      { plan_id: p.id, interval },
      {
        onSuccess: (r) => window.location.assign(r.url),
        onError: (e) => {
          toast.error(errorMessage(e));
          setBusy(null);
        },
      },
    );
  };

  if (plans.isPending) return <Skeleton className="h-32" />;
  if (plans.isError) return <LoadError error={plans.error} retry={() => plans.refetch()} />;
  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Plan</TableHead>
            <TableHead>Per day</TableHead>
            <TableHead>Per month</TableHead>
            <TableHead>Per send</TableHead>
            <TableHead>Phones</TableHead>
            <TableHead className="text-right">Price</TableHead>
            <TableHead className="text-right" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {(plans.data?.data ?? []).map((p) => {
            const paid = p.monthly_price_cents > 0;
            const current = isCurrent(p);
            return (
              <TableRow key={p.id}>
                <TableCell className="font-medium">{p.name}</TableCell>
                <TableCell>{limitLabel(p.daily_limit)}</TableCell>
                <TableCell>{limitLabel(p.monthly_limit)}</TableCell>
                <TableCell>{limitLabel(p.batch_limit)}</TableCell>
                <TableCell>{limitLabel(p.device_limit)}</TableCell>
                <TableCell className="text-right">
                  {priceLabel(price(p))}
                  {paid ? <span className="text-muted-foreground">{interval === "year" ? "/yr" : "/mo"}</span> : null}
                </TableCell>
                <TableCell className="text-right">
                  {current ? (
                    <span className="font-mono text-[11px] text-muted-foreground">current</span>
                  ) : paid && (status.checkout.available || managed) ? (
                    <Button size="sm" variant={managed ? "outline" : "default"} disabled={busy !== null} onClick={() => choose(p)}>
                      {busy === p.id ? "One moment" : managed ? `Switch to ${p.name}` : `Choose ${p.name}`}
                    </Button>
                  ) : null}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function BillingPage() {
  const router = useRouter();
  const params = useSearchParams();
  const checkoutID = params.get("checkout");
  const billing = useBilling();
  const { portal, cancel, resume } = useBillingMutations();
  const [interval, setInterval] = useState<Interval>("month");
  const status = billing.data;
  const sub = status?.subscription;
  const managed = !!sub?.managed && (sub.status === "active" || sub.status === "trialing" || sub.status === "past_due");

  // The interval follows the subscription until the reader changes it.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !sub?.interval) return;
    seeded.current = true;
    setInterval(sub.interval === "year" ? "year" : "month");
  }, [sub?.interval]);

  const clearCheckout = () => router.replace("/billing");

  return (
    <>
      <PageHeader title="Billing" description="Your plan, what it costs, and how to change it." />
      {checkoutID ? <CheckoutReturn id={checkoutID} onDone={clearCheckout} /> : null}
      {billing.isPending ? (
        <Skeleton className="h-40" />
      ) : billing.isError ? (
        <LoadError error={billing.error} retry={() => billing.refetch()} />
      ) : status ? (
        <div className="grid gap-6">
          <Card>
            <CardHeader>
              <CardTitle>Plan</CardTitle>
              <PlanLine status={status} />
            </CardHeader>
            <CardContent className="grid gap-4">
              {!status.checkout.available && !managed ? (
                <p className="text-sm text-muted-foreground">{closedCopy[status.checkout.reason ?? "closed"] ?? closedCopy.closed}</p>
              ) : (
                <div className="flex items-center gap-4 text-sm">
                  <span className="font-mono text-xs tracking-wide text-muted-foreground">billed</span>
                  {(["month", "year"] as const).map((i) => (
                    <button
                      key={i}
                      type="button"
                      aria-pressed={interval === i}
                      className={interval === i ? "font-medium underline underline-offset-4" : textLink}
                      onClick={() => setInterval(i)}
                    >
                      {i === "month" ? "monthly" : "yearly"}
                    </button>
                  ))}
                  {interval === "year" ? <span className="text-muted-foreground">two months free</span> : null}
                </div>
              )}
              <PlansTable status={status} interval={interval} />
              <div className="flex flex-wrap items-center gap-5">
                {managed && !sub?.cancel_at_period_end ? (
                  <button
                    type="button"
                    className={textLink}
                    disabled={cancel.isPending}
                    onClick={() =>
                      cancel.mutate(undefined, {
                        onSuccess: (r) =>
                          toast.success(
                            r.subscription?.current_period_end
                              ? `Your plan ends on ${absoluteTime(r.subscription.current_period_end)}. You keep it until then.`
                              : "Your plan ends at the end of this period.",
                          ),
                        onError: (e) => toast.error(errorMessage(e)),
                      })
                    }
                  >
                    Cancel plan
                  </button>
                ) : null}
                {managed && sub?.cancel_at_period_end ? (
                  <button
                    type="button"
                    className={textLink}
                    disabled={resume.isPending}
                    onClick={() =>
                      resume.mutate(undefined, {
                        onSuccess: () => toast.success("Your plan continues."),
                        onError: (e) => toast.error(errorMessage(e)),
                      })
                    }
                  >
                    Keep plan
                  </button>
                ) : null}
                {status.portal_available ? (
                  <button
                    type="button"
                    className={textLink}
                    disabled={portal.isPending}
                    onClick={() =>
                      portal.mutate(undefined, {
                        onSuccess: (r) => window.location.assign(r.url),
                        onError: (e) => toast.error(errorMessage(e)),
                      })
                    }
                  >
                    Invoices and payment method
                  </button>
                ) : null}
              </div>
              {status.provider === "polar" ? (
                <p className="text-sm text-muted-foreground">
                  Payments are handled by Polar, our merchant of record. Its name is on the card statement, tax is added at checkout where it applies, and
                  simhook never sees your card. Moving up is charged and applied right away; moving down applies at the next renewal. Cancelling keeps the
                  plan until the end of the paid period.
                </p>
              ) : null}
            </CardContent>
          </Card>
        </div>
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
