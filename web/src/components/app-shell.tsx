"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  BookOpen,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  MessageSquareText,
  Settings,
  Smartphone,
  Webhook,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { API_DOCS_URL, isApiError } from "@/lib/api";
import { useAuthMutations, useMe } from "@/lib/queries";
import { cn } from "@/lib/utils";

const nav = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/devices", label: "Devices", icon: Smartphone },
  { href: "/messages", label: "Messages", icon: MessageSquareText },
  { href: "/webhooks", label: "Webhooks", icon: Webhook },
  { href: "/api-keys", label: "API keys", icon: KeyRound },
  { href: "/settings", label: "Settings", icon: Settings },
] as const;

function Logo() {
  return (
    <Link href="/dashboard" className="flex items-center gap-2 px-2 font-semibold tracking-tight">
      <span className="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground text-sm font-bold">S</span>
      simhook
    </Link>
  );
}

function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  return (
    <nav className="grid gap-1">
      {nav.map(({ href, label, icon: Icon }) => {
        const active = pathname === href || pathname.startsWith(href + "/");
        return (
          <Link
            key={href}
            href={href}
            onClick={onNavigate}
            className={cn(
              "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
              active ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
            )}
          >
            <Icon className="size-4" />
            {label}
          </Link>
        );
      })}
      <a
        href={API_DOCS_URL}
        target="_blank"
        rel="noreferrer"
        className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground"
      >
        <BookOpen className="size-4" />
        API reference
      </a>
    </nav>
  );
}

function UserBox() {
  const me = useMe();
  const { logout } = useAuthMutations();
  const router = useRouter();
  const email = me.data?.user.email ?? "";
  return (
    <div className="flex items-center justify-between gap-2 rounded-md border bg-card px-2.5 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{me.data?.user.name || email.split("@")[0]}</p>
        <p className="truncate text-xs text-muted-foreground">{email}</p>
      </div>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Sign out"
        onClick={() => logout.mutate(undefined, { onSuccess: () => router.replace("/login") })}
      >
        <LogOut className="size-4" />
      </Button>
    </div>
  );
}

function VerifyBanner() {
  const me = useMe();
  const pathname = usePathname();
  if (!me.data || me.data.user.email_verified_at || pathname === "/verify-email") return null;
  return (
    <Alert className="mb-6">
      <AlertTitle>Verify your email to start sending</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center gap-x-2">
        We sent a code to {me.data.user.email}.
        <Link href="/verify-email" className="font-medium text-primary underline-offset-4 hover:underline">
          Enter the code
        </Link>
      </AlertDescription>
    </Alert>
  );
}

/** Signed-in frame: sidebar on desktop, sheet on mobile, and the auth gate. */
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
      <div className="mx-auto w-full max-w-3xl p-8">
        <Skeleton className="mb-4 h-8 w-48" />
        <Skeleton className="mb-2 h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    );
  }

  if (me.isError) {
    return (
      <div className="mx-auto w-full max-w-lg p-8">
        <Alert variant="destructive">
          <AlertTitle>The API is not reachable</AlertTitle>
          <AlertDescription>
            {me.error.message} <button className="underline" onClick={() => me.refetch()}>Try again</button>
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-1">
      <aside className="hidden w-60 shrink-0 flex-col gap-4 border-r bg-sidebar p-3 md:flex">
        <div className="py-1">
          <Logo />
        </div>
        <NavLinks />
        <div className="mt-auto">
          <UserBox />
        </div>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center gap-2 border-b px-4 py-2 md:hidden">
          <Sheet>
            <SheetTrigger render={<Button variant="ghost" size="icon" aria-label="Open menu" />}>
              <Menu className="size-5" />
            </SheetTrigger>
            <SheetContent side="left" className="flex w-64 flex-col gap-4 bg-sidebar p-3">
              <SheetTitle className="sr-only">Navigation</SheetTitle>
              <Logo />
              <NavLinks />
              <div className="mt-auto">
                <UserBox />
              </div>
            </SheetContent>
          </Sheet>
          <Logo />
        </header>
        <main className="mx-auto w-full max-w-6xl flex-1 p-4 md:p-8">
          <VerifyBanner />
          {children}
        </main>
      </div>
    </div>
  );
}
