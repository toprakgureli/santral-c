// GameCard: the match as a line in the room. The lobby shows who has sat
// down, who is invited and still coming, and the empty seats; while
// playing, the score; when over, the winner. Opening it shows the window.

import { Clock, Crown, Gamepad2, Trophy, UserPlus } from "lucide-react";
import { Button } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { ICONS } from "@/games/StartGameDialog";
import { useGame } from "@/games/useGame";
import { cn } from "@/lib/utils";

export default function GameCard({ gameId, onOpen }: { gameId: number; onOpen: () => void }) {
  const h = useGame(gameId, false);
  const g = h.game;
  if (!g) return <div className="mt-1 h-24 w-full max-w-md animate-pulse rounded-2xl bg-muted/40" />;
  const seated = g.players.filter((p) => !p.left);
  const winners = g.players.filter((p) => g.winners.includes(p.id));
  const need = Math.max(0, g.minPlayers - seated.length);
  const empties = g.status === "lobby" ? Math.max(need, g.maxPlayers > 0 ? Math.min(2, g.maxPlayers - seated.length - g.invited.length) : Math.min(2, need)) : 0;
  const accent = g.status === "playing" ? "from-primary/25 to-primary/5 ring-primary/40" : g.status === "finished" ? "from-success/25 to-success/5 ring-success/40" : g.status === "cancelled" ? "from-muted to-muted/40 ring-border/60 opacity-70" : "from-violet-500/25 to-violet-500/5 ring-violet-500/40";

  return (
    <div className={cn("mt-1 w-full max-w-md overflow-hidden rounded-2xl bg-card shadow-md ring-1", accent.split(" ").pop())}>
      <div className={cn("flex items-center gap-3 bg-gradient-to-r px-4 py-3", accent)}>
        <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-background/70 text-2xl shadow-sm ring-1 ring-black/5">{g.status === "finished" ? "🏆" : ICONS[g.kind] ?? "🎮"}</span>
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 text-sm font-semibold">
            {g.kindName}
            <span className={cn("rounded-full px-2 py-0.5 text-[0.6rem] font-bold uppercase tracking-wider", g.status === "playing" ? "bg-primary text-primary-foreground" : g.status === "finished" ? "bg-success text-white" : g.status === "cancelled" ? "bg-muted text-muted-foreground" : "bg-violet-500 text-white")}>
              {g.status === "lobby" ? "Lobi" : g.status === "playing" ? (g.paused ? "Duraklatıldı" : "Oynanıyor") : g.status === "finished" ? "Bitti" : "İptal"}
            </span>
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {g.status === "lobby" && (need > 0 ? `${seated.length} kişi oturdu · ${need} kişi daha gerekli` : `${seated.length} kişi hazır · kurucu başlatabilir`)}
            {g.status === "playing" && (g.paused ? `${g.pausedBy.join(", ")} çağrıda, oyun bekliyor` : `${seated.length} oyuncu masada`)}
            {g.status === "finished" && (winners.length ? `${winners.map((w) => w.name).join(", ")} kazandı` : "Kazanan çıkmadı")}
            {g.status === "cancelled" && "Oyun iptal edildi"}
          </p>
        </div>
        {g.status === "lobby" && !g.joined && <Button onClick={() => void h.join()} disabled={h.busy} className="h-8 bg-violet-500 px-3 text-xs text-white hover:bg-violet-500/90">Katıl</Button>}
        {(g.status === "playing" || (g.status === "lobby" && g.joined)) && <Button onClick={onOpen} className="h-8 px-3 text-xs">{g.status === "playing" ? (g.joined ? "Oyuna dön" : "İzle") : "Lobiyi aç"}</Button>}
        {g.status === "finished" && <Button variant="secondary" onClick={onOpen} className="h-8 px-3 text-xs">Sonuç</Button>}
      </div>

      {g.status !== "cancelled" && (
        <div className="flex flex-wrap items-center gap-2 px-4 py-2.5">
          {g.players.map((p) => (
            <span key={p.id} className={cn("inline-flex items-center gap-1.5 rounded-full bg-muted/60 py-0.5 pr-2.5 pl-0.5 text-xs", p.left && "line-through opacity-50", g.winners.includes(p.id) && "bg-success/15 text-success")} data-tip={`${p.name}${g.status !== "lobby" ? ` · ${p.score} puan` : ""}`}>
              <UserAvatar userId={p.id} name={p.name} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
              {p.name.split(" ")[0]}
              {p.id === g.hostId && <Crown className="size-3 text-warning" />}
              {g.status !== "lobby" && <span className="font-mono tabular-nums text-muted-foreground">{p.score}</span>}
            </span>
          ))}
          {g.status === "lobby" && g.invited.map((p) => (
            <span key={`i-${p.id}`} className="inline-flex items-center gap-1.5 rounded-full border border-dashed border-warning/60 py-0.5 pr-2.5 pl-0.5 text-xs text-muted-foreground" data-tip={`${p.name} davet edildi, bekleniyor`}>
              <UserAvatar userId={p.id} name={p.name} className="size-6 opacity-60" fallbackClassName="bg-muted text-[0.55rem]" />
              {p.name.split(" ")[0]}
              <Clock className="size-3 text-warning" />
            </span>
          ))}
          {Array.from({ length: empties }).map((_, i) => (
            <span key={`e-${i}`} className="inline-flex items-center gap-1.5 rounded-full border border-dashed border-border py-0.5 pr-2.5 pl-1 text-xs text-muted-foreground/70">
              <span className="flex size-5 items-center justify-center rounded-full bg-muted/60"><UserPlus className="size-3" /></span>
              boş koltuk
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

export { Gamepad2, Trophy };
