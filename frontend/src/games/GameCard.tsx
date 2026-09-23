// GameCard: the match as a line in the room. In the lobby it shows the
// seats and a way in; while playing, the score; when over, the winner.
// Opening it shows the full game window.

import { Gamepad2, Trophy, Users } from "lucide-react";
import { Button } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { useGame } from "@/games/useGame";
import { cn } from "@/lib/utils";

export default function GameCard({ gameId, onOpen }: { gameId: number; onOpen: () => void }) {
  const h = useGame(gameId, false);
  const g = h.game;
  if (!g) return <div className="mt-1 h-20 w-full max-w-md animate-pulse rounded-xl bg-muted/40" />;
  const winners = g.players.filter((p) => g.winners.includes(p.id));
  const seats = g.maxPlayers > 0 ? `${g.players.filter((p) => !p.left).length} / ${g.maxPlayers}` : `${g.players.filter((p) => !p.left).length}`;
  const tone = g.status === "playing" ? "border-primary/50" : g.status === "finished" ? "border-success/50" : g.status === "cancelled" ? "border-border/60 opacity-70" : "border-violet-500/50";
  return (
    <div className={cn("mt-1 w-full max-w-md overflow-hidden rounded-2xl border bg-card shadow-sm", tone)}>
      <div className="flex items-center gap-3 px-4 py-3">
        <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl", g.status === "finished" ? "bg-success/15 text-success" : "bg-violet-500/15 text-violet-500")}>
          {g.status === "finished" ? <Trophy className="size-5" /> : <Gamepad2 className="size-5" />}
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{g.kindName}</p>
          <p className="truncate text-xs text-muted-foreground">
            {g.status === "lobby" && (() => {
              const n = g.players.filter((p) => !p.left).length;
              return n < g.minPlayers ? `${n} kişi katıldı · başlamak için ${g.minPlayers - n} kişi daha gerekli` : `${n} kişi hazır · kurucu başlatabilir`;
            })()}
            {g.status === "playing" && (g.paused ? `Duraklatıldı · ${g.pausedBy.join(", ")} çağrıda` : `Oynanıyor · ${seats} oyuncu`)}
            {g.status === "finished" && (winners.length ? `${winners.map((w) => w.name).join(", ")} kazandı` : "Bitti")}
            {g.status === "cancelled" && "İptal edildi"}
          </p>
        </div>
        {g.status === "lobby" && !g.joined && <Button onClick={() => void h.join()} disabled={h.busy} className="h-8 px-3 text-xs">Katıl</Button>}
        {(g.status === "playing" || (g.status === "lobby" && g.joined)) && <Button onClick={onOpen} className="h-8 px-3 text-xs">{g.status === "playing" ? (g.joined ? "Oyuna dön" : "İzle") : "Lobiyi aç"}</Button>}
        {g.status === "finished" && <Button variant="secondary" onClick={onOpen} className="h-8 px-3 text-xs">Sonuç</Button>}
      </div>
      {g.players.length > 0 && g.status !== "cancelled" && (
        <div className="flex flex-wrap items-center gap-1.5 border-t border-border/60 px-4 py-2">
          <Users className="size-3.5 text-muted-foreground" />
          {g.players.map((p) => (
            <span key={p.id} className={cn("inline-flex items-center gap-1 rounded-full bg-muted/50 py-0.5 pr-2 pl-0.5 text-[0.7rem]", p.left && "line-through opacity-50", g.winners.includes(p.id) && "bg-success/15 text-success")} title={`${p.name} · ${p.score} puan`}>
              <UserAvatar userId={p.id} name={p.name} className="size-4" fallbackClassName="bg-primary/10 text-[0.5rem] text-primary" />
              {p.name.split(" ")[0]}
              {g.status !== "lobby" && <span className="tabular-nums text-muted-foreground">{p.score}</span>}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
