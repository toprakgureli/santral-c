// IntegrationsTab keeps the outside systems a chatbot can ask: an order
// status address, a customer lookup. Headers such as keys are stored
// encrypted and never shown again.

import { useEffect, useState } from "react";
import { Pencil, Play, Plug, Plus, Trash2, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { ListRow } from "@/components/ui/rows";
import { areaCls, FormField, inputCls } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAIntegration } from "@/whatsapp/types";

export default function IntegrationsTab() {
  const [items, setItems] = useState<WAIntegration[]>([]);
  const [edit, setEdit] = useState<WAIntegration | "new" | null>(null);
  const [test, setTest] = useState<WAIntegration | null>(null);
  const [del, setDel] = useState<WAIntegration | null>(null);
  const load = () => waApi.integrations().then(setItems).catch(() => setItems([]));
  useEffect(() => { void load(); }, []);
  return (
    <Card title="Dış sistemler" icon={Plug} actions={<Button onClick={() => setEdit("new")}><Plus /> Yeni bağlantı</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Chatbot'un soru sorabileceği adresler. Örneğin müşterinin yazdığı sipariş numarasıyla kargo durumunu sorup cevabı müşteriye yazdırabilirsiniz. Adreste <code className="rounded bg-muted px-1 font-mono text-xs">{"{siparis}"}</code> gibi alanlar chatbot'un topladığı bilgilerle doldurulur.</p>
      {items.length === 0 ? (
        <EmptyState icon={<Plug />} title="Henüz bağlantı yok" description="Chatbot'lar başka bir sisteme soru sormayacaksa buna gerek yok." />
      ) : (
        <div className="space-y-1.5">
          {items.map((x) => (
            <ListRow key={x.id} className="ring-1 ring-border/60" icon={Plug} tone="violet" title={x.name}
              sub={<span className="font-mono"><b className="mr-1.5 text-foreground/70">{x.method}</b>{x.url}</span>}
              trailing={<>
                {x.headerNames.length > 0 && <span className="hidden rounded-full bg-muted px-2 py-0.5 text-[0.65rem] text-muted-foreground sm:inline" data-tip={x.headerNames.join(", ")}>{x.headerNames.length} başlık</span>}
                <button type="button" data-tip="Dene" onClick={() => setTest(x)} className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground"><Play className="size-3.5" /></button>
                <button type="button" data-tip="Düzenle" onClick={() => setEdit(x)} className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-3.5" /></button>
                <button type="button" data-tip="Sil" onClick={() => setDel(x)} className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-3.5" /></button>
              </>}
            />
          ))}
        </div>
      )}
      {edit && <Form item={edit === "new" ? null : edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); void load(); }} />}
      {test && <TestDialog item={test} onClose={() => setTest(null)} />}
      <ConfirmDialog open={!!del} title="Bağlantı silinsin mi?" description={`${del?.name} silinir. Bunu kullanan chatbot adımları hata verir ve müşteri temsilciye aktarılır.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteIntegration(del.id).then(() => { setDel(null); void load(); })} />
    </Card>
  );
}

function Form({ item, onClose, onSaved }: { item: WAIntegration | null; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(item?.name ?? "");
  const [method, setMethod] = useState(item?.method ?? "GET");
  const [url, setUrl] = useState(item?.url ?? "https://");
  const [body, setBody] = useState(item?.body ?? "");
  const [timeoutSec, setTimeoutSec] = useState(item?.timeoutSec ?? 8);
  const [headers, setHeaders] = useState<{ k: string; v: string }[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const hs = headers.filter((h) => h.k.trim());
      await waApi.saveIntegration(item?.id ?? 0, { name, method, url, body, timeoutSec, headers: hs.length || !item ? Object.fromEntries(hs.map((h) => [h.k.trim(), h.v])) : undefined });
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={item ? "Bağlantıyı düzenle" : "Yeni bağlantı"} size="lg" footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !name.trim() || !url.startsWith("http")}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
    </>}>
      <div className="space-y-4">
        <FormField label="Adı"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} placeholder="Sipariş durumu" /></FormField>
        <div className="grid gap-2 sm:grid-cols-[7rem_1fr_6rem]">
          <FormField label="Yöntem"><select className={inputCls} value={method} onChange={(e) => setMethod(e.target.value)}>{["GET", "POST", "PUT", "PATCH"].map((m) => <option key={m}>{m}</option>)}</select></FormField>
          <FormField label="Adres"><input className={cn(inputCls, "font-mono")} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://api.site.com/siparis/{siparis}" /></FormField>
          <FormField label="Süre (sn)"><input type="number" min={1} max={30} className={inputCls} value={timeoutSec} onChange={(e) => setTimeoutSec(Number(e.target.value) || 8)} /></FormField>
        </div>
        {method !== "GET" && <FormField label="Gönderilecek JSON"><textarea className={cn(areaCls, "font-mono")} rows={4} value={body} onChange={(e) => setBody(e.target.value)} placeholder={'{"telefon": "{numara}"}'} /></FormField>}
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">Başlıklar (anahtar, yetki bilgisi)</p>
          {item && item.headerNames.length > 0 && headers.length === 0 && <p className="text-[0.7rem] text-muted-foreground">Kayıtlı: {item.headerNames.join(", ")}. Değiştirmek için hepsini yeniden girin; boş bırakırsanız olduğu gibi kalır.</p>}
          {headers.map((h, i) => (
            <div key={i} className="flex gap-2">
              <input className={cn(inputCls, "h-9 w-48 font-mono")} value={h.k} onChange={(e) => setHeaders((c) => c.map((x, j) => (j === i ? { ...x, k: e.target.value } : x)))} placeholder="Authorization" />
              <input type="password" autoComplete="off" className={cn(inputCls, "h-9 flex-1 font-mono")} value={h.v} onChange={(e) => setHeaders((c) => c.map((x, j) => (j === i ? { ...x, v: e.target.value } : x)))} placeholder="Bearer ..." />
              <button type="button" onClick={() => setHeaders((c) => c.filter((_, j) => j !== i))} className="rounded-lg p-1.5 text-muted-foreground hover:text-destructive" aria-label="Kaldır"><X className="size-4" /></button>
            </div>
          ))}
          <button type="button" onClick={() => setHeaders((c) => [...c, { k: "", v: "" }])} className="flex h-8 items-center gap-1 rounded-full px-3 text-xs font-semibold text-primary ring-1 ring-primary/30 hover:bg-primary/10"><Plus className="size-3.5" /> Başlık ekle</button>
        </div>
      </div>
    </Modal>
  );
}

function TestDialog({ item, onClose }: { item: WAIntegration; onClose: () => void }) {
  const names = [...new Set([...(item.url + " " + item.body).matchAll(/\{(\w+)\}/g)].map((m) => m[1]))];
  const [vars, setVars] = useState<Record<string, string>>({});
  const [out, setOut] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async () => {
    setBusy(true);
    setOut(null);
    setError(null);
    try {
      setOut(JSON.stringify(await waApi.testIntegration(item.id, vars), null, 2));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Olmadı.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={`${item.name} · dene`} description="Sorgu gerçekten gönderilir. Gelen cevaptaki alanları chatbot'ta kullanabilirsiniz." size="lg" footer={<><Button variant="secondary" onClick={onClose}>Kapat</Button><Button onClick={() => void run()} disabled={busy}><Play /> {busy ? "Soruluyor..." : "Gönder"}</Button></>}>
      <div className="space-y-3">
        {names.length > 0 && (
          <div className="grid gap-2 sm:grid-cols-2">
            {names.map((n) => <FormField key={n} label={n}><input className={inputCls} value={vars[n] ?? ""} onChange={(e) => setVars((c) => ({ ...c, [n]: e.target.value }))} /></FormField>)}
          </div>
        )}
        {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        {out && <pre className="max-h-80 overflow-auto rounded-xl bg-muted/50 p-3 font-mono text-xs">{out}</pre>}
      </div>
    </Modal>
  );
}
