// AudioWave draws one voice as a moving line: flat while quiet, waving
// while someone speaks. It reads samples every frame from the call's
// audio graph and draws nothing when that side is not wired.

import { useEffect, useRef } from "react";
import { cn } from "@/lib/utils";

export default function AudioWave({ read, color, className }: { read: () => Float32Array | null; color: string; className?: string }) {
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    let raf = 0;
    const draw = () => {
      raf = requestAnimationFrame(draw);
      const el = canvas.current;
      if (!el) return;
      const dpr = window.devicePixelRatio || 1;
      const w = el.clientWidth;
      const h = el.clientHeight;
      if (el.width !== w * dpr || el.height !== h * dpr) {
        el.width = w * dpr;
        el.height = h * dpr;
      }
      const ctx = el.getContext("2d");
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);
      const data = read();
      const mid = h / 2;
      ctx.lineWidth = 1.5;
      ctx.lineJoin = "round";
      ctx.strokeStyle = color;
      ctx.beginPath();
      if (!data) {
        ctx.globalAlpha = 0.35;
        ctx.moveTo(0, mid);
        ctx.lineTo(w, mid);
        ctx.stroke();
        ctx.globalAlpha = 1;
        return;
      }
      // A little lift so a quiet voice still shows, and a cap so a loud one
      // stays inside the box.
      const n = data.length;
      for (let i = 0; i < n; i++) {
        const v = Math.max(-1, Math.min(1, data[i] * 2.2));
        const x = (i / (n - 1)) * w;
        const y = mid - v * (mid - 2);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.stroke();
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [read, color]);

  return <canvas ref={canvas} className={cn("block h-6 w-full", className)} aria-hidden />;
}
