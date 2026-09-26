// AdSource shows the ad or post a customer tapped to start writing: its
// picture, title and text, and a link to open it.

import { ExternalLink, Megaphone } from "lucide-react";
import { cn } from "@/lib/utils";
import type { WAReferral } from "@/whatsapp/types";

export function asReferral(v: unknown): WAReferral | null {
  if (!v || typeof v !== "object") return null;
  const r = v as WAReferral;
  return r.source_url || r.headline || r.body || r.source_id ? r : null;
}

export default function AdSource({ r, compact }: { r: WAReferral; compact?: boolean }) {
  const pic = r.thumbnail_url || r.image_url;
  const what = r.source_type === "post" ? "Gönderiden geldi" : "Reklamdan geldi";
  return (
    <div className={cn("overflow-hidden rounded-xl bg-foreground/5 text-left", compact ? "mb-1.5" : "")}>
      <div className="flex gap-2.5 p-2">
        {pic ? (
          <img src={pic} alt="" className={cn("shrink-0 rounded-lg object-cover", compact ? "size-11" : "size-14")} referrerPolicy="no-referrer" />
        ) : (
          <span className={cn("flex shrink-0 items-center justify-center rounded-lg bg-sky-500/12 text-sky-600 dark:text-sky-400", compact ? "size-11" : "size-14")}><Megaphone className="size-5" /></span>
        )}
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1 text-[0.62rem] font-semibold tracking-wide text-sky-700 uppercase dark:text-sky-400"><Megaphone className="size-3" /> {what}</span>
          {r.headline && <span className="block truncate text-xs font-semibold">{r.headline}</span>}
          {r.body && <span className={cn("block text-[0.7rem] text-muted-foreground", compact ? "line-clamp-1" : "line-clamp-3")}>{r.body}</span>}
        </span>
      </div>
      {r.source_url && (
        <a href={r.source_url} target="_blank" rel="noopener noreferrer" className="flex items-center justify-center gap-1 border-t border-foreground/10 py-1.5 text-[0.7rem] font-medium text-primary hover:bg-foreground/5">
          <ExternalLink className="size-3" /> {r.source_type === "post" ? "Gönderiyi aç" : "Reklamı aç"}
        </a>
      )}
    </div>
  );
}
