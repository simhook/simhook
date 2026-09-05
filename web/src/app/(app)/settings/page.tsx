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
import { errorMessage } from "@/lib/api";
import { absoluteTime, formatCount, limitLabel, priceLabel } from "@/lib/format";
import { useAuthMutations, useMe, usePlans } from "@/lib/queries";

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

export default function SettingsPage() {
  const me = useMe();
  const plans = usePlans();
  const { changePassword } = useAuthMutations();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");

  const user = me.data?.user;
  const limits = me.data?.limits;
  const usage = me.data?.usage;

  return (
    <>
      <PageHeader title="Settings" description="Your account and plan." />
      <div className="grid gap-6 lg:grid-cols-2">
        {user ? <ProfileCard key={user.id} user={user} /> : <Skeleton className="h-48" />}

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
              You are on the {limits?.plan_name ?? "…"} plan
              {usage && limits ? ` · ${formatCount(usage.sent_this_month)} of ${limitLabel(limits.monthly_limit)} messages used this month` : ""}.
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
                        {p.id === limits?.plan_id ? <Badge variant="secondary" className="ml-2">current</Badge> : null}
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
      </div>
    </>
  );
}
