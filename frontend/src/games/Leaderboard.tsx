// Leaderboard: this month's or all-time standings, optionally per game.

import { useEffect, useState } from "react";
import { Crown } from "lucide-react";
import { ApiError } from "@/api/client";
import { Modal } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { gamesApi } from "@/games/api";
import type { GameMeta, LeaderRow } from "@/games/types";
import { cn } from "@/lib/utils";

export default function Leaderboard({ open, onClose, kinds, selfId }: { open: boolean; onClose: () => void; kinds: GameMeta[]; selfId: number }) {
  const [period, setPeriod] = useState<"month" | "all">("month");
  const [kind, setKind] = useState("");
  const [rows, setRows] = useState<LeaderRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setRows(null);
    gamesApi
      .leaderboard(period, kind)
      .then(setRows)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Sıralama alınamadı."));
  }, [open, period, kind]);

  const month = new Date().toLocaleDateString("tr-TR", { month: "long" });
  return (
    <Modal open={open} onClose={onClose} title="Oyun sıralaması" description="Galibiyete göre; eşitlikte puan belirler." size="md">
      <div className="space-y-3">
        <div className="flex flex-wrap gap-1.5">
          {(["month", "all"] as const).map((p) => (
            <button key={p} type="button" onClick={() => setPeriod(p)} className={cn("rounded-full px-3 py-1 text-xs font-medium", period === p ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-accent")}>
              {p === "month" ? month : "Tüm zamanlar"}
            </button>
          ))}
          <span className="mx-1 w-px bg-border" />
          <button type="button" onClick={() => setKind("")} className={cn("rounded-full px-3 py-1 text-xs font-medium", kind === "" ? "bg-violet-500 text-white" : "bg-muted text-muted-foreground hover:bg-accent")}>Hepsi</button>
          {kinds.map((k) => (
            <button key={k.key} type="button" onClick={() => setKind(k.key)} className={cn("rounded-full px-3 py-1 text-xs font-medium", kind === k.key ? "bg-violet-500 text-white" : "bg-muted text-muted-foreground hover:bg-accent")}>{k.name}</button>
          ))}
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        {rows === null && !error && <p className="py-6 text-center text-sm text-muted-foreground">Yükleniyor...</p>}
        {rows?.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">Henüz oynanmış oyun yok.</p>}
        {rows && rows.length > 0 && (
          <ol className="space-y-1">
            {rows.map((r, i) => (
              <li key={r.userId} className={cn("flex items-center gap-3 rounded-xl px-3 py-2", i === 0 ? "bg-warning/10" : "bg-muted/30", r.userId === selfId && "ring-1 ring-primary/40")}>
                <span className="w-5 text-right text-xs tabular-nums text-muted-foreground">{i + 1}.</span>
                <UserAvatar userId={r.userId} name={r.name} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
                <span className="min-w-0 flex-1 truncate text-sm font-medium">{r.name}{i === 0 && <Crown className="ml-1.5 inline size-4 text-warning" />}</span>
                <span className="text-xs text-muted-foreground">{r.played} oyun</span>
                <span className="rounded-full bg-success/15 px-2 text-xs font-semibold tabular-nums text-success">{r.wins} galibiyet</span>
                <span className="w-14 text-right font-mono text-xs tabular-nums">{r.points} p</span>
              </li>
            ))}
          </ol>
        )}
      </div>
    </Modal>
  );
}
