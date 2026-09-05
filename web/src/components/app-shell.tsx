"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { API_DOCS_URL, isApiError } from "@/lib/api";
import { useAuthMutations, useMe } from "@/lib/queries";
import { cn } from "@/lib/utils";

const nav = [
  { href: "/dashboard", label: "Overview" },
  { href: "/devices", label: "Phones" },
  { href: "/messages", label: "Messages" },
  { href: "/webhooks", label: "Webhooks" },
  { href: "/api-keys", label: "API keys" },
  { href: "/settings", label: "Settings" },
] as const;

function TopNav() {
  const pathname = usePathname();
  const me = useMe();
  const { logout } = useAuthMutations();
  const router = useRouter();
  const email = me.data?.user.email ?? "";
  return (
    <nav className="flex flex-wrap items-baseline gap-x-5 gap-y-2 border-b py-4 text-sm" aria-label="Main">
      <Link href="/dashboard" className="mr-1 font-mono text-[15px] font-medium">
        simhook
      </Link>
      {nav.map(({ href, label }) => {
        const active = pathname === href || pathname.startsWith(href + "/");
        return (
          <Link
            key={href}
            href={href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "transition-colors",
              active ? "text-foreground underline underline-offset-[6px]" : "text-muted-foreground hover:text-foreground",
            )}
          >
            {label}
          </Link>
        );
      })}
      <span className="ml-auto flex items-center gap-4 text-muted-foreground">
        <span className="hidden max-w-[240px] truncate sm:inline" title={email}>
          {email}
        </span>
        <a href={API_DOCS_URL} target="_blank" rel="noreferrer" className="hover:text-foreground">
          Docs
        </a>
        <button
          type="button"
          className="hover:text-foreground"
          onClick={() => logout.mutate(undefined, { onSuccess: () => router.replace("/login") })}
        >
          Sign out
        </button>
      </span>
    </nav>
  );
}

function VerifyBanner() {
  const me = useMe();
  const pathname = usePathname();
  if (!me.data || me.data.user.email_verified_at || pathname === "/verify-email") return null;
  return (
    <p className="mt-6 border-l-2 border-foreground pl-4 text-sm">
      <span className="font-medium">Verify your email to start sending.</span> We sent a code to {me.data.user.email}.{" "}
      <Link href="/verify-email" className="underline">
        Enter the code
      </Link>
    </p>
  );
}

/** Signed-in frame: one column with a row of words at the top, and the auth gate. */
export function AppShell({ children }: { children: ReactNode }) {
  const me = useMe();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (me.isError && isApiError(me.error) && me.error.isUnauthenticated) {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [me.isError, me.error, router, pathname]);

  if (me.isPending || (me.isError && isApiError(me.error) && me.error.isUnauthenticated)) {
    return (
      <div className="mx-auto w-full max-w-[960px] px-6 py-10 text-sm text-muted-foreground">Loading…</div>
    );
  }

  if (me.isError) {
    return (
      <div className="mx-auto w-full max-w-[960px] px-6 py-10 text-sm">
        <p className="border-l-2 border-destructive pl-4">
          <span className="font-medium text-destructive">The API is not reachable.</span> {me.error.message}{" "}
          <button className="underline" onClick={() => me.refetch()}>
            Try again
          </button>
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-[960px] px-6 pb-16">
      <TopNav />
      <main>
        <VerifyBanner />
        {children}
      </main>
    </div>
  );
}
