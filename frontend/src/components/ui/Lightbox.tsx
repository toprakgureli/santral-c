// Lightbox: full-screen viewer for pictures and videos with zoom, pan, rotate,
// keyboard navigation, and gallery support. Used across Teams and WhatsApp.

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ChevronLeft,
  ChevronRight,
  Download,
  RotateCcw,
  RotateCw,
  X,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { useTopmost } from "@/components/ui/windowStack";
import { cn } from "@/lib/utils";

export interface LightboxItem {
  id: string | number;
  url: string;
  downloadUrl?: string;
  name?: string;
  size?: number;
  width?: number;
  height?: number;
  kind: "image" | "video" | "file";
  hasThumb?: boolean;
  thumbUrl?: string;
}

function formatSize(bytes?: number): string {
  if (!bytes) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

const MIN_SCALE = 0.5;
const MAX_SCALE = 8.0;

export default function Lightbox({
  items,
  index,
  onIndex,
  onClose,
}: {
  items: LightboxItem[];
  index: number;
  onIndex?: (i: number) => void;
  onClose: () => void;
}) {
  const item = items[index];

  // Zoom & Pan state
  const [scale, setScale] = useState(1);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const [rotation, setRotation] = useState(0);

  // Dragging state
  const isDragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const posStart = useRef({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const clickDist = useRef(0);

  const containerRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);

  // Reset transforms when item changes
  const resetTransform = useCallback(() => {
    setScale(1);
    setPos({ x: 0, y: 0 });
    setRotation(0);
  }, []);

  useEffect(() => {
    resetTransform();
  }, [index, resetTransform]);

  // Topmost window stack management
  const isTop = useTopmost(true);

  // Keyboard navigation & controls
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!isTop()) return;
      if (e.key === "Escape") {
        onClose();
      } else if (e.key === "ArrowRight" && onIndex && index < items.length - 1) {
        onIndex(index + 1);
      } else if (e.key === "ArrowLeft" && onIndex && index > 0) {
        onIndex(index - 1);
      } else if (e.key === "+" || e.key === "=") {
        setScale((s) => Math.min(MAX_SCALE, Number((s * 1.25).toFixed(2))));
      } else if (e.key === "-") {
        setScale((s) => Math.max(MIN_SCALE, Number((s / 1.25).toFixed(2))));
      } else if (e.key === "0") {
        resetTransform();
      } else if (e.key === "r" || e.key === "R") {
        setRotation((r) => (r + 90) % 360);
      }
    };

    window.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
    };
  }, [index, items.length, onClose, onIndex, isTop, resetTransform]);

  // Zoom towards a specific focal point (e.g. mouse cursor or center)
  const zoomAt = useCallback(
    (factor: number, clientX?: number, clientY?: number) => {
      setScale((prevScale) => {
        const nextScale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, prevScale * factor));
        if (nextScale === prevScale) return prevScale;

        if (clientX != null && clientY != null && containerRef.current) {
          const rect = containerRef.current.getBoundingClientRect();
          const cx = clientX - (rect.left + rect.width / 2);
          const cy = clientY - (rect.top + rect.height / 2);
          const ratio = nextScale / prevScale;
          setPos((p) => ({
            x: cx - (cx - p.x) * ratio,
            y: cy - (cy - p.y) * ratio,
          }));
        } else if (nextScale === 1) {
          setPos({ x: 0, y: 0 });
        }
        return Number(nextScale.toFixed(2));
      });
    },
    [],
  );

  // Wheel zoom handler: supports Ctrl + Wheel (and standard wheel over image)
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const onWheel = (e: WheelEvent) => {
      // Zoom with Ctrl + Wheel, or when cursor is over image
      e.preventDefault();
      const zoomFactor = e.deltaY < 0 ? 1.15 : 1 / 1.15;
      zoomAt(zoomFactor, e.clientX, e.clientY);
    };

    el.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      el.removeEventListener("wheel", onWheel);
    };
  }, [zoomAt]);

  // Mouse pan / drag handlers
  const onMouseDown = (e: React.MouseEvent) => {
    if (e.button !== 0) return; // Only left mouse button
    isDragging.current = true;
    dragStart.current = { x: e.clientX, y: e.clientY };
    posStart.current = { ...pos };
    clickDist.current = 0;
    setDragging(true);
  };

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!isDragging.current) return;
      const dx = e.clientX - dragStart.current.x;
      const dy = e.clientY - dragStart.current.y;
      clickDist.current = Math.hypot(dx, dy);

      setPos({
        x: posStart.current.x + dx,
        y: posStart.current.y + dy,
      });
    };

    const onMouseUp = () => {
      if (isDragging.current) {
        isDragging.current = false;
        setDragging(false);
      }
    };

    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, []);

  // Image click handler: toggle between 1x and 2.5x if not dragged
  const onImageClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (clickDist.current > 6) {
      // Was dragging, do not treat as toggle click
      return;
    }

    if (scale > 1.05) {
      // Zoom out to fit
      resetTransform();
    } else {
      // Zoom in towards clicked spot
      zoomAt(2.5, e.clientX, e.clientY);
    }
  };

  if (!item) return null;

  const downloadUrl = item.downloadUrl || item.url;
  const isZoomed = scale > 1.05;

  return (
    <div
      className="fixed inset-0 z-[95] flex select-none flex-col bg-black/95 text-white"
      onClick={onClose}
    >
      {/* Top Header Bar */}
      <div
        className="flex h-14 shrink-0 items-center gap-3 px-4"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">
            {item.name || "Görsel"}
          </span>
          <span className="block text-xs text-white/60">
            {formatSize(item.size)}
            {item.width && item.height ? ` · ${item.width}×${item.height}` : ""}
            {items.length > 1 ? ` · ${index + 1} / ${items.length}` : ""}
          </span>
        </span>

        {/* Toolbar Controls */}
        <div className="flex items-center gap-1 rounded-xl bg-white/10 p-1 backdrop-blur-md">
          <button
            type="button"
            onClick={() => zoomAt(1.25)}
            disabled={scale >= MAX_SCALE}
            className="flex size-8 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/15 hover:text-white disabled:opacity-40"
            title="Yakınlaştır (+)"
          >
            <ZoomIn className="size-4" />
          </button>
          <button
            type="button"
            onClick={() => zoomAt(1 / 1.25)}
            disabled={scale <= MIN_SCALE}
            className="flex size-8 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/15 hover:text-white disabled:opacity-40"
            title="Uzaklaştır (-)"
          >
            <ZoomOut className="size-4" />
          </button>
          <button
            type="button"
            onClick={resetTransform}
            className="flex h-8 items-center justify-center rounded-lg px-2 text-xs font-semibold tabular-nums text-white/80 transition-colors hover:bg-white/15 hover:text-white"
            title="Boyutu Sıfırla (0)"
          >
            {Math.round(scale * 100)}%
          </button>
          <button
            type="button"
            onClick={() => setRotation((r) => (r + 90) % 360)}
            className="flex size-8 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/15 hover:text-white"
            title="90° Döndür (R)"
          >
            <RotateCw className="size-4" />
          </button>
          {isZoomed && (
            <button
              type="button"
              onClick={resetTransform}
              className="flex size-8 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/15 hover:text-white"
              title="Konumu ve Boyutu Sıfırla"
            >
              <RotateCcw className="size-4" />
            </button>
          )}
        </div>

        <a
          href={downloadUrl}
          download={item.name || "download"}
          onClick={(e) => e.stopPropagation()}
          className="flex size-9 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/10 hover:text-white"
          title="İndir"
        >
          <Download className="size-5" />
        </a>
        <button
          type="button"
          onClick={onClose}
          className="flex size-9 items-center justify-center rounded-lg text-white/80 transition-colors hover:bg-white/10 hover:text-white"
          title="Kapat (Esc)"
        >
          <X className="size-5" />
        </button>
      </div>

      {/* Main Viewport */}
      <div
        ref={containerRef}
        className={cn(
          "relative flex min-h-0 flex-1 items-center justify-center overflow-hidden p-4",
          isZoomed
            ? dragging
              ? "cursor-grabbing"
              : "cursor-grab"
            : "cursor-zoom-in"
        )}
      >
        {item.kind === "image" ? (
          <img
            ref={imageRef}
            src={item.url}
            alt={item.name || ""}
            draggable={false}
            onMouseDown={onMouseDown}
            onClick={onImageClick}
            style={{
              transform: `translate3d(${pos.x}px, ${pos.y}px, 0) scale(${scale}) rotate(${rotation}deg)`,
              transition: dragging ? "none" : "transform 150ms ease-out",
            }}
            className="max-h-full max-w-full select-none object-contain will-change-transform"
          />
        ) : (
          <video
            key={item.id}
            src={item.url}
            controls
            autoPlay
            playsInline
            onClick={(e) => e.stopPropagation()}
            className="max-h-full max-w-full rounded-lg bg-black outline-none"
          />
        )}

        {/* Previous / Next Arrow Controls */}
        {index > 0 && onIndex && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onIndex(index - 1);
            }}
            aria-label="Önceki"
            className="absolute left-4 top-1/2 flex size-11 -translate-y-1/2 items-center justify-center rounded-full bg-black/50 text-white shadow-lg backdrop-blur-sm transition-transform hover:scale-105 hover:bg-black/75"
          >
            <ChevronLeft className="size-6" />
          </button>
        )}
        {index < items.length - 1 && onIndex && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onIndex(index + 1);
            }}
            aria-label="Sonraki"
            className="absolute right-4 top-1/2 flex size-11 -translate-y-1/2 items-center justify-center rounded-full bg-black/50 text-white shadow-lg backdrop-blur-sm transition-transform hover:scale-105 hover:bg-black/75"
          >
            <ChevronRight className="size-6" />
          </button>
        )}
      </div>

      {/* Bottom Thumbnail Strip */}
      {items.length > 1 && onIndex && (
        <div
          className="flex h-16 shrink-0 items-center justify-center gap-2 overflow-x-auto px-4 py-2"
          onClick={(e) => e.stopPropagation()}
        >
          {items.map((a, i) => (
            <button
              key={a.id}
              type="button"
              onClick={() => onIndex(i)}
              aria-label={a.name}
              className={cn(
                "relative size-12 shrink-0 overflow-hidden rounded-lg border-2 bg-white/10 transition-all",
                i === index
                  ? "border-white ring-2 ring-white/30 scale-105"
                  : "border-transparent opacity-60 hover:opacity-100"
              )}
            >
              {a.hasThumb || a.kind === "image" ? (
                <img
                  src={a.thumbUrl || a.url}
                  alt=""
                  className="size-full object-cover"
                />
              ) : (
                <span className="flex size-full items-center justify-center text-[0.65rem] font-medium text-white/80">
                  video
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
