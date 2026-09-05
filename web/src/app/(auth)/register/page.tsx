"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/field";
import { errorMessage, isApiError } from "@/lib/api";
import { useAuthMutations } from "@/lib/queries";

const schema = z.object({
  name: z.string().max(100).optional(),
  email: z.string().email("Enter a valid email"),
  password: z.string().min(10, "At least 10 characters"),
});
type Values = z.infer<typeof schema>;

export default function RegisterPage() {
  const router = useRouter();
  const { register: signUp } = useAuthMutations();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: "", email: "", password: "" } });

  const onSubmit = form.handleSubmit((values) =>
    signUp.mutate(
      { email: values.email, password: values.password, name: values.name || undefined },
      {
        onSuccess: () => router.replace("/verify-email"),
        onError: (e) => {
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
        <h1 className="text-xl font-semibold">Create your account</h1>
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
      {signUp.isError && !Object.keys(form.formState.errors).length ? (
        <p className="text-sm text-destructive">{errorMessage(signUp.error)}</p>
      ) : null}
      <Button type="submit" disabled={signUp.isPending}>
        {signUp.isPending ? "Creating…" : "Create account"}
      </Button>
      <p className="text-center text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link href="/login" className="text-foreground underline-offset-4 hover:underline">
          Sign in
        </Link>
      </p>
    </form>
  );
}
