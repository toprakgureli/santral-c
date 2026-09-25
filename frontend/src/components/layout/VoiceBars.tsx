// VoiceBars draws one voice as a short row of bars mirrored around the
// middle: taller and fuller while someone speaks, a faint pulse while
// nobody does. Colours come from the theme's own tokens, read from the
// page at draw time, so the bars sit in the palette in light and dark.

import { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";

type Props = {
  read: () => Uint8Array | null;
  // A CSS custom property of the theme, such as "--primary" or "--success".
  token: string;
  bars?: number;
  className?: string;
};

// themeColor resolves a token to a colour the canvas accepts, once per
// draw, with a neutral fallback when the token is missing.
function themeColor(token: string, ctx: CanvasRenderingContext2D): string {
  const raw = getComputedStyle(document.documentElement).getPropertyValue(token).trim();
  if (!raw) return "#6b7280";
  ctx.fillStyle = "#010203";
  ctx.fillStyle = raw;
  return ctx.fillStyle === "#010203" ? "#6b7280" : raw;
}

export default function VoiceBars({ read, token, bars = 16, className }: Props) {
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
          const span = Math.max(1, Math.floor(data.length / 3 / bars));
          let sum = 0;
          for (let j = 0; j < span; j++) sum += data[i * span + j] ?? 0;
          target = Math.min(1, (sum / span / 255) * 1.6);
        }
        lv[i] = target > lv[i] ? target : lv[i] * 0.82;
      }

      const color = themeColor(token, ctx);
      const gap = 2;
      const bw = Math.max(2, (w - gap * (bars - 1)) / bars);
      const mid = h / 2;
      ctx.fillStyle = color;
      for (let i = 0; i < bars; i++) {
        // A faint breathing floor so silence is never a flat line.
        const idle = 0.08 + 0.05 * Math.sin(t / 18 + i / 2.5);
        const v = Math.max(idle, lv[i]);
        const bh = Math.max(2, v * (h - 2));
        const x = i * (bw + gap);
        ctx.globalAlpha = 0.22 + 0.7 * v;
        ctx.beginPath();
        ctx.roundRect(x, mid - bh / 2, bw, bh, bw / 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [read, token, bars]);

  return <canvas ref={canvas} className={cn("block h-5 w-20", className)} aria-hidden />;
}
