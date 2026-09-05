"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/field";
import { Turnstile } from "@/components/turnstile";
import { API_URL, errorMessage, isApiError } from "@/lib/api";
import { useAuthConfig, useAuthMutations } from "@/lib/queries";
import { safeNext } from "@/lib/session-cookie";

const schema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(1, "Enter your password"),
});
type Values = z.infer<typeof schema>;

/** What the API says when it sends someone back here from Google sign-in. */
const GOOGLE_ERRORS: Record<string, string> = {
  google_cancelled: "Google sign-in was cancelled.",
  google_state: "That Google sign-in took too long or was started in another tab. Try again.",
  google_failed: "Google sign-in did not go through. Try again, or sign in with your password.",
  google_email_unverified: "Google has not verified that email address, so it cannot sign in here. Use your password instead.",
  account_suspended: "This account is suspended.",
};

function LoginForm() {
  const params = useSearchParams();
  const config = useAuthConfig();
  const { login } = useAuthMutations();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { email: "", password: "" } });
  const [token, setToken] = useState<string | null>("");
  const [resetKey, setResetKey] = useState(0);
  const siteKey = config.data?.turnstile_site_key ?? "";
  const next = safeNext(params.get("next"));
  const fromGoogle = params.get("error");
  // Once the API has signed us in, the layout's redirect moves on as soon as
  // the account arrives; the form stays busy until then.
  const busy = login.isPending || login.isSuccess;

  const onSubmit = form.handleSubmit((values) =>
    login.mutate(
      { ...values, turnstile_token: siteKey && token ? token : undefined },
      {
        onError: (e) => {
          setToken("");
          setResetKey((k) => k + 1);
          if (isApiError(e)) {
            const fields = e.fieldMessages();
            for (const [k, v] of Object.entries(fields)) form.setError(k as keyof Values, { message: v });
          }
        },
      },
    ),
  );

  return (
    <form onSubmit={onSubmit} className="grid gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Sign in</h1>
        <p className="text-sm text-muted-foreground">Welcome back.</p>
      </div>
      {fromGoogle ? (
        <p className="border-l-2 border-destructive pl-4 text-sm text-destructive">{GOOGLE_ERRORS[fromGoogle] ?? "Sign-in did not go through."}</p>
      ) : null}
      <Field label="Email" htmlFor="email" error={form.formState.errors.email?.message}>
        <Input id="email" type="email" autoComplete="email" {...form.register("email")} />
      </Field>
      <Field label="Password" htmlFor="password" error={form.formState.errors.password?.message}>
        <Input id="password" type="password" autoComplete="current-password" {...form.register("password")} />
      </Field>
      {siteKey ? <Turnstile siteKey={siteKey} onToken={setToken} resetKey={resetKey} /> : null}
      {login.isError && !Object.keys(form.formState.errors).length ? (
        <p className="text-sm text-destructive">{errorMessage(login.error)}</p>
      ) : null}
      <Button type="submit" disabled={busy || config.isPending || (!!siteKey && token === "")}>
        {busy ? "Signing in…" : "Sign in"}
      </Button>
      {config.data?.google_sign_in ? (
        <p className="text-sm text-muted-foreground">
          or{" "}
          <a href={`${API_URL}/v1/auth/google/start?next=${encodeURIComponent(next)}`} className="text-foreground underline-offset-4 hover:underline">
            continue with Google
          </a>
        </p>
      ) : null}
      <div className="flex justify-between text-sm text-muted-foreground">
        <Link href="/reset-password" className="hover:text-foreground">
          Forgot password?
        </Link>
        <Link href="/register" className="hover:text-foreground">
          Create an account
        </Link>
      </div>
    </form>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
