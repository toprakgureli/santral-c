// Hikâye Zinciri: one sentence each, in turn.

import { useState } from "react";
import { Button } from "@/components/ui";
import { nameOf, Note, Round, type KindProps } from "@/games/kinds/shared";

export default function Story({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const [text, setText] = useState("");
  const mine = d.writer === selfId;
  const sentences = (d.sentences ?? []) as { userId: number; name: string; text: string }[];

  const send = async () => {
    const t = text.trim();
    if (!t) return;
    setText("");
    await h.act("add", { text: t }).catch(() => undefined);
  };

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={sentences.length + 1} total={d.total} label="Cümle" />
      <div className="rounded-2xl border border-border/60 bg-card px-5 py-4 text-base leading-loose">
        <span className="font-medium">{d.prompt}</span>{" "}
        {sentences.map((s, i) => (
          <span key={i} data-tip={s.name} className={s.userId === selfId ? "text-primary" : ""}>{s.text} </span>
        ))}
        <span className="animate-pulse text-muted-foreground">▍</span>
      </div>
      {mine ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
          className="flex gap-2"
        >
          <input value={text} onChange={(e) => setText(e.target.value)} maxLength={200} placeholder="Sıra sende: bir cümle ekle" autoFocus className="h-11 flex-1 rounded-xl border border-primary/60 bg-card px-4 text-sm outline-none focus:ring-4 focus:ring-ring/20" />
          <Button type="submit" disabled={!text.trim()} className="h-11">Ekle</Button>
        </form>
      ) : (
        <Note>{nameOf(g.players, d.writer) || "Biri"} yazıyor...</Note>
      )}
    </div>
  );
}
