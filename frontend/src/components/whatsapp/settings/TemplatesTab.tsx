// TemplatesTab lists the message templates of a number's business account,
// shows whether Meta approved them, and sends new ones for approval with a
// live preview of what the customer will see.

import { useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, FileText, ImageIcon, Phone, Plus, RefreshCw, Reply, Search, Trash2, Upload, Video, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { Toolbar } from "@/components/ui/rows";
import { areaCls, FormField, inputCls } from "@/components/whatsapp/settings/parts";
import { waText } from "@/components/whatsapp/waText";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WATemplate } from "@/whatsapp/types";
import { since } from "@/whatsapp/util";

const STATUS: Record<string, { label: string; tone: string; tip: string }> = {
  APPROVED: { label: "Onaylandı", tone: "bg-success/12 text-success", tip: "Kullanılabilir" },
  PENDING: { label: "Onay bekliyor", tone: "bg-warning/12 text-warning", tip: "Meta inceliyor. Genelde birkaç dakika, bazen bir gün sürer." },
  REJECTED: { label: "Reddedildi", tone: "bg-destructive/10 text-destructive", tip: "Meta kabul etmedi" },
  PAUSED: { label: "Durduruldu", tone: "bg-warning/12 text-warning", tip: "Müşteriler çok şikâyet ettiği için Meta geçici olarak durdurdu" },
  DISABLED: { label: "Kapatıldı", tone: "bg-muted text-muted-foreground", tip: "Meta kalıcı olarak kapattı" },
};

const CATEGORY: Record<string, string> = { MARKETING: "Pazarlama", UTILITY: "Hizmet", AUTHENTICATION: "Doğrulama" };

