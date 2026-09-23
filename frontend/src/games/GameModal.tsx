// GameModal: the game window. Lobby with seats and rules, then the game
// itself with a scoreboard and a clock, then the result. The window is
// calm on purpose: one accent colour, big type, nothing blinking.

import { useEffect, useState } from "react";
import { Crown, Pause, Trophy, X } from "lucide-react";
import { Button, ConfirmDialog } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { useGame } from "@/games/useGame";
import Draw from "@/games/kinds/Draw";
import Poll from "@/games/kinds/Poll";
import Truth from "@/games/kinds/Truth";
import Solve from "@/games/kinds/Solve";
import Story from "@/games/kinds/Story";
import WhoSaid from "@/games/kinds/WhoSaid";
import Connect4 from "@/games/kinds/Connect4";
import Hockey from "@/games/kinds/Hockey";
import Bingo from "@/games/kinds/Bingo";
import type { GameMeta } from "@/games/types";
import { cn } from "@/lib/utils";

export default function GameModal({ gameId, selfId, metas, pauseOnCall, onClose }: { gameId: number; selfId: number; metas: GameMeta[]; pauseOnCall: boolean; onClose: () => void }) {
  const h = useGame(gameId, pauseOnCall);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const g = h.game;
  const meta = metas.find((m) => m.key === g?.kind);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const sorted = g ? [...g.players].sort((a, b) => b.score - a.score) : [];
  const active = g ? g.players.filter((p) => !p.left).length : 0;
  const canStart = g ? g.status === "lobby" && (g.isHost || g.canManage) && active >= g.minPlayers : false;

  return (
    <div className="fixed inset-0 z-[85] flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm" onClick={onClose}>
      <div className="flex h-[min(92svh,820px)] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border/60 px-4">
          <span className="flex size-9 items-center justify-center rounded-xl bg-violet-500/15 text-violet-500">🎮</span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">{g?.kindName ?? "Oyun"}</p>
            <p className="truncate text-xs text-muted-foreground">{meta?.tagline}</p>
          </div>
          {g?.status === "playing" && g.paused && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-warning/15 px-3 py-1 text-xs font-medium text-warning"><Pause className="size-3.5" /> Duraklatıldı · {g.pausedBy.join(", ")} çağrıda</span>
          )}
          {g?.status === "playing" && !g.paused && h.seconds > 0 && (
            <span className={cn("rounded-full px-3 py-1 font-mono text-sm font-semibold tabular-nums", h.seconds <= 5 ? "bg-destructive/15 text-destructive" : "bg-muted text-foreground")}>{h.seconds}s</span>
          )}
          <button type="button" onClick={onClose} aria-label="Kapat" className="flex size-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-5" /></button>
        </header>

        {h.error && <p className="border-b border-destructive/30 bg-destructive/10 px-4 py-1.5 text-xs text-destructive">{h.error}</p>}

        {!g ? (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">Yükleniyor...</div>
        ) : (
          <div className="flex min-h-0 flex-1">
            <main className="relative flex min-w-0 flex-1 flex-col overflow-y-auto p-4">
              {g.status === "lobby" && (
                <div className="mx-auto flex w-full max-w-lg flex-1 flex-col justify-center gap-5">
                  <div className="rounded-2xl border border-border/60 bg-card p-5">
                    <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Nasıl oynanır</p>
                    <p className="mt-1.5 text-sm leading-relaxed">{meta?.how}</p>
                    <p className="mt-3 text-xs text-muted-foreground">
                      {g.minPlayers === g.maxPlayers ? `${g.minPlayers} oyuncu` : g.maxPlayers > 0 ? `${g.minPlayers} ile ${g.maxPlayers} oyuncu` : `en az ${g.minPlayers} oyuncu`}
                      {meta?.roundsLabel ? ` · ${meta.roundsLabel}: ${g.config.rounds}` : ""}
                      {meta?.secondsLabel ? ` · ${meta.secondsLabel}: ${g.config.seconds}` : ""}
                    </p>
                  </div>
                  <div className="text-center">
                    <p className="text-sm text-muted-foreground">{active < g.minPlayers ? `Başlamak için ${g.minPlayers - active} oyuncu daha gerekli` : "Herkes hazır"}</p>
                    <div className="mt-3 flex flex-wrap justify-center gap-2">
                      {!g.joined && <Button onClick={() => void h.join()} disabled={h.busy} className="h-10">Katıl</Button>}
                      {g.joined && <Button variant="secondary" onClick={() => void h.leave()} disabled={h.busy} className="h-10">Ayrıl</Button>}
                      {canStart && <Button onClick={() => void h.start()} disabled={h.busy} className="h-10 bg-success text-white hover:bg-success/90">Başlat</Button>}
                      {(g.isHost || g.canManage) && <Button variant="ghost" onClick={() => setConfirmCancel(true)} disabled={h.busy} className="h-10 text-destructive">İptal et</Button>}
                    </div>
                  </div>
                </div>
              )}

              {g.status === "playing" && (
                <>
                  {g.paused && (
                    <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 bg-background/80 backdrop-blur-sm">
                      <Pause className="size-8 text-warning" />
                      <p className="text-sm font-medium">Oyun duraklatıldı</p>
                      <p className="text-xs text-muted-foreground">{g.pausedBy.join(", ")} çağrıda, bitince kaldığı yerden devam eder.</p>
                    </div>
                  )}
                  {g.kind === "draw" && <Draw h={h} selfId={selfId} />}
                  {g.kind === "poll" && <Poll h={h} selfId={selfId} />}
                  {g.kind === "truth" && <Truth h={h} selfId={selfId} />}
                  {g.kind === "solve" && <Solve h={h} selfId={selfId} />}
                  {g.kind === "story" && <Story h={h} selfId={selfId} />}
                  {g.kind === "whosaid" && <WhoSaid h={h} selfId={selfId} />}
                  {g.kind === "connect4" && <Connect4 h={h} selfId={selfId} />}
                  {g.kind === "hockey" && <Hockey h={h} selfId={selfId} />}
                  {g.kind === "bingo" && <Bingo h={h} selfId={selfId} />}
                </>
              )}

              {(g.status === "finished" || g.status === "cancelled") && (
                <div className="mx-auto flex w-full max-w-lg flex-1 flex-col justify-center gap-4 text-center">
                  {g.status === "cancelled" ? (
                    <p className="text-sm text-muted-foreground">Oyun iptal edildi.</p>
                  ) : (
                    <>
                      <span className="mx-auto flex size-16 items-center justify-center rounded-full bg-success/15 text-success"><Trophy className="size-8" /></span>
                      <p className="text-lg font-semibold">
                        {g.winners.length === 0 ? "Kazanan çıkmadı" : g.winners.length === 1 ? `${g.players.find((p) => p.id === g.winners[0])?.name} kazandı!` : `${g.players.filter((p) => g.winners.includes(p.id)).map((p) => p.name).join(", ")} berabere`}
                      </p>
                      <ol className="mx-auto w-full max-w-sm space-y-1">
                        {sorted.map((p, i) => (
                          <li key={p.id} className={cn("flex items-center gap-3 rounded-xl px-3 py-2", g.winners.includes(p.id) ? "bg-success/10" : "bg-muted/40")}>
                            <span className="w-5 text-right text-xs tabular-nums text-muted-foreground">{i + 1}.</span>
                            <UserAvatar userId={p.id} name={p.name} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                            <span className="min-w-0 flex-1 truncate text-left text-sm">{p.name}</span>
                            {g.winners.includes(p.id) && <Crown className="size-4 text-warning" />}
                            <span className="font-mono text-sm tabular-nums">{p.score}</span>
                          </li>
                        ))}
                      </ol>
                      {g.kind === "story" && g.data?.sentences && (
                        <p className="rounded-xl bg-muted/40 p-4 text-left text-sm leading-relaxed">
                          {g.data.prompt} {(g.data.sentences as { text: string }[]).map((s) => s.text).join(" ")}
                        </p>
                      )}
                    </>
                  )}
                  <Button variant="secondary" onClick={onClose} className="mx-auto h-9">Kapat</Button>
                </div>
              )}
            </main>

            {g.status !== "lobby" && (
              <aside className="hidden w-56 shrink-0 flex-col border-l border-sidebar-border bg-sidebar sm:flex">
                <p className="px-4 pt-3 pb-2 text-[0.6875rem] font-semibold tracking-wider text-muted-foreground/70 uppercase">Skor</p>
                <ol className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2">
                  {sorted.map((p, i) => (
                    <li key={p.id} className={cn("flex items-center gap-2 rounded-xl px-2 py-1.5", p.id === selfId && "bg-sidebar-accent/60", p.left && "opacity-50")}>
                      <span className="w-4 text-right text-[0.65rem] tabular-nums text-muted-foreground">{i + 1}</span>
                      <UserAvatar userId={p.id} name={p.name} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                      <span className={cn("min-w-0 flex-1 truncate text-xs", p.left && "line-through")}>{p.name.split(" ")[0]}</span>
                      <span className="font-mono text-xs tabular-nums">{p.score}</span>
                    </li>
                  ))}
                </ol>
                {g.status === "playing" && (
                  <div className="border-t border-sidebar-border p-2">
                    {g.joined ? (
                      <Button variant="ghost" onClick={() => void h.leave()} disabled={h.busy} className="h-8 w-full text-xs text-muted-foreground">Oyundan ayrıl</Button>
                    ) : g.joinLate ? (
                      <Button onClick={() => void h.join()} disabled={h.busy} className="h-8 w-full text-xs">Katıl</Button>
                    ) : null}
                    {(g.isHost || g.canManage) && <Button variant="ghost" onClick={() => setConfirmCancel(true)} disabled={h.busy} className="mt-1 h-8 w-full text-xs text-destructive">Oyunu bitir</Button>}
                  </div>
                )}
              </aside>
            )}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={confirmCancel}
        title="Oyunu iptal et"
        description="Oyun kapanır, skor kaydedilmez."
        confirmLabel="Evet, iptal et"
        busy={h.busy}
        onConfirm={() => {
          setConfirmCancel(false);
          void h.cancel();
        }}
        onCancel={() => setConfirmCancel(false)}
      />
    </div>
  );
}
