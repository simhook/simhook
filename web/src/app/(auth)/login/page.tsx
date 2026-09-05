"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/field";
import { errorMessage, isApiError } from "@/lib/api";
import { useAuthMutations } from "@/lib/queries";

const schema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(1, "Enter your password"),
});
type Values = z.infer<typeof schema>;

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { login } = useAuthMutations();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { email: "", password: "" } });
  const next = params.get("next");
  const target = next && next.startsWith("/") ? next : "/dashboard";

  const onSubmit = form.handleSubmit((values) =>
    login.mutate(values, {
      onSuccess: () => router.replace(target),
      onError: (e) => {
        if (isApiError(e)) {
          const fields = e.fieldMessages();
          for (const [k, v] of Object.entries(fields)) form.setError(k as keyof Values, { message: v });
        }
      },
    }),
  );

  return (
    <form onSubmit={onSubmit} className="grid gap-4">
      <div>
        <h1 className="text-xl font-semibold">Sign in</h1>
        <p className="text-sm text-muted-foreground">Welcome back.</p>
      </div>
      <Field label="Email" htmlFor="email" error={form.formState.errors.email?.message}>
        <Input id="email" type="email" autoComplete="email" {...form.register("email")} />
      </Field>
      <Field label="Password" htmlFor="password" error={form.formState.errors.password?.message}>
        <Input id="password" type="password" autoComplete="current-password" {...form.register("password")} />
      </Field>
      {login.isError && !Object.keys(form.formState.errors).length ? (
        <p className="text-sm text-destructive">{errorMessage(login.error)}</p>
      ) : null}
      <Button type="submit" disabled={login.isPending}>
        {login.isPending ? "Signing in…" : "Sign in"}
      </Button>
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
