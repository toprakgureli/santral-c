// 60 Saniyede Çöz: the same scenario for everyone, then a vote on the
// best answer.

import { useEffect, useState } from "react";
import { Button } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { nameOf, Note, Prompt, Round, type KindProps } from "@/games/kinds/shared";

export default function Solve({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const [text, setText] = useState("");
  useEffect(() => setText(""), [d.round]);
  const answers = (d.answers ?? {}) as Record<string, string>;
  const last = d.results?.[d.results.length - 1];

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.round} total={d.total} label="Senaryo" />
      <Prompt className="whitespace-pre-wrap">{d.scenario}</Prompt>

      {d.phase === "write" && (
        d.mine ? (
          <Note>Çözümün alındı. {d.answered} / {g.players.filter((p) => !p.left).length} kişi yazdı.</Note>
        ) : (
          <div className="space-y-2">
            <textarea value={text} onChange={(e) => setText(e.target.value)} rows={5} maxLength={600} placeholder="1. Önce... 2. Sonra..." className="w-full resize-none rounded-xl border border-border/60 bg-card px-4 py-3 text-sm outline-none focus:border-ring/60 focus:ring-4 focus:ring-ring/20" autoFocus />
            <Button onClick={() => void h.act("answer", { text }).catch(() => undefined)} disabled={!g.joined || !text.trim()} className="h-10 w-full">Çözümü gönder</Button>
          </div>
        )
      )}

      {d.phase === "vote" && (
        <>
          <Note>En iyi çözüme oy ver (kendine veremezsin)</Note>
          <div className="space-y-2">
            {Object.entries(answers).map(([uid, a], i) => {
              const mine = Number(uid) === selfId;
              return (
                <button
                  key={uid}
                  type="button"
                  disabled={mine || !g.joined}
                  onClick={() => void h.act("vote", { target: Number(uid) }).catch(() => undefined)}
                  className={cn("w-full rounded-xl border px-4 py-3 text-left text-sm whitespace-pre-wrap transition-colors", d.myVote === Number(uid) ? "border-primary bg-primary/10" : "border-border/60 bg-card hover:bg-accent", mine && "opacity-60")}
                >
                  <span className="mr-2 text-xs font-semibold text-muted-foreground">Çözüm {i + 1}{mine ? " (senin)" : ""}</span>
                  <span className="block">{a}</span>
                </button>
              );
            })}
          </div>
          <Note>{d.voted} kişi oy verdi</Note>
        </>
      )}

      {d.phase === "reveal" && last && (
        <div className="space-y-2">
          {Object.entries(last.answers as Record<string, string>).sort((a, b) => ((last.votes as Record<string, number>)[b[0]] ?? 0) - ((last.votes as Record<string, number>)[a[0]] ?? 0)).map(([uid, a]) => {
            const n = (last.votes as Record<string, number>)[uid] ?? 0;
            const best = (last.best as number[]).includes(Number(uid));
            return (
              <div key={uid} className={cn("rounded-xl border px-4 py-3 text-sm", best ? "border-success/60 bg-success/10" : "border-border/60 bg-card")}>
                <div className="mb-1 flex items-center gap-2">
                  <UserAvatar userId={Number(uid)} name={nameOf(g.players, Number(uid))} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                  <span className="font-medium">{nameOf(g.players, Number(uid))}</span>
                  <span className="ml-auto rounded-full bg-muted px-2 text-xs tabular-nums">{n} oy</span>
                </div>
                <p className="whitespace-pre-wrap text-muted-foreground">{a}</p>
              </div>
            );
          })}
          {d.sample && (
            <div className="rounded-xl border border-dashed border-border/60 px-4 py-3 text-sm">
              <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Örnek çözüm</p>
              <p className="whitespace-pre-wrap">{d.sample}</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
