"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";

/** A word that copies a value: "Copy", then "Copied" for a moment. */
export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={cn("text-sm underline decoration-underline underline-offset-4 hover:decoration-foreground", className)}
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
      {copied ? "Copied" : label}
    </button>
  );
}

/** A value in mono with a Copy word after it. Secrets select in one click. */
export function CopyField({ value, secret = false }: { value: string; secret?: boolean }) {
  return (
    <div className="flex items-baseline gap-4 border-y py-2.5">
      <code className={cn("min-w-0 flex-1 truncate font-mono text-sm", secret && "select-all")}>{value}</code>
      <CopyButton value={value} />
    </div>
  );
}
