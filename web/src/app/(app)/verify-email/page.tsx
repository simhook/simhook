"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field } from "@/components/field";
import { errorMessage } from "@/lib/api";
import { useAuthMutations, useMe } from "@/lib/queries";

const schema = z.object({ code: z.string().length(6, "The code has 6 digits") });

export default function VerifyEmailPage() {
  const me = useMe();
  const router = useRouter();
  const { verifyEmail, sendVerification } = useAuthMutations();
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { code: "" } });

  useEffect(() => {
    if (me.data?.user.email_verified_at) router.replace("/dashboard");
  }, [me.data, router]);

  return (
    <div className="mx-auto max-w-md">
      <Card>
        <CardHeader>
          <CardTitle>Verify your email</CardTitle>
          <CardDescription>We sent a 6-digit code to {me.data?.user.email}. Sending is unlocked once it&apos;s confirmed.</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-4"
            onSubmit={form.handleSubmit((v) =>
              verifyEmail.mutate(v, {
                onSuccess: () => {
                  toast.success("Email verified.");
                  router.replace("/dashboard");
                },
              }),
            )}
          >
            <Field label="Code" htmlFor="code" error={form.formState.errors.code?.message}>
              <Input id="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} className="font-mono text-lg tracking-widest" {...form.register("code")} />
            </Field>
            {verifyEmail.isError ? <p className="text-sm text-destructive">{errorMessage(verifyEmail.error)}</p> : null}
            <div className="flex items-center gap-2">
              <Button type="submit" disabled={verifyEmail.isPending}>
                {verifyEmail.isPending ? "Checking…" : "Verify"}
              </Button>
              <Button
                type="button"
                variant="ghost"
                disabled={sendVerification.isPending}
                onClick={() => sendVerification.mutate(undefined, { onSuccess: () => toast.success("A new code is on its way.") })}
              >
                Resend code
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
