// Çiz & Bil: a canvas the drawer paints on, strokes streamed to the room,
// a guess box for the rest. Like Gartic, a correct or nearly correct guess
// is announced by name only, never by text.

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";
import Sketchpad, { type SketchpadHandle, type Stroke } from "@/games/kinds/Sketchpad";
import { nameOf, Note, Round, type KindProps } from "@/games/kinds/shared";

type Guess = { userId: number; name: string; text: string; correct: boolean; close: boolean };

export default function Draw({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const drawer = d.drawer === selfId;
  const pad = useRef<SketchpadHandle>(null);
  const [guess, setGuess] = useState("");
  const [flash, setFlash] = useState<string | null>(null);

  // Repaint from the server's stroke list whenever the turn changes.
  useEffect(() => {
    pad.current?.reset((d.strokes ?? []) as Stroke[]);
  }, [d.turn, d.phase]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    return h.onStroke((p) => {
      if (p) pad.current?.paint(p as Stroke);
    });
  }, [h]);

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
  const guesses = (d.guesses ?? []) as Guess[];
  const lastMine = [...guesses].reverse().find((x) => x.userId === selfId);
  const nearly = !guessed && d.phase === "draw" && lastMine?.close;

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
        <Sketchpad
          ref={pad}
          enabled={drawer && d.phase === "draw"}
          onStroke={(s) => void h.act("clear" in s ? "clear" : "stroke", "clear" in s ? undefined : s).catch(() => undefined)}
          overlay={
            <>
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
            </>
          }
        />
        {!drawer && d.phase === "draw" && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void submitGuess();
            }}
            className="flex flex-col gap-1.5"
          >
            <div className="flex gap-2">
              <input
                value={guess}
                onChange={(e) => setGuess(e.target.value)}
                disabled={guessed}
                placeholder={guessed ? "Bildin! Diğerlerini bekle." : "Tahminini yaz ve Enter'a bas"}
                className={cn("h-11 flex-1 rounded-xl border bg-muted/40 px-4 text-sm outline-none focus:ring-4 disabled:opacity-60", nearly ? "border-warning/70 focus:border-warning focus:ring-warning/20" : "border-border/70 focus:border-ring/60 focus:ring-ring/20")}
                autoFocus
              />
              <Button type="submit" disabled={guessed || !guess.trim()} className="h-11">Tahmin</Button>
            </div>
            {nearly && <p className="text-xs font-medium text-warning">🔥 Çok yaklaştın! Bir harf oynat ya da kelimeyi tek başına yaz.</p>}
          </form>
        )}
      </div>
      <aside className="flex w-full flex-col rounded-2xl border border-border/60 bg-card lg:w-64">
        <p className="border-b border-border/60 px-3 py-2 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Tahminler</p>
        <ul className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2 text-sm">
          {guesses.map((x, i) => (
            <li key={i} className={cn("rounded-lg px-2 py-1", x.correct ? "bg-success/15 font-medium text-success" : x.close ? "bg-warning/15 font-medium text-warning" : "bg-muted/40")}>
              {x.correct ? (
                <span>{x.name.split(" ")[0]} bildi! 🎉</span>
              ) : x.close ? (
                <span>{x.name.split(" ")[0]} yaklaştı! 🔥</span>
              ) : (
                <><span className="text-muted-foreground">{x.name.split(" ")[0]}:</span> {x.text}</>
              )}
            </li>
          ))}
          {guesses.length === 0 && <li><Note>Henüz tahmin yok</Note></li>}
        </ul>
      </aside>
    </div>
  );
}
