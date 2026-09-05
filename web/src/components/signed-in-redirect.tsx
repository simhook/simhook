"use client";

import { useEffect, useSyncExternalStore } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSession } from "@/components/session-provider";
import { hasSignedInCookie, safeNext } from "@/lib/session-cookie";

// document.cookie is not observable, so this store re-reads it on each
// render that asks; the account arriving is what triggers that render.
const noSubscribe = () => () => {};
const readFlag = () => hasSignedInCookie();
const readFlagOnServer = () => true;

/**
 * Someone who is already signed in has no business on the sign-in pages:
 * send them on, to the email check first if the address is unverified,
 * otherwise wherever they were going.
 *
 * One case must not redirect. The dashboard's gate bounced them here because
 * the flag cookie was missing (gate=1), the API says they are signed in, and
 * the flag is still missing after that answer, which would have put it back:
 * this browser cannot see the cookie domain. Sending them on would loop, so
 * say what is wrong instead.
 */
export function SignedInRedirect() {
  const session = useSession();
  const router = useRouter();
  const params = useSearchParams();
  const flag = useSyncExternalStore(noSubscribe, readFlag, readFlagOnServer);
  const stuck = session.status === "authenticated" && params.get("gate") === "1" && !flag;

  useEffect(() => {
    if (session.status !== "authenticated" || stuck) return;
    const verified = !!session.account.user.email_verified_at;
    router.replace(verified ? safeNext(params.get("next")) : "/verify-email");
  }, [session, stuck, params, router]);

  if (!stuck) return null;
  return (
    <p className="mb-8 border-l-2 border-destructive pl-4 text-sm">
      <span className="font-medium text-destructive">Signed in, but this browser never receives the sign-in cookie.</span> The API says you
      are signed in, yet the cookie the dashboard checks before loading a page is not set for this address, so every page sends you back
      here. If this is your own simhook, set <code className="font-mono text-[13px]">SIMHOOK_COOKIE_DOMAIN</code> on the API to a domain
      both it and the dashboard are under, or set <code className="font-mono text-[13px]">SIMHOOK_SESSION_FLAG=off</code> on the dashboard.{" "}
      <button type="button" className="underline" onClick={() => void session.signOut()}>
        Sign out
      </button>
    </p>
  );
}
