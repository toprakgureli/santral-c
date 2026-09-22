// AvatarCropper: drag to pan, wheel or slider to zoom, 90 degree turns.
// The preview canvas uses the same drawAvatar as the output, so the square
// on screen is exactly the square that gets saved.

import { useCallback, useEffect, useRef, useState } from "react";
import { RotateCw, ZoomIn, ZoomOut } from "lucide-react";
import { Button, Modal } from "@/components/ui";
import {
  AVATAR_SIZE,
  AVATAR_ZOOM_MAX,
  AVATAR_ZOOM_MIN,
  DEFAULT_TRANSFORM,
  clampTransform,
  drawAvatar,
  encodeAvatar,
  type AvatarSource,
  type AvatarTransform,
} from "@/lib/avatar";

const PREVIEW = 288;
const ZOOM_STEP = 0.05;

export default function AvatarCropper({ image, onCancel, onApply }: { image: AvatarSource; onCancel: () => void; onApply: (dataUrl: string) => void }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const drag = useRef<{ x: number; y: number } | null>(null);
  const [t, setT] = useState<AvatarTransform>(() => clampTransform(DEFAULT_TRANSFORM, image));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (canvas.current) drawAvatar(canvas.current, image, PREVIEW, t);
  }, [image, t]);

  const update = useCallback((patch: Partial<AvatarTransform>) => setT((prev) => clampTransform({ ...prev, ...patch }, image)), [image]);

  function move(dx: number, dy: number) {
    setT((prev) => clampTransform({ ...prev, x: prev.x + dx / PREVIEW, y: prev.y + dy / PREVIEW }, image));
  }

  function apply() {
    try {
      onApply(encodeAvatar(image, t));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Görsel işlenemedi.");
    }
  }

  const fill = ((t.zoom - AVATAR_ZOOM_MIN) / (AVATAR_ZOOM_MAX - AVATAR_ZOOM_MIN)) * 100;

  return (
    <Modal
      open
      onClose={onCancel}
      title="Fotoğrafı düzenle"
      description="Sürükleyerek konumlandır, yakınlaştır veya döndür."
      footer={
        <>
          <Button variant="secondary" onClick={onCancel} className="h-9">Vazgeç</Button>
          <Button onClick={apply} className="h-9">Uygula</Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="flex justify-center">
          <div className="relative max-w-full" style={{ width: PREVIEW, height: PREVIEW }}>
            <canvas
              ref={canvas}
              role="img"
              aria-label="Fotoğraf önizlemesi"
              onPointerDown={(e) => {
                e.currentTarget.setPointerCapture(e.pointerId);
                drag.current = { x: e.clientX, y: e.clientY };
              }}
              onPointerMove={(e) => {
                if (!drag.current) return;
                move(e.clientX - drag.current.x, e.clientY - drag.current.y);
                drag.current = { x: e.clientX, y: e.clientY };
              }}
              onPointerUp={(e) => {
                if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
                drag.current = null;
              }}
              onPointerCancel={() => { drag.current = null; }}
              onWheel={(e) => update({ zoom: t.zoom + (e.deltaY > 0 ? -ZOOM_STEP : ZOOM_STEP) })}
              style={{ width: PREVIEW, height: PREVIEW, touchAction: "none" }}
              className="block max-w-full cursor-grab rounded-2xl border border-border/60 active:cursor-grabbing"
            />
            {/* Rule of thirds guide; pointer events off so dragging is unaffected. */}
            <div className="pointer-events-none absolute inset-0 overflow-hidden rounded-2xl">
              <span className="absolute inset-y-0 left-1/3 w-px bg-white/25" />
              <span className="absolute inset-y-0 left-2/3 w-px bg-white/25" />
              <span className="absolute inset-x-0 top-1/3 h-px bg-white/25" />
              <span className="absolute inset-x-0 top-2/3 h-px bg-white/25" />
              <span className="absolute inset-0 rounded-2xl ring-[999px] ring-inset ring-black/0 [mask:radial-gradient(circle_at_center,transparent_49%,black_50%)] bg-black/35" />
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <ZoomOut className="size-4 shrink-0 text-muted-foreground" />
          <div className="relative flex-1">
            <span className="pointer-events-none absolute top-1/2 h-2 w-full -translate-y-1/2 rounded-full bg-muted" />
            <span className="pointer-events-none absolute top-1/2 h-2 -translate-y-1/2 rounded-full bg-foreground" style={{ width: `${fill}%` }} />
            <input
              type="range"
              min={AVATAR_ZOOM_MIN}
              max={AVATAR_ZOOM_MAX}
              step={0.01}
              value={t.zoom}
              onChange={(e) => update({ zoom: Number(e.target.value) })}
              aria-label="Yakınlaştırma"
              className="avatar-slider relative w-full cursor-pointer"
            />
          </div>
          <ZoomIn className="size-4 shrink-0 text-muted-foreground" />
          <Button variant="secondary" onClick={() => update({ rotation: t.rotation + 90 })} aria-label="Döndür" className="h-9 w-9 shrink-0 px-0">
            <RotateCw className="size-4" />
          </Button>
        </div>

        <p className="text-center text-xs text-muted-foreground/70">Kaydedilen fotoğraf {AVATAR_SIZE}×{AVATAR_SIZE} boyutunda, yuvarlak alan görünen kısım.</p>
      </div>
    </Modal>
  );
}
