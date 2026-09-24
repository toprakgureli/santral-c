// RangePicker: the preset dropdown plus the day fields it reveals. The
// same control on every page that filters by date, so the hand learns it
// once.

import { useState } from "react";
import { CalendarRange } from "lucide-react";
import { DateField, Select } from "@/components/ui";
import { PRESETS, presetRange, ymd, type Preset } from "@/lib/dateRange";

export type Range = { from: string; to: string };

export function useRange(initial: Preset = "today"): { preset: Preset; range: Range; choose: (p: Preset) => void; setFrom: (v: string) => void; setTo: (v: string) => void } {
  const [preset, setPreset] = useState<Preset>(initial);
  const [range, setRange] = useState<Range>(() => presetRange(initial));
  const choose = (key: Preset) => {
    setPreset(key);
    if (key === "day") setRange((r) => ({ from: r.from, to: r.from }));
    else if (key !== "custom") setRange(presetRange(key));
  };
  return {
    preset,
    range,
    choose,
    setFrom: (v) => setRange((r) => ({ from: v, to: preset === "day" ? v : r.to })),
    setTo: (v) => setRange((r) => ({ ...r, to: v })),
  };
}

export default function RangePicker({ preset, range, onPreset, onFrom, onTo, compact }: { preset: Preset; range: Range; onPreset: (p: Preset) => void; onFrom: (v: string) => void; onTo: (v: string) => void; compact?: boolean }) {
  const today = ymd(new Date());
  const h = compact ? "h-8" : "h-9";
  const field = compact ? "w-40 [&>input]:h-8" : "w-40 [&>input]:h-9";
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label className="relative" title="Tarih">
        <CalendarRange className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Select value={preset} onChange={(e) => onPreset(e.target.value as Preset)} className={`${h} w-40 pl-9`}>
          {PRESETS.map((p) => (
            <option key={p.key} value={p.key}>{p.label}</option>
          ))}
        </Select>
      </label>
      {preset === "day" && <DateField value={range.from} max={today} onChange={onFrom} className={field} title="Gün" />}
      {preset === "custom" && (
        <div className="flex items-center gap-1">
          <DateField value={range.from} max={range.to || undefined} onChange={onFrom} className={field} title="Başlangıç" />
          <span className="text-muted-foreground">-</span>
          <DateField value={range.to} min={range.from || undefined} max={today} onChange={onTo} className={field} title="Bitiş" />
        </div>
      )}
    </div>
  );
}
