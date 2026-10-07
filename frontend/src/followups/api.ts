// The follow-up endpoints: the customers the team still owes a call, what
// the team knows about a number during a call, and each person's notices.

import { request } from "@/api/client";

export interface FollowPerson {
  id: number;
  name: string;
}

export interface Unreached {
  id: number;
  number: string;
  user: FollowPerson;
  mine: boolean;
  attempts: number;
  firstAt: string;
  lastAt: string;
  // why the last try did not get through: no_answer, busy, canceled, failed, short
  reason: string;
  status: "open" | "reached" | "dropped";
  reached?: { by: FollowPerson; direction: string; seconds: number; at: string };
  claim?: { by: FollowPerson; until: string; mine: boolean };
  droppedBy?: FollowPerson;
  droppedAt?: string;
}

export interface PeerContext {
  // the calls the number made today before the current one
  inboundBefore: number;
  lastTalk?: { by: FollowPerson; mine: boolean; at: string; seconds: number; direction: string };
  unreached: Unreached[];
}

export interface Notice {
  id: number;
  kind: string;
  text: string;
  link?: string;
  createdAt: string;
  read: boolean;
}

export const followApi = {
  unreached: (all: boolean, closed: boolean) =>
    request<Unreached[]>(`/followups/unreached?scope=${all ? "all" : "mine"}&state=${closed ? "closed" : "open"}`),
  claim: (id: number) => request<void>(`/followups/unreached/${id}/claim`, { method: "POST" }),
  unclaim: (id: number) => request<void>(`/followups/unreached/${id}/claim`, { method: "DELETE" }),
  drop: (id: number) => request<void>(`/followups/unreached/${id}/drop`, { method: "POST" }),
  peer: (number: string) => request<PeerContext>(`/followups/peer?number=${encodeURIComponent(number)}`),
  notices: (after = 0) => request<{ items: Notice[]; unread: number }>(`/notices${after ? `?after=${after}` : ""}`),
  readNotices: (body: { ids?: number[]; all?: boolean }) => request<void>("/notices/read", { method: "POST", body: JSON.stringify(body) }),
};

export const REASON_LABEL: Record<string, string> = {
  no_answer: "Cevap vermedi",
  busy: "Meşgul",
  canceled: "Çalarken kapatıldı",
  failed: "Arama başarısız",
  short: "Santral anonsu",
};

// talkLabel writes a length as minutes and seconds.
export function talkLabel(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m === 0) return `${s} sn`;
  return s === 0 ? `${m} dk` : `${m} dk ${s} sn`;
}
