// Ses Tahmini: everyone says the same line into the microphone, the clips
// come back pitched up or down, and the room guesses whose voice it is.

import { useEffect, useRef, useState } from "react";
import { Mic, Play, Square } from "lucide-react";
import { Button } from "@/components/ui";
import { cn } from "@/lib/utils";
import { nameOf, Note, PeoplePick, Prompt, Round, type KindProps } from "@/games/kinds/shared";

const CLIP_MS = 3000;

async function record(onTick: (left: number) => void): Promise<string> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const mime = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"].find((m) => MediaRecorder.isTypeSupported(m));
  const rec = new MediaRecorder(stream, { mimeType: mime, audioBitsPerSecond: 24000 });
  const chunks: Blob[] = [];
  rec.ondataavailable = (e) => e.data.size && chunks.push(e.data);
  const done = new Promise<Blob>((resolve) => {
    rec.onstop = () => resolve(new Blob(chunks, { type: rec.mimeType }));
  });
  rec.start();
  const started = Date.now();
  const timer = window.setInterval(() => onTick(Math.max(0, CLIP_MS - (Date.now() - started))), 100);
  await new Promise((r) => window.setTimeout(r, CLIP_MS));
  window.clearInterval(timer);
  rec.stop();
  const blob = await done;
  stream.getTracks().forEach((t) => t.stop());
  return new Promise((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(String(fr.result));
    fr.onerror = () => reject(new Error("Kayıt okunamadı."));
    fr.readAsDataURL(blob);
  });
}

export default function Voice({ h, selfId }: KindProps) {
  const g = h.game!;
  const d = g.data ?? {};
  const [left, setLeft] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const audio = useRef<HTMLAudioElement | null>(null);
  const active = g.players.filter((p) => !p.left);

  const capture = async () => {
    setError(null);
    setLeft(CLIP_MS);
    try {
      const data = await record(setLeft);
      await h.act("clip", { data });
    } catch (e) {
      setError(e instanceof Error && e.name === "NotAllowedError" ? "Mikrofon izni verilmedi." : e instanceof Error ? e.message : "Kayıt alınamadı.");
    } finally {
      setLeft(null);
    }
  };

  // Play the round's clip with its disguise.
  const play = () => {
    if (!d.clip) return;
    audio.current?.pause();
    const a = new Audio(d.clip);
    // The rate is the disguise; keeping the pitch would undo it.
    (a as HTMLAudioElement & { preservesPitch?: boolean }).preservesPitch = false;
    a.playbackRate = d.rate ?? 1;
    a.onended = () => setPlaying(false);
    audio.current = a;
    setPlaying(true);
    void a.play().catch(() => setPlaying(false));
  };
  useEffect(() => {
    return () => audio.current?.pause();
  }, []);
  useEffect(() => {
    if (d.phase === "guess") play();
  }, [d.round, d.phase]); // eslint-disable-line react-hooks/exhaustive-deps

  if (d.phase === "record") {
    return (
      <div className="mx-auto flex w-full max-w-xl flex-col gap-4 text-center">
        <Prompt className="text-lg font-medium">Mikrofona bunu söyle: <strong>{d.phrase}</strong></Prompt>
        <div className="flex flex-col items-center gap-3 py-4">
          <button
            type="button"
            onClick={() => void capture()}
            disabled={left !== null}
            className={cn(
              "relative flex size-28 items-center justify-center rounded-full border-4 text-white shadow-lg transition-transform",
              left !== null ? "border-red-300 bg-red-500 animate-pulse" : d.mine ? "border-success/40 bg-success hover:scale-105" : "border-violet-300 bg-violet-500 hover:scale-105",
            )}
          >
            {left !== null ? <Square className="size-9" /> : <Mic className="size-10" />}
          </button>
          <p className="text-sm font-medium">
            {left !== null ? `Kaydediliyor... ${(left / 1000).toFixed(1)} sn` : d.mine ? "Kaydın alındı. İstersen yeniden söyle." : "Basınca 3 saniye kaydeder"}
          </p>
          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
        <Note>{d.recorded} / {active.length} kayıt geldi · herkes kaydedince ya da süre bitince tahmin başlar</Note>
      </div>
    );
  }

  const owner = d.owner as number | undefined;
  const votes = (d.votes ?? {}) as Record<string, number>;
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Round n={d.round} total={d.total} label="Ses" />
      <Prompt className="text-base">Söylenen: <strong>{d.phrase}</strong></Prompt>
      <div className="flex items-center justify-center gap-3">
        <Button onClick={play} disabled={!d.clip} className="h-12 px-6 text-base">
          <Play className={cn("mr-2 size-5", playing && "animate-pulse")} /> {playing ? "Çalıyor..." : "Tekrar dinle"}
        </Button>
      </div>
      {d.phase === "guess" ? (
        d.isOwner ? (
          <Note>Bu senin sesin. Diğerleri tahmin ediyor, sen sadece izle.</Note>
        ) : (
          <>
            <PeoplePick players={g.players} selfId={selfId} exclude={[selfId]} picked={d.myVote || undefined} disabled={!g.joined} onPick={(id) => void h.act("guess", { owner: id }).catch(() => undefined)} />
            <Note>{d.voted} / {Math.max(0, active.length - 1)} tahmin etti{d.myVote ? " · tahminini değiştirebilirsin" : ""}</Note>
          </>
        )
      ) : (
        <div className="rounded-2xl border border-border/60 bg-card/95 p-4 backdrop-blur">
          <p className="mb-3 text-center text-lg font-semibold">Bu ses: {nameOf(g.players, owner)} 🎤</p>
          <ul className="space-y-1 text-sm">
            {active.filter((p) => p.id !== owner).map((p) => {
              const v = votes[String(p.id)];
              return (
                <li key={p.id} className={cn("flex items-center justify-between rounded-lg px-2 py-1", v === owner ? "bg-success/15 text-success" : "bg-muted/40 text-muted-foreground")}>
                  <span>{p.name}</span>
                  <span>{v ? (v === owner ? "bildi, +10" : `${nameOf(g.players, v).split(" ")[0]} dedi`) : "tahmin etmedi"}</span>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}
