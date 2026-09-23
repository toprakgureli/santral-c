// MediaTile: one pending file in the composer tray. Images and videos show
// themselves, other files show a card with the extension. While uploading
// the tile darkens and carries the percentage; a failed one stays so it
// can be retried.

import { FileText, Loader2, Pencil, Play, RefreshCw, TriangleAlert, X } from "lucide-react";
import { extensionOf, formatSize, type PendingAttachment } from "@/lib/attachments";
import { cn } from "@/lib/utils";

export default function MediaTile({ item, onRemove, onRetry, onEdit }: { item: PendingAttachment; onRemove: () => void; onRetry?: () => void; onEdit?: () => void }) {
  const media = item.kind === "image" || item.kind === "video";
  const busy = item.status === "yükleniyor" || item.status === "hazırlanıyor";
  return (
    <div className={cn("group relative overflow-hidden rounded-xl border bg-muted/40", item.status === "hata" ? "border-destructive/60" : "border-border/60", media ? "size-24" : "h-14 w-56")}>
      {media ? (
        <>
          {item.kind === "image" ? (
            <img src={item.previewUrl} alt={item.name} className="size-full object-cover" />
          ) : (
            <video src={item.previewUrl} muted playsInline preload="metadata" className="size-full object-cover" />
          )}
          {item.kind === "video" && (
            <span className="pointer-events-none absolute inset-0 flex items-center justify-center">
              <span className="flex size-8 items-center justify-center rounded-full bg-black/55"><Play className="size-3.5 fill-white text-white" /></span>
            </span>
          )}
        </>
      ) : (
        <div className="flex h-full items-center gap-2.5 px-2.5" title={item.name}>
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><FileText className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-medium">{item.name}</span>
            <span className="block text-[0.65rem] text-muted-foreground">{extensionOf(item.name, item.mime)} · {formatSize(item.size)}</span>
          </span>
        </div>
      )}

      {busy && (
        <span className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/55 text-white">
          <Loader2 className="size-4 animate-spin" />
          <span className="text-[0.65rem] tabular-nums">{item.status === "hazırlanıyor" ? "hazırlanıyor" : `%${item.progress}`}</span>
        </span>
      )}
      {busy && item.status === "yükleniyor" && (
        <span className="absolute inset-x-0 bottom-0 h-1 bg-black/40">
          <span className="block h-full bg-primary transition-[width] duration-200" style={{ width: `${item.progress}%` }} />
        </span>
      )}

      {item.status === "hata" && (
        <span className="pointer-events-none absolute inset-x-0 bottom-0 flex items-center gap-1 bg-destructive/85 px-1.5 py-0.5 text-[0.6rem] text-white" title={item.error}>
          <TriangleAlert className="size-3 shrink-0" /> Yüklenemedi
        </span>
      )}

      <span className={cn("absolute top-1 right-1 flex gap-0.5", media ? "opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100" : "")}>
        {item.status === "hata" && onRetry && (
          <button type="button" onClick={onRetry} title="Yeniden dene" className="flex size-6 items-center justify-center rounded-md bg-black/60 text-white hover:bg-black/80"><RefreshCw className="size-3" /></button>
        )}
        {item.kind === "image" && onEdit && item.status !== "hazırlanıyor" && (
          <button type="button" onClick={onEdit} title="İşaretle / gizle" className="flex size-6 items-center justify-center rounded-md bg-black/60 text-white hover:bg-black/80"><Pencil className="size-3" /></button>
        )}
        <button type="button" onClick={onRemove} title="Kaldır" className={cn("flex size-6 items-center justify-center rounded-md", media ? "bg-black/60 text-white hover:bg-black/80" : "text-muted-foreground hover:bg-accent hover:text-foreground")}><X className="size-3" /></button>
      </span>
    </div>
  );
}
