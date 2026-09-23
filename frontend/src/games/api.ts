// The games' HTTP calls, kept apart from the chat client.

import { request } from "@/api/client";
import type { GameItem, GameRecord, GameSettings, GamesConfig, GameView, LeaderRow } from "@/games/types";

export const gamesApi = {
  config: () => request<GamesConfig>("/games/config"),
  updateSettings: (s: GameSettings) => request<void>("/games/settings", { method: "PUT", body: JSON.stringify(s) }),
  items: (kind: string) => request<{ items: GameItem[] }>(`/games/items?kind=${encodeURIComponent(kind)}`).then((r) => r.items),
  createItem: (kind: string, body: Partial<GameItem>) => request<GameItem>(`/games/items?kind=${encodeURIComponent(kind)}`, { method: "POST", body: JSON.stringify(body) }),
  updateItem: (id: number, body: Partial<GameItem>) => request<void>(`/games/items/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteItem: (id: number) => request<void>(`/games/items/${id}`, { method: "DELETE" }),
  importItems: (kind: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<{ added: number }>(`/games/items/import?kind=${encodeURIComponent(kind)}`, { method: "POST", body: form });
  },
  create: (groupId: number, body: { kind: string; rounds?: number; seconds?: number }) => request<GameView>(`/games/groups/${groupId}`, { method: "POST", body: JSON.stringify(body) }),
  open: (groupId: number) => request<{ items: GameView[] }>(`/games/groups/${groupId}/open`).then((r) => r.items),
  get: (id: number) => request<GameView>(`/games/${id}`),
  join: (id: number) => request<GameView>(`/games/${id}/join`, { method: "POST" }),
  leave: (id: number) => request<GameView>(`/games/${id}/leave`, { method: "POST" }),
  start: (id: number) => request<GameView>(`/games/${id}/start`, { method: "POST" }),
  cancel: (id: number) => request<void>(`/games/${id}/cancel`, { method: "POST" }),
  pause: (id: number, paused: boolean) => request<GameView>(`/games/${id}/pause`, { method: "POST", body: JSON.stringify({ paused }) }),
  act: (id: number, action: string, payload: unknown = {}) => request<GameView>(`/games/${id}/action`, { method: "POST", body: JSON.stringify({ action, payload }) }),
  leaderboard: (period: "month" | "all", kind = "") => request<{ items: LeaderRow[] }>(`/games/leaderboard?period=${period}&kind=${encodeURIComponent(kind)}`).then((r) => r.items),
  record: (userId: number) => request<GameRecord>(`/games/record/${userId}`),
};
