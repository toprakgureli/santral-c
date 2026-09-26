// Time pieces shared by working hours, chatbot hours and the chatbot's
// time condition. Everything is 24-hour Turkey time, whatever language the
// browser speaks.

import { useEffect, useState } from "react";
import { Plus, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { TimeSpan } from "@/whatsapp/types";

export const DAY_SHORT = ["Pzt", "Sal", "Çar", "Per", "Cum", "Cmt", "Paz"];

// normalizeClock turns what was typed ("9", "930", "18.5", "7:05") into
// "09:00" style, or null when it cannot be read.
export function normalizeClock(raw: string): string | null {
  const t = raw.trim().replace(/[.,\s]/g, ":");
  let h: number, m: number;
  const parts = t.split(":");
  if (parts.length === 2 && parts[0] !== "") {
    h = Number(parts[0]);
    m = Number(parts[1] || 0);
    if (parts[1].length === 1) m *= 10;
  } else if (/^\d{1,4}$/.test(t)) {
    if (t.length <= 2) {
      h = Number(t);
      m = 0;
    } else {
      h = Number(t.slice(0, t.length - 2));
      m = Number(t.slice(-2));
    }
  } else {
    return null;
  }
  if (!Number.isInteger(h) || !Number.isInteger(m) || h < 0 || h > 23 || m < 0 || m > 59) return null;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

function shift(clock: string, minutes: number): string {
  const [h, m] = clock.split(":").map(Number);
  const total = (((h * 60 + m + minutes) % 1440) + 1440) % 1440;
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

// TimeInput is a 24-hour clock box. Typing "930" gives 09:30; the arrow keys
// move it by 15 minutes (an hour with Shift).
export function TimeInput({ value, onChange, disabled, className, label }: { value: string; onChange: (v: string) => void; disabled?: boolean; className?: string; label?: string }) {
  const [draft, setDraft] = useState(value);
  const [bad, setBad] = useState(false);
  useEffect(() => {
    setDraft(value);
    setBad(false);
  }, [value]);
  const commit = () => {
    const v = normalizeClock(draft);
    if (v === null) {
      if (draft.trim() === "") setDraft(value);
      else setBad(true);
      return;
    }
    setBad(false);
    setDraft(v);
    if (v !== value) onChange(v);
  };
  return (
    <input
      type="text"
      inputMode="numeric"
      aria-label={label}
      disabled={disabled}
      value={draft}
      maxLength={5}
      placeholder="09:00"
      onChange={(e) => {
        let v = e.target.value.replace(/[^\d:.,]/g, "");
        if (/^\d{4}$/.test(v)) v = `${v.slice(0, 2)}:${v.slice(2)}`;
        setDraft(v);
        setBad(false);
      }}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
        if (e.key === "ArrowUp" || e.key === "ArrowDown") {
          e.preventDefault();
          const base = normalizeClock(draft) ?? value ?? "09:00";
          const next = shift(base, (e.key === "ArrowUp" ? 1 : -1) * (e.shiftKey ? 60 : 15));
          setDraft(next);
          onChange(next);
        }
      }}
      data-tip={bad ? "Saati 09:00 gibi yazın" : undefined}
      className={cn(
        "h-9 w-[4.5rem] rounded-lg border bg-card px-2 text-center text-sm font-medium tabular-nums outline-none transition focus:ring-4 disabled:opacity-60",
        bad ? "border-destructive/60 text-destructive focus:ring-destructive/15" : "border-border/60 focus:border-ring/50 focus:ring-ring/15",
        className,
      )}
    />
  );
}

// DayChips picks weekdays; none picked means every day.
export function DayChips({ value, onChange, disabled }: { value: number[]; onChange: (v: number[]) => void; disabled?: boolean }) {
  const all = value.length === 0 || value.length === 7;
  const on = (i: number) => all || value.includes(i);
  const toggle = (i: number) => {
    const cur = all ? [0, 1, 2, 3, 4, 5, 6] : value;
    const next = cur.includes(i) ? cur.filter((d) => d !== i) : [...cur, i].sort();
    onChange(next.length === 0 || next.length === 7 ? [] : next);
  };
  const same = (a: number[]) => !all && a.length === value.length && a.every((d) => value.includes(d));
  return (
    <div className="flex flex-wrap items-center gap-1">
      {DAY_SHORT.map((d, i) => (
        <button key={d} type="button" disabled={disabled} onClick={() => toggle(i)} className={cn("h-7 min-w-9 rounded-full px-2 text-[0.72rem] font-semibold transition-colors", on(i) ? "bg-primary/12 text-primary ring-1 ring-primary/30" : "text-muted-foreground ring-1 ring-border/60 hover:bg-accent")}>
          {d}
        </button>
      ))}
      <span className="mx-1 h-4 w-px bg-border" />
      {([["Her gün", []], ["Hafta içi", [0, 1, 2, 3, 4]], ["Hafta sonu", [5, 6]]] as const).map(([l, days]) => (
        <button key={l} type="button" disabled={disabled} onClick={() => onChange([...days])} className={cn("h-7 rounded-full px-2 text-[0.7rem] font-medium", (days.length === 0 ? all : same([...days])) ? "text-primary" : "text-muted-foreground hover:text-foreground")}>
          {l}
        </button>
      ))}
    </div>
  );
}

export function daysText(days: number[]): string {
  const d = [...days].sort();
  if (d.length === 0 || d.length === 7) return "Her gün";
  if (d.join() === "0,1,2,3,4") return "Hafta içi";
  if (d.join() === "5,6") return "Hafta sonu";
  const run = d.every((x, i) => i === 0 || x === d[i - 1] + 1);
  if (run && d.length > 2) return `${DAY_SHORT[d[0]]}–${DAY_SHORT[d[d.length - 1]]}`;
  return d.map((x) => DAY_SHORT[x]).join(", ");
}

export function spanText(sp: TimeSpan): string {
  const days = daysText(sp.days ?? []);
  if (sp.from === sp.to) return `${days}, bütün gün`;
  return `${days} ${sp.from}–${sp.to}${sp.to < sp.from ? " (ertesi sabaha kadar)" : ""}`;
}

// SpanRow is one stretch of time: days, from and to.
export function SpanRow({ span, onChange, onRemove, disabled }: { span: TimeSpan; onChange: (s: TimeSpan) => void; onRemove?: () => void; disabled?: boolean }) {
  return (
    <div className="space-y-2 rounded-xl bg-muted/40 p-2.5">
      <div className="flex items-center gap-2">
        <TimeInput value={span.from} onChange={(from) => onChange({ ...span, from })} disabled={disabled} label="Başlangıç" />
        <span className="text-muted-foreground">–</span>
        <TimeInput value={span.to} onChange={(to) => onChange({ ...span, to })} disabled={disabled} label="Bitiş" />
        <span className="min-w-0 flex-1 truncate text-[0.7rem] text-muted-foreground">
          {span.from === span.to ? "bütün gün" : span.to < span.from ? "ertesi sabaha kadar" : ""}
        </span>
        {onRemove && <button type="button" onClick={onRemove} disabled={disabled} aria-label="Kaldır" className="rounded-lg p-1 text-muted-foreground hover:text-destructive"><X className="size-4" /></button>}
      </div>
      <DayChips value={span.days ?? []} onChange={(days) => onChange({ ...span, days })} disabled={disabled} />
    </div>
  );
}

export function SpanList({ spans, onChange, disabled }: { spans: TimeSpan[]; onChange: (v: TimeSpan[]) => void; disabled?: boolean }) {
  return (
    <div className="space-y-2">
      {spans.map((sp, i) => (
        <SpanRow key={i} span={sp} disabled={disabled} onChange={(s) => onChange(spans.map((x, j) => (j === i ? s : x)))} onRemove={spans.length > 1 ? () => onChange(spans.filter((_, j) => j !== i)) : undefined} />
      ))}
      <button type="button" disabled={disabled} onClick={() => onChange([...spans, { days: [0, 1, 2, 3, 4], from: "12:00", to: "13:00" }])} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-primary ring-1 ring-primary/30 hover:bg-primary/10 disabled:opacity-50">
        <Plus className="size-3.5" /> Saat aralığı ekle
      </button>
    </div>
  );
}
