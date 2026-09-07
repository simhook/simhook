import type { ReactNode } from "react";
import Link from "next/link";
import { NavRow } from "@/components/nav-row";
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

/**
 * The bar: the wordmark, the words, and the account at the right. Two rows
 * below 1024 px (wordmark and account, then the words), one row above. The
 * words never wrap: a row that does not fit scrolls sideways.
 */
export function Bar({
  links,
  right,
  isActive,
}: {
  links: ReadonlyArray<{ href: string; label: string }>;
  right?: ReactNode;
  isActive?: (href: string) => boolean;
}) {
  const activeHref = links.find(({ href }) => isActive?.(href))?.href;
  return (
    <nav className="grid grid-cols-[1fr_auto] items-baseline gap-y-2.5 border-b py-4 text-[14px] lg:flex lg:gap-x-5" aria-label="Main">
      <a href={SITE_URL} className="mr-1 font-mono text-[15px] font-medium text-foreground">
        simhook
      </a>
      <NavRow activeHref={activeHref} className="order-last col-span-2 lg:order-none lg:col-span-1 lg:flex-1">
        {links.map(({ href, label }) => {
          const active = href === activeHref;
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
      </NavRow>
      {right ? <span className="flex items-center gap-[18px] justify-self-end whitespace-nowrap text-muted-foreground lg:ml-auto">{right}</span> : null}
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
