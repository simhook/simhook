"use client";

import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";
import type { User } from "@simhook/contracts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Field } from "@/components/field";
import { LoadError, PageHeader, textLink } from "@/components/page-header";
import { useAccount } from "@/components/session-provider";
import { StatusBadge } from "@/components/status-badge";
import { errorMessage } from "@/lib/api";
import { absoluteTime, browserName, relativeTime } from "@/lib/format";
import { useAuthMutations, useSessionMutations, useSessions } from "@/lib/queries";

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
        <div className="flex flex-wrap items-center gap-5">
          <button
            type="button"
            className={textLink}
            disabled={updateProfile.isPending || name.trim() === (user.name ?? "")}
            onClick={() => updateProfile.mutate({ name: name.trim() }, { onSuccess: () => toast.success("Profile saved."), onError: (e) => toast.error(errorMessage(e)) })}
          >
            Save name
          </button>
          <span className="inline-flex items-center gap-3 text-sm text-muted-foreground">
            {user.email_verified_at ? <StatusBadge status="delivered" label="Email verified" /> : <StatusBadge status="unknown" label="Email not verified" />}
            {!user.email_verified_at ? (
              <button
                type="button"
                className={textLink}
                disabled={sendVerification.isPending}
                onClick={() => sendVerification.mutate(undefined, { onSuccess: () => toast.success("Code sent."), onError: (e) => toast.error(errorMessage(e)) })}
              >
                Send code
              </button>
            ) : null}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">
          Member since {absoluteTime(user.created_at)}.{user.google_linked ? " Signs in with Google." : ""}
        </p>
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
  const link = textLink;
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
        <LoadError error={sessions.error} retry={() => sessions.refetch()} />
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
  const { user } = useAccount();
  const { changePassword } = useAuthMutations();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");

  return (
    <>
      <PageHeader title="Settings" description="Your account. The plan is under Billing." />
      <div className="grid gap-6 lg:grid-cols-2">
        <ProfileCard key={user.id} user={user} />

        <Card>
          <CardHeader>
            <CardTitle>Password</CardTitle>
            <CardDescription>
              {user.has_password ? "At least 10 characters." : "This account has no password and signs in with Google."}
            </CardDescription>
          </CardHeader>
          {!user.has_password ? (
            <CardContent>
              <p className="text-sm text-muted-foreground">
                To add one, use{" "}
                <Link href="/reset-password" className="text-foreground underline decoration-underline underline-offset-4 hover:decoration-foreground">
                  Forgot password
                </Link>{" "}
                on the sign-in page: a code goes to your email and sets it.
              </p>
            </CardContent>
          ) : (
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
          )}
        </Card>

        <SessionsSection />
      </div>
    </>
  );
}
