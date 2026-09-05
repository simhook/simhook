"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSession } from "@/components/session-provider";
import { Bar, Footer, Shell, SITE_URL } from "@/components/site-chrome";

const nav = [
  { href: "/dashboard", label: "Overview" },
  { href: "/devices", label: "Phones" },
  { href: "/messages", label: "Messages" },
  { href: "/webhooks", label: "Webhooks" },
  { href: "/api-keys", label: "API keys" },
  { href: "/settings", label: "Settings" },
] as const;

function AppBar() {
  const pathname = usePathname();
  const session = useSession();
  const email = session.status === "authenticated" ? session.account.user.email : "";
  return (
    <Bar
      links={nav}
      isActive={(href) => pathname === href || pathname.startsWith(href + "/")}
      right={
        <>
          <span className="hidden max-w-[240px] truncate sm:inline" title={email}>
            {email}
          </span>
          <a href={`${SITE_URL}/docs`} className="hover:text-foreground">
            Docs
          </a>
          <button type="button" className="hover:text-foreground" onClick={() => void session.signOut()}>
            Sign out
          </button>
        </>
      }
    />
  );
}

function VerifyBanner() {
  const session = useSession();
  const pathname = usePathname();
  if (session.status !== "authenticated" || session.account.user.email_verified_at || pathname === "/verify-email") return null;
  return (
    <p className="mt-6 border-l-2 border-foreground pl-4 text-sm">
      <span className="font-medium">Verify your email to start sending.</span> We sent a code to {session.account.user.email}.{" "}
      <Link href="/verify-email" className="underline">
        Enter the code
      </Link>
    </p>
  );
}

/**
 * Signed-in frame: the shared shell with the app's words in the bar. The
 * frame paints at once; the words wait for the account. A visitor with no
 * session sees the same frame while the session provider sends them to
 * sign-in.
 */
export function AppShell({ children }: { children: ReactNode }) {
  const session = useSession();

  if (session.status === "loading" || session.status === "unauthenticated") {
    return (
      <Shell>
        <AppBar />
        <main className="flex-1" aria-busy="true">
          <div className="mt-12 grid gap-3">
            <div className="h-6 w-40 bg-muted" />
            <div className="h-4 w-80 max-w-full bg-muted" />
          </div>
        </main>
        <Footer />
      </Shell>
    );
  }

  if (session.status === "error") {
    return (
      <Shell>
        <AppBar />
        <main className="flex-1">
          <p className="mt-10 border-l-2 border-destructive pl-4 text-sm">
            <span className="font-medium text-destructive">The API is not reachable.</span> {session.error.message}{" "}
            <button type="button" className="underline" onClick={session.retry}>
              Try again
            </button>
          </p>
        </main>
        <Footer />
      </Shell>
    );
  }

  return (
    <Shell>
      <AppBar />
      <main className="flex-1">
        <VerifyBanner />
        {children}
      </main>
      <Footer />
    </Shell>
  );
}
