// Small helpers for the WhatsApp screens: which list a conversation
// belongs to, times in words, and WhatsApp's own text styling.

import type { WAConversation, WAMessage } from "@/whatsapp/types";

export type Bucket = "mine" | "waiting" | "pool" | "team" | "resolved";

export function isMine(c: WAConversation, me: number): boolean {
  const t = c.ticket;
  if (!t) return false;
  return t.owner?.id === me || t.participants.some((p) => p.id === me);
}

export function isWaiting(c: WAConversation): boolean {
  return !!c.ticket?.waitingListedAt && c.ticket.status !== "resolved";
}

export function isPool(c: WAConversation): boolean {
  const t = c.ticket;
  return !!t && !t.owner && (t.status === "open" || t.status === "pending");
}

export function isResolved(c: WAConversation): boolean {
  return c.ticket?.status === "resolved";
}

export function inBucket(c: WAConversation, b: Bucket, me: number): boolean {
  switch (b) {
    case "mine":
      return isMine(c, me) && !isResolved(c);
    case "waiting":
      return isWaiting(c);
    case "pool":
      return isPool(c);
    case "team":
      return !isResolved(c);
    case "resolved":
      return isResolved(c);
  }
}

export function sortTime(c: WAConversation): number {
  return Date.parse(c.last?.at ?? c.ticket?.createdAt ?? "") || 0;
}

export function windowLeft(c: WAConversation, now: number): number {
  if (!c.windowEndsAt) return 0;
  return Math.max(0, Date.parse(c.windowEndsAt) - now);
}

export function hm(ms: number): string {
  const m = Math.floor(ms / 60000);
  if (m < 60) return `${m} dk`;
  const h = Math.floor(m / 60);
  return `${h} sa ${String(m % 60).padStart(2, "0")} dk`;
}

export function since(iso: string | undefined, now: number): string {
  if (!iso) return "";
  const s = Math.max(0, Math.floor((now - Date.parse(iso)) / 1000));
  if (s < 60) return "az önce";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} dk`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} sa`;
  return `${Math.floor(h / 24)} gün`;
}

export function clock(iso: string): string {
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

export function listTime(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return clock(iso);
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return "Dün";
  return d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" });
}

export function dayLabel(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return "Bugün";
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  if (d.toDateString() === y.toDateString()) return "Dün";
  return d.toLocaleDateString("tr-TR", { day: "numeric", month: "long", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
}

export function prettyPhone(waId: string): string {
  const d = waId.replace(/\D/g, "");
  if (d.startsWith("90") && d.length === 12) return `+90 ${d.slice(2, 5)} ${d.slice(5, 8)} ${d.slice(8, 10)} ${d.slice(10)}`;
  return "+" + d;
}

export function newClientId(): string {
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export const STATUS_WORD: Record<string, string> = {
  bot: "Chatbot'ta",
  open: "Açık",
  pending: "Müşteri bekleniyor",
  resolved: "Çözüldü",
};

export const PRIORITY_WORD: Record<string, string> = {
  low: "Düşük",
  normal: "Normal",
  high: "Yüksek",
  urgent: "Acil",
};

export const TRIGGER_WORD: Record<string, string> = {
  message_in: "Müşteri mesaj yazınca",
  first_message: "Müşteri ilk kez yazınca",
  ticket_created: "Yeni sohbet açılınca",
  ticket_reopened: "Çözülmüş sohbet yeniden açılınca",
  ticket_assigned: "Sohbet birine atanınca",
  ticket_resolved: "Sohbet çözülünce",
  outside_hours: "Mesai dışında mesaj gelince",
  no_reply: "Müşteri belli bir süre cevap alamayınca",
};

// Menu choices a customer can pick, from an outgoing interactive message.
export function menuOptions(m: WAMessage): { title: string; description?: string }[] {
  const p = m.payload as { interactive?: { type?: string; action?: { buttons?: { reply?: { title?: string } }[]; sections?: { rows?: { title?: string; description?: string }[] }[] } } } | undefined;
  const a = p?.interactive?.action;
  if (!a) return [];
  if (a.buttons) return a.buttons.map((b) => ({ title: b.reply?.title ?? "" }));
  return (a.sections ?? []).flatMap((s) => (s.rows ?? []).map((r) => ({ title: r.title ?? "", description: r.description })));
}

export function menuText(m: WAMessage): string {
  const p = m.payload as { interactive?: { body?: { text?: string } } } | undefined;
  return p?.interactive?.body?.text ?? m.body;
}
