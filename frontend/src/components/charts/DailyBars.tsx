// DailyBars: one series of daily counts as thin bars on a quiet grid, with
// a hover tooltip. One hue, no legend; the title above names the series.

import { useState } from "react";
import { cn } from "@/lib/utils";

export type DayPoint = { day: string; value: number };

const W = 640;
const H = 160;
const PAD = { top: 12, right: 8, bottom: 22, left: 28 };

function dm(iso: string) {
  const [, m, d] = iso.split("-");
  return `${d}.${m}`;
}

// niceMax rounds the axis top to a friendly step.
function niceMax(max: number) {
  if (max <= 4) return 4;
  const pow = Math.pow(10, Math.floor(Math.log10(max)));
  const n = max / pow;
  const step = n <= 2 ? 2 : n <= 5 ? 5 : 10;
  return step * pow;
}

export default function DailyBars({ points, unit = "gerçek çağrı", className }: { points: DayPoint[]; unit?: string; className?: string }) {
  const [hover, setHover] = useState<number | null>(null);
  const top = niceMax(Math.max(0, ...points.map((p) => p.value)));
  const iw = W - PAD.left - PAD.right;
  const ih = H - PAD.top - PAD.bottom;
  const slot = iw / Math.max(1, points.length);
  const bar = Math.max(3, Math.min(22, slot - 4));
  const y = (v: number) => PAD.top + ih - (v / top) * ih;
  const ticks = [0, top / 2, top];
  // Label every day when there is room, else about six evenly spaced ones.
  const every = Math.max(1, Math.ceil(points.length / 7));

  return (
    <div className={cn("relative", className)}>
      <svg viewBox={`0 0 ${W} ${H}`} className="block h-auto w-full" role="img" aria-label={`Günlük ${unit}`} onMouseLeave={() => setHover(null)}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={PAD.left} x2={W - PAD.right} y1={y(t)} y2={y(t)} className="stroke-border/70" strokeWidth={1} strokeDasharray={t === 0 ? undefined : "2 4"} />
            <text x={PAD.left - 6} y={y(t) + 3.5} textAnchor="end" className="fill-muted-foreground text-[10px] tabular-nums">{t}</text>
          </g>
        ))}
        {points.map((p, i) => {
          const x = PAD.left + i * slot + (slot - bar) / 2;
          const h = Math.max(p.value > 0 ? 2 : 0, ih * (p.value / top));
          const on = hover === i;
          return (
            <g key={p.day} onMouseEnter={() => setHover(i)}>
              {/* hit target wider than the bar */}
              <rect x={PAD.left + i * slot} y={PAD.top} width={slot} height={ih} fill="transparent" />
              <rect x={x} y={y(p.value)} width={bar} height={h} rx={Math.min(4, bar / 2)} className={cn("transition-opacity", p.value > 0 ? "fill-success" : "fill-border", on ? "opacity-100" : "opacity-80")} />
              {i % every === 0 && (
                <text x={PAD.left + i * slot + slot / 2} y={H - 6} textAnchor="middle" className={cn("text-[10px] tabular-nums", on ? "fill-foreground" : "fill-muted-foreground")}>{dm(p.day)}</text>
              )}
            </g>
          );
        })}
      </svg>
      {hover !== null && points[hover] && (
        <div
          className="pointer-events-none absolute -top-1 -translate-x-1/2 -translate-y-full rounded-lg border border-border bg-popover px-2 py-1 text-xs shadow-md"
          style={{ left: `${((PAD.left + hover * slot + slot / 2) / W) * 100}%` }}
        >
          <span className="text-muted-foreground">{dm(points[hover].day)}</span> · <span className="font-semibold tabular-nums">{points[hover].value}</span> {unit}
        </div>
      )}
    </div>
  );
}
