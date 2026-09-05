import type { ReactNode } from "react";
import { errorMessage } from "@/lib/api";

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: ReactNode }) {
  return (
    <div className="mb-8 mt-12 flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
      <div className="max-w-[62ch]">
        <h1 className="text-2xl font-semibold leading-tight tracking-tight">{title}</h1>
        {description ? <p className="mt-1.5 text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-4">{actions}</div> : null}
    </div>
  );
}

export function EmptyState({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="border-y py-8 text-sm">
      <p className="font-medium">{title}</p>
      {description ? <p className="mt-1 max-w-[62ch] text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/** A list that could not be loaded says so, in place, with a way to try again. */
export function LoadError({ error, retry }: { error: unknown; retry: () => void }) {
  return (
    <p className="border-y py-6 text-sm">
      <span className="text-destructive">{errorMessage(error)}</span>{" "}
      <button type="button" className="underline decoration-underline underline-offset-4 hover:decoration-foreground" onClick={retry}>
        Try again
      </button>
    </p>
  );
}

/** The one text-link style every page uses for an action that is not the page's button. */
export const textLink = "text-sm underline decoration-underline underline-offset-4 hover:decoration-foreground disabled:opacity-50";
