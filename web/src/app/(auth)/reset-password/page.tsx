"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/field";
import { errorMessage } from "@/lib/api";
import { useAuthMutations } from "@/lib/queries";

const requestSchema = z.object({ email: z.string().email("Enter a valid email") });
const resetSchema = z.object({
  email: z.string().email("Enter a valid email"),
  code: z.string().length(6, "The code has 6 digits"),
  new_password: z.string().min(10, "At least 10 characters"),
});

export default function ResetPasswordPage() {
  const router = useRouter();
  const { requestReset, resetPassword } = useAuthMutations();
  const [stage, setStage] = useState<"request" | "reset">("request");
  const [email, setEmail] = useState("");

  const requestForm = useForm<z.infer<typeof requestSchema>>({ resolver: zodResolver(requestSchema), defaultValues: { email: "" } });
  const resetForm = useForm<z.infer<typeof resetSchema>>({
    resolver: zodResolver(resetSchema),
    defaultValues: { email: "", code: "", new_password: "" },
  });

  if (stage === "request") {
    return (
      <form
        className="grid gap-4"
        onSubmit={requestForm.handleSubmit((v) =>
          requestReset.mutate(v, {
            onSuccess: () => {
              setEmail(v.email);
              resetForm.setValue("email", v.email);
              setStage("reset");
            },
          }),
        )}
      >
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Reset your password</h1>
          <p className="text-sm text-muted-foreground">We email a code if the address has an account.</p>
        </div>
        <Field label="Email" htmlFor="email" error={requestForm.formState.errors.email?.message}>
          <Input id="email" type="email" autoComplete="email" {...requestForm.register("email")} />
        </Field>
        {requestReset.isError ? <p className="text-sm text-destructive">{errorMessage(requestReset.error)}</p> : null}
        <Button type="submit" disabled={requestReset.isPending}>
          {requestReset.isPending ? "Sending…" : "Send code"}
        </Button>
        <p className="text-center text-sm text-muted-foreground">
          <Link href="/login" className="text-foreground underline-offset-4 hover:underline">
            Back to sign in
          </Link>
        </p>
      </form>
    );
  }

  return (
    <form
      className="grid gap-4"
      onSubmit={resetForm.handleSubmit((v) =>
        resetPassword.mutate(v, {
          onSuccess: () => {
            toast.success("Password updated. Sign in with the new one.");
            router.replace("/login");
          },
        }),
      )}
    >
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Enter the code</h1>
        <p className="text-sm text-muted-foreground">Check {email} for a 6-digit code. It expires in 30 minutes.</p>
      </div>
      <Field label="Code" htmlFor="code" error={resetForm.formState.errors.code?.message}>
        <Input id="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} {...resetForm.register("code")} />
      </Field>
      <Field label="New password" htmlFor="new_password" error={resetForm.formState.errors.new_password?.message}>
        <Input id="new_password" type="password" autoComplete="new-password" {...resetForm.register("new_password")} />
      </Field>
      {resetPassword.isError ? <p className="text-sm text-destructive">{errorMessage(resetPassword.error)}</p> : null}
      <Button type="submit" disabled={resetPassword.isPending}>
        {resetPassword.isPending ? "Saving…" : "Set new password"}
      </Button>
      <button type="button" className="text-sm text-muted-foreground hover:text-foreground" onClick={() => setStage("request")}>
        Use a different email
      </button>
    </form>
  );
}
