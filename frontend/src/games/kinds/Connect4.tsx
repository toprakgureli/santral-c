// Bağlantı Dört: seven columns, six rows, four in a row wins.

import { useState } from "react";
import { cn } from "@/lib/utils";
import { nameOf, Note, type KindProps } from "@/games/kinds/shared";

export default function Connect4({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const board = (d.board ?? []) as number[][];
  const myTurn = d.turn === selfId;
  const line = ((d.line ?? []) as [number, number][]).map(([r, c]) => `${r}-${c}`);
  const seatColor = (s: number) => (s === 1 ? "bg-destructive" : s === 2 ? "bg-warning" : "bg-background");
  const [hover, setHover] = useState<number | null>(null);
  // Where a disc dropped in the hovered column would land.
  const landing = hover !== null && board.length === 6 ? (() => { for (let r = 5; r >= 0; r--) if (board[r][hover] === 0) return r; return -1; })() : -1;
  return (
    <div className="mx-auto flex w-full max-w-md flex-col items-center gap-4">
      <div className="flex items-center gap-4 text-sm">
        {g.players.map((p, i) => (
          <span key={p.id} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1", d.turn === p.id ? "bg-primary/10 font-semibold" : "text-muted-foreground")}>
            <span className={cn("size-3 rounded-full", seatColor(i + 1))} /> {p.name.split(" ")[0]}
          </span>
        ))}
      </div>
      <div className="grid w-full grid-cols-7 gap-1.5 rounded-2xl bg-primary/80 p-3 shadow-inner">
        {board.length === 6 &&
          Array.from({ length: 6 }, (_, r) =>
            Array.from({ length: 7 }, (_, c) => (
              <button
                key={`${r}-${c}`}
                type="button"
                disabled={!myTurn || !g.joined || board[0][c] !== 0}
                onClick={() => void h.act("drop", { col: c }).catch(() => undefined)}
                onMouseEnter={() => setHover(c)}
                onMouseLeave={() => setHover(null)}
                className={cn(
                  "aspect-square rounded-full border-2 border-black/10 transition-[transform,background-color,opacity]",
                  seatColor(board[r][c]),
                  line.includes(`${r}-${c}`) && "ring-4 ring-white",
                  myTurn && hover === c && board[r][c] === 0 && r !== landing && "opacity-80",
                  myTurn && hover === c && r === landing && (d.mySeat === 1 ? "bg-destructive/50" : "bg-warning/50"),
                )}
                aria-label={`Sütun ${c + 1}`}
              />
            )),
          )}
      </div>
      <Note>{myTurn ? "Sıra sende: bir sütuna tıkla, taşın o sütunda en alttaki boş yere düşer" : `${nameOf(g.players, d.turn)} düşünüyor...`}</Note>
    </div>
  );
}
