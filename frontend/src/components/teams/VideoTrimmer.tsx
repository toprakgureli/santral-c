// Video trim and compress window. It opens for every
// video, not only those over the limit: whoever wants to cut should not
// first upload and then shrink. An untouched video under the limit is
// sent as is, without re-encoding.
//
// Two cuts: in time and on screen. Time first, because it lowers both the
// file and the waiting time at once; the work runs in real time.

import { useEffect, useRef, useState } from "react";
import { Check, Loader2, Pause, Play, RotateCcw, TriangleAlert, X } from "lucide-react";
import { Button, Modal } from "@/components/ui";
import { formatSize } from "@/lib/attachments";
import { CANCEL_MESSAGE, FULL_CROP, compressVideo, estimateBytes, isEdited, loadVideoFile, outputSize, targetBitrate, type VideoCrop } from "@/lib/video";
import { cn } from "@/lib/utils";

type Props = { file: File; maxBytes: number; onCancel: () => void; onReady: (file: File) => void };

const CURSOR = { move: "move", nw: "nwse", se: "nwse", ne: "nesw", sw: "nesw" } as const;

function lockCursor(kind: string) {
  document.body.dataset.drag = kind;
}
function freeCursor() {
  delete document.body.dataset.drag;
}

type Handle = "move" | "nw" | "ne" | "sw" | "se";

function clamp(v: number, min: number, max: number) {
  return Math.min(max, Math.max(min, v));
}

function clock(v: number) {
  const m = Math.floor(v / 60);
  const s = Math.floor(v % 60);
  return `${m}:${String(s).padStart(2, "0")}`;
}

function Row({ label, from, to, good }: { label: string; from: string; to: string; good?: boolean }) {
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-20 shrink-0 text-muted-foreground">{label}</span>
      <span className="text-muted-foreground/70 line-through">{from}</span>
      <span className="text-muted-foreground/50">→</span>
      <span className={cn("font-medium", good && "text-success")}>{to}</span>
    </div>
  );
}

