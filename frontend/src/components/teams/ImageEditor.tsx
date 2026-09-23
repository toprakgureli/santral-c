// Image marking and masking, as in Devtrack. Arrows and boxes show where
// to look; the mosaic hides what must not be shown (a customer's name, a
// phone number). Mosaic, not blur: blur can be undone, mosaic cannot.
// The result is burned into the file.

import { useEffect, useRef, useState } from "react";
import { ArrowUpRight, Eraser, Loader2, Square, Undo2 } from "lucide-react";
import { Button, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";

type Tool = "arrow" | "rect" | "blur";

export type Shape = { tool: Tool; color: string; x1: number; y1: number; x2: number; y2: number };

const COLORS = ["#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#ffffff"];
const MOSAIC = 12;
const TOOLS: { value: Tool; label: string; icon: typeof Square }[] = [
  { value: "arrow", label: "Ok", icon: ArrowUpRight },
  { value: "rect", label: "Kutu", icon: Square },
  { value: "blur", label: "Gizle", icon: Eraser },
];
const MIN = 4;

function isDrawn(s: Shape): boolean {
  return Math.abs(s.x2 - s.x1) > MIN || Math.abs(s.y2 - s.y1) > MIN;
}

function penWidth(imageWidth: number): number {
  return Math.max(2, Math.round(imageWidth / 320));
}

function drawArrow(ctx: CanvasRenderingContext2D, s: Shape, pen: number) {
  const angle = Math.atan2(s.y2 - s.y1, s.x2 - s.x1);
  const head = Math.max(10, pen * 3.5);
  ctx.strokeStyle = s.color;
  ctx.fillStyle = s.color;
  ctx.lineWidth = pen;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(s.x1, s.y1);
  ctx.lineTo(s.x2, s.y2);
  ctx.stroke();
  ctx.beginPath();
  ctx.moveTo(s.x2, s.y2);
  ctx.lineTo(s.x2 - head * Math.cos(angle - Math.PI / 7), s.y2 - head * Math.sin(angle - Math.PI / 7));
  ctx.lineTo(s.x2 - head * Math.cos(angle + Math.PI / 7), s.y2 - head * Math.sin(angle + Math.PI / 7));
  ctx.closePath();
  ctx.fill();
}

function drawMosaic(ctx: CanvasRenderingContext2D, source: CanvasImageSource, s: Shape) {
  const x = Math.min(s.x1, s.x2);
  const y = Math.min(s.y1, s.y2);
  const w = Math.abs(s.x2 - s.x1);
  const h = Math.abs(s.y2 - s.y1);
  if (w < 2 || h < 2) return;
  const small = document.createElement("canvas");
  small.width = Math.max(1, Math.round(w / MOSAIC));
  small.height = Math.max(1, Math.round(h / MOSAIC));
  const sctx = small.getContext("2d");
  if (!sctx) return;
  sctx.drawImage(source, x, y, w, h, 0, 0, small.width, small.height);
  ctx.save();
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(small, 0, 0, small.width, small.height, x, y, w, h);
  ctx.restore();
}

function render(ctx: CanvasRenderingContext2D, image: HTMLImageElement, shapes: Shape[]) {
  ctx.drawImage(image, 0, 0);
  const pen = penWidth(image.naturalWidth);
  for (const s of shapes) {
    if (s.tool === "blur") drawMosaic(ctx, image, s);
    else if (s.tool === "arrow") drawArrow(ctx, s, pen);
    else {
      ctx.strokeStyle = s.color;
      ctx.lineWidth = pen;
      ctx.strokeRect(Math.min(s.x1, s.x2), Math.min(s.y1, s.y2), Math.abs(s.x2 - s.x1), Math.abs(s.y2 - s.y1));
    }
  }
}

export default function ImageEditor({ file, onCancel, onReady }: { file: File; onCancel: () => void; onReady: (file: File) => void }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const drawing = useRef<Shape | null>(null);
  const [url] = useState(() => URL.createObjectURL(file));
  const [image, setImage] = useState<HTMLImageElement | null>(null);
  const [tool, setTool] = useState<Tool>("arrow");
  const [color, setColor] = useState(COLORS[0]);
  const [shapes, setShapes] = useState<Shape[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => () => URL.revokeObjectURL(url), [url]);

  useEffect(() => {
    const img = new Image();
    img.addEventListener("load", () => {
      const el = canvas.current;
      if (el) {
        el.width = img.naturalWidth;
        el.height = img.naturalHeight;
      }
      setImage(img);
    });
    img.src = url;
  }, [url]);

  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (!el || !image || !ctx) return;
    render(ctx, image, shapes);
  }, [shapes, image]);

  const point = (e: React.PointerEvent) => {
    const el = canvas.current;
    if (!el) return { x: 0, y: 0 };
    const box = el.getBoundingClientRect();
    return { x: ((e.clientX - box.left) / box.width) * el.width, y: ((e.clientY - box.top) / box.height) * el.height };
  };

  const down = (e: React.PointerEvent<HTMLCanvasElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    const p = point(e);
    drawing.current = { tool, color, x1: p.x, y1: p.y, x2: p.x, y2: p.y };
  };
  const move = (e: React.PointerEvent) => {
    if (!drawing.current) return;
    const p = point(e);
    drawing.current = { ...drawing.current, x2: p.x, y2: p.y };
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (el && image && ctx) render(ctx, image, [...shapes, drawing.current]);
  };
  const up = () => {
    const s = drawing.current;
    drawing.current = null;
    if (s && isDrawn(s)) setShapes((prev) => [...prev, s]);
  };

  const save = () => {
    const el = canvas.current;
    if (!el) return;
    setBusy(true);
    const type = file.type === "image/jpeg" ? "image/jpeg" : "image/png";
    el.toBlob(
      (blob) => {
        setBusy(false);
        if (!blob) return;
        const base = file.name.replace(/\.[^.]+$/, "");
        onReady(new File([blob], base + "-duzenlendi" + (type === "image/jpeg" ? ".jpg" : ".png"), { type }));
      },
      type,
      0.92,
    );
  };

  return (
    <Modal
      open
      onClose={busy ? () => undefined : onCancel}
      title="Görseli düzenle"
      description="Ok ve kutuyla işaretle, gizlemek istediğin yeri kapat."
      size="lg"
      footer={
        <>
          <Button variant="ghost" onClick={onCancel} disabled={busy} className="h-9">Vazgeç</Button>
          <Button onClick={save} disabled={busy || !image} className="h-9">{busy && <Loader2 className="animate-spin" />} Kaydet</Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-xl border border-border/60 p-1">
            {TOOLS.map((t) => (
              <button
                key={t.value}
                type="button"
                onClick={() => setTool(t.value)}
                title={t.label}
                aria-pressed={tool === t.value}
                className={cn("flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs transition-colors duration-150", tool === t.value ? "bg-accent text-foreground" : "text-muted-foreground hover:bg-muted")}
              >
                <t.icon className="size-3.5" />
                {t.label}
              </button>
            ))}
          </div>
          {tool !== "blur" && (
            <div className="flex items-center gap-1">
              {COLORS.map((c) => (
                <button key={c} type="button" onClick={() => setColor(c)} aria-label={`Renk ${c}`} aria-pressed={color === c} style={{ backgroundColor: c }} className={cn("size-6 rounded-full border-2 transition-transform duration-150", color === c ? "scale-110 border-foreground" : "border-transparent")} />
              ))}
            </div>
          )}
          <Button variant="ghost" className="ml-auto h-8 px-2.5 text-xs" disabled={!shapes.length} onClick={() => setShapes((prev) => prev.slice(0, -1))}>
            <Undo2 /> Geri al
          </Button>
        </div>
        <div className="flex justify-center rounded-xl bg-muted/30 p-2">
          <canvas ref={canvas} onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up} className="max-h-[55vh] max-w-full cursor-crosshair touch-none rounded-lg" />
        </div>
        {!image && <p className="text-center text-xs text-muted-foreground">Görsel yükleniyor...</p>}
      </div>
    </Modal>
  );
}
