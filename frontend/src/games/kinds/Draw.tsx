// Çiz & Bil: a canvas the drawer paints on, strokes streamed to the room,
// a guess box for the rest.

import { useEffect, useRef, useState } from "react";
import { Eraser, Trash2 } from "lucide-react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";
import { nameOf, Note, Round, type KindProps } from "@/games/kinds/shared";

type Stroke = { c: string; w: number; p: number[] } | { clear: true };

const COLORS = ["#111827", "#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#a855f7", "#ffffff"];
const W = 800;
const H = 500;

function paint(ctx: CanvasRenderingContext2D, s: Stroke) {
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

export default function Draw({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const drawer = d.drawer === selfId;
  const canvas = useRef<HTMLCanvasElement>(null);
  const current = useRef<number[] | null>(null);
  const [color, setColor] = useState(COLORS[0]);
  const [width, setWidth] = useState(4);
  const [guess, setGuess] = useState("");
  const [flash, setFlash] = useState<string | null>(null);

  // Repaint from the server's stroke list whenever the turn changes.
  useEffect(() => {
    const ctx = canvas.current?.getContext("2d");
    if (!ctx) return;
    paint(ctx, { clear: true });
    for (const s of (d.strokes ?? []) as Stroke[]) paint(ctx, s);
  }, [d.turn, d.phase]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    return h.onStroke((p) => {
      const ctx = canvas.current?.getContext("2d");
      if (ctx && p) paint(ctx, p as Stroke);
    });
  }, [h]);

  const point = (e: React.PointerEvent) => {
    const el = canvas.current!;
    const r = el.getBoundingClientRect();
    return [Math.round(((e.clientX - r.left) / r.width) * W), Math.round(((e.clientY - r.top) / r.height) * H)];
  };
  const down = (e: React.PointerEvent<HTMLCanvasElement>) => {
    if (!drawer || d.phase !== "draw") return;
    e.currentTarget.setPointerCapture(e.pointerId);
    current.current = point(e);
  };
  const move = (e: React.PointerEvent) => {
    if (!current.current) return;
    const [x, y] = point(e);
    const last = current.current;
    const ctx = canvas.current?.getContext("2d");
    if (ctx) paint(ctx, { c: color, w: width, p: [last[last.length - 2], last[last.length - 1], x, y] });
    last.push(x, y);
    // Send in chunks so the room sees the line grow, not appear at the end.
    if (last.length >= 40) {
      void h.act("stroke", { c: color, w: width, p: last.slice() }).catch(() => undefined);
      current.current = [x, y];
    }
  };
  const up = () => {
    const last = current.current;
    current.current = null;
    if (last && last.length >= 2) void h.act("stroke", { c: color, w: width, p: last }).catch(() => undefined);
  };

  const submitGuess = async () => {
    const text = guess.trim();
    if (!text) return;
    setGuess("");
    try {
      await h.act("guess", { text });
    } catch {
      // the box shows the reason
    }
  };

  useEffect(() => {
    if (d.phase === "reveal") {
      setFlash(`Kelime: ${d.word}`);
      const t = window.setTimeout(() => setFlash(null), 4000);
      return () => window.clearTimeout(t);
    }
  }, [d.phase, d.word]);

  const guessed = d.guessed?.[selfId] !== undefined;

  return (
    <div className="flex h-full flex-col gap-3 lg:flex-row">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex items-center justify-between gap-3">
          <Round n={d.turn} total={d.total} label="Çizim" />
          <p className="text-sm">
            {d.phase === "choose" && (drawer ? "Bir kelime seç" : `${nameOf(g.players, d.drawer)} kelime seçiyor...`)}
            {d.phase === "draw" && (drawer ? <span>Çiziyorsun: <strong>{d.word}</strong></span> : <span>{nameOf(g.players, d.drawer)} çiziyor · <span className="font-mono tracking-[0.3em]">{d.hint}</span> <span className="text-xs text-muted-foreground">({d.wordLength} harf)</span></span>)}
            {d.phase === "reveal" && <span>Kelime: <strong>{d.word}</strong></span>}
          </p>
        </div>
        <div className="relative w-full overflow-hidden rounded-2xl border border-border/60 bg-white shadow-inner" style={{ aspectRatio: `${W} / ${H}` }}>
          <canvas ref={canvas} width={W} height={H} onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up} className={cn("size-full touch-none", drawer && d.phase === "draw" ? "cursor-crosshair" : "cursor-default")} />
          {d.phase === "choose" && drawer && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-white/85">
              <p className="text-sm font-medium text-neutral-700">Hangisini çizeceksin?</p>
              <div className="flex flex-wrap justify-center gap-2">
                {(d.choices as string[] | undefined)?.map((c, i) => (
                  <Button key={c} onClick={() => void h.act("choose", { index: i })} className="h-11 px-5 text-base">{c}</Button>
                ))}
              </div>
            </div>
          )}
          {flash && <div className="absolute inset-x-0 top-3 mx-auto w-fit rounded-full bg-neutral-900/85 px-4 py-1.5 text-sm font-semibold text-white">{flash}</div>}
        </div>
        {drawer && d.phase === "draw" && (
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
            <button type="button" onClick={() => { setColor("#ffffff"); setWidth(20); }} title="Silgi" className="ml-1 rounded-lg border border-border p-1.5 text-muted-foreground hover:bg-accent"><Eraser className="size-4" /></button>
            <button type="button" onClick={() => void h.act("clear").catch(() => undefined)} title="Temizle" className="rounded-lg border border-border p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>
          </div>
        )}
        {!drawer && d.phase === "draw" && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void submitGuess();
            }}
            className="flex gap-2"
          >
            <input
              value={guess}
              onChange={(e) => setGuess(e.target.value)}
              disabled={guessed}
              placeholder={guessed ? "Bildin! Diğerlerini bekle." : "Tahminini yaz ve Enter'a bas"}
              className="h-11 flex-1 rounded-xl border border-border/70 bg-muted/40 px-4 text-sm outline-none focus:border-ring/60 focus:ring-4 focus:ring-ring/20 disabled:opacity-60"
              autoFocus
            />
            <Button type="submit" disabled={guessed || !guess.trim()} className="h-11">Tahmin</Button>
          </form>
        )}
      </div>
      <aside className="flex w-full flex-col rounded-2xl border border-border/60 bg-card lg:w-64">
        <p className="border-b border-border/60 px-3 py-2 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Tahminler</p>
        <ul className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2 text-sm">
          {((d.guesses ?? []) as { userId: number; name: string; text: string; correct: boolean }[]).map((x, i) => (
            <li key={i} className={cn("rounded-lg px-2 py-1", x.correct ? "bg-success/15 font-medium text-success" : "bg-muted/40")}>
              <span className="text-muted-foreground">{x.name.split(" ")[0]}:</span> {x.correct ? "bildi! 🎉" : x.text}
            </li>
          ))}
          {(!d.guesses || d.guesses.length === 0) && <li><Note>Henüz tahmin yok</Note></li>}
        </ul>
      </aside>
    </div>
  );
}
