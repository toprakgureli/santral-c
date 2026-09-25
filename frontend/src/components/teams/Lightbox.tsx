// Lightbox: full-screen viewer for a line's pictures and videos. Arrow
// keys and buttons move between them, Esc closes, a picture toggles
// between fit and actual size on click, videos play in the browser's
// player, everything can be downloaded.

import { useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, Download, X } from "lucide-react";
import type { TeamsAttachment } from "@/api/types";
import { attachmentUrl, formatSize } from "@/lib/attachments";
import { cn } from "@/lib/utils";

export default function Lightbox({ items, index, onIndex, onClose }: { items: TeamsAttachment[]; index: number; onIndex: (i: number) => void; onClose: () => void }) {
  const [zoom, setZoom] = useState(false);
  const item = items[index];

  useEffect(() => setZoom(false), [index]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowRight" && index < items.length - 1) onIndex(index + 1);
      else if (e.key === "ArrowLeft" && index > 0) onIndex(index - 1);
    };
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [index, items.length, onClose, onIndex]);

  if (!item) return null;
  const url = attachmentUrl(item.id);

  return (
    <div className="fixed inset-0 z-[90] flex flex-col bg-black/95 text-white" onClick={onClose}>
      <div className="flex h-14 shrink-0 items-center gap-3 px-4" onClick={(e) => e.stopPropagation()}>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{item.name}</span>
          <span className="block text-xs text-white/60">
            {formatSize(item.size)}
            {item.width && item.height ? ` · ${item.width}×${item.height}` : ""}
            {items.length > 1 ? ` · ${index + 1} / ${items.length}` : ""}
          </span>
        </span>
        <a href={attachmentUrl(item.id, true)} download={item.name} data-tip="İndir" className="flex size-9 items-center justify-center rounded-lg text-white/80 hover:bg-white/10 hover:text-white"><Download className="size-5" /></a>
        <button type="button" onClick={onClose} aria-label="Kapat" className="flex size-9 items-center justify-center rounded-lg text-white/80 hover:bg-white/10 hover:text-white" data-tip="Kapat"><X className="size-5" /></button>
      </div>

      <div className={cn("relative flex min-h-0 flex-1 items-center justify-center p-4", zoom && "overflow-auto")}>
        {item.kind === "image" ? (
          <img
            src={url}
            alt={item.name}
            onClick={(e) => {
              e.stopPropagation();
              setZoom((z) => !z);
            }}
            className={cn("select-none", zoom ? "max-w-none cursor-zoom-out" : "max-h-full max-w-full cursor-zoom-in object-contain")}
            draggable={false}
          />
        ) : (
          <video key={item.id} src={url} controls autoPlay playsInline onClick={(e) => e.stopPropagation()} className="max-h-full max-w-full rounded-lg bg-black outline-none" />
        )}

        {index > 0 && (
          <button type="button" onClick={(e) => { e.stopPropagation(); onIndex(index - 1); }} aria-label="Önceki" className="absolute left-3 top-1/2 flex size-11 -translate-y-1/2 items-center justify-center rounded-full bg-black/50 text-white hover:bg-black/70"><ChevronLeft className="size-6" /></button>
        )}
        {index < items.length - 1 && (
          <button type="button" onClick={(e) => { e.stopPropagation(); onIndex(index + 1); }} aria-label="Sonraki" className="absolute right-3 top-1/2 flex size-11 -translate-y-1/2 items-center justify-center rounded-full bg-black/50 text-white hover:bg-black/70"><ChevronRight className="size-6" /></button>
        )}
      </div>

      {items.length > 1 && (
        <div className="flex h-16 shrink-0 items-center justify-center gap-1.5 px-4" onClick={(e) => e.stopPropagation()}>
          {items.map((a, i) => (
            <button key={a.id} type="button" onClick={() => onIndex(i)} className={cn("size-12 overflow-hidden rounded-md border-2 bg-white/10", i === index ? "border-white" : "border-transparent opacity-60 hover:opacity-100")}>
              {a.hasThumb || a.kind === "image" ? <img src={a.hasThumb ? `/api/v1/teams/attachments/${a.id}/thumb` : attachmentUrl(a.id)} alt="" className="size-full object-cover" /> : <span className="flex size-full items-center justify-center text-[0.6rem]">video</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
