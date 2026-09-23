// Small pieces the game screens share: a phase banner, a big prompt, a
// people picker for votes and a round counter.

import type { ReactNode } from "react";
import UserAvatar from "@/components/ui/UserAvatar";
import type { GameHandle } from "@/games/useGame";
import type { GamePlayer } from "@/games/types";
import { cn } from "@/lib/utils";

export type KindProps = { h: GameHandle; selfId: number };

export function Round({ n, total, label = "Tur" }: { n: number; total: number; label?: string }) {
  return (
    <p className="inline-flex w-fit items-center gap-2 rounded-full bg-violet-500/15 px-3 py-1 text-[0.7rem] font-bold uppercase tracking-widest text-violet-500">
      {label} {n} / {total}
      <span className="flex gap-0.5">
        {Array.from({ length: Math.min(total, 12) }).map((_, i) => (
          <span key={i} className={cn("size-1.5 rounded-full", i < n ? "bg-violet-500" : "bg-violet-500/30")} />
        ))}
      </span>
    </p>
  );
}

export function Prompt({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className={cn("relative overflow-hidden rounded-2xl border border-border/60 bg-card/95 px-5 py-4 text-base leading-relaxed shadow-sm backdrop-blur", className)}>
      <span className="absolute inset-y-0 left-0 w-1.5 bg-gradient-to-b from-violet-500 to-primary" />
      <div className="pl-2">{children}</div>
    </div>
  );
}

export function Note({ children }: { children: ReactNode }) {
  return <p className="text-center text-sm text-muted-foreground">{children}</p>;
}

// PeoplePick lists players as big buttons for a vote.
export function PeoplePick({ players, selfId, exclude = [], picked, disabled, onPick, counts }: { players: GamePlayer[]; selfId: number; exclude?: number[]; picked?: number; disabled?: boolean; onPick: (id: number) => void; counts?: Record<number, number> }) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {players.filter((p) => !p.left && !exclude.includes(p.id)).map((p) => (
        <button
          key={p.id}
          type="button"
          disabled={disabled}
          onClick={() => onPick(p.id)}
          className={cn(
            "flex items-center gap-2.5 rounded-xl border px-3 py-2.5 text-left transition-colors",
            picked === p.id ? "border-violet-500 bg-violet-500/10 shadow-md shadow-violet-500/10" : "border-border/60 bg-card/95 backdrop-blur hover:border-violet-500/40 hover:bg-accent",
            disabled && picked !== p.id && "opacity-70",
          )}
        >
          <UserAvatar userId={p.id} name={p.name} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{p.name}{p.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</span>
          {counts && counts[p.id] ? <span className="rounded-full bg-primary/15 px-2 text-xs font-semibold tabular-nums text-primary">{counts[p.id]}</span> : null}
        </button>
      ))}
    </div>
  );
}

export function nameOf(players: GamePlayer[], id: number | undefined): string {
  return players.find((p) => p.id === id)?.name ?? "";
}
