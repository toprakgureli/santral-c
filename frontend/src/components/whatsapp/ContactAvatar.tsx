// ContactAvatar is a customer's round badge: their initials on a soft
// colour picked from their number, so the same customer always looks the
// same.

import { cn } from "@/lib/utils";

const COLORS = [
  "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  "bg-sky-500/15 text-sky-600 dark:text-sky-400",
  "bg-violet-500/15 text-violet-600 dark:text-violet-400",
  "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  "bg-rose-500/15 text-rose-600 dark:text-rose-400",
  "bg-teal-500/15 text-teal-600 dark:text-teal-400",
  "bg-indigo-500/15 text-indigo-600 dark:text-indigo-400",
];

function initials(name: string): string {
  const parts = name.replace(/^\+/, "").trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (/^\d/.test(parts[0])) return parts[0].slice(-2);
  return parts.slice(0, 2).map((p) => p[0]?.toLocaleUpperCase("tr") ?? "").join("");
}

export default function ContactAvatar({ name, seed, className, children }: { name: string; seed: string; className?: string; children?: React.ReactNode }) {
  let h = 0;
  for (const ch of seed) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return (
    <span className={cn("relative inline-flex size-10 shrink-0 items-center justify-center rounded-full text-sm font-semibold", COLORS[h % COLORS.length], className)}>
      {initials(name)}
      {children}
    </span>
  );
}
