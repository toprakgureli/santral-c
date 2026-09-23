// Kalem Kâğıt Anketi: a question, everyone points at someone in the
// company (the room first, then the rest, with a search), the crown goes
// to the most named.

import { useMemo, useState } from "react";
import { Crown, Search } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { Note, Prompt, Round, type KindProps } from "@/games/kinds/shared";

type Candidate = { id: number; name: string; hasAvatar: boolean };

export default function Poll({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const [q, setQ] = useState("");
  const last = d.results?.[d.results.length - 1];
  const candidates = (d.candidates ?? []) as Candidate[];
  const seated = new Set(g.players.filter((p) => !p.left).map((p) => p.id));
  const nameOf = (id: number) => candidates.find((c) => c.id === id)?.name ?? g.players.find((p) => p.id === id)?.name ?? "";

  const { inRoom, others } = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase("tr");
    const hit = (c: Candidate) => !needle || c.name.toLocaleLowerCase("tr").includes(needle);
    return {
      inRoom: candidates.filter((c) => seated.has(c.id) && hit(c)),
      others: candidates.filter((c) => !seated.has(c.id) && hit(c)),
    };
  }, [candidates, q, seated]);

  const Pick = ({ c }: { c: Candidate }) => (
    <button
      type="button"
      disabled={!g.joined}
      onClick={() => void h.act("vote", { target: c.id }).catch(() => undefined)}
      className={cn(
        "flex items-center gap-2.5 rounded-xl border px-3 py-2 text-left transition-colors",
        d.myVote === c.id ? "border-violet-500 bg-violet-500/10 shadow-md shadow-violet-500/10" : "border-border/60 bg-card/95 backdrop-blur hover:border-violet-500/40 hover:bg-accent",
      )}
    >
      <UserAvatar userId={c.id} name={c.name} hasAvatar={c.hasAvatar} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{c.name}{c.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</span>
    </button>
  );

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.round} total={d.total} label="Soru" />
      <Prompt className="text-lg font-medium">{d.question}</Prompt>
      {d.phase === "vote" ? (
        <>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Şirketten herkes seçilebilir, isim ara..." className="h-10 w-full rounded-xl border border-border/70 bg-card/95 pl-10 pr-3 text-sm outline-none backdrop-blur focus:border-violet-500/60 focus:ring-4 focus:ring-violet-500/15" />
          </div>
          {inRoom.length > 0 && (
            <div>
              <p className="mb-1.5 text-[0.65rem] font-bold uppercase tracking-widest text-muted-foreground">Odadakiler</p>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">{inRoom.map((c) => <Pick key={c.id} c={c} />)}</div>
            </div>
          )}
          {others.length > 0 && (
            <div>
              <p className="mb-1.5 text-[0.65rem] font-bold uppercase tracking-widest text-muted-foreground">Diğerleri</p>
              <div className="grid max-h-64 grid-cols-2 gap-2 overflow-y-auto pr-1 sm:grid-cols-3">{others.map((c) => <Pick key={c.id} c={c} />)}</div>
            </div>
          )}
          <Note>{d.voted} / {g.players.filter((p) => !p.left).length} oy verdi{d.myVote ? " · oyunu değiştirebilirsin" : ""}</Note>
        </>
      ) : last ? (
        <div className="rounded-2xl border border-border/60 bg-card/95 p-4 backdrop-blur">
          <p className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <Crown className="size-4 text-warning" />
            {(last.top as number[]).map((id) => nameOf(id) + (seated.has(id) ? "" : " (odada değil)")).join(", ") || "Kimse"}
          </p>
          <ul className="space-y-1.5">
            {Object.entries(last.counts as Record<string, number>).sort((a, b) => b[1] - a[1]).map(([id, n]) => (
              <li key={id} className="flex items-center gap-2 text-sm">
                <UserAvatar userId={Number(id)} name={nameOf(Number(id))} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                <span className="min-w-0 flex-1 truncate">{nameOf(Number(id))}{!seated.has(Number(id)) && <span className="text-xs text-muted-foreground"> · odada değil</span>}</span>
                <span className="h-2 rounded-full bg-violet-500/60" style={{ width: `${Math.max(8, (n / Math.max(1, d.voted || n)) * 160)}px` }} />
                <span className="w-6 text-right font-mono text-xs tabular-nums">{n}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
