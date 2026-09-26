// TemplatePicker chooses an approved template, fills its blanks and shows
// what the customer will read before it goes.

import { useEffect, useMemo, useState } from "react";
import { FileText, Search } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WATemplate, WATemplateComponent } from "@/whatsapp/types";

const CATEGORY: Record<string, string> = { MARKETING: "Pazarlama", UTILITY: "Hizmet", AUTHENTICATION: "Doğrulama" };

function vars(text?: string): number {
  let n = 0;
  for (const m of (text ?? "").matchAll(/\{\{(\d+)\}\}/g)) n = Math.max(n, Number(m[1]));
  return n;
}

function fill(text: string, vals: string[]): string {
  return text.replace(/\{\{(\d+)\}\}/g, (all, k) => vals[Number(k) - 1] || all);
}

export interface TemplateChoice {
  templateId: number;
  params: { header: string[]; body: string[]; buttons: string[]; headerMedia: string };
  name: string;
}

export default function TemplatePicker({ channelId, open, onClose, onSend, defaults }: { channelId: number; open: boolean; onClose: () => void; onSend: (c: TemplateChoice) => Promise<void>; defaults?: Record<string, string> }) {
  const [list, setList] = useState<WATemplate[] | null>(null);
  const [q, setQ] = useState("");
  const [pick, setPick] = useState<WATemplate | null>(null);
  const [header, setHeader] = useState<string[]>([]);
  const [body, setBody] = useState<string[]>([]);
  const [buttons, setButtons] = useState<string[]>([]);
  const [media, setMedia] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setPick(null);
    setError(null);
    waApi.templates(channelId).then((l) => setList(l.filter((t) => t.status === "APPROVED"))).catch((e) => {
      setList([]);
      setError(e instanceof ApiError ? e.message : "Şablonlar alınamadı.");
    });
  }, [open, channelId]);

  const parts = useMemo(() => {
    const c = (pick?.components ?? []) as WATemplateComponent[];
    const find = (t: string) => c.find((x) => x.type?.toUpperCase() === t);
    return { header: find("HEADER"), body: find("BODY"), footer: find("FOOTER"), buttons: find("BUTTONS") };
  }, [pick]);

  useEffect(() => {
    if (!pick) return;
    const bn = vars(parts.body?.text);
    const pre = defaults ?? {};
    setHeader(Array.from({ length: vars(parts.header?.text) }, () => ""));
    setBody(Array.from({ length: bn }, (_, i) => (i === 0 && pre.musteri ? pre.musteri : "")));
    setButtons((parts.buttons?.buttons ?? []).filter((b) => b.type?.toUpperCase() === "URL" && vars(b.url) > 0).map(() => ""));
    setMedia("");
  }, [pick]); // eslint-disable-line react-hooks/exhaustive-deps

  const shown = (list ?? []).filter((t) => !q || t.name.includes(q.toLowerCase()) || (t.components.find((c) => c.type === "BODY")?.text ?? "").toLocaleLowerCase("tr").includes(q.toLocaleLowerCase("tr")));
  const mediaHeader = parts.header && parts.header.format && parts.header.format !== "TEXT";
  const ready = pick && header.every((v) => v.trim()) && body.every((v) => v.trim()) && buttons.every((v) => v.trim()) && (!mediaHeader || media.trim());

  const send = async () => {
    if (!pick || !ready) return;
    setBusy(true);
    setError(null);
    try {
      await onSend({ templateId: pick.id, name: pick.name, params: { header, body, buttons, headerMedia: media } });
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Şablonla yaz" description="24 saat penceresi kapalıyken ya da müşteriye ilk kez yazarken yalnızca Meta'nın onayladığı şablonlar gönderilebilir." size="lg"
      footer={<>
        {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
        <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
        <Button onClick={() => void send()} disabled={!ready || busy}>{busy ? "Gönderiliyor..." : "Gönder"}</Button>
      </>}
    >
      <div className="grid gap-4 md:grid-cols-[15rem_1fr]">
        <div className="space-y-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Şablon ara" className="h-9 w-full rounded-xl border border-border/60 bg-muted/40 pl-9 pr-3 text-sm outline-none focus:border-ring/50" />
          </div>
          <div className="max-h-80 space-y-1 overflow-y-auto">
            {list === null && <p className="px-2 py-4 text-xs text-muted-foreground">Yükleniyor...</p>}
            {list?.length === 0 && <p className="px-2 py-4 text-xs text-muted-foreground">Onaylı şablon yok. Ayarlar &gt; Şablonlar ekranından oluşturabilirsiniz.</p>}
            {shown.map((t) => (
              <button key={t.id} type="button" onClick={() => setPick(t)} className={cn("flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors", pick?.id === t.id ? "bg-primary/10 ring-1 ring-primary/30" : "hover:bg-accent/60")}>
                <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground"><FileText className="size-4" /></span>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{t.name}</span>
                  <span className="block text-[0.65rem] text-muted-foreground">{CATEGORY[t.category] ?? t.category} · {t.language}</span>
                </span>
              </button>
            ))}
          </div>
        </div>
        <div className="min-w-0">
          {!pick ? (
            <p className="flex h-full items-center justify-center rounded-2xl bg-muted/30 p-6 text-center text-sm text-muted-foreground">Soldan bir şablon seçin.</p>
          ) : (
            <div className="space-y-4">
              {(header.length > 0 || body.length > 0 || buttons.length > 0 || mediaHeader) && (
                <div className="space-y-2 rounded-2xl bg-muted/30 p-3">
                  <p className="text-xs font-semibold text-muted-foreground">Boşlukları doldurun</p>
                  {mediaHeader && <Field label={`Başlık dosyasının linki (${parts.header?.format?.toLowerCase()})`} value={media} onChange={setMedia} placeholder="https://..." />}
                  {header.map((v, i) => <Field key={`h${i}`} label={`Başlık {{${i + 1}}}`} value={v} onChange={(x) => setHeader((a) => a.map((y, j) => (j === i ? x : y)))} />)}
                  {body.map((v, i) => <Field key={`b${i}`} label={`Metin {{${i + 1}}}`} value={v} onChange={(x) => setBody((a) => a.map((y, j) => (j === i ? x : y)))} />)}
                  {buttons.map((v, i) => <Field key={`u${i}`} label={`Düğme linkinin sonu ${i + 1}`} value={v} onChange={(x) => setButtons((a) => a.map((y, j) => (j === i ? x : y)))} />)}
                </div>
              )}
              <div>
                <p className="mb-1.5 text-xs font-semibold text-muted-foreground">Müşterinin göreceği</p>
                <div className="rounded-2xl rounded-tr-md bg-emerald-500/12 p-3 text-sm shadow-sm ring-1 ring-emerald-600/20">
                  {parts.header?.format === "TEXT" && parts.header.text && <p className="mb-1 font-semibold">{fill(parts.header.text, header)}</p>}
                  {mediaHeader && <p className="mb-1 rounded-lg bg-foreground/5 px-2 py-1.5 text-xs text-muted-foreground">{parts.header?.format?.toLowerCase()} başlık</p>}
                  <p className="whitespace-pre-wrap leading-relaxed">{fill(parts.body?.text ?? "", body)}</p>
                  {parts.footer?.text && <p className="mt-1 text-xs text-muted-foreground">{parts.footer.text}</p>}
                  {(parts.buttons?.buttons ?? []).length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1">
                      {parts.buttons!.buttons!.map((b, i) => <span key={i} className="rounded-full bg-card/80 px-2.5 py-1 text-xs font-medium text-sky-600 ring-1 ring-border/60 dark:text-sky-400">{b.text}</span>)}
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
}

function Field({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <label className="block space-y-1">
      <span className="text-[0.7rem] font-medium text-muted-foreground">{label}</span>
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className="h-9 w-full rounded-xl border border-border/60 bg-card px-3 text-sm outline-none focus:border-ring/50 focus:ring-4 focus:ring-ring/15" />
    </label>
  );
}
