// Kim Söyledi: a line from this room, who wrote it?

import { nameOf, Note, PeoplePick, Prompt, Round, type KindProps } from "@/games/kinds/shared";

export default function WhoSaid({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const votes = (d.votes ?? {}) as Record<string, number>;
  const counts: Record<number, number> = {};
  for (const v of Object.values(votes)) counts[v] = (counts[v] ?? 0) + 1;
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.round} total={d.total} label="Mesaj" />
      <Prompt className="text-lg italic">"{d.text}"</Prompt>
      {d.phase === "guess" ? (
        d.isAuthor ? (
          <Note>Bunu sen yazmışsın. Sessizce izle, diğerleri tahmin etsin.</Note>
        ) : (
          <>
            <PeoplePick players={g.players} selfId={selfId} exclude={[selfId]} picked={d.myVote || undefined} disabled={!g.joined} onPick={(id) => void h.act("guess", { author: id }).catch(() => undefined)} />
            <Note>{d.voted} kişi tahmin etti</Note>
          </>
        )
      ) : (
        <>
          <p className="text-center text-base">Yazan: <strong>{nameOf(g.players, d.author)}</strong></p>
          <PeoplePick players={g.players} selfId={selfId} picked={d.author} disabled onPick={() => undefined} counts={counts} />
        </>
      )}
    </div>
  );
}
