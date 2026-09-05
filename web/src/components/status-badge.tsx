import { cn } from "@/lib/utils";
import { statusTone } from "@/lib/format";

const dotClass: Record<ReturnType<typeof statusTone>, string> = {
  ok: "bg-ok",
  warn: "bg-warn",
  bad: "bg-destructive",
  muted: "bg-[#c9c9c5]",
};

/** A status is a dot next to a word. */
export function StatusBadge({ status, label, className }: { status: string; label?: string; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-2 whitespace-nowrap text-sm", className)}>
      <span className={cn("size-[7px] shrink-0 rounded-full", dotClass[statusTone(status)])} />
      {label ?? status}
    </span>
  );
}

/** Presence, for phones. */
export function OnlineDot({ online }: { online: boolean }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm">
      <span className={cn("size-[7px] rounded-full", online ? "bg-ok" : "bg-[#c9c9c5]")} />
      {online ? "Online" : "Offline"}
    </span>
  );
}
