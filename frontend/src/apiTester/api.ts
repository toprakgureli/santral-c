import { request } from "@/api/client";
import type {
  ApiCollectionItem,
  ApiRequestItem,
  CollectionInput,
  CollectionListResponse,
  ExecutionResult,
  RequestInput,
  RunLogItem,
} from "./types";

export const apiTesterApi = {
  status: () => request<{ enabled: boolean }>("/api-tester/status"),
  setStatus: (enabled: boolean) =>
    request<{ enabled: boolean }>("/api-tester/status", {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  collections: () => request<CollectionListResponse>("/api-tester/collections"),
  createCollection: (body: CollectionInput) =>
    request<ApiCollectionItem>("/api-tester/collections", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateCollection: (id: number, body: CollectionInput) =>
    request<ApiCollectionItem>(`/api-tester/collections/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteCollection: (id: number) =>
    request<void>(`/api-tester/collections/${id}`, { method: "DELETE" }),
  createRequest: (collectionId: number, body: RequestInput) =>
    request<ApiRequestItem>(`/api-tester/collections/${collectionId}/requests`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateRequest: (id: number, body: RequestInput) =>
    request<ApiRequestItem>(`/api-tester/requests/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteRequest: (id: number) =>
    request<void>(`/api-tester/requests/${id}`, { method: "DELETE" }),
  executeProxy: (id: number, variables: Record<string, string>) =>
    request<ExecutionResult>(`/api-tester/requests/${id}/proxy`, {
      method: "POST",
      body: JSON.stringify({ variables }),
    }),
  logRun: (
    id: number,
    body: {
      method: string;
      targetUrl: string;
      executionMode: "extension" | "browser";
      statusCode: number;
      durationMs: number;
      errorMessage?: string;
    },
  ) =>
    request<void>(`/api-tester/requests/${id}/log`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  logs: (requestId?: number, limit = 25) => {
    const qs = new URLSearchParams({ limit: String(limit) });
    if (requestId) qs.set("requestId", String(requestId));
    return request<{ items: RunLogItem[] }>(`/api-tester/logs?${qs.toString()}`).then((r) => r.items ?? []);
  },
};
