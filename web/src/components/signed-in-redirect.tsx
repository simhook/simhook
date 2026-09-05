"use client";

import { useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMe } from "@/lib/queries";

/** Someone who is already signed in has no business on the sign-in pages: send them on. */
export function SignedInRedirect() {
  const me = useMe();
  const router = useRouter();
  const params = useSearchParams();
  useEffect(() => {
    if (!me.data) return;
    const next = params.get("next");
    router.replace(next && next.startsWith("/") ? next : "/dashboard");
  }, [me.data, params, router]);
  return null;
}
