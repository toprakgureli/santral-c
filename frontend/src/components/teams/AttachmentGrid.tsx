// AttachmentGrid: a line's files in the room. Up to four pictures and
// videos in a tight grid (a "+N" on the last one when there are more),
// other files as download rows underneath. Clicking a picture or video
// opens the lightbox.

import { useEffect, useState } from "react";
import { Download, FileText, Play } from "lucide-react";
import type { TeamsAttachment } from "@/api/types";
import { attachmentUrl, extensionOf, formatDuration, formatSize, thumbUrl } from "@/lib/attachments";
import { cn } from "@/lib/utils";

export function mediaOf(list: TeamsAttachment[]): TeamsAttachment[] {
  return list.filter((a) => a.kind === "image" || a.kind === "video");
}

export default function AttachmentGrid({ attachments, onOpen, className }: { attachments: TeamsAttachment[]; onOpen: (index: number) => void; className?: string }) {
  const media = mediaOf(attachments);
  const files = attachments.filter((a) => a.kind === "file");
  const shown = media.slice(0, 4);
  const extra = media.length - shown.length;

  return (
    <div className={cn("space-y-1.5", className ?? "mt-1.5")}>
      {shown.length > 0 && (
        <div className={cn("grid gap-1 overflow-hidden rounded-xl", shown.length === 1 ? "max-w-[18rem] grid-cols-1" : "max-w-[22rem] grid-cols-2", shown.length === 3 && "grid-rows-2")}>
          {shown.map((a, i) => {
            const url = thumbUrl(a);
            const single = shown.length === 1;
            const ratio = a.width && a.height ? a.width / a.height : 16 / 10;
            return (
              <button
                key={a.id}
                type="button"
                onClick={() => onOpen(i)}
                data-tip={`${a.name} · ${formatSize(a.size)}`}
                className={cn("group relative block overflow-hidden bg-muted/60 text-left", single ? "" : "aspect-[4/3]", shown.length === 3 && i === 0 && "row-span-2 aspect-auto")}
                style={single ? { aspectRatio: `${Math.max(0.6, Math.min(2.2, ratio))}`, maxHeight: 220 } : undefined}
              >
                <Thumb primary={url} fallback={a.kind === "image" ? attachmentUrl(a.id) : null} alt={a.name} />
                {a.kind === "video" && (
                  <>
                    <span className="pointer-events-none absolute inset-0 flex items-center justify-center">
                      <span className="flex size-11 items-center justify-center rounded-full bg-black/55 backdrop-blur-sm transition-transform group-hover:scale-105"><Play className="size-4 fill-white text-white" /></span>
                    </span>
                    {a.durationMs ? <span className="pointer-events-none absolute right-1.5 bottom-1.5 rounded bg-black/65 px-1.5 py-0.5 text-[0.625rem] font-medium tabular-nums text-white">{formatDuration(a.durationMs)}</span> : null}
                  </>
                )}
                {extra > 0 && i === shown.length - 1 && (
                  <span className="absolute inset-0 flex items-center justify-center bg-black/60 text-xl font-semibold text-white">+{extra}</span>
                )}
              </button>
            );
          })}
        </div>
      )}
      {files.map((a) => (
        <a
          key={a.id}
          href={attachmentUrl(a.id, true)}
          download={a.name}
          className="flex w-full max-w-[22rem] items-center gap-3 rounded-xl border border-border/60 bg-muted/30 px-3 py-2 transition-colors hover:bg-muted/60"
        >
          <span className="flex size-10 shrink-0 flex-col items-center justify-center rounded-lg bg-primary/10 text-primary">
            <FileText className="size-4" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{a.name}</span>
            <span className="block text-xs text-muted-foreground">{extensionOf(a.name, a.mime)} · {formatSize(a.size)}</span>
          </span>
          <Download className="size-4 shrink-0 text-muted-foreground" />
        </a>
      ))}
    </div>
  );
}

// Thumb shows the small preview, falls back to the file itself when the
// preview cannot load, and to an icon when nothing can.
function Thumb({ primary, fallback, alt }: { primary: string | null; fallback: string | null; alt: string }) {
  const [src, setSrc] = useState<string | null>(primary);
  useEffect(() => setSrc(primary), [primary]);
  if (!src) return <span className="flex size-full items-center justify-center text-muted-foreground"><FileText className="size-6" /></span>;
  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      onError={() => setSrc(src === primary && fallback && fallback !== primary ? fallback : null)}
      className="size-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
    />
  );
}
