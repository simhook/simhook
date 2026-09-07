"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * A row of words that never wraps. When it does not fit, it scrolls
 * sideways, with the scrollbar hidden: the word cut off at the edge says
 * there is more. The current word is brought into view on arrival.
 */
export function NavRow({ activeHref, className, children }: { activeHref?: string; className?: string; children: ReactNode }) {
  const ref = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('[aria-current="page"]')?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [activeHref]);
  return (
    <span ref={ref} className={cn("flex min-w-0 gap-x-5 overflow-x-auto whitespace-nowrap [scrollbar-width:none] [&::-webkit-scrollbar]:hidden", className)}>
      {children}
    </span>
  );
}
