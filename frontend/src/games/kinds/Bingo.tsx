// Eskalasyon Bingo: a 5x5 card that fills up as the day goes by.

import { cn } from "@/lib/utils";
import { Note, type KindProps } from "@/games/kinds/shared";

export default function Bingo({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const card = (d.card ?? []) as string[];
  const marks = (d.marks ?? []) as boolean[];
  const counts = (d.counts ?? {}) as Record<string, number>;
  if (!card.length) return <Note>{g.joined ? "Kartın hazırlanıyor..." : "Katıl, sana da bir kart dağıtılsın."}</Note>;
  return (
    <div className="mx-auto flex w-full max-w-lg flex-col gap-4">
      <div className="grid grid-cols-5 gap-1.5">
        {card.map((text, i) => (
          <button
            key={i}
            type="button"
            disabled={i === 12 || !g.joined}
            onClick={() => void h.act("mark", { index: i }).catch(() => undefined)}
            className={cn(
              "flex aspect-square items-center justify-center rounded-xl border p-1 text-center text-[0.7rem] leading-tight transition-colors sm:text-xs",
              marks[i] ? "border-success bg-success/20 font-semibold text-success line-through" : "border-border/60 bg-card hover:bg-accent",
              i === 12 && "bg-violet-500/15 text-violet-500 no-underline",
            )}
          >
            {text}
          </button>
        ))}
      </div>
      <Note>Başına gelen kutuya tıkla. Eskalasyon kategorisiyle aynı adlı kutular kayıt girince kendiliğinden işaretlenir. İlk satır, sütun ya da çapraz: BINGO.</Note>
      <ul className="flex flex-wrap justify-center gap-2 text-xs text-muted-foreground">
        {g.players.filter((p) => !p.left).map((p) => (
          <li key={p.id} className={cn("rounded-full bg-muted/50 px-2.5 py-0.5", p.id === selfId && "text-foreground")}>{p.name.split(" ")[0]}: {counts[p.id] ?? 1} / 25</li>
        ))}
      </ul>
    </div>
  );
}