export default function TemplatesTab({ channels }: { channels: WAChannel[] }) {
  const { user } = useAuth();
  const manage = can(user, "whatsapp.template_manage");
  const [channelId, setChannelId] = useState(channels[0]?.id ?? 0);
  const [items, setItems] = useState<WATemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false);
  const [open, setOpen] = useState<WATemplate | null>(null);
  const [del, setDel] = useState<WATemplate | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const now = Date.now();

  const load = () => {
    if (!channelId) return;
    setLoading(true);
    waApi.templates(channelId).then(setItems).catch(() => setItems([])).finally(() => setLoading(false));
  };
  useEffect(load, [channelId]); // eslint-disable-line react-hooks/exhaustive-deps

  const sync = async () => {
    setSyncing(true);
    setMsg(null);
    try {
      const r = await waApi.syncTemplates(channelId);
      setMsg(`Meta'dan ${r.count} şablon alındı.`);
      load();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Alınamadı.");
    } finally {
      setSyncing(false);
    }
  };

  const shown = useMemo(() => {
    const t = search.trim().toLocaleLowerCase("tr");
    return items.filter((x) => (!status || x.status === status) && (!t || x.name.includes(t) || bodyOf(x).toLocaleLowerCase("tr").includes(t)));
  }, [items, search, status]);

  if (channels.length === 0) return <p className="rounded-2xl bg-card p-8 text-center text-sm text-muted-foreground ring-1 ring-border/60">Önce bir numara ekleyin.</p>;

  return (
    <Card title="Şablon mesajlar" icon={FileText} actions={
      <span className="flex items-center gap-2">
        <Button variant="secondary" onClick={() => void sync()} disabled={syncing}><RefreshCw className={cn(syncing && "animate-spin")} /> Meta'dan yenile</Button>
        {manage && <Button onClick={() => setCreating(true)}><Plus /> Yeni şablon</Button>}
      </span>
    }>
      <p className="mb-4 text-sm text-muted-foreground">Müşteri son 24 saatte yazmadıysa ona sadece Meta'nın onayladığı şablonlarla yazabilirsiniz. Şablonlar işletme hesabına bağlıdır; aynı hesaptaki numaralar aynı şablonları kullanır.</p>
      <Toolbar className="mb-3">
        {channels.length > 1 && (
          <select value={channelId} onChange={(e) => setChannelId(Number(e.target.value))} className="h-9 rounded-xl border border-border/60 bg-card px-3 text-sm">
            {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        )}
        <span className="relative min-w-48 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Ad ya da metin ara" className="h-9 w-full rounded-xl border border-border/60 bg-card pr-3 pl-9 text-sm outline-none focus:border-ring/50" />
        </span>
        <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-9 rounded-xl border border-border/60 bg-card px-3 text-sm">
          <option value="">Tüm durumlar</option>
          {Object.entries(STATUS).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
        </select>
        {msg && <span className="text-xs text-muted-foreground">{msg}</span>}
      </Toolbar>
      {loading ? (
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{Array.from({ length: 6 }).map((_, i) => <div key={i} className="h-32 animate-pulse rounded-2xl bg-muted/50" />)}</div>
      ) : shown.length === 0 ? (
        <EmptyState icon={<FileText />} title={items.length ? "Aramaya uyan şablon yok" : "Henüz şablon yok"} description={items.length ? undefined : "Meta'da zaten şablonunuz varsa \"Meta'dan yenile\"ye basın."} />
      ) : (
        <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {shown.map((t) => {
            const st = STATUS[t.status] ?? { label: t.status, tone: "bg-muted text-muted-foreground", tip: "" };
            return (
              <button key={t.id} type="button" onClick={() => setOpen(t)} className="group flex flex-col gap-2 rounded-2xl bg-muted/25 p-3 text-left ring-1 ring-border/50 transition-colors hover:bg-accent/50">
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-mono text-xs font-semibold">{t.name}</span>
                  <span className={cn("shrink-0 rounded-full px-2 py-0.5 text-[0.62rem] font-semibold", st.tone)} data-tip={st.tip}>{st.label}</span>
                </span>
                <span className="line-clamp-3 text-sm text-muted-foreground">{bodyOf(t)}</span>
                <span className="mt-auto flex items-center gap-2 text-[0.68rem] text-muted-foreground">
                  <span className="rounded bg-muted px-1.5 py-px font-medium">{CATEGORY[t.category] ?? t.category}</span>
                  <span className="uppercase">{t.language}</span>
                  {t.quality && t.quality !== "UNKNOWN" && <span className={cn(t.quality === "RED" ? "text-destructive" : t.quality === "YELLOW" ? "text-warning" : "text-success")}>● kalite</span>}
                  <span className="ml-auto">{since(t.updatedAt, now)} önce</span>
                </span>
                {t.status === "REJECTED" && t.rejectedReason && <span className="rounded-lg bg-destructive/8 px-2 py-1 text-[0.7rem] text-destructive">Neden: {rejectWord(t.rejectedReason)}</span>}
              </button>
            );
          })}
        </div>
      )}
      {open && (
        <Modal open onClose={() => setOpen(null)} title={open.name} description={`${CATEGORY[open.category] ?? open.category} · ${open.language.toUpperCase()} · ${(STATUS[open.status] ?? { label: open.status }).label}`}
          footer={<>
            {manage && <Button variant="danger" className="mr-auto" onClick={() => { setDel(open); setOpen(null); }}><Trash2 /> Sil</Button>}
            <Button onClick={() => setOpen(null)}>Kapat</Button>
          </>}>
          <PhonePreview t={toDraft(open)} />
        </Modal>
      )}
      {creating && <TemplateForm channelId={channelId} onClose={() => setCreating(false)} onSaved={() => { setCreating(false); load(); }} />}
      <ConfirmDialog open={!!del} title="Şablon silinsin mi?" description={`${del?.name} Meta'dan da silinir. Aynı adla 30 gün yeni şablon açılamaz.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteTemplate(del.id).then(() => { setDel(null); load(); }).catch((e) => { setMsg(e instanceof ApiError ? e.message : "Silinemedi."); setDel(null); })} />
    </Card>
  );
}

function bodyOf(t: WATemplate): string {
  return t.components.find((c) => c.type === "BODY")?.text ?? "";
}

function rejectWord(r: string): string {
  const map: Record<string, string> = {
    INVALID_FORMAT: "biçim hatalı",
    TAG_CONTENT_MISMATCH: "kategori içerikle uyuşmuyor",
    ABUSIVE_CONTENT: "uygunsuz içerik",
    PROMOTIONAL: "reklam içeriği hizmet kategorisinde",
    SCAM: "dolandırıcılık şüphesi",
    INCORRECT_CATEGORY: "yanlış kategori",
  };
  return map[r] ?? r.toLowerCase().replace(/_/g, " ");
}

interface Draft {
  headerFormat: string;
  headerText: string;
  body: string;
  footer: string;
  buttons: { type: string; text: string; url: string; phone: string; example: string }[];
  mediaName?: string;
}

function toDraft(t: WATemplate): Draft {
  const h = t.components.find((c) => c.type === "HEADER");
  return {
    headerFormat: h?.format ?? "NONE",
    headerText: h?.text ?? "",
    body: bodyOf(t),
    footer: t.components.find((c) => c.type === "FOOTER")?.text ?? "",
    buttons: (t.components.find((c) => c.type === "BUTTONS")?.buttons ?? []).map((b) => ({ type: b.type, text: b.text, url: b.url ?? "", phone: b.phone_number ?? "", example: "" })),
  };
}

function PhonePreview({ t, examples }: { t: Draft; examples?: string[] }) {
  const fill = (s: string) => s.replace(/\{\{(\d+)\}\}/g, (m, n) => (examples && examples[Number(n) - 1]) || m);
  const MediaIcon = t.headerFormat === "VIDEO" ? Video : t.headerFormat === "DOCUMENT" ? FileText : ImageIcon;
  return (
    <div className="rounded-3xl bg-[#efe7dd] p-4 dark:bg-[#0b141a]">
      <div className="max-w-[20rem] overflow-hidden rounded-2xl rounded-tl-sm bg-white text-slate-900 shadow-sm dark:bg-[#202c33] dark:text-slate-100">
        {["IMAGE", "VIDEO", "DOCUMENT"].includes(t.headerFormat) && (
          <div className="m-1 flex h-32 flex-col items-center justify-center gap-1 rounded-xl bg-slate-200/80 text-slate-500 dark:bg-slate-700/60 dark:text-slate-300">
            <MediaIcon className="size-8" />
            {t.mediaName && <span className="max-w-[90%] truncate text-[0.65rem]">{t.mediaName}</span>}
          </div>
        )}
        <div className="space-y-1 px-3 pt-2 pb-1.5">
          {t.headerFormat === "TEXT" && t.headerText && <p className="text-sm font-bold">{fill(t.headerText)}</p>}
          <p className="text-sm leading-snug whitespace-pre-wrap">{t.body ? waText(fill(t.body)) : <span className="text-slate-400">Mesaj metni burada görünür</span>}</p>
          {t.footer && <p className="text-xs text-slate-500 dark:text-slate-400">{t.footer}</p>}
          <p className="text-right text-[0.6rem] text-slate-400">12:30</p>
        </div>
        {t.buttons.filter((b) => b.text).map((b, i) => (
          <div key={i} className="flex items-center justify-center gap-1.5 border-t border-slate-200 py-2 text-sm font-medium text-sky-600 dark:border-slate-600 dark:text-sky-400">
            {b.type === "URL" ? <ExternalLink className="size-3.5" /> : b.type === "PHONE_NUMBER" ? <Phone className="size-3.5" /> : <Reply className="size-3.5" />}
            {b.text}
          </div>
        ))}
      </div>
    </div>
  );
}

function TemplateForm({ channelId, onClose, onSaved }: { channelId: number; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState("");
  const [language, setLanguage] = useState("tr");
  const [category, setCategory] = useState("UTILITY");
  const [d, setD] = useState<Draft>({ headerFormat: "NONE", headerText: "", body: "", footer: "", buttons: [] });
  const [headerExample, setHeaderExample] = useState("");
  const [examples, setExamples] = useState<string[]>([]);
  const [handle, setHandle] = useState("");
  const [uploading, setUploading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const varCount = useMemo(() => {
    let n = 0;
    for (const m of d.body.matchAll(/\{\{(\d+)\}\}/g)) n = Math.max(n, Number(m[1]));
    return n;
  }, [d.body]);

  const addVar = () => {
    const el = bodyRef.current;
    const tag = `{{${varCount + 1}}}`;
    if (!el) return setD((c) => ({ ...c, body: c.body + tag }));
    const s = el.selectionStart, e = el.selectionEnd;
    setD((c) => ({ ...c, body: c.body.slice(0, s) + tag + c.body.slice(e) }));
    requestAnimationFrame(() => { el.focus(); el.setSelectionRange(s + tag.length, s + tag.length); });
  };

  const upload = async (f: File) => {
    setUploading(true);
    setError(null);
    try {
      const r = await waApi.templateMedia(channelId, f);
      setHandle(r.handle);
      setD((c) => ({ ...c, mediaName: f.name }));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Dosya yüklenemedi.");
    } finally {
      setUploading(false);
    }
  };

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await waApi.createTemplate({
        channelId, name, language, category,
        headerFormat: d.headerFormat, headerText: d.headerText, headerExample, headerHandle: handle,
        body: d.body, bodyExamples: examples.slice(0, varCount), footer: d.footer, buttons: d.buttons,
      });
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const setBtn = (i: number, patch: Partial<Draft["buttons"][number]>) => setD((c) => ({ ...c, buttons: c.buttons.map((b, j) => (j === i ? { ...b, ...patch } : b)) }));
  const hasAction = d.buttons.some((b) => b.type !== "QUICK_REPLY");
  const quickCount = d.buttons.filter((b) => b.type === "QUICK_REPLY").length;

  return (
    <Modal open onClose={onClose} title="Yeni şablon" description="Meta'ya onaya gönderilir. Onaylanınca kullanılabilir hale gelir." size="lg" footer={<>
      {error && <span className="mr-auto max-w-md text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !name || !d.body.trim()}>{busy ? "Gönderiliyor..." : "Onaya gönder"}</Button>
    </>}>
      <div className="grid gap-5 lg:grid-cols-[1fr_18rem]">
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-[1fr_6rem]">
            <FormField label="Şablon adı" hint="Küçük harf, rakam ve alt çizgi. Örn. siparis_hazir">
              <input className={cn(inputCls, "font-mono")} value={name} onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_"))} />
            </FormField>
            <FormField label="Dil">
              <select className={inputCls} value={language} onChange={(e) => setLanguage(e.target.value)}>
                <option value="tr">Türkçe</option>
                <option value="en">İngilizce</option>
                <option value="en_US">İngilizce (ABD)</option>
                <option value="de">Almanca</option>
                <option value="ar">Arapça</option>
                <option value="ru">Rusça</option>
              </select>
            </FormField>
          </div>
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Kategori</p>
            <div className="grid gap-1.5 sm:grid-cols-3">
              {([["UTILITY", "Hizmet", "Sipariş, randevu, destek talebi gibi bilgilendirme"], ["MARKETING", "Pazarlama", "Kampanya, duyuru, tanıtım"], ["AUTHENTICATION", "Doğrulama", "Tek kullanımlık kod"]] as const).map(([k, l, s]) => (
                <button key={k} type="button" onClick={() => setCategory(k)} className={cn("rounded-xl px-3 py-2 text-left ring-1 transition-colors", category === k ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
                  <span className="block text-sm font-medium">{l}</span>
                  <span className="block text-[0.68rem] leading-snug text-muted-foreground">{s}</span>
                </button>
              ))}
            </div>
          </div>
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Başlık (isteğe bağlı)</p>
            <div className="flex flex-wrap gap-1 rounded-2xl bg-muted/50 p-1">
              {([["NONE", "Yok"], ["TEXT", "Yazı"], ["IMAGE", "Görsel"], ["VIDEO", "Video"], ["DOCUMENT", "Belge"]] as const).map(([k, l]) => (
                <button key={k} type="button" onClick={() => { setD((c) => ({ ...c, headerFormat: k, mediaName: undefined })); setHandle(""); }} className={cn("flex-1 rounded-xl px-2 py-1.5 text-xs font-medium", d.headerFormat === k ? "bg-card shadow-sm ring-1 ring-border/60" : "text-muted-foreground hover:text-foreground")}>{l}</button>
              ))}
            </div>
            {d.headerFormat === "TEXT" && (
              <div className="grid gap-2 sm:grid-cols-2">
                <input className={inputCls} maxLength={60} value={d.headerText} onChange={(e) => setD((c) => ({ ...c, headerText: e.target.value }))} placeholder="Başlık metni" />
                {/\{\{1\}\}/.test(d.headerText) && <input className={inputCls} value={headerExample} onChange={(e) => setHeaderExample(e.target.value)} placeholder="{{1}} için örnek" />}
              </div>
            )}
            {["IMAGE", "VIDEO", "DOCUMENT"].includes(d.headerFormat) && (
              <div className="flex items-center gap-2">
                <input ref={fileRef} type="file" hidden accept={d.headerFormat === "IMAGE" ? "image/jpeg,image/png" : d.headerFormat === "VIDEO" ? "video/mp4" : "application/pdf"} onChange={(e) => { const f = e.target.files?.[0]; if (f) void upload(f); e.target.value = ""; }} />
                <Button variant="secondary" onClick={() => fileRef.current?.click()} disabled={uploading}><Upload /> {uploading ? "Yükleniyor..." : handle ? "Başka dosya seç" : "Örnek dosya yükle"}</Button>
                <span className="text-xs text-muted-foreground">{handle ? `${d.mediaName} yüklendi` : "Meta onay için bir örnek ister. Gönderirken asıl dosyayı seçersiniz."}</span>
              </div>
            )}
          </div>
          <FormField label="Mesaj metni" hint={<>Müşteriye göre değişecek yerler için değişken ekleyin. *kalın*, _eğik_, ~üstü çizili~ yazabilirsiniz.</>}>
            <textarea ref={bodyRef} className={areaCls} rows={5} maxLength={1024} value={d.body} onChange={(e) => setD((c) => ({ ...c, body: e.target.value }))} placeholder="Merhaba {{1}}, {{2}} numaralı siparişiniz hazır." />
          </FormField>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" onClick={addVar} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-primary ring-1 ring-primary/30 hover:bg-primary/10"><Plus className="size-3.5" /> Değişken ekle</button>
            <span className="text-[0.7rem] text-muted-foreground">{d.body.length} / 1024</span>
          </div>
          {varCount > 0 && (
            <div className="space-y-1.5 rounded-2xl bg-muted/30 p-3">
              <p className="text-xs font-medium text-muted-foreground">Meta onay için her değişkene bir örnek ister</p>
              <div className="grid gap-2 sm:grid-cols-2">
                {Array.from({ length: varCount }).map((_, i) => (
                  <label key={i} className="flex items-center gap-2">
                    <code className="w-10 shrink-0 font-mono text-xs text-muted-foreground">{`{{${i + 1}}}`}</code>
                    <input className={cn(inputCls, "h-9")} value={examples[i] ?? ""} onChange={(e) => setExamples((cur) => { const n = [...cur]; n[i] = e.target.value; return n; })} placeholder={i === 0 ? "Ayşe" : "örnek değer"} />
                  </label>
                ))}
              </div>
            </div>
          )}
          <FormField label="Alt yazı (isteğe bağlı)">
            <input className={inputCls} maxLength={60} value={d.footer} onChange={(e) => setD((c) => ({ ...c, footer: e.target.value }))} placeholder="Mesaj almak istemiyorsanız DUR yazın" />
          </FormField>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <p className="text-xs font-medium text-muted-foreground">Düğmeler (isteğe bağlı)</p>
              <span className="ml-auto flex gap-1">
                <SmallBtn disabled={hasAction || quickCount >= 3} onClick={() => setD((c) => ({ ...c, buttons: [...c.buttons, { type: "QUICK_REPLY", text: "", url: "", phone: "", example: "" }] }))}><Reply className="size-3.5" /> Hızlı cevap</SmallBtn>
                <SmallBtn disabled={quickCount > 0 || d.buttons.filter((b) => b.type === "URL").length >= 2} onClick={() => setD((c) => ({ ...c, buttons: [...c.buttons, { type: "URL", text: "", url: "https://", phone: "", example: "" }] }))}><ExternalLink className="size-3.5" /> Link</SmallBtn>
                <SmallBtn disabled={quickCount > 0 || d.buttons.some((b) => b.type === "PHONE_NUMBER")} onClick={() => setD((c) => ({ ...c, buttons: [...c.buttons, { type: "PHONE_NUMBER", text: "", url: "", phone: "+90", example: "" }] }))}><Phone className="size-3.5" /> Telefon</SmallBtn>
              </span>
            </div>
            {d.buttons.map((b, i) => (
              <div key={i} className="flex flex-wrap items-center gap-2 rounded-xl bg-muted/30 p-2">
                <input className={cn(inputCls, "h-9 min-w-32 flex-1")} maxLength={25} value={b.text} onChange={(e) => setBtn(i, { text: e.target.value })} placeholder="Düğme yazısı" />
                {b.type === "URL" && <input className={cn(inputCls, "h-9 min-w-48 flex-[2]")} value={b.url} onChange={(e) => setBtn(i, { url: e.target.value })} placeholder="https://site.com/siparis/{{1}}" />}
                {b.type === "URL" && /\{\{1\}\}/.test(b.url) && <input className={cn(inputCls, "h-9 w-40")} value={b.example} onChange={(e) => setBtn(i, { example: e.target.value })} placeholder="Tam örnek link" />}
                {b.type === "PHONE_NUMBER" && <input className={cn(inputCls, "h-9 w-44")} value={b.phone} onChange={(e) => setBtn(i, { phone: e.target.value })} placeholder="+90..." />}
                <button type="button" onClick={() => setD((c) => ({ ...c, buttons: c.buttons.filter((_, j) => j !== i) }))} className="rounded-lg p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
              </div>
            ))}
            <p className="text-[0.7rem] text-muted-foreground">En fazla 3 hızlı cevap ya da 2 link ve 1 telefon. İki tür aynı şablonda kullanılamaz.</p>
          </div>
        </div>
        <div className="space-y-2 lg:sticky lg:top-0 lg:self-start">
          <p className="text-xs font-medium text-muted-foreground">Müşterinin göreceği</p>
          <PhonePreview t={d} examples={examples} />
        </div>
      </div>
    </Modal>
  );
}

function SmallBtn({ onClick, disabled, children }: { onClick: () => void; disabled?: boolean; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} disabled={disabled} className="flex h-7 items-center gap-1 rounded-full px-2.5 text-[0.7rem] font-semibold text-primary ring-1 ring-primary/25 hover:bg-primary/10 disabled:opacity-35">{children}</button>;
}
