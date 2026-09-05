"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Bar, Footer, Shell, SITE_URL } from "@/components/site-chrome";
import { isApiError } from "@/lib/api";
import { useAuthMutations, useMe } from "@/lib/queries";

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
  const me = useMe();
  const { logout } = useAuthMutations();
  const router = useRouter();
  const email = me.data?.user.email ?? "";
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
          <button
            type="button"
            className="hover:text-foreground"
            onClick={() => logout.mutate(undefined, { onSuccess: () => router.replace("/login") })}
          >
            Sign out
          </button>
        </>
      }
    />
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

/** Signed-in frame: the shared shell with the app's words in the bar, and the auth gate. */
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
      <Shell>
        <div className="py-10 text-sm text-muted-foreground">Loading…</div>
      </Shell>
    );
  }

  if (me.isError) {
    return (
      <Shell>
        <p className="mt-10 border-l-2 border-destructive pl-4 text-sm">
          <span className="font-medium text-destructive">The API is not reachable.</span> {me.error.message}{" "}
          <button className="underline" onClick={() => me.refetch()}>
            Try again
          </button>
        </p>
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
