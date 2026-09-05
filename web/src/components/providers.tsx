"use client";

import type { ReactNode } from "react";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SessionProvider } from "@/components/session-provider";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { isApiError } from "@/lib/api";
import { keys } from "@/lib/queries";

let browserQueryClient: QueryClient | undefined;

function makeQueryClient() {
  // Any request that finds the session gone tells the account query so, and
  // nothing else: the shell reacts to that one value.
  const sessionLost = (error: unknown) => {
    if (isApiError(error) && error.isSessionLost) client.setQueryData(keys.me, null);
  };
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError: sessionLost }),
    mutationCache: new MutationCache({ onError: sessionLost }),
    defaultOptions: {
      queries: {
        staleTime: 10_000,
        // A 401 or 404 is an answer, not a transient failure.
        retry: (count, error) => !(isApiError(error) && error.status < 500) && count < 2,
        refetchOnWindowFocus: true,
      },
    },
  });
  return client;
}

function getQueryClient() {
  if (typeof window === "undefined") return makeQueryClient();
  browserQueryClient ??= makeQueryClient();
  return browserQueryClient;
}

export function Providers({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={getQueryClient()}>
      <SessionProvider>
        <TooltipProvider delay={300}>{children}</TooltipProvider>
      </SessionProvider>
      <Toaster position="bottom-right" closeButton />
    </QueryClientProvider>
  );
}
