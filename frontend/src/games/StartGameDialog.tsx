// StartGameDialog: pick a game, set its two knobs, open the lobby.

import { useState } from "react";
import { ApiError } from "@/api/client";
import { Button, Modal } from "@/components/ui";
import { gamesApi } from "@/games/api";
import type { GameMeta, GamesConfig, GameView } from "@/games/types";
import { cn } from "@/lib/utils";

export default function StartGameDialog({ groupId, config, open, onClose, onCreated }: { groupId: number; config: GamesConfig; open: boolean; onClose: () => void; onCreated: (g: GameView) => void }) {
  const [kind, setKind] = useState<GameMeta | null>(null);
  const [rounds, setRounds] = useState(0);
  const [seconds, setSeconds] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pick = (m: GameMeta) => {
    setKind(m);
    setRounds(m.defaultRounds);
    setSeconds(m.defaultSeconds);
    setError(null);
  };

  const create = async () => {
    if (!kind) return;
    setBusy(true);
    setError(null);
    try {
      const g = await gamesApi.create(groupId, { kind: kind.key, rounds, seconds });
      onCreated(g);
      onClose();
      setKind(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Oyun açılamadı.");
    } finally {
      setBusy(false);
    }
  };

  const lacking = (m: GameMeta) => m.itemKind && (config.itemCounts[m.itemKind] ?? 0) < m.minItems;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Oyun başlat"
      description={kind ? kind.how : "Bir oyun seç; kart odaya düşer, isteyen koltuğa oturur."}
      size="lg"
      footer={
        <>
          {kind && <Button variant="ghost" onClick={() => setKind(null)} className="mr-auto h-9">Geri</Button>}
          <Button variant="secondary" onClick={onClose} className="h-9" disabled={busy}>Vazgeç</Button>
          {kind && <Button onClick={() => void create()} disabled={busy || !!lacking(kind)} className="h-9">{busy ? "Açılıyor..." : "Lobiyi aç"}</Button>}
        </>
      }
    >
      {!kind ? (
        <div className="grid gap-2 sm:grid-cols-2">
          {config.kinds.map((m) => {
            const missing = lacking(m);
            return (
              <button
                key={m.key}
                type="button"
                onClick={() => pick(m)}
                className={cn("flex items-start gap-3 rounded-xl border border-border/60 bg-card p-3 text-left transition-colors hover:border-primary/50 hover:bg-accent", missing && "opacity-60")}
              >
                <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-violet-500/15 text-lg">{ICONS[m.key] ?? "🎮"}</span>
                <span className="min-w-0">
                  <span className="block text-sm font-semibold">{m.name}</span>
                  <span className="block text-xs text-muted-foreground">{m.tagline}</span>
                  <span className="mt-1 block text-[0.65rem] text-muted-foreground/70">
                    {m.minPlayers === m.maxPlayers ? `${m.minPlayers} kişi` : m.maxPlayers > 0 ? `${m.minPlayers} ile ${m.maxPlayers} kişi` : `${m.minPlayers}+ kişi`}
                    {m.realtime ? " · gerçek zamanlı" : ""}
                    {missing ? ` · içerik eksik (${config.itemCounts[m.itemKind] ?? 0}/${m.minItems})` : ""}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      ) : (
        <div className="space-y-4">
          {kind.roundsLabel && (
            <label className="block space-y-1.5">
              <span className="text-xs text-muted-foreground">{kind.roundsLabel}</span>
              <input type="number" min={1} max={30} value={rounds} onChange={(e) => setRounds(Number(e.target.value))} className="h-10 w-32 rounded-xl border border-border/70 bg-muted/40 px-3 text-sm outline-none focus:border-ring/60" />
            </label>
          )}
          {kind.secondsLabel && (
            <label className="block space-y-1.5">
              <span className="text-xs text-muted-foreground">{kind.secondsLabel}</span>
              <input type="number" min={5} max={600} value={seconds} onChange={(e) => setSeconds(Number(e.target.value))} className="h-10 w-32 rounded-xl border border-border/70 bg-muted/40 px-3 text-sm outline-none focus:border-ring/60" />
              <span className="block text-[0.65rem] text-muted-foreground/70">İçeriğin kendi süresi varsa o geçerli olur.</span>
            </label>
          )}
          {lacking(kind) && <p className="text-sm text-destructive">Bu oyun için yeterli içerik yok. Yönetim &gt; Mini oyunlar ekranından ekleyin.</p>}
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
      )}
    </Modal>
  );
}

export const ICONS: Record<string, string> = {
  draw: "🎨",
  poll: "🗳️",
  truth: "🎭",
  solve: "⏱️",
  story: "📖",
  whosaid: "💬",
  connect4: "🔴",
  hockey: "🏒",
  bingo: "🎯",
};
