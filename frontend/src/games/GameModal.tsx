// GameModal: the game window. A lobby with seats, invites and the rules;
// then the game with a scoreboard and a clock; then the result. It should
// feel like a game, not a form: one bold accent, a big emblem, a soft
// patterned floor, and nothing that blinks for no reason.

import { useEffect, useState } from "react";
import { Clock, Crown, Pause, Trophy, UserPlus, X } from "lucide-react";
import { Button, ConfirmDialog, Modal } from "@/components/ui";
import PeoplePicker from "@/components/teams/PeoplePicker";
import UserAvatar from "@/components/ui/UserAvatar";
import { ICONS } from "@/games/StartGameDialog";
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

const FLOOR = "bg-[radial-gradient(circle_at_1px_1px,color-mix(in_oklab,var(--foreground)_7%,transparent)_1px,transparent_0)] bg-[size:18px_18px]";

export default function GameModal({ gameId, selfId, metas, pauseOnCall, members, onClose }: { gameId: number; selfId: number; metas: GameMeta[]; pauseOnCall: boolean; members: number[]; onClose: () => void }) {
  const h = useGame(gameId, pauseOnCall);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [inviting, setInviting] = useState(false);
  const [picked, setPicked] = useState<number[]>([]);
  const g = h.game;
  const meta = metas.find((m) => m.key === g?.kind);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !inviting && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, inviting]);

  const sorted = g ? [...g.players].sort((a, b) => b.score - a.score) : [];
  const seated = g ? g.players.filter((p) => !p.left) : [];
  const need = g ? Math.max(0, g.minPlayers - seated.length) : 0;
  const canStart = g ? g.status === "lobby" && (g.isHost || g.canManage) && need === 0 : false;
  const emptySeats = g ? Math.max(need, g.maxPlayers > 0 ? Math.min(3, g.maxPlayers - seated.length - g.invited.length) : Math.min(3, need)) : 0;

  return (
    <div className="fixed inset-0 z-[85] flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm" onClick={onClose}>
      <div className="flex h-[min(92svh,820px)] w-full max-w-5xl flex-col overflow-hidden rounded-3xl border border-border bg-background shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <header className="relative flex h-16 shrink-0 items-center gap-3 overflow-hidden bg-gradient-to-r from-violet-600 via-indigo-600 to-primary px-4 text-white">
          <span className="pointer-events-none absolute -top-10 -right-10 text-[9rem] opacity-15 select-none">{ICONS[g?.kind ?? ""] ?? "🎮"}</span>
          <span className="flex size-10 items-center justify-center rounded-xl bg-white/15 text-2xl shadow-inner ring-1 ring-white/20">{ICONS[g?.kind ?? ""] ?? "🎮"}</span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-bold tracking-tight">{g?.kindName ?? "Oyun"}</p>
            <p className="truncate text-xs text-white/75">{meta?.tagline}</p>
          </div>
          {g?.status === "playing" && g.paused && (
            <span className="inline-flex items-center gap-1.5 rounded-full bg-white/15 px-3 py-1 text-xs font-medium"><Pause className="size-3.5" /> {g.pausedBy.join(", ")} çağrıda</span>
          )}
          {g?.status === "playing" && !g.paused && h.seconds > 0 && (
            <span className={cn("rounded-full px-3.5 py-1 font-mono text-base font-bold tabular-nums shadow-sm", h.seconds <= 5 ? "bg-red-500 text-white" : "bg-white text-indigo-700")}>{h.seconds}</span>
          )}
          <button type="button" onClick={onClose} aria-label="Kapat" className="relative flex size-9 items-center justify-center rounded-lg text-white/80 hover:bg-white/15 hover:text-white"><X className="size-5" /></button>
        </header>

        {h.error && <p className="border-b border-destructive/30 bg-destructive/10 px-4 py-1.5 text-xs text-destructive">{h.error}</p>}

        {!g ? (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">Yükleniyor...</div>
        ) : (
          <div className="flex min-h-0 flex-1">
            <main className={cn("relative flex min-w-0 flex-1 flex-col overflow-y-auto p-4", FLOOR)}>
              {g.status === "lobby" && (
                <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col justify-center gap-5">
                  <div className="rounded-2xl border border-border/60 bg-card/90 p-5 shadow-sm backdrop-blur">
                    <p className="text-[0.65rem] font-bold uppercase tracking-widest text-violet-500">Nasıl oynanır</p>
                    <p className="mt-1.5 text-sm leading-relaxed">{meta?.how}</p>
                    <p className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      <span>👥 {g.minPlayers === g.maxPlayers ? `${g.minPlayers} oyuncu` : g.maxPlayers > 0 ? `${g.minPlayers} ile ${g.maxPlayers} oyuncu` : `en az ${g.minPlayers} oyuncu`}</span>
                      {meta?.roundsLabel && <span>🔁 {meta.roundsLabel}: {g.config.rounds}</span>}
                      {meta?.secondsLabel && <span>⏱ {meta.secondsLabel}: {g.config.seconds}</span>}
                    </p>
                  </div>

                  <div className="rounded-2xl border border-border/60 bg-card/90 p-5 shadow-sm backdrop-blur">
                    <div className="mb-3 flex items-center justify-between">
                      <p className="text-[0.65rem] font-bold uppercase tracking-widest text-muted-foreground">Koltuklar</p>
                      {g.joined && <button type="button" onClick={() => { setPicked([]); setInviting(true); }} className="inline-flex items-center gap-1 text-xs font-medium text-violet-500 hover:underline"><UserPlus className="size-3.5" /> Davet et</button>}
                    </div>
                    <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                      {g.players.filter((p) => !p.left).map((p) => (
                        <div key={p.id} className={cn("flex flex-col items-center gap-1.5 rounded-xl border bg-background/80 p-3", p.id === selfId ? "border-violet-500/60" : "border-border/60")}>
                          <span className="relative">
                            <UserAvatar userId={p.id} name={p.name} className="size-12" fallbackClassName="bg-primary/10 text-sm text-primary" />
                            {p.id === g.hostId && <span className="absolute -top-1 -right-1 flex size-5 items-center justify-center rounded-full bg-warning text-black ring-2 ring-background"><Crown className="size-3" /></span>}
                          </span>
                          <span className="max-w-full truncate text-xs font-medium">{p.name.split(" ")[0]}{p.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</span>
                          <span className="text-[0.6rem] text-success">hazır</span>
                        </div>
                      ))}
                      {g.invited.map((p) => (
                        <div key={`i-${p.id}`} className="flex flex-col items-center gap-1.5 rounded-xl border border-dashed border-warning/60 bg-background/50 p-3">
                          <UserAvatar userId={p.id} name={p.name} className="size-12 opacity-60" fallbackClassName="bg-muted text-sm" />
                          <span className="max-w-full truncate text-xs font-medium text-muted-foreground">{p.name.split(" ")[0]}</span>
                          <span className="inline-flex items-center gap-1 text-[0.6rem] text-warning"><Clock className="size-3" /> bekleniyor</span>
                        </div>
                      ))}
                      {Array.from({ length: emptySeats }).map((_, i) => (
                        <button key={`e-${i}`} type="button" onClick={() => { if (g.joined) { setPicked([]); setInviting(true); } }} className="flex flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed border-border/70 bg-background/40 p-3 text-muted-foreground/60 transition-colors hover:border-violet-500/60 hover:text-violet-500">
                          <span className="flex size-12 items-center justify-center rounded-full bg-muted/60"><UserPlus className="size-5" /></span>
                          <span className="text-xs">boş koltuk</span>
                          <span className="text-[0.6rem]">{i === 0 && need > 0 ? `${need} kişi gerekli` : "davet et"}</span>
                        </button>
                      ))}
                    </div>
                  </div>

                  <div className="flex flex-wrap items-center justify-center gap-2">
                    {!g.joined && <Button onClick={() => void h.join()} disabled={h.busy} className="h-11 bg-violet-500 px-6 text-white hover:bg-violet-500/90">Koltuğa otur</Button>}
                    {g.joined && <Button variant="secondary" onClick={() => void h.leave()} disabled={h.busy} className="h-11">Kalk</Button>}
                    {canStart && <Button onClick={() => void h.start()} disabled={h.busy} className="h-11 bg-success px-8 text-base text-white shadow-lg shadow-success/30 hover:bg-success/90">Başlat</Button>}
                    {g.joined && !canStart && g.isHost && <span className="text-sm text-muted-foreground">Başlamak için {need} kişi daha lazım</span>}
                    {g.joined && !g.isHost && <span className="text-sm text-muted-foreground">{need > 0 ? `${need} kişi daha bekleniyor` : "Kurucunun başlatması bekleniyor"}</span>}
                    {(g.isHost || g.canManage) && <Button variant="ghost" onClick={() => setConfirmCancel(true)} disabled={h.busy} className="h-11 text-destructive">İptal et</Button>}
                  </div>
                </div>
              )}

              {g.status === "playing" && (
                <>
                  {g.paused && (
                    <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 bg-background/85 backdrop-blur-sm">
                      <span className="flex size-14 items-center justify-center rounded-full bg-warning/15 text-warning"><Pause className="size-7" /></span>
                      <p className="text-base font-semibold">Oyun duraklatıldı</p>
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
                      <span className="mx-auto flex size-20 items-center justify-center rounded-full bg-gradient-to-br from-amber-300 to-amber-500 text-4xl shadow-lg shadow-amber-500/30">🏆</span>
                      <p className="text-xl font-bold tracking-tight">
                        {g.winners.length === 0 ? "Kazanan çıkmadı" : g.winners.length === 1 ? `${g.players.find((p) => p.id === g.winners[0])?.name} kazandı!` : `${g.players.filter((p) => g.winners.includes(p.id)).map((p) => p.name).join(", ")} berabere`}
                      </p>
                      <ol className="mx-auto w-full max-w-sm space-y-1.5">
                        {sorted.map((p, i) => (
                          <li key={p.id} className={cn("flex items-center gap-3 rounded-xl px-3 py-2 backdrop-blur", g.winners.includes(p.id) ? "bg-success/15" : "bg-card/90")}>
                            <span className={cn("flex size-6 items-center justify-center rounded-full text-[0.65rem] font-bold", i === 0 ? "bg-amber-400 text-black" : i === 1 ? "bg-slate-300 text-black" : i === 2 ? "bg-amber-700 text-white" : "bg-muted text-muted-foreground")}>{i + 1}</span>
                            <UserAvatar userId={p.id} name={p.name} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                            <span className="min-w-0 flex-1 truncate text-left text-sm">{p.name}</span>
                            {g.winners.includes(p.id) && <Crown className="size-4 text-warning" />}
                            <span className="font-mono text-sm font-semibold tabular-nums">{p.score}</span>
                          </li>
                        ))}
                      </ol>
                      {g.kind === "story" && g.data?.sentences && (
                        <p className="rounded-xl bg-card/90 p-4 text-left text-sm leading-relaxed backdrop-blur">
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
                <p className="flex items-center gap-1.5 px-4 pt-3 pb-2 text-[0.65rem] font-bold tracking-widest text-muted-foreground/70 uppercase"><Trophy className="size-3.5 text-warning" /> Skor</p>
                <ol className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2">
                  {sorted.map((p, i) => (
                    <li key={p.id} className={cn("flex items-center gap-2 rounded-xl px-2 py-1.5", p.id === selfId && "bg-sidebar-accent/60", p.left && "opacity-50")}>
                      <span className={cn("flex size-5 items-center justify-center rounded-full text-[0.6rem] font-bold", i === 0 && p.score > 0 ? "bg-amber-400 text-black" : "bg-muted text-muted-foreground")}>{i + 1}</span>
                      <UserAvatar userId={p.id} name={p.name} className="size-6" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />
                      <span className={cn("min-w-0 flex-1 truncate text-xs", p.left && "line-through")}>{p.name.split(" ")[0]}</span>
                      <span className="font-mono text-xs font-semibold tabular-nums">{p.score}</span>
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

      {g && (
        <Modal
          open={inviting}
          onClose={() => setInviting(false)}
          title="Oyuna davet et"
          description="Odadaki kişilere davet kartı gider; kabul edince koltuğa otururlar."
          footer={
            <>
              <Button variant="secondary" onClick={() => setInviting(false)} className="h-9" disabled={h.busy}>Vazgeç</Button>
              <Button onClick={() => void h.invite(picked).then(() => setInviting(false))} disabled={h.busy || picked.length === 0} className="h-9">Davet gönder ({picked.length})</Button>
            </>
          }
        >
          <PeoplePicker selected={picked} onChange={setPicked} only={members} exclude={[...g.players.filter((p) => !p.left).map((p) => p.id), ...g.invited.map((p) => p.id)]} height={280} />
        </Modal>
      )}
    </div>
  );
}
