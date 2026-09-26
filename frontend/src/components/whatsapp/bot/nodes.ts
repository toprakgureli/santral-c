// The kinds of boxes a chatbot flow is drawn with: how each is named,
// coloured and previewed, which exits it has, and what a new one holds.

import { CircleDot, Flag, GitFork, Globe, HelpCircle, ListChecks, MessageSquareText, PhoneCall, Star, Tag, UserRound, type LucideIcon } from "lucide-react";
import type { BotData, BotNode, BotNodeType } from "@/whatsapp/types";

export interface NodeKind {
  label: string;
  hint: string;
  icon: LucideIcon;
  // tailwind classes: the icon chip and the accent bar
  chip: string;
  bar: string;
  data: () => BotData;
}

export const KINDS: Record<BotNodeType, NodeKind> = {
  start: { label: "Başlangıç", hint: "Akış buradan başlar.", icon: CircleDot, chip: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400", bar: "bg-emerald-500", data: () => ({}) },
  message: { label: "Mesaj", hint: "Müşteriye bir mesaj ya da dosya gönderir, sonra devam eder.", icon: MessageSquareText, chip: "bg-sky-500/15 text-sky-600 dark:text-sky-400", bar: "bg-sky-500", data: () => ({ text: "" }) },
  menu: { label: "Menü", hint: "Seçenek sunar. Her seçenek ayrı bir yola gider.", icon: ListChecks, chip: "bg-violet-500/15 text-violet-600 dark:text-violet-400", bar: "bg-violet-500", data: () => ({ text: "Size nasıl yardımcı olabiliriz?", style: "buttons", options: [{ id: rid(), label: "Seçenek 1" }, { id: rid(), label: "Seçenek 2" }] }) },
  ask: { label: "Soru", hint: "Bir şey sorar, cevabı bir değişkene kaydeder.", icon: HelpCircle, chip: "bg-amber-500/15 text-amber-600 dark:text-amber-400", bar: "bg-amber-500", data: () => ({ text: "", var: "", validate: "any" }) },
  condition: { label: "Koşul", hint: "Bir bilgiye bakar, evet ya da hayır yoluna gider.", icon: GitFork, chip: "bg-orange-500/15 text-orange-600 dark:text-orange-400", bar: "bg-orange-500", data: () => ({ match: "all", rules: [{ var: "", op: "hours_open", value: "" }] }) },
  api: { label: "Dış sorgu", hint: "Başka bir sisteme sorar, cevabı değişkenlere yazar.", icon: Globe, chip: "bg-fuchsia-500/15 text-fuchsia-600 dark:text-fuchsia-400", bar: "bg-fuchsia-500", data: () => ({ map: [] }) },
  tag: { label: "Etiketle", hint: "Sohbete etiket, öncelik ya da konu yazar.", icon: Tag, chip: "bg-slate-500/15 text-slate-600 dark:text-slate-300", bar: "bg-slate-500", data: () => ({ tags: [] }) },
  handoff: { label: "Temsilciye aktar", hint: "Chatbot'u bitirir, sohbeti bir kişiye verir.", icon: UserRound, chip: "bg-rose-500/15 text-rose-600 dark:text-rose-400", bar: "bg-rose-500", data: () => ({ text: "Sizi bir arkadaşımıza aktarıyorum, birazdan yazacak." }) },
  callback: { label: "Geri arama", hint: "Müşteri için geri arama talebi açar.", icon: PhoneCall, chip: "bg-teal-500/15 text-teal-600 dark:text-teal-400", bar: "bg-teal-500", data: () => ({ text: "Talebinizi aldık, sizi en kısa sürede arayacağız.", note: "" }) },
  survey: { label: "Anket", hint: "1'den 5'e puan ister.", icon: Star, chip: "bg-yellow-500/15 text-yellow-600 dark:text-yellow-400", bar: "bg-yellow-500", data: () => ({}) },
  end: { label: "Bitir", hint: "Chatbot'u bitirir. İsterseniz sohbeti de kapatır.", icon: Flag, chip: "bg-zinc-500/15 text-zinc-600 dark:text-zinc-300", bar: "bg-zinc-500", data: () => ({ text: "", resolve: true }) },
};

export const PALETTE: BotNodeType[] = ["message", "menu", "ask", "condition", "api", "tag", "handoff", "callback", "survey", "end"];

export interface Port {
  id: string;
  label: string;
  tone?: "ok" | "bad" | "muted";
}

export function portsOf(n: BotNode): Port[] {
  switch (n.type) {
    case "handoff":
    case "end":
      return [];
    case "menu":
      return [...(n.data.options ?? []).map((o, i) => ({ id: o.id, label: o.label || `Seçenek ${i + 1}` })), { id: "fallback", label: "Anlaşılmazsa", tone: "muted" as const }];
    case "ask":
      return [{ id: "next", label: "Cevap gelince" }, { id: "fallback", label: "Anlaşılmazsa", tone: "muted" }];
    case "condition":
      return [{ id: "yes", label: "Evet", tone: "ok" }, { id: "no", label: "Hayır", tone: "bad" }];
    case "api":
      return [{ id: "ok", label: "Başarılı", tone: "ok" }, { id: "fail", label: "Başarısız", tone: "bad" }];
    default:
      return [{ id: "next", label: "Sonra" }];
  }
}

const OPS: Record<string, string> = {
  equals: "eşitse",
  not_equals: "eşit değilse",
  contains: "içeriyorsa",
  gt: "büyükse",
  lt: "küçükse",
  exists: "doluysa",
  empty: "boşsa",
};
export const OP_WORD = OPS;

export function preview(n: BotNode, names: { integrations: Record<number, string>; teams: Record<number, string> }): string {
  const d = n.data;
  switch (n.type) {
    case "start":
      return "Müşteri yazınca";
    case "message":
      return d.mediaUrl ? `📎 ${d.text || "dosya"}` : d.text || "Metin yazılmadı";
    case "menu":
      return d.text || "Soru yazılmadı";
    case "ask":
      return d.text ? `${d.text}${d.var ? ` → {${d.var}}` : ""}` : "Soru yazılmadı";
    case "condition":
      return (d.rules ?? []).map((r) => (r.op === "hours_open" ? "mesai içiyse" : r.op === "hours_closed" ? "mesai dışıysa" : `{${r.var || "?"}} ${OPS[r.op] ?? r.op}${r.op === "exists" || r.op === "empty" ? "" : ` ${r.value}`}`)).join(d.match === "any" ? " ya da " : " ve ") || "Şart yok";
    case "api":
      return d.integration ? names.integrations[d.integration] ?? "Sorgu seçildi" : "Sorgu seçilmedi";
    case "tag":
      return [...(d.tags ?? []), d.priority && `öncelik: ${d.priority}`, d.category && `konu: ${d.category}`].filter(Boolean).join(", ") || "Bir şey seçilmedi";
    case "handoff":
      return d.teamId ? `${names.teams[d.teamId] ?? "ekip"} ekibine` : "Müsait temsilciye";
    case "callback":
      return d.note || d.text || "Geri arama talebi";
    case "survey":
      return "1-5 puan listesi";
    case "end":
      return d.resolve ? "Bitir ve sohbeti kapat" : "Bitir, sohbet açık kalsın";
  }
  return "";
}

export function rid(): string {
  return Math.random().toString(36).slice(2, 9);
}

// Box geometry. Fixed heights keep the exits at known places so arrows can
// be drawn without measuring the page.
export const W = 248;
export const HEAD = 38;
export const BODY = 46;
export const ROW = 26;

export function heightOf(n: BotNode): number {
  const p = portsOf(n).length;
  return HEAD + BODY + (p ? p * ROW + 8 : 6);
}

export function portPoint(n: BotNode, portId: string): { x: number; y: number } | null {
  const i = portsOf(n).findIndex((p) => p.id === portId || (portId === "" && p.id === "next"));
  if (i < 0) return null;
  return { x: n.x + W, y: n.y + HEAD + BODY + 4 + i * ROW + ROW / 2 };
}

export function inPoint(n: BotNode): { x: number; y: number } {
  return { x: n.x, y: n.y + HEAD / 2 };
}
