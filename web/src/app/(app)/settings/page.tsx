"use client";

import { useState } from "react";
import { toast } from "sonner";
import type { User } from "@simhook/contracts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Field } from "@/components/field";
import { PageHeader } from "@/components/page-header";
import { useAccount } from "@/components/session-provider";
import { errorMessage } from "@/lib/api";
import { absoluteTime, browserName, formatCount, limitLabel, priceLabel, relativeTime } from "@/lib/format";
import { useAuthMutations, usePlans, useSessionMutations, useSessions } from "@/lib/queries";

function ProfileCard({ user }: { user: User }) {
  const { updateProfile, sendVerification } = useAuthMutations();
  const [name, setName] = useState(user.name ?? "");
  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>{user.email}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <Field label="Name" htmlFor="name">
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} maxLength={100} />
        </Field>
        <div className="flex items-center gap-3">
          <Button
            disabled={updateProfile.isPending || name.trim() === (user.name ?? "")}
            onClick={() => updateProfile.mutate({ name: name.trim() }, { onSuccess: () => toast.success("Profile saved."), onError: (e) => toast.error(errorMessage(e)) })}
          >
            Save
          </Button>
          <span className="text-sm text-muted-foreground">
            Email {user.email_verified_at ? <Badge variant="secondary">verified</Badge> : <Badge variant="outline">not verified</Badge>}
            {!user.email_verified_at ? (
              <Button variant="link" className="h-auto p-0 pl-2" onClick={() => sendVerification.mutate(undefined, { onSuccess: () => toast.success("Code sent.") })}>
                Send code
              </Button>
            ) : null}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">Member since {absoluteTime(user.created_at)}.</p>
      </CardContent>
    </Card>
  );
}

/** Every browser signed in to the account, and a way to end any of them. */
function SessionsSection() {
  const sessions = useSessions();
  const { revoke, revokeOthers } = useSessionMutations();
  const list = sessions.data?.data ?? [];
  const others = list.filter((s) => !s.current).length;
  const link = "text-sm underline decoration-[#b8b8b4] underline-offset-4 hover:decoration-foreground disabled:opacity-50";
  return (
    <section className="lg:col-span-2">
      <div className="mb-2 flex items-baseline justify-between gap-4">
        <h2 className="font-mono text-xs tracking-wide text-muted-foreground">sessions</h2>
        {others > 0 ? (
          <button
            type="button"
            className={link}
            disabled={revokeOthers.isPending}
            onClick={() =>
              revokeOthers.mutate(undefined, {
                onSuccess: () => toast.success("Every other browser is signed out."),
                onError: (e) => toast.error(errorMessage(e)),
              })
            }
          >
            Sign out everywhere else
          </button>
        ) : null}
      </div>
      {sessions.isPending ? (
        <Skeleton className="h-24" />
      ) : sessions.isError ? (
        <p className="border-y py-6 text-sm">
          <span className="text-destructive">{errorMessage(sessions.error)}</span>{" "}
          <button type="button" className={link} onClick={() => sessions.refetch()}>
            Try again
          </button>
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Browser</TableHead>
              <TableHead>Address</TableHead>
              <TableHead>Signed in</TableHead>
              <TableHead>Last seen</TableHead>
              <TableHead className="text-right" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((s) => (
              <TableRow key={s.id}>
                <TableCell>{browserName(s.user_agent)}</TableCell>
                <TableCell className="font-mono text-xs">{s.ip ?? ""}</TableCell>
                <TableCell title={absoluteTime(s.created_at)}>{relativeTime(s.created_at)}</TableCell>
                <TableCell title={absoluteTime(s.last_seen_at)}>{relativeTime(s.last_seen_at)}</TableCell>
                <TableCell className="text-right">
                  {s.current ? (
                    <span className="text-muted-foreground">This browser</span>
                  ) : (
                    <button
                      type="button"
                      className={link}
                      disabled={revoke.isPending}
                      onClick={() => revoke.mutate(s.id, { onError: (e) => toast.error(errorMessage(e)) })}
                    >
                      Sign out
                    </button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <p className="mt-3 text-sm text-muted-foreground">
        A session ends after 30 days unused, and 180 days after signing in at the latest. Changing your password signs out every other browser.
      </p>
    </section>
  );
}

export default function SettingsPage() {
  const { user, limits, usage } = useAccount();
  const plans = usePlans();
  const { changePassword } = useAuthMutations();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");

  return (
    <>
      <PageHeader title="Settings" description="Your account and plan." />
      <div className="grid gap-6 lg:grid-cols-2">
        <ProfileCard key={user.id} user={user} />

        <Card>
          <CardHeader>
            <CardTitle>Password</CardTitle>
            <CardDescription>At least 10 characters.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <Field label="Current password" htmlFor="current">
              <Input id="current" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} />
            </Field>
            <Field label="New password" htmlFor="next">
              <Input id="next" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
            </Field>
            <div>
              <Button
                disabled={changePassword.isPending || !current || next.length < 10}
                onClick={() =>
                  changePassword.mutate(
                    { current_password: current, new_password: next },
                    {
                      onSuccess: () => {
                        toast.success("Password changed.");
                        setCurrent("");
                        setNext("");
                      },
                      onError: (e) => toast.error(errorMessage(e)),
                    },
                  )
                }
              >
                Change password
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Plan</CardTitle>
            <CardDescription>
              You are on the {limits.plan_name} plan, {formatCount(usage.sent_this_month)} of {limitLabel(limits.monthly_limit)} messages used this month.
            </CardDescription>
          </CardHeader>
          <CardContent>
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
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(plans.data?.data ?? []).map((p) => (
                    <TableRow key={p.id}>
                      <TableCell className="font-medium">
                        {p.name}
                        {p.id === limits.plan_id ? <Badge variant="secondary" className="ml-2">current</Badge> : null}
                      </TableCell>
                      <TableCell>{limitLabel(p.daily_limit)}</TableCell>
                      <TableCell>{limitLabel(p.monthly_limit)}</TableCell>
                      <TableCell>{limitLabel(p.batch_limit)}</TableCell>
                      <TableCell>{limitLabel(p.device_limit)}</TableCell>
                      <TableCell className="text-right">
                        {priceLabel(p.monthly_price_cents)}
                        {p.monthly_price_cents ? <span className="text-muted-foreground">/mo</span> : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <p className="mt-3 text-sm text-muted-foreground">Upgrades open soon. Until then, limits above apply.</p>
          </CardContent>
        </Card>

        <SessionsSection />
      </div>
    </>
  );
}
