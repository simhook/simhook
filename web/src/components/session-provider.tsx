"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { keys, sessionQueryOptions, type Account } from "@/lib/queries";

/**
 * Who is signed in, for the whole tree. One query answers it, and the answer
 * has exactly four shapes: still asking, nobody, the API could not be
 * reached, or an account. "Nobody" is data, not an error: a 401 from the
 * account query resolves to null, and any other request that comes back
 * with a lost session sets the same null (see providers.tsx), so every
 * screen learns of a sign-out at once and in the same way.
 */
export type Session =
  | { status: "loading" }
  | { status: "unauthenticated" }
  | { status: "error"; error: Error; retry: () => void }
  | { status: "authenticated"; account: Account };

type SessionContextValue = Session & {
  /** Ends this session on the API, forgets everything cached for it, and goes to sign-in. */
  signOut: () => Promise<void>;
  /** Asks the API again. */
  refresh: () => Promise<unknown>;
};

const SessionContext = createContext<SessionContextValue | null>(null);

/** The pages a signed-out visitor may be on. Everywhere else is the app. */
const AUTH_PATHS = ["/login", "/register", "/reset-password"];

export function SessionProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const query = useQuery(sessionQueryOptions);
  // Whether the current sign-out was this browser's own doing, as opposed
  // to a session that was lost. Set by signOut, forgotten on sign-in.
  const deliberate = useRef(false);

  // The one place a signed-out visitor is sent to sign-in: a session lost
  // while inside the app goes to sign-in remembering where it was; a
  // deliberate sign-out is already on its way there.
  useEffect(() => {
    if (query.data) {
      deliberate.current = false;
      return;
    }
    if (query.data !== null || deliberate.current) return;
    if (AUTH_PATHS.some((p) => pathname === p || pathname.startsWith(p + "/"))) return;
    router.replace(`/login?next=${encodeURIComponent(pathname + window.location.search)}`);
  }, [query.data, pathname, router]);

  const signOut = useCallback(async () => {
    deliberate.current = true;
    try {
      await api.POST("/v1/auth/logout");
    } catch {
      // The API is down or the cookies are already gone; this browser is signed out either way.
    }
    qc.setQueryData(keys.me, null);
    qc.removeQueries({ predicate: (q) => q.queryKey[0] !== keys.me[0] });
    router.replace("/login");
  }, [qc, router]);

  const refresh = useCallback(() => qc.invalidateQueries({ queryKey: keys.me }), [qc]);

  const value = useMemo<SessionContextValue>(() => {
    let session: Session;
    if (query.isPending) session = { status: "loading" };
    else if (query.isError) session = { status: "error", error: query.error, retry: () => void query.refetch() };
    else if (query.data === null) session = { status: "unauthenticated" };
    else session = { status: "authenticated", account: query.data };
    return { ...session, signOut, refresh };
  }, [query, signOut, refresh]);

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession needs a SessionProvider above it");
  return ctx;
}

/**
 * The signed-in account, for screens inside the app shell, which renders
 * them only once the session is known to be authenticated.
 */
export function useAccount(): Account {
  const session = useSession();
  if (session.status !== "authenticated") throw new Error("useAccount called outside a signed-in screen");
  return session.account;
}
