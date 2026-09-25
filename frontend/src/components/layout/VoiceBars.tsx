// VoiceBars draws one voice as a row of bars mirrored around the middle,
// the way a music player shows sound: tall and bright while someone
// speaks, a low quiet pulse while nobody does. It reads the spectrum every
// frame from the call's audio graph.

import { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";

type Props = {
  read: () => Uint8Array | null;
  // Two stops of the bar gradient, top to bottom, as real colours (a canvas
  // cannot read CSS variables).
  colors: [string, string];
  bars?: number;
  className?: string;
};

export default function VoiceBars({ read, colors, bars = 28, className }: Props) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const levels = useRef<number[]>([]);

  useEffect(() => {
    let raf = 0;
    let t = 0;
    const draw = () => {
      raf = requestAnimationFrame(draw);
      t += 1;
      const el = canvas.current;
      if (!el) return;
      const dpr = window.devicePixelRatio || 1;
      const w = el.clientWidth;
      const h = el.clientHeight;
      if (w === 0 || h === 0) return;
      if (el.width !== Math.round(w * dpr) || el.height !== Math.round(h * dpr)) {
        el.width = Math.round(w * dpr);
        el.height = Math.round(h * dpr);
      }
      const ctx = el.getContext("2d");
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      const data = read();
      if (levels.current.length !== bars) levels.current = new Array(bars).fill(0);
      const lv = levels.current;
      // Voice lives in the low bins; spread the first third of the spectrum
      // over the bars and let each bar fall slowly so the motion is smooth.
      for (let i = 0; i < bars; i++) {
        let target = 0;
        if (data) {
          const span = Math.max(1, Math.floor((data.length / 3) / bars));
          let sum = 0;
          for (let j = 0; j < span; j++) sum += data[i * span + j] ?? 0;
          target = Math.min(1, (sum / span / 255) * 1.6);
        }
        lv[i] = target > lv[i] ? target : lv[i] * 0.82;
      }

      const gap = 3;
      const bw = Math.max(2, (w - gap * (bars - 1)) / bars);
      const mid = h / 2;
      const grad = ctx.createLinearGradient(0, 0, 0, h);
      grad.addColorStop(0, colors[0]);
      grad.addColorStop(1, colors[1]);
      ctx.fillStyle = grad;
      for (let i = 0; i < bars; i++) {
        // A faint breathing floor so silence is never a flat line.
        const idle = 0.06 + 0.04 * Math.sin(t / 18 + i / 2.5);
        const v = Math.max(idle, lv[i]);
        const bh = Math.max(3, v * (h - 2));
        const x = i * (bw + gap);
        ctx.globalAlpha = 0.35 + 0.65 * v;
        ctx.beginPath();
        ctx.roundRect(x, mid - bh / 2, bw, bh, bw / 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [read, colors, bars]);

  return <canvas ref={canvas} className={cn("block h-8 w-full", className)} aria-hidden />;
}
