// Kalem Kâğıt Anketi: a question, everyone points at someone, the crown
// goes to the most named.

import { Crown } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import { nameOf, Note, PeoplePick, Prompt, Round, type KindProps } from "@/games/kinds/shared";

export default function Poll({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const last = d.results?.[d.results.length - 1];
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.round} total={d.total} label="Soru" />
      <Prompt className="text-lg font-medium">{d.question}</Prompt>
      {d.phase === "vote" ? (
        <>
          <PeoplePick players={g.players} selfId={selfId} picked={d.myVote || undefined} disabled={!g.joined} onPick={(id) => void h.act("vote", { target: id }).catch(() => undefined)} />
          <Note>{d.voted} / {g.players.filter((p) => !p.left).length} oy verdi{d.myVote ? " · oyunu değiştirebilirsin" : ""}</Note>
        </>
      ) : last ? (
        <div className="rounded-2xl border border-border/60 bg-card p-4">
          <p className="mb-3 flex items-center gap-2 text-sm font-semibold"><Crown className="size-4 text-warning" /> {(last.top as number[]).map((id) => nameOf(g.players, id)).join(", ") || "Kimse"}</p>
          <ul className="space-y-1.5">
            {Object.entries(last.counts as Record<string, number>).sort((a, b) => b[1] - a[1]).map(([id, n]) => (
              <li key={id} className="flex items-center gap-2 text-sm">
                <UserAvatar userId={Number(id)} name={nameOf(g.players, Number(id))} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                <span className="min-w-0 flex-1 truncate">{nameOf(g.players, Number(id))}</span>
                <span className="h-2 rounded-full bg-primary/60" style={{ width: `${Math.max(8, (n / Math.max(1, d.voted || n)) * 160)}px` }} />
                <span className="w-6 text-right font-mono text-xs tabular-nums">{n}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