export default function VideoTrimmer({ file, maxBytes, onCancel, onReady }: Props) {
  const video = useRef<HTMLVideoElement>(null);
  const frame = useRef<HTMLDivElement>(null);
  const drag = useRef<{ handle: Handle; x: number; y: number } | null>(null);
  const track = useRef<HTMLDivElement>(null);
  const scrubbing = useRef(false);

  const [meta, setMeta] = useState<{ duration: number; width: number; height: number } | null>(null);
  const [start, setStart] = useState(0);
  const [end, setEnd] = useState(0);
  const [now, setNow] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [crop, setCrop] = useState<VideoCrop>(FULL_CROP);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState(0);
  const [result, setResult] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [url] = useState(() => URL.createObjectURL(file));
  const aborter = useRef<AbortController | null>(null);

  useEffect(() => () => URL.revokeObjectURL(url), [url]);

  useEffect(() => {
    window.addEventListener("pointerup", freeCursor);
    window.addEventListener("pointercancel", freeCursor);
    return () => {
      window.removeEventListener("pointerup", freeCursor);
      window.removeEventListener("pointercancel", freeCursor);
      freeCursor();
    };
  }, []);

  useEffect(() => {
    loadVideoFile(file)
      .then((v) => {
        setMeta({ duration: v.duration, width: v.videoWidth, height: v.videoHeight });
        setEnd(v.duration);
        URL.revokeObjectURL(v.src);
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : "Video okunamadı."));
  }, [file]);

  useEffect(() => {
    const el = video.current;
    if (!el) return;
    const watch = () => {
      setNow(el.currentTime);
      if (el.currentTime > end) el.currentTime = start;
    };
    el.addEventListener("timeupdate", watch);
    return () => el.removeEventListener("timeupdate", watch);
  }, [start, end]);

  const seconds = Math.max(0.5, end - start);
  const cropped = crop.width < 1 || crop.height < 1;
  const big = file.size > maxBytes;
  const edited = meta ? isEdited({ start, end, crop }, meta.duration) : false;
  const passThrough = !big && !edited;
  const budget = big ? maxBytes : Math.min(maxBytes, file.size);
  const bitrate = targetBitrate(seconds, budget);
  const estimate = estimateBytes(seconds, bitrate);
  const out = meta ? outputSize({ videoWidth: meta.width, videoHeight: meta.height }, crop) : { width: 0, height: 0 };

  const toggle = () => {
    const el = video.current;
    if (!el) return;
    if (el.paused) {
      if (el.currentTime < start || el.currentTime >= end) el.currentTime = start;
      void el.play();
      setPlaying(true);
      return;
    }
    el.pause();
    setPlaying(false);
  };

  const onPointerDown = (handle: Handle) => (e: React.PointerEvent) => {
    e.stopPropagation();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    drag.current = { handle, x: e.clientX, y: e.clientY };
    lockCursor(CURSOR[handle]);
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    const box = frame.current?.getBoundingClientRect();
    if (!d || !box) return;
    const dx = (e.clientX - d.x) / box.width;
    const dy = (e.clientY - d.y) / box.height;
    drag.current = { ...d, x: e.clientX, y: e.clientY };
    setCrop((prev) => {
      const next = { ...prev };
      if (d.handle === "move") {
        next.x = clamp(prev.x + dx, 0, 1 - prev.width);
        next.y = clamp(prev.y + dy, 0, 1 - prev.height);
        return next;
      }
      const min = 0.15;
      if (d.handle === "nw" || d.handle === "sw") {
        const nx = clamp(prev.x + dx, 0, prev.x + prev.width - min);
        next.width = prev.width + (prev.x - nx);
        next.x = nx;
      }
      if (d.handle === "ne" || d.handle === "se") next.width = clamp(prev.width + dx, min, 1 - prev.x);
      if (d.handle === "nw" || d.handle === "ne") {
        const ny = clamp(prev.y + dy, 0, prev.y + prev.height - min);
        next.height = prev.height + (prev.y - ny);
        next.y = ny;
      }
      if (d.handle === "sw" || d.handle === "se") next.height = clamp(prev.height + dy, min, 1 - prev.y);
      return next;
    });
  };

  const onPointerUp = (e: React.PointerEvent) => {
    const el = e.currentTarget as HTMLElement;
    if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
    drag.current = null;
    freeCursor();
  };

  const seekTo = (clientX: number) => {
    const box = track.current?.getBoundingClientRect();
    const el = video.current;
    if (!box || !el || !meta) return;
    const ratio = clamp((clientX - box.left) / box.width, 0, 1);
    const t = clamp(ratio * meta.duration, start, end);
    el.currentTime = t;
    setNow(t);
  };

  const onTrackDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if ((e.target as HTMLElement).tagName === "INPUT") return;
    e.currentTarget.setPointerCapture(e.pointerId);
    scrubbing.current = true;
    lockCursor("grab");
    seekTo(e.clientX);
  };
  const onTrackMove = (e: React.PointerEvent) => {
    if (scrubbing.current) seekTo(e.clientX);
  };
  const onTrackUp = (e: React.PointerEvent<HTMLDivElement>) => {
    if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
    scrubbing.current = false;
    freeCursor();
  };

  const apply = async () => {
    video.current?.pause();
    setPlaying(false);
    if (passThrough) {
      onReady(file);
      return;
    }
    setBusy(true);
    setProgress(0);
    setError(null);
    const ctl = new AbortController();
    aborter.current = ctl;
    try {
      const done = await compressVideo(file, { start, end, crop }, budget, (r) => setProgress(Math.round(clamp(r, 0, 1) * 100)), ctl.signal);
      setBusy(false);
      if (done.size > maxBytes) {
        setError(`Sıkıştırma sonrası ${formatSize(done.size)} oldu, sınır ${formatSize(maxBytes)}. Süreyi biraz daha kısalt.`);
        return;
      }
      setResult(done);
    } catch (e: unknown) {
      setBusy(false);
      const msg = e instanceof Error ? e.message : "Video işlenemedi.";
      if (msg !== CANCEL_MESSAGE) setError(msg);
    } finally {
      aborter.current = null;
    }
  };

  const handle = "absolute size-3.5 rounded-full border-2 border-primary bg-background";
  const pct = (v: number) => (meta?.duration ? (v / meta.duration) * 100 : 0);
  const near = (a: number, b: number) => (meta ? Math.abs(a - b) / meta.duration < 0.02 : false);
  const nudge = near(now, start) ? 9 : near(now, end) ? -9 : 0;

  if (result) {
    const gain = Math.round((1 - result.size / file.size) * 100);
    return (
      <Modal open onClose={onCancel} title="Video hazır" description={big ? "Sıkıştırma tamamlandı." : "Düzenleme tamamlandı."} footer={<Button onClick={() => onReady(result)} className="h-9"><Check /> Ekle</Button>}>
        <div className="space-y-3">
          <div className="space-y-2 rounded-xl border border-success/40 bg-success/5 p-4">
            <p className="flex items-center gap-2 text-sm font-medium text-success">
              <Check className="size-4" />
              {gain > 0 ? `%${gain} küçüldü` : "Video hazırlandı"}
            </p>
            <Row label="Boyut" from={formatSize(file.size)} to={formatSize(result.size)} good />
            {meta && <Row label="Süre" from={clock(meta.duration)} to={clock(seconds)} />}
            {meta && <Row label="Çözünürlük" from={`${meta.width}×${meta.height}`} to={`${out.width}×${out.height}`} />}
          </div>
          {big && (
            <p className="flex items-start gap-2 rounded-xl border border-warning/40 bg-warning/5 p-3 text-xs text-warning">
              <TriangleAlert className="mt-px size-4 shrink-0" />
              <span>Video boyutu sınırın üzerindeydi, küçültüldü. Görüntü kalitesinde düşüş olabilir.</span>
            </p>
          )}
          <p className="text-xs text-muted-foreground/70">Video webm biçimine çevrildi. Ses varsa korundu.</p>
        </div>
      </Modal>
    );
  }

  return (
    <Modal
      open
      onClose={busy ? () => undefined : onCancel}
      title={big ? "Videoyu küçült" : "Videoyu düzenle"}
      description={big ? `Dosya ${formatSize(file.size)}, sınır ${formatSize(maxBytes)}. Süreden ve ekrandan kırparak sığdır.` : "İstersen süreden ve ekrandan kırp. Dokunmazsan video olduğu gibi eklenir."}
      size="lg"
      footer={
        <>
          {busy ? (
            <Button variant="ghost" onClick={() => aborter.current?.abort()} className="h-9" data-tip="İşlemi durdur"><X /> İşlemi iptal et</Button>
          ) : (
            <Button variant="ghost" onClick={onCancel} className="h-9">Vazgeç</Button>
          )}
          <Button onClick={() => void apply()} disabled={busy || !meta} className="h-9">
            {busy && <Loader2 className="animate-spin" />}
            {busy ? `İşleniyor %${progress}` : big ? "Küçült" : edited ? "Kırp ve ekle" : "Olduğu gibi ekle"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}

        <div ref={frame} className="relative mx-auto w-fit touch-none select-none overflow-hidden rounded-xl bg-black" onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}>
          <video ref={video} src={url} muted playsInline draggable={false} className="block max-h-[42vh] max-w-full select-none" onDragStart={(e) => e.preventDefault()} onEnded={() => setPlaying(false)} />
          <div
            className="absolute cursor-move border-2 border-primary shadow-[0_0_0_9999px_rgba(0,0,0,0.6)]"
            style={{ left: `${crop.x * 100}%`, top: `${crop.y * 100}%`, width: `${crop.width * 100}%`, height: `${crop.height * 100}%`, touchAction: "none" }}
            onPointerDown={onPointerDown("move")}
          >
            <span className={cn(handle, "-top-1.5 -left-1.5 cursor-nwse-resize")} onPointerDown={onPointerDown("nw")} />
            <span className={cn(handle, "-top-1.5 -right-1.5 cursor-nesw-resize")} onPointerDown={onPointerDown("ne")} />
            <span className={cn(handle, "-bottom-1.5 -left-1.5 cursor-nesw-resize")} onPointerDown={onPointerDown("sw")} />
            <span className={cn(handle, "-right-1.5 -bottom-1.5 cursor-nwse-resize")} onPointerDown={onPointerDown("se")} />
          </div>
        </div>

        {meta && (
          <div className="space-y-3">
            <div className="flex items-center gap-3">
              <Button variant="secondary" disabled={busy} onClick={toggle} aria-label={playing ? "Duraklat" : "Oynat"} className="h-9 w-9 px-0">
                {playing ? <Pause /> : <Play />}
              </Button>
              <span className="text-xs tabular-nums text-muted-foreground">{clock(now)} / {clock(meta.duration)}</span>
              {cropped && (
                <button type="button" disabled={busy} onClick={() => setCrop(FULL_CROP)} className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground">
                  <RotateCcw className="size-3" /> Kırpmayı sıfırla
                </button>
              )}
            </div>

            <div className="space-y-1">
              <div ref={track} className="relative h-9 cursor-grab touch-none active:cursor-grabbing" onPointerDown={onTrackDown} onPointerMove={onTrackMove} onPointerUp={onTrackUp} onPointerCancel={onTrackUp}>
                <span className="pointer-events-none absolute top-0 bottom-0 z-10 flex flex-col items-center" style={{ left: `${pct(now)}%`, transform: `translateX(calc(-50% + ${nudge}px))` }}>
                  <span className="size-3 shrink-0 rounded-full border-2 border-background bg-primary shadow-sm" />
                  <span className="w-0.5 flex-1 rounded-full bg-primary/70" />
                </span>
                <span className="pointer-events-none absolute bottom-1.5 h-2 w-full rounded-full bg-muted" />
                <span className="pointer-events-none absolute bottom-1.5 h-2 rounded-full bg-foreground" style={{ left: `${pct(start)}%`, width: `${pct(end - start)}%` }} />
                <input
                  type="range"
                  min={0}
                  max={meta.duration}
                  step={0.1}
                  value={start}
                  disabled={busy}
                  aria-label="Başlangıç"
                  onPointerDown={() => lockCursor("ew")}
                  onPointerUp={freeCursor}
                  onChange={(e) => {
                    const v = clamp(Number(e.target.value), 0, end - 0.5);
                    setStart(v);
                    if (video.current) video.current.currentTime = v;
                  }}
                  className="range-stack top-auto bottom-0"
                />
                <input
                  type="range"
                  min={0}
                  max={meta.duration}
                  step={0.1}
                  value={end}
                  disabled={busy}
                  aria-label="Bitiş"
                  onPointerDown={() => lockCursor("ew")}
                  onPointerUp={freeCursor}
                  onChange={(e) => setEnd(clamp(Number(e.target.value), start + 0.5, meta.duration))}
                  className="range-stack top-auto bottom-0"
                />
              </div>
              <div className="flex justify-between text-[0.6875rem] tabular-nums text-muted-foreground/70">
                <span>{clock(start)}</span>
                <span>{seconds.toFixed(1)} sn seçili</span>
                <span>{clock(end)}</span>
              </div>
            </div>

            {passThrough ? (
              <p className="rounded-xl border border-border/60 bg-muted/30 p-3 text-xs text-muted-foreground">Video olduğu gibi eklenecek, yeniden kodlanmayacak; görüntü kalitesi aynı kalır. Kırparsan yeniden kodlanır.</p>
            ) : (
              <div className="space-y-1.5 rounded-xl border border-border/60 bg-muted/30 p-3">
                <p className="text-xs font-medium text-muted-foreground">{big ? "Sıkıştırma sonrası" : "İşlem sonrası"}</p>
                <Row label="Boyut" from={formatSize(file.size)} to={`~${formatSize(estimate)}`} good={estimate <= maxBytes} />
                <Row label="Süre" from={clock(meta.duration)} to={clock(seconds)} />
                <Row label="Çözünürlük" from={`${meta.width}×${meta.height}`} to={`${out.width}×${out.height}`} />
              </div>
            )}

            {busy && (
              <div className="space-y-1.5">
                <span className="block h-1.5 w-full overflow-hidden rounded-full bg-muted">
                  <span className="block h-full rounded-full bg-foreground transition-[width] duration-200 ease-out" style={{ width: `${progress}%` }} />
                </span>
                <p className="text-center text-xs text-muted-foreground/70">Video gerçek zamanlı işleniyor, süresi kadar sürüyor. Sekme değiştirebilirsin, işlem arka planda devam eder; sekmeyi kapatırsan iptal olur.</p>
              </div>
            )}
          </div>
        )}
      </div>
    </Modal>
  );
}
