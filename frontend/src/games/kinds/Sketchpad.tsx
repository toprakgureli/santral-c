// Sketchpad: the drawing board Çiz & Bil and Kulaktan Kulağa share. It
// paints locally, hands each stroke to the owner (who may stream it to the
// room), and can replay a stroke list or export the picture.

import { forwardRef, useImperativeHandle, useRef, useState, type ReactNode } from "react";
import { Eraser, Trash2 } from "lucide-react";
import { cn } from "@/lib/utils";

export type Stroke = { c: string; w: number; p: number[] } | { clear: true };

export const COLORS = ["#111827", "#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#a855f7", "#ffffff"];
export const W = 800;
export const H = 500;

export function paint(ctx: CanvasRenderingContext2D, s: Stroke) {
  if ("clear" in s) {
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, W, H);
    return;
  }
  ctx.strokeStyle = s.c;
  ctx.lineWidth = s.w;
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.beginPath();
  for (let i = 0; i + 1 < s.p.length; i += 2) {
    if (i === 0) ctx.moveTo(s.p[i], s.p[i + 1]);
    else ctx.lineTo(s.p[i], s.p[i + 1]);
  }
  if (s.p.length === 2) ctx.lineTo(s.p[0] + 0.1, s.p[1] + 0.1);
  ctx.stroke();
}

export type SketchpadHandle = {
  paint: (s: Stroke) => void;
  reset: (strokes?: Stroke[]) => void;
  // A small JPEG of the board, for games that send the whole picture once.
  toDataURL: () => string;
};

type Props = {
  enabled: boolean;
  onStroke?: (s: Stroke) => void;
  overlay?: ReactNode;
  className?: string;
};

const Sketchpad = forwardRef<SketchpadHandle, Props>(function Sketchpad({ enabled, onStroke, overlay, className }, ref) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const current = useRef<number[] | null>(null);
  const [color, setColor] = useState(COLORS[0]);
  const [width, setWidth] = useState(4);

  const ctx = () => canvas.current?.getContext("2d") ?? null;

  useImperativeHandle(ref, () => ({
    paint: (s) => {
      const c = ctx();
      if (c) paint(c, s);
    },
    reset: (strokes) => {
      const c = ctx();
      if (!c) return;
      paint(c, { clear: true });
      for (const s of strokes ?? []) paint(c, s);
    },
    toDataURL: () => {
      const small = document.createElement("canvas");
      small.width = 480;
      small.height = 300;
      const c = small.getContext("2d")!;
      c.fillStyle = "#ffffff";
      c.fillRect(0, 0, small.width, small.height);
      if (canvas.current) c.drawImage(canvas.current, 0, 0, small.width, small.height);
      return small.toDataURL("image/jpeg", 0.72);
    },
  }));

  const point = (e: React.PointerEvent) => {
    const el = canvas.current!;
    const r = el.getBoundingClientRect();
    return [Math.round(((e.clientX - r.left) / r.width) * W), Math.round(((e.clientY - r.top) / r.height) * H)];
  };
  const down = (e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!enabled) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    current.current = point(e);
  };
  const move = (e: React.PointerEvent) => {
    if (!current.current) return;
    const [x, y] = point(e);
    const last = current.current;
    const c = ctx();
    if (c) paint(c, { c: color, w: width, p: [last[last.length - 2], last[last.length - 1], x, y] });
    last.push(x, y);
    // Hand over in chunks so a watching room sees the line grow.
    if (last.length >= 40) {
      onStroke?.({ c: color, w: width, p: last.slice() });
      current.current = [x, y];
    }
  };
  const up = () => {
    const last = current.current;
    current.current = null;
    if (last && last.length >= 2) onStroke?.({ c: color, w: width, p: last });
  };
  const clear = () => {
    const c = ctx();
    if (c) paint(c, { clear: true });
    onStroke?.({ clear: true });
  };

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="relative w-full overflow-hidden rounded-2xl border border-border/60 bg-white shadow-inner" style={{ aspectRatio: `${W} / ${H}` }}>
        <canvas ref={canvas} width={W} height={H} onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up} className={cn("size-full touch-none", enabled ? "cursor-crosshair" : "cursor-default")} />
        {overlay}
      </div>
      {enabled && (
        <div className="flex flex-wrap items-center gap-2">
          {COLORS.map((c) => (
            <button key={c} type="button" onClick={() => setColor(c)} style={{ backgroundColor: c }} className={cn("size-7 rounded-full border-2", color === c ? "border-primary scale-110" : "border-border")} aria-label={c} />
          ))}
          <span className="mx-1 h-5 w-px bg-border" />
          {[2, 4, 8, 14].map((w) => (
            <button key={w} type="button" onClick={() => setWidth(w)} className={cn("flex size-7 items-center justify-center rounded-full border", width === w ? "border-primary bg-primary/10" : "border-border")}>
              <span className="rounded-full bg-foreground" style={{ width: w + 2, height: w + 2 }} />
            </button>
          ))}
          <button type="button" onClick={() => { setColor("#ffffff"); setWidth(20); }} data-tip="Silgi" className="ml-1 rounded-lg border border-border p-1.5 text-muted-foreground hover:bg-accent"><Eraser className="size-4" /></button>
          <button type="button" onClick={clear} data-tip="Temizle" className="rounded-lg border border-border p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>
        </div>
      )}
    </div>
  );
});

export default Sketchpad;
