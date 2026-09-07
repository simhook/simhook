"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";

/**
 * A word that copies a value: "Copy", then "Copied" for a moment. The wider
 * word reserves the space, so the swap moves nothing around it.
 */
export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cn("grid shrink-0 text-right text-sm underline decoration-underline underline-offset-4 hover:decoration-foreground", className)}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        } catch {
          /* clipboard unavailable; the value is still visible on screen */
        }
      }}
    >
      <span className="invisible col-start-1 row-start-1" aria-hidden="true">
        Copied
      </span>
      <span className="invisible col-start-1 row-start-1" aria-hidden="true">
        {label}
      </span>
      <span className="col-start-1 row-start-1">{copied ? "Copied" : label}</span>
    </button>
  );
}

/**
 * A value in mono with a Copy word after it, shown whole: a value one has
 * to copy is never cut short, it wraps. Secrets select in one click.
 */
export function CopyField({ value, secret = false }: { value: string; secret?: boolean }) {
  return (
    <div className="flex min-w-0 items-baseline gap-4 border-y py-2.5">
      <code className={cn("min-w-0 flex-1 font-mono text-sm break-all", secret && "select-all")}>{value}</code>
      <CopyButton value={value} />
    </div>
  );
}
