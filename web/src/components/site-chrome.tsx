import type { ReactNode } from "react";
import Link from "next/link";
import { cn } from "@/lib/utils";

/**
 * The shell every simhook page shares with simhook.dev: an 1120 px container,
 * a bar of words at the top, and the same footer at the bottom. Only the
 * words in the bar differ between the site, sign-in, and the app.
 */

export const SITE_URL = "https://simhook.dev";

export const PUBLIC_LINKS = [
  { href: `${SITE_URL}/docs`, label: "Docs" },
  { href: `${SITE_URL}/pricing`, label: "Pricing" },
  { href: `${SITE_URL}/download`, label: "Download" },
  { href: "https://github.com/simhook/simhook", label: "GitHub" },
] as const;

export function Shell({ children }: { children: ReactNode }) {
  return <div className="mx-auto flex min-h-screen w-full max-w-[1120px] flex-col px-6">{children}</div>;
}

export function Bar({
  links,
  right,
  isActive,
}: {
  links: ReadonlyArray<{ href: string; label: string }>;
  right?: ReactNode;
  isActive?: (href: string) => boolean;
}) {
  return (
    // Two rows on a phone (wordmark and account, then the words), one row from 640 px up.
    <nav className="grid grid-cols-[1fr_auto] gap-y-2.5 border-b py-4 text-[14px] sm:flex sm:items-baseline sm:gap-x-5" aria-label="Main">
      <a href={SITE_URL} className="mr-1 font-mono text-[15px] font-medium text-foreground">
        simhook
      </a>
      <span className="order-last col-span-2 flex flex-wrap gap-x-4 gap-y-1 sm:order-none sm:col-span-1 sm:gap-x-5">
      {links.map(({ href, label }) => {
        const active = isActive?.(href) ?? false;
        const className = cn("transition-colors", active ? "text-foreground underline underline-offset-[6px]" : "text-muted-foreground hover:text-foreground");
        return href.startsWith("/") ? (
          <Link key={href} href={href} aria-current={active ? "page" : undefined} className={className}>
            {label}
          </Link>
        ) : (
          <a key={href} href={href} className={className}>
            {label}
          </a>
        );
      })}
      </span>
      {right ? <span className="flex items-center gap-[18px] justify-self-end whitespace-nowrap text-muted-foreground sm:ml-auto">{right}</span> : null}
    </nav>
  );
}

export function Footer() {
  return (
    <footer className="mt-[72px] flex flex-wrap gap-[18px] border-t pb-10 pt-[18px] text-[13px] text-muted-foreground [&_a]:text-muted-foreground [&_a]:no-underline [&_a:hover]:text-foreground">
      <span>Open source, AGPL-3.0</span>
      <a href="https://github.com/simhook/simhook">GitHub</a>
      <a href={`${SITE_URL}/docs`}>Docs</a>
      <a href={`${SITE_URL}/changelog`}>Changelog</a>
      <a href={`${SITE_URL}/about`}>About</a>
      <a href="https://github.com/simhook/simhook/blob/main/SECURITY.md">Security</a>
      <a href={`${SITE_URL}/privacy`}>Privacy</a>
      <a href={`${SITE_URL}/terms`}>Terms</a>
    </footer>
  );
}
