// Masa Hokeyi: the server moves the puck, the browser draws frames and
// sends the paddle. Your own half is always at the bottom.

import { useEffect, useRef } from "react";
import { Note, type KindProps } from "@/games/kinds/shared";

type Frame = { puck: number[]; pads: number[][]; score: number[]; phase: string };

export default function Hockey({ h }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const canvas = useRef<HTMLCanvasElement>(null);
  const frame = useRef<Frame>({ puck: d.puck ?? [50, 80, 0, 0], pads: d.pads ?? [[50, 140], [50, 20]], score: d.score ?? [0, 0], phase: d.phase ?? "play" });
  const lastSent = useRef(0);
  const W = d.width ?? 100;
  const H = d.height ?? 160;
  const seat = d.mySeat ?? -1;
  const flip = seat === 1; // seat 1 defends the top; show them their half at the bottom

  useEffect(() => h.onFrame((p) => { frame.current = p as Frame; }), [h]);

  useEffect(() => {
    let raf = 0;
    const draw = () => {
      const el = canvas.current;
      const ctx = el?.getContext("2d");
      if (el && ctx) {
        const f = frame.current;
        const sx = el.width / W;
        const sy = el.height / H;
        const ty = (y: number) => (flip ? H - y : y);
        const tx = (x: number) => (flip ? W - x : x);
        ctx.fillStyle = "#0f172a";
        ctx.fillRect(0, 0, el.width, el.height);
        ctx.strokeStyle = "rgba(255,255,255,0.25)";
        ctx.lineWidth = 2;
        ctx.strokeRect(1, 1, el.width - 2, el.height - 2);
        ctx.beginPath();
        ctx.moveTo(0, el.height / 2);
        ctx.lineTo(el.width, el.height / 2);
        ctx.stroke();
        ctx.beginPath();
        ctx.arc(el.width / 2, el.height / 2, 12 * sx, 0, Math.PI * 2);
        ctx.stroke();
        const goal = (d.goal ?? 36) * sx;
        ctx.strokeStyle = "#f59e0b";
        ctx.lineWidth = 4;
        ctx.beginPath();
        ctx.moveTo(el.width / 2 - goal / 2, 2);
        ctx.lineTo(el.width / 2 + goal / 2, 2);
        ctx.moveTo(el.width / 2 - goal / 2, el.height - 2);
        ctx.lineTo(el.width / 2 + goal / 2, el.height - 2);
        ctx.stroke();
        for (let i = 0; i < 2; i++) {
          const [px, py] = f.pads[i];
          ctx.beginPath();
          ctx.arc(tx(px) * sx, ty(py) * sy, (d.pad ?? 6) * sx, 0, Math.PI * 2);
          ctx.fillStyle = (i === 0) !== flip ? "#3b82f6" : "#ef4444";
          ctx.fill();
          ctx.strokeStyle = "rgba(255,255,255,0.6)";
          ctx.lineWidth = 2;
          ctx.stroke();
        }
        ctx.beginPath();
        ctx.arc(tx(f.puck[0]) * sx, ty(f.puck[1]) * sy, (d.puckR ?? 3) * sx, 0, Math.PI * 2);
        ctx.fillStyle = "#f8fafc";
        ctx.fill();
        ctx.fillStyle = "rgba(255,255,255,0.85)";
        ctx.font = `bold ${18 * sx}px system-ui`;
        ctx.textAlign = "center";
        const top = flip ? f.score[0] : f.score[1];
        const bottom = flip ? f.score[1] : f.score[0];
        ctx.fillText(String(top), el.width / 2, el.height / 2 - 20 * sy);
        ctx.fillText(String(bottom), el.width / 2, el.height / 2 + 30 * sy);
        if (f.phase === "goal") {
          ctx.fillStyle = "#f59e0b";
          ctx.font = `bold ${14 * sx}px system-ui`;
          ctx.fillText("GOL!", el.width / 2, el.height / 2 + 5 * sy);
        }
      }
      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [W, H, flip, d.goal, d.pad, d.puckR]);

  const move = (e: React.PointerEvent<HTMLCanvasElement>) => {
    if (seat < 0 || !g.joined) return;
    const now = performance.now();
    if (now - lastSent.current < 50) return;
    lastSent.current = now;
    const r = e.currentTarget.getBoundingClientRect();
    let x = ((e.clientX - r.left) / r.width) * W;
    let y = ((e.clientY - r.top) / r.height) * H;
    if (flip) {
      x = W - x;
      y = H - y;
    }
    frame.current.pads[seat] = [x, y];
    void h.act("move", { x, y }).catch(() => undefined);
  };

  const me = g.players[seat];
  const other = g.players[1 - seat];
  return (
    <div className="mx-auto flex h-full w-full max-w-sm flex-col items-center gap-2">
      <p className="text-xs text-muted-foreground">{other ? `Üst: ${other.name.split(" ")[0]}` : ""}</p>
      <canvas ref={canvas} width={400} height={640} onPointerMove={move} onPointerDown={move} className="w-full max-h-[70vh] cursor-none touch-none rounded-2xl border border-border/60 shadow-lg" style={{ aspectRatio: `${W} / ${H}` }} />
      <p className="text-xs text-muted-foreground">{me ? `Alt: sen (${me.name.split(" ")[0]})` : "İzleyicisin"} · {d.target} golde biter</p>
      {seat < 0 && <Note>Bu masada iki kişi oynuyor, sen izliyorsun.</Note>}
    </div>
  );
}
