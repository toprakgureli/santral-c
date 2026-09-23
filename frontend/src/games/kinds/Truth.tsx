// Yalan mı Gerçek mi: three stories, one made up.

import { useState } from "react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";
import { nameOf, Note, Prompt, Round, type KindProps } from "@/games/kinds/shared";

export default function Truth({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const [texts, setTexts] = useState(["", "", ""]);
  const [lie, setLie] = useState(0);
  const isAuthor = d.author === selfId;

  if (d.phase === "write") {
    const done = !!d.mine;
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
        <Prompt>Başından geçen üç çağrı hikâyesi yaz. Biri uydurma olsun ve hangisinin uydurma olduğunu işaretle. Diğerleri bulmaya çalışacak.</Prompt>
        {done ? (
          <Note>Hikâyelerin alındı. {d.written} / {g.players.filter((p) => !p.left).length} kişi yazdı, kalanlar bekleniyor.</Note>
        ) : (
          <div className="space-y-3">
            {texts.map((t, i) => (
              <div key={i} className={cn("rounded-xl border p-3", lie === i ? "border-destructive/60 bg-destructive/5" : "border-border/60 bg-card")}>
                <div className="mb-1.5 flex items-center justify-between">
                  <span className="text-xs font-semibold text-muted-foreground">Hikâye {i + 1}</span>
                  <label className="flex items-center gap-1.5 text-xs">
                    <input type="radio" name="lie" checked={lie === i} onChange={() => setLie(i)} /> Bu uydurma
                  </label>
                </div>
                <textarea value={t} onChange={(e) => setTexts((cur) => cur.map((x, j) => (j === i ? e.target.value : x)))} rows={2} maxLength={300} placeholder="Müşteri aradı ve..." className="w-full resize-none rounded-lg border border-border/60 bg-muted/30 px-3 py-2 text-sm outline-none focus:border-ring/60" />
              </div>
            ))}
            <Button onClick={() => void h.act("write", { statements: texts, lie }).catch(() => undefined)} disabled={!g.joined || texts.some((t) => !t.trim())} className="h-10 w-full">Gönder</Button>
          </div>
        )}
      </div>
    );
  }

  const statements = (d.statements ?? []) as string[];
  const votes = (d.votes ?? {}) as Record<string, number>;
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.turn} total={d.order?.length ?? 0} label="Yazar" />
      <p className="text-sm text-muted-foreground"><strong className="text-foreground">{nameOf(g.players, d.author)}</strong>{isAuthor ? " (sen)" : ""} anlatıyor. Hangisi uydurma?</p>
      <div className="space-y-2">
        {statements.map((s, i) => {
          const isLie = d.phase === "reveal" && d.lie === i;
          const voters = d.phase === "reveal" ? Object.entries(votes).filter(([, v]) => v === i).map(([uid]) => nameOf(g.players, Number(uid)).split(" ")[0]) : [];
          return (
            <button
              key={i}
              type="button"
              disabled={isAuthor || d.phase !== "vote" || !g.joined}
              onClick={() => void h.act("vote", { index: i }).catch(() => undefined)}
              className={cn(
                "w-full rounded-xl border px-4 py-3 text-left text-sm leading-relaxed transition-colors",
                d.myVote === i && d.phase === "vote" ? "border-primary bg-primary/10" : "border-border/60 bg-card hover:bg-accent",
                isLie && "border-destructive bg-destructive/10",
                d.phase === "reveal" && !isLie && "opacity-70",
              )}
            >
              <span className="mr-2 text-xs font-semibold text-muted-foreground">{i + 1}.</span>
              {s}
              {isLie && <span className="ml-2 text-xs font-semibold text-destructive">UYDURMA</span>}
              {voters.length > 0 && <span className="mt-1 block text-xs text-muted-foreground">Bunu seçenler: {voters.join(", ")}</span>}
            </button>
          );
        })}
      </div>
      {d.phase === "vote" && <Note>{isAuthor ? "Sen izliyorsun; diğerleri tahmin ediyor." : `${d.voted} kişi seçti`}</Note>}
    </div>
  );
}
