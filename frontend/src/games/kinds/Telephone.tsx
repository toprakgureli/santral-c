// Kulaktan Kulağa Çizim: a word becomes a drawing, the drawing a guess, the
// guess a drawing again, hand to hand. At the end the album is turned page
// by page and everyone sees what the first word turned into.

import { useEffect, useRef, useState } from "react";
import { ArrowRight, Check, Pencil, Type } from "lucide-react";
import { Button } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import Sketchpad, { type SketchpadHandle } from "@/games/kinds/Sketchpad";
import { nameOf, Note, Prompt, Round, type KindProps } from "@/games/kinds/shared";

type Step = { kind: "word" | "draw" | "guess"; by: number; text?: string; image?: string };

export default function Telephone({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const pad = useRef<SketchpadHandle>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // A fresh board for every drawing step.
  useEffect(() => {
    pad.current?.reset();
    setText("");
    setError(null);
  }, [d.step]);

  const send = async () => {
    setBusy(true);
    setError(null);
    try {
      if (d.kind === "draw") await h.act("submit", { image: pad.current?.toDataURL() ?? "" });
      else await h.act("submit", { text: text.trim() });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Gönderilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const active = g.players.filter((p) => !p.left).length;

  if (d.phase === "work") {
    const mine = d.chain !== undefined;
    return (
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Round n={d.step} total={Math.max(1, d.steps - 1)} label="Adım" />
          <Note>{d.done} / {active} gönderdi</Note>
        </div>
        {!mine ? (
          <Note>Bu adımda sana iş düşmedi, diğerleri bitirsin.</Note>
        ) : d.submitted ? (
          <div className="flex flex-col items-center gap-2 py-10 text-center">
            <span className="flex size-12 items-center justify-center rounded-full bg-success/15 text-success"><Check className="size-6" /></span>
            <p className="text-sm font-medium">Gönderdin, elden ele geçiyor.</p>
            <Note>Diğerleri bitirince sıradaki adım açılır.</Note>
          </div>
        ) : d.kind === "draw" ? (
          <>
            <Prompt className="text-lg font-medium"><span className="mr-2 inline-flex size-7 items-center justify-center rounded-lg bg-violet-500/15 text-violet-500 align-middle"><Pencil className="size-4" /></span>Bunu çiz: <strong>{d.promptText}</strong></Prompt>
            <Sketchpad ref={pad} enabled />
            <div className="flex items-center justify-end gap-3">
              {error && <p className="text-xs text-destructive">{error}</p>}
              <Button onClick={() => void send()} disabled={busy} className="h-11 px-6">Çizimi gönder <ArrowRight className="ml-1 size-4" /></Button>
            </div>
          </>
        ) : (
          <>
            <Prompt className="text-base"><span className="mr-2 inline-flex size-7 items-center justify-center rounded-lg bg-violet-500/15 text-violet-500 align-middle"><Type className="size-4" /></span>Bu çizim ne anlatıyor? Tek kelime ya da kısa bir cümleyle yaz.</Prompt>
            <div className="mx-auto w-full max-w-xl overflow-hidden rounded-2xl border border-border/60 bg-white shadow-inner" style={{ aspectRatio: "8 / 5" }}>
              {d.promptImage ? <img src={d.promptImage} alt="" className="size-full object-contain" draggable={false} /> : <p className="flex size-full items-center justify-center text-sm text-neutral-400">Boş sayfa, süre bitmiş</p>}
            </div>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (text.trim()) void send();
              }}
              className="flex gap-2"
            >
              <input value={text} onChange={(e) => setText(e.target.value)} maxLength={80} placeholder="Bence bu..." autoFocus className="h-11 flex-1 rounded-xl border border-border/70 bg-muted/40 px-4 text-sm outline-none focus:border-ring/60 focus:ring-4 focus:ring-ring/20" />
              <Button type="submit" disabled={busy || !text.trim()} className="h-11">Gönder</Button>
            </form>
            {error && <p className="text-xs text-destructive">{error}</p>}
          </>
        )}
      </div>
    );
  }

  // The album.
  const album = (d.album ?? []) as Step[][];
  const chain = album[d.revealChain] ?? [];
  const ended = g.status !== "playing";
  const pages = ended ? album : [chain];
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <Round n={Math.min(d.revealChain + 1, album.length)} total={album.length} label="Albüm" />
        {selfId === d.host && !ended && <Button onClick={() => void h.act("next").catch(() => undefined)} className="h-9">Sonraki <ArrowRight className="ml-1 size-4" /></Button>}
      </div>
      {pages.map((steps, ci) => (
        <ol key={ci} className="space-y-3">
          {steps.map((s, i) => (
            <li key={i} className="flex items-start gap-3 animate-in fade-in slide-in-from-bottom-2 duration-500">
              <div className="flex w-28 shrink-0 flex-col items-center gap-1 pt-1 text-center">
                {s.kind === "word" ? (
                  <span className="flex size-9 items-center justify-center rounded-full bg-violet-500/15 text-lg">📜</span>
                ) : (
                  <UserAvatar userId={s.by} name={nameOf(g.players, s.by)} className="size-9" fallbackClassName="bg-primary/10 text-xs text-primary" />
                )}
                <span className="text-[0.7rem] font-medium leading-tight text-muted-foreground">{s.kind === "word" ? "Başlangıç" : s.kind === "draw" ? `${nameOf(g.players, s.by).split(" ")[0]} çizdi` : `${nameOf(g.players, s.by).split(" ")[0]} yazdı`}</span>
              </div>
              {s.kind === "draw" ? (
                <div className="min-w-0 flex-1 overflow-hidden rounded-2xl border border-border/60 bg-white shadow-sm" style={{ aspectRatio: "8 / 5" }}>
                  {s.image ? <img src={s.image} alt="" className="size-full object-contain" draggable={false} /> : <p className="flex size-full items-center justify-center text-sm text-neutral-400">Boş sayfa</p>}
                </div>
              ) : (
                <Prompt className={s.kind === "word" ? "flex-1 text-lg font-semibold" : "flex-1 text-base"}>{s.text}</Prompt>
              )}
            </li>
          ))}
        </ol>
      ))}
      {!ended && <Note>Sayfa kendiliğinden çevrilir{selfId === d.host ? ", istersen sen de çevir" : ""}.</Note>}
    </div>
  );
}
