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

// A call back someone planned for a time.
export interface Reminder {
  id: number;
  number: string;
  note?: string;
  dueAt: string;
  user: FollowPerson;
  mine: boolean;
  snoozes: number;
  createdAt: string;
  doneAt?: string;
  // done, canceled, or reached (anyone had a real conversation with the number)
  doneReason?: string;
  doneBy?: FollowPerson;
}

export interface PeerContext {
  // the calls the number made today before the current one
  inboundBefore: number;
  lastTalk?: { by: FollowPerson; mine: boolean; at: string; seconds: number; direction: string };
  unreached: Unreached[];
  reminders: Reminder[];
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
  unreached: (all: boolean, closed: boolean, roleId?: number | "all") => {
    let url = `/followups/unreached?scope=${all ? "all" : "mine"}&state=${closed ? "closed" : "open"}`;
    if (roleId !== undefined && roleId !== "all") {
      url += `&role=${roleId}`;
    }
    return request<Unreached[]>(url);
  },
  claim: (id: number) => request<void>(`/followups/unreached/${id}/claim`, { method: "POST" }),
  unclaim: (id: number) => request<void>(`/followups/unreached/${id}/claim`, { method: "DELETE" }),
  drop: (id: number) => request<void>(`/followups/unreached/${id}/drop`, { method: "POST" }),
  reminders: (all: boolean, done: boolean) =>
    request<Reminder[]>(`/followups/reminders?scope=${all ? "all" : "mine"}&state=${done ? "done" : "open"}`),
  createReminder: (body: { number: string; note: string; dueAt: string }) => request<Reminder>("/followups/reminders", { method: "POST", body: JSON.stringify(body) }),
  snooze: (id: number, minutes: number) => request<void>(`/followups/reminders/${id}/snooze`, { method: "POST", body: JSON.stringify({ minutes }) }),
  doneReminder: (id: number) => request<void>(`/followups/reminders/${id}/done`, { method: "POST" }),
  cancelReminder: (id: number) => request<void>(`/followups/reminders/${id}/cancel`, { method: "POST" }),
  peer: (number: string) => request<PeerContext>(`/followups/peer?number=${encodeURIComponent(number)}`),
  notices: (after = 0) => request<{ items: Notice[]; unread: number }>(`/notices${after ? `?after=${after}` : ""}`),
  readNotices: (body: { ids?: number[]; all?: boolean }) => request<void>("/notices/read", { method: "POST", body: JSON.stringify(body) }),
};

// FOLLOWUPS_CHANGED is announced on window when a call back or a follow-up
// changed somewhere in the panel, so every list showing them reads again.
export const FOLLOWUPS_CHANGED = "santral:followups-changed";

export function announceFollowupsChanged() {
  window.dispatchEvent(new Event(FOLLOWUPS_CHANGED));
}

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
