// Masa Hokeyi: the server moves the puck, the browser draws the table and
// sends the paddle. Your own half is always at the bottom. Frames arrive
// thirty times a second and are interpolated to the screen's rate; the
// puck leaves a short trail, a goal flashes the table and a banner.

import { useEffect, useRef } from "react";
import { tones } from "@/softphone/tones";
import { Note, type KindProps } from "@/games/kinds/shared";

type Frame = { puck: number[]; pads: number[][]; score: number[]; phase: string; scorer?: number };

export default function Hockey({ h }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const canvas = useRef<HTMLCanvasElement>(null);
  const prev = useRef<Frame | null>(null);
  const cur = useRef<Frame>({ puck: d.puck ?? [50, 80, 0, 0], pads: d.pads ?? [[50, 140], [50, 20]], score: d.score ?? [0, 0], phase: d.phase ?? "play", scorer: d.scorer });
  const arrivedAt = useRef(performance.now());
  const trail = useRef<number[][]>([]);
  const goalAt = useRef<number>(0);
  const lastPhase = useRef<string>(cur.current.phase);
  const lastSent = useRef(0);
  const myPad = useRef<number[] | null>(null);
  const W = d.width ?? 100;
  const H = d.height ?? 160;
  const seat = d.mySeat ?? -1;
  const flip = seat === 1;

  useEffect(
    () =>
      h.onFrame((p) => {
        const f = p as Frame;
        prev.current = cur.current;
        cur.current = f;
        arrivedAt.current = performance.now();
        if (f.phase === "goal" && lastPhase.current !== "goal") {
          goalAt.current = performance.now();
          tones.mention();
        }
        lastPhase.current = f.phase;
      }),
    [h],
  );

  useEffect(() => {
    let raf = 0;
    const draw = () => {
      const el = canvas.current;
      const ctx = el?.getContext("2d");
      if (el && ctx) {
        const now = performance.now();
        const a = prev.current;
        const b = cur.current;
        // Interpolate between the last two frames (33 ms apart).
        const t = a ? Math.min(1, (now - arrivedAt.current) / 33) : 1;
        const lerp = (x: number, y: number) => x + (y - x) * t;
        const puck = a ? [lerp(a.puck[0], b.puck[0]), lerp(a.puck[1], b.puck[1])] : b.puck;
        const pads = b.pads.map((pd, i) => (i === seat && myPad.current ? myPad.current : a ? [lerp(a.pads[i][0], pd[0]), lerp(a.pads[i][1], pd[1])] : pd));
        const sx = el.width / W;
        const sy = el.height / H;
        const X = (x: number) => (flip ? W - x : x) * sx;
        const Y = (y: number) => (flip ? H - y : y) * sy;
        const goalW = (d.goal ?? 34) * sx;
        const padR = (d.pad ?? 6) * sx;
        const puckR = (d.puckR ?? 3) * sx;

        // Ice.
        const ice = ctx.createLinearGradient(0, 0, 0, el.height);
        ice.addColorStop(0, "#e8f1fb");
        ice.addColorStop(0.5, "#f6fafe");
        ice.addColorStop(1, "#e8f1fb");
        ctx.fillStyle = ice;
        ctx.fillRect(0, 0, el.width, el.height);
        ctx.strokeStyle = "rgba(120,150,190,0.08)";
        ctx.lineWidth = 1;
        for (let i = 0; i < 24; i++) {
          ctx.beginPath();
          ctx.moveTo((i * 137) % el.width, 0);
          ctx.lineTo(((i * 137) % el.width) + 40, el.height);
          ctx.stroke();
        }
        // Markings.
        ctx.strokeStyle = "rgba(220,38,38,0.75)";
        ctx.lineWidth = 3 * sx;
        ctx.beginPath();
        ctx.moveTo(0, el.height / 2);
        ctx.lineTo(el.width, el.height / 2);
        ctx.stroke();
        ctx.strokeStyle = "rgba(37,99,235,0.6)";
        ctx.lineWidth = 2 * sx;
        ctx.beginPath();
        ctx.arc(el.width / 2, el.height / 2, 14 * sx, 0, Math.PI * 2);
        ctx.stroke();
        ctx.fillStyle = "rgba(37,99,235,0.6)";
        ctx.beginPath();
        ctx.arc(el.width / 2, el.height / 2, 2.2 * sx, 0, Math.PI * 2);
        ctx.fill();
        for (const y of [0, el.height]) {
          ctx.beginPath();
          ctx.arc(el.width / 2, y, 20 * sx, 0, Math.PI * 2);
          ctx.fillStyle = "rgba(96,165,250,0.18)";
          ctx.fill();
          ctx.strokeStyle = "rgba(220,38,38,0.55)";
          ctx.lineWidth = 1.5 * sx;
          ctx.stroke();
        }
        // Boards.
        ctx.strokeStyle = "#1e3a8a";
        ctx.lineWidth = 5 * sx;
        ctx.strokeRect(2.5 * sx, 2.5 * sx, el.width - 5 * sx, el.height - 5 * sx);
        // Goal mouths.
        for (const [y, top] of [[0, true], [el.height, false]] as [number, boolean][]) {
          const gy = y + (top ? 3 * sx : -3 * sx);
          const net = ctx.createLinearGradient(0, top ? 0 : el.height - 10 * sx, 0, top ? 10 * sx : el.height);
          net.addColorStop(0, "rgba(15,23,42,0.85)");
          net.addColorStop(1, "rgba(15,23,42,0.2)");
          ctx.fillStyle = net;
          ctx.fillRect(el.width / 2 - goalW / 2, top ? 0 : el.height - 10 * sx, goalW, 10 * sx);
          ctx.strokeStyle = "#f59e0b";
          ctx.lineWidth = 4 * sx;
          ctx.beginPath();
          ctx.moveTo(el.width / 2 - goalW / 2, gy);
          ctx.lineTo(el.width / 2 + goalW / 2, gy);
          ctx.stroke();
        }
        // Scores, large and quiet, on the ice of each half.
        ctx.fillStyle = "rgba(30,58,138,0.12)";
        ctx.font = `800 ${42 * sx}px system-ui`;
        ctx.textAlign = "center";
        ctx.textBaseline = "middle";
        const topScore = flip ? b.score[0] : b.score[1];
        const bottomScore = flip ? b.score[1] : b.score[0];
        ctx.fillText(String(topScore), el.width / 2, el.height * 0.25);
        ctx.fillText(String(bottomScore), el.width / 2, el.height * 0.75);

        // Puck trail.
        const tr = trail.current;
        tr.push([X(puck[0]), Y(puck[1])]);
        if (tr.length > 10) tr.shift();
        tr.forEach(([tx, ty], i) => {
          ctx.beginPath();
          ctx.arc(tx, ty, puckR * (0.3 + (i / tr.length) * 0.6), 0, Math.PI * 2);
          ctx.fillStyle = `rgba(15,23,42,${0.04 + (i / tr.length) * 0.16})`;
          ctx.fill();
        });

        // Mallets with a dome.
        pads.forEach((pd, i) => {
          const cx = X(pd[0]);
          const cy = Y(pd[1]);
          const mine = i === seat;
          const base = (i === 0) !== flip ? ["#2563eb", "#93c5fd"] : ["#dc2626", "#fca5a5"];
          ctx.beginPath();
          ctx.arc(cx, cy + 2 * sx, padR * 1.05, 0, Math.PI * 2);
          ctx.fillStyle = "rgba(0,0,0,0.18)";
          ctx.fill();
          const dome = ctx.createRadialGradient(cx - padR * 0.35, cy - padR * 0.35, padR * 0.1, cx, cy, padR);
          dome.addColorStop(0, base[1]);
          dome.addColorStop(1, base[0]);
          ctx.beginPath();
          ctx.arc(cx, cy, padR, 0, Math.PI * 2);
          ctx.fillStyle = dome;
          ctx.fill();
          ctx.strokeStyle = mine ? "#ffffff" : "rgba(255,255,255,0.7)";
          ctx.lineWidth = (mine ? 2.5 : 1.5) * sx;
          ctx.stroke();
          ctx.beginPath();
          ctx.arc(cx, cy, padR * 0.35, 0, Math.PI * 2);
          ctx.fillStyle = "rgba(255,255,255,0.55)";
          ctx.fill();
        });

        // Puck with a glow.
        const px = X(puck[0]);
        const py = Y(puck[1]);
        ctx.shadowColor = "rgba(15,23,42,0.5)";
        ctx.shadowBlur = 8 * sx;
        ctx.beginPath();
        ctx.arc(px, py, puckR, 0, Math.PI * 2);
        ctx.fillStyle = "#0f172a";
        ctx.fill();
        ctx.shadowBlur = 0;
        ctx.beginPath();
        ctx.arc(px - puckR * 0.3, py - puckR * 0.3, puckR * 0.35, 0, Math.PI * 2);
        ctx.fillStyle = "rgba(255,255,255,0.35)";
        ctx.fill();

        // Goal: a flash on the table and a banner that pops and settles.
        if (b.phase === "goal") {
          const age = (now - goalAt.current) / 1000;
          const flash = Math.max(0, 0.35 - age * 0.5);
          if (flash > 0) {
            ctx.fillStyle = `rgba(245,158,11,${flash})`;
            ctx.fillRect(0, 0, el.width, el.height);
          }
          const scale = age < 0.25 ? 0.6 + (age / 0.25) * 0.5 : 1.1 - Math.min(0.1, (age - 0.25) * 0.4);
          const iScored = b.scorer === seat;
          ctx.save();
          ctx.translate(el.width / 2, el.height / 2);
          ctx.scale(scale, scale);
          ctx.font = `900 ${34 * sx}px system-ui`;
          ctx.lineWidth = 6 * sx;
          ctx.strokeStyle = "rgba(15,23,42,0.8)";
          ctx.strokeText("GOL!", 0, 0);
          ctx.fillStyle = iScored || seat < 0 ? "#f59e0b" : "#fca5a5";
          ctx.fillText("GOL!", 0, 0);
          ctx.font = `700 ${11 * sx}px system-ui`;
          ctx.fillStyle = "#0f172a";
          const who = b.scorer !== undefined ? g.players[b.scorer]?.name.split(" ")[0] : "";
          ctx.fillText(seat < 0 ? `${who} attı` : iScored ? "Senin golün!" : `${who} attı`, 0, 26 * sx);
          ctx.restore();
        }
      }
      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [W, H, flip, seat, d.goal, d.pad, d.puckR, g.players]);

  const move = (e: React.PointerEvent<HTMLCanvasElement>) => {
    if (seat < 0 || !g.joined) return;
    const r = e.currentTarget.getBoundingClientRect();
    let x = ((e.clientX - r.left) / r.width) * W;
    let y = ((e.clientY - r.top) / r.height) * H;
    if (flip) {
      x = W - x;
      y = H - y;
    }
    const pad = d.pad ?? 6;
    x = Math.max(pad, Math.min(W - pad, x));
    y = seat === 0 ? Math.max(H / 2 + pad, Math.min(H - pad, y)) : Math.max(pad, Math.min(H / 2 - pad, y));
    // Draw my own mallet at once; the server confirms with the next frame.
    myPad.current = [x, y];
    const now = performance.now();
    if (now - lastSent.current < 40) return;
    lastSent.current = now;
    void h.act("move", { x, y }).catch(() => undefined);
  };

  const me = seat >= 0 ? g.players[seat] : undefined;
  const other = seat >= 0 ? g.players[1 - seat] : undefined;
  return (
    <div className="mx-auto flex h-full w-full max-w-sm flex-col items-center gap-2">
      <div className="flex w-full items-center justify-between text-xs">
        <span className="inline-flex items-center gap-1.5 text-muted-foreground"><span className="size-2.5 rounded-full bg-red-500" /> {other ? other.name.split(" ")[0] : g.players[1]?.name.split(" ")[0]}</span>
        <span className="rounded-full bg-muted px-2.5 py-0.5 font-mono text-xs tabular-nums">{d.target} golde biter</span>
      </div>
      <canvas ref={canvas} width={500} height={800} onPointerMove={move} onPointerDown={move} className="w-full max-h-[70vh] cursor-none touch-none rounded-2xl shadow-2xl ring-1 ring-black/20" style={{ aspectRatio: `${W} / ${H}` }} />
      <div className="flex w-full items-center justify-between text-xs">
        <span className="inline-flex items-center gap-1.5 text-muted-foreground"><span className="size-2.5 rounded-full bg-blue-500" /> {me ? `Sen (${me.name.split(" ")[0]})` : g.players[0]?.name.split(" ")[0]}</span>
        {seat < 0 && <Note>İzliyorsun</Note>}
      </div>
    </div>
  );
}
