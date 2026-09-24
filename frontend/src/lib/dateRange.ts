// Day-range presets shared by the pages that filter figures by date: the
// team page, the profile record and the call list. Values are local
// YYYY-MM-DD, inclusive on both ends.

export type Preset = "today" | "yesterday" | "last7" | "last30" | "month" | "day" | "custom";

export const PRESETS: { key: Preset; label: string }[] = [
  { key: "today", label: "Bugün" },
  { key: "yesterday", label: "Dün" },
  { key: "last7", label: "Son 7 gün" },
  { key: "last30", label: "Son 30 gün" },
  { key: "month", label: "Bu ay" },
  { key: "day", label: "Belirli gün" },
  { key: "custom", label: "Tarih aralığı" },
];

// ymd formats a Date as a local YYYY-MM-DD.
export function ymd(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// dmy turns YYYY-MM-DD into dd.mm.yyyy.
export function dmy(iso: string): string {
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
}

// presetRange resolves a preset to an inclusive local [from, to]. "day" and
// "custom" keep whatever is already picked, so they resolve to today.
export function presetRange(key: Preset): { from: string; to: string } {
  const now = new Date();
  const today = ymd(now);
  const shift = (days: number) => ymd(new Date(now.getFullYear(), now.getMonth(), now.getDate() - days));
  switch (key) {
    case "yesterday": return { from: shift(1), to: shift(1) };
    case "last7": return { from: shift(6), to: today };
    case "last30": return { from: shift(29), to: today };
    case "month": return { from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: today };
    default: return { from: today, to: today };
  }
}

// rangeLabel is the short human form: "bugün", "12.03.2026", "01.03.2026 - 12.03.2026".
export function rangeLabel(from: string, to: string): string {
  const today = ymd(new Date());
  if (from === today && to === today) return "bugün";
  if (from === to) return dmy(from);
  return `${dmy(from)} - ${dmy(to)}`;
}
