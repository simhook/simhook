"use client";

import { queryOptions, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { MeOutputBody, paths } from "@simhook/contracts";
import { api, isApiError, unwrap } from "./api";

type Body<P extends keyof paths, M extends keyof paths[P]> = paths[P][M] extends {
  requestBody: { content: { "application/json": infer B } };
}
  ? B
  : never;

export const keys = {
  me: ["me"] as const,
  sessions: ["sessions"] as const,
  stats: ["stats"] as const,
  devices: ["devices"] as const,
  device: (id: string) => ["devices", id] as const,
  messages: (filter: MessageFilter) => ["messages", filter] as const,
  message: (id: string) => ["messages", "one", id] as const,
  batches: ["batches"] as const,
  batch: (id: string) => ["batches", id] as const,
  webhooks: ["webhooks"] as const,
  webhook: (id: string) => ["webhooks", id] as const,
  deliveries: (filter: DeliveryFilter) => ["deliveries", filter] as const,
  apiKeys: ["api-keys"] as const,
  plans: ["plans"] as const,
};

// ---------------------------------------------------------------------------
// Account and sessions
// ---------------------------------------------------------------------------

/** The signed-in account with its plan limits and usage. */
export type Account = MeOutputBody;

/**
 * The one query that says who is signed in. A lost session resolves to
 * null rather than throwing: not being signed in is an answer.
 */
export const sessionQueryOptions = queryOptions({
  queryKey: keys.me,
  queryFn: async (): Promise<Account | null> => {
    try {
      return await unwrap(api.GET("/v1/auth/me"));
    } catch (e) {
      if (isApiError(e) && e.isSessionLost) return null;
      throw e;
    }
  },
  staleTime: 60_000,
  retry: false,
});

export function useAuthMutations() {
  const qc = useQueryClient();
  const reset = () => qc.invalidateQueries({ queryKey: keys.me });
  return {
    login: useMutation({
      mutationFn: (body: Body<"/v1/auth/login", "post">) => unwrap(api.POST("/v1/auth/login", { body })),
      onSuccess: reset,
    }),
    register: useMutation({
      mutationFn: (body: Body<"/v1/auth/register", "post">) => unwrap(api.POST("/v1/auth/register", { body })),
      onSuccess: reset,
    }),
    verifyEmail: useMutation({
      mutationFn: (body: Body<"/v1/auth/verify-email", "post">) => unwrap(api.POST("/v1/auth/verify-email", { body })),
      onSuccess: reset,
    }),
    sendVerification: useMutation({
      mutationFn: () => unwrap(api.POST("/v1/auth/verify-email/send")),
    }),
    requestReset: useMutation({
      mutationFn: (body: Body<"/v1/auth/password-reset/request", "post">) =>
        unwrap(api.POST("/v1/auth/password-reset/request", { body })),
    }),
    resetPassword: useMutation({
      mutationFn: (body: Body<"/v1/auth/password-reset", "post">) => unwrap(api.POST("/v1/auth/password-reset", { body })),
    }),
    changePassword: useMutation({
      mutationFn: (body: Body<"/v1/auth/password", "post">) => unwrap(api.POST("/v1/auth/password", { body })),
      // Every other session is signed out by the change.
      onSuccess: () => qc.invalidateQueries({ queryKey: keys.sessions }),
    }),
    updateProfile: useMutation({
      mutationFn: (body: Body<"/v1/auth/profile", "patch">) => unwrap(api.PATCH("/v1/auth/profile", { body })),
      onSuccess: reset,
    }),
  };
}

export function useSessions() {
  return useQuery({ queryKey: keys.sessions, queryFn: () => unwrap(api.GET("/v1/auth/sessions")) });
}

export function useSessionMutations() {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.sessions });
  return {
    revoke: useMutation({
      mutationFn: (id: string) => unwrap(api.DELETE("/v1/auth/sessions/{id}", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
    revokeOthers: useMutation({
      mutationFn: () => unwrap(api.POST("/v1/auth/sessions/revoke-others")),
      onSuccess: refresh,
    }),
  };
}

export function useStats() {
  return useQuery({ queryKey: keys.stats, queryFn: () => unwrap(api.GET("/v1/stats")), staleTime: 15_000 });
}

export function usePlans() {
  return useQuery({ queryKey: keys.plans, queryFn: () => unwrap(api.GET("/v1/plans")), staleTime: 300_000 });
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

export function useApiKeys(includeRevoked = false) {
  return useQuery({
    queryKey: [...keys.apiKeys, includeRevoked],
    queryFn: () => unwrap(api.GET("/v1/api-keys", { params: { query: { include_revoked: includeRevoked } } })),
  });
}

export function useApiKeyMutations() {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.apiKeys });
  return {
    create: useMutation({
      mutationFn: (body: Body<"/v1/api-keys", "post">) => unwrap(api.POST("/v1/api-keys", { body })),
      onSuccess: refresh,
    }),
    rename: useMutation({
      mutationFn: ({ id, name }: { id: string; name: string }) =>
        unwrap(api.PATCH("/v1/api-keys/{id}", { params: { path: { id } }, body: { name } })),
      onSuccess: refresh,
    }),
    revoke: useMutation({
      mutationFn: (id: string) => unwrap(api.POST("/v1/api-keys/{id}/revoke", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: (id: string) => unwrap(api.DELETE("/v1/api-keys/{id}", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
  };
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

export function useDevices(refetchInterval?: number) {
  return useQuery({
    queryKey: keys.devices,
    queryFn: () => unwrap(api.GET("/v1/devices")),
    refetchInterval,
  });
}

export function useDevice(id: string) {
  return useQuery({
    queryKey: keys.device(id),
    queryFn: () => unwrap(api.GET("/v1/devices/{id}", { params: { path: { id } } })),
    enabled: !!id,
  });
}

export function useDeviceMutations() {
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: keys.devices });
    qc.invalidateQueries({ queryKey: keys.stats });
  };
  return {
    createPairingCode: useMutation({
      mutationFn: () => unwrap(api.POST("/v1/devices/pairing-codes")),
    }),
    update: useMutation({
      mutationFn: ({ id, ...body }: { id: string } & Body<"/v1/devices/{id}", "patch">) =>
        unwrap(api.PATCH("/v1/devices/{id}", { params: { path: { id } }, body })),
      onSuccess: refresh,
    }),
    setDefault: useMutation({
      mutationFn: (id: string) => unwrap(api.POST("/v1/devices/{id}/default", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
    unpair: useMutation({
      mutationFn: (id: string) => unwrap(api.DELETE("/v1/devices/{id}", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
  };
}

// ---------------------------------------------------------------------------
// Messages and sends
// ---------------------------------------------------------------------------

export type MessageFilter = {
  direction?: "outbound" | "inbound";
  status?: string;
  device_ids?: string;
  batch_id?: string;
  q?: string;
  from?: string;
  to?: string;
};

export function useMessages(filter: MessageFilter, limit = 50) {
  return useInfiniteQuery({
    queryKey: keys.messages(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/messages", {
          params: {
            query: {
              ...Object.fromEntries(Object.entries(filter).filter(([, v]) => v !== undefined && v !== "")),
              cursor: pageParam || undefined,
              limit,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: 10_000,
  });
}

export function useMessage(id: string) {
  return useQuery({
    queryKey: keys.message(id),
    queryFn: () => unwrap(api.GET("/v1/messages/{id}", { params: { path: { id } } })),
    enabled: !!id,
  });
}

export function useBatches(limit = 50) {
  return useInfiniteQuery({
    queryKey: keys.batches,
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/batches", { params: { query: { cursor: pageParam || undefined, limit } } })),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: 10_000,
  });
}

export function useBatch(id: string, live = false) {
  return useQuery({
    queryKey: keys.batch(id),
    queryFn: () => unwrap(api.GET("/v1/batches/{id}", { params: { path: { id } } })),
    enabled: !!id,
    refetchInterval: live ? 3_000 : false,
  });
}

export function useSendMessage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Body<"/v1/messages", "post">) => unwrap(api.POST("/v1/messages", { body })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["messages"] });
      qc.invalidateQueries({ queryKey: keys.batches });
      qc.invalidateQueries({ queryKey: keys.me });
      qc.invalidateQueries({ queryKey: keys.stats });
    },
  });
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

export function useWebhooks() {
  return useQuery({ queryKey: keys.webhooks, queryFn: () => unwrap(api.GET("/v1/webhooks")) });
}

export type DeliveryFilter = {
  webhook_id?: string;
  status?: string;
  event?: string;
};

export function useDeliveries(filter: DeliveryFilter, limit = 50) {
  return useInfiniteQuery({
    queryKey: keys.deliveries(filter),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/webhooks/deliveries", {
          params: {
            query: {
              ...Object.fromEntries(Object.entries(filter).filter(([, v]) => v !== undefined && v !== "")),
              cursor: pageParam || undefined,
              limit,
            },
          },
        }),
      ),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: 10_000,
  });
}

export function useWebhookMutations() {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.webhooks });
  return {
    create: useMutation({
      mutationFn: (body: Body<"/v1/webhooks", "post">) => unwrap(api.POST("/v1/webhooks", { body })),
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: ({ id, ...body }: { id: string } & Body<"/v1/webhooks/{id}", "patch">) =>
        unwrap(api.PATCH("/v1/webhooks/{id}", { params: { path: { id } }, body })),
      onSuccess: refresh,
    }),
    rotateSecret: useMutation({
      mutationFn: (id: string) => unwrap(api.POST("/v1/webhooks/{id}/rotate-secret", { params: { path: { id } } })),
    }),
    test: useMutation({
      mutationFn: (id: string) => unwrap(api.POST("/v1/webhooks/{id}/test", { params: { path: { id } } })),
      onSuccess: () => qc.invalidateQueries({ queryKey: ["deliveries"] }),
    }),
    remove: useMutation({
      mutationFn: (id: string) => unwrap(api.DELETE("/v1/webhooks/{id}", { params: { path: { id } } })),
      onSuccess: refresh,
    }),
  };
}
