// FileUpload picks a file from the computer, keeps it on the server and
// shows what was chosen: a thumbnail for pictures, the name and size for
// the rest. Used by chatbot boxes and template headers.

import { useRef, useState } from "react";
import { FileText, Film, Music, Upload, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAFile } from "@/whatsapp/types";

export interface PickedFile {
  id: number;
  name: string;
  kind?: string;
}

function sizeWord(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${Math.round(n / 1024)} KB`;
  return `${(n / (1 << 20)).toFixed(1)} MB`;
}

export default function FileUpload({ value, onChange, accept, hint, disabled }: { value: PickedFile | null; onChange: (f: (PickedFile & Partial<WAFile>) | null) => void; accept?: string; hint?: string; disabled?: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [size, setSize] = useState<number | null>(null);

  const pick = async (f: File) => {
    setBusy(true);
    setError(null);
    try {
      const up = await waApi.uploadFile(f);
      setSize(up.size);
      onChange(up);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Dosya yüklenemedi.");
    } finally {
      setBusy(false);
    }
  };

  const Icon = value?.kind === "video" ? Film : value?.kind === "audio" ? Music : FileText;
  return (
    <div className="space-y-1.5">
      <input ref={input} type="file" hidden accept={accept} onChange={(e) => { const f = e.target.files?.[0]; if (f) void pick(f); e.target.value = ""; }} />
      {value ? (
        <div className="flex items-center gap-3 rounded-2xl bg-muted/40 p-2 ring-1 ring-border/60">
          {value.kind === "image" ? (
            <img src={waApi.fileUrl(value.id)} alt="" className="size-14 shrink-0 rounded-xl object-cover" />
          ) : (
            <span className="flex size-14 shrink-0 items-center justify-center rounded-xl bg-card text-muted-foreground"><Icon className="size-6" /></span>
          )}
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{value.name}</span>
            <span className="block text-[0.7rem] text-muted-foreground">{size != null ? sizeWord(size) : "Yüklendi"}</span>
          </span>
          {!disabled && (
            <span className="flex shrink-0 gap-1">
              <button type="button" onClick={() => input.current?.click()} className="rounded-lg px-2 py-1 text-xs font-medium text-primary hover:bg-primary/10">Değiştir</button>
              <button type="button" onClick={() => onChange(null)} aria-label="Kaldır" className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><X className="size-4" /></button>
            </span>
          )}
        </div>
      ) : (
        <button type="button" disabled={disabled || busy} onClick={() => input.current?.click()} className={cn("flex w-full flex-col items-center gap-1.5 rounded-2xl border-2 border-dashed border-border/70 px-4 py-5 text-center transition-colors hover:border-primary/50 hover:bg-primary/5 disabled:opacity-60")}>
          <Upload className={cn("size-5 text-muted-foreground", busy && "animate-bounce")} />
          <span className="text-sm font-medium">{busy ? "Yükleniyor..." : "Bilgisayardan dosya seç"}</span>
          {hint && <span className="text-[0.7rem] text-muted-foreground">{hint}</span>}
        </button>
      )}
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  );
}
