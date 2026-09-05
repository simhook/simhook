"use client";

import { useState } from "react";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/field";
import { Turnstile } from "@/components/turnstile";
import { API_URL, errorMessage, isApiError } from "@/lib/api";
import { useAuthConfig, useAuthMutations } from "@/lib/queries";

const schema = z.object({
  name: z.string().max(100).optional(),
  email: z.string().email("Enter a valid email"),
  password: z.string().min(10, "At least 10 characters"),
});
type Values = z.infer<typeof schema>;

export default function RegisterPage() {
  const config = useAuthConfig();
  const { register: signUp } = useAuthMutations();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: "", email: "", password: "" } });
  const [token, setToken] = useState<string | null>("");
  const [resetKey, setResetKey] = useState(0);
  const siteKey = config.data?.turnstile_site_key ?? "";
  // The new account is signed in; the layout's redirect takes it to the
  // email check as soon as it arrives.
  const busy = signUp.isPending || signUp.isSuccess;

  const onSubmit = form.handleSubmit((values) =>
    signUp.mutate(
      {
        email: values.email,
        password: values.password,
        name: values.name || undefined,
        turnstile_token: siteKey && token ? token : undefined,
      },
      {
        onError: (e) => {
          setToken("");
          setResetKey((k) => k + 1);
          if (isApiError(e)) {
            for (const [k, v] of Object.entries(e.fieldMessages())) form.setError(k as keyof Values, { message: v });
          }
        },
      },
    ),
  );

  return (
    <form onSubmit={onSubmit} className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Create your account</h1>
        <p className="text-sm text-muted-foreground">Free to start. No card needed.</p>
      </div>
      <Field label="Name" htmlFor="name" error={form.formState.errors.name?.message}>
        <Input id="name" autoComplete="name" {...form.register("name")} />
      </Field>
      <Field label="Email" htmlFor="email" error={form.formState.errors.email?.message}>
        <Input id="email" type="email" autoComplete="email" {...form.register("email")} />
      </Field>
      <Field label="Password" htmlFor="password" error={form.formState.errors.password?.message} hint="At least 10 characters.">
        <Input id="password" type="password" autoComplete="new-password" {...form.register("password")} />
      </Field>
      {siteKey ? <Turnstile siteKey={siteKey} onToken={setToken} resetKey={resetKey} /> : null}
      {signUp.isError && !Object.keys(form.formState.errors).length ? (
        <p className="text-sm text-destructive">{errorMessage(signUp.error)}</p>
      ) : null}
      <Button type="submit" disabled={busy || config.isPending || (!!siteKey && token === "")}>
        {busy ? "Creating…" : "Create account"}
      </Button>
      {config.data?.google_sign_in ? (
        <p className="text-sm text-muted-foreground">
          or{" "}
          <a href={`${API_URL}/v1/auth/google/start`} className="text-foreground underline-offset-4 hover:underline">
            continue with Google
          </a>
        </p>
      ) : null}
      <p className="text-center text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link href="/login" className="text-foreground underline-offset-4 hover:underline">
          Sign in
        </Link>
      </p>
    </form>
  );
}
