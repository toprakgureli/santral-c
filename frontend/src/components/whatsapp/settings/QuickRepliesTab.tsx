// QuickRepliesTab keeps the ready answers people pull up with "/" while
// writing. Each answer is on the numbers it is chosen for.

import { useEffect, useMemo, useState } from "react";
import { Copy, Pencil, Plus, Search, Trash2, Zap } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { Toolbar } from "@/components/ui/rows";
import { areaCls, DeviceChips, DevicePicker, FormField, inputCls } from "@/components/whatsapp/settings/parts";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAQuickReply } from "@/whatsapp/types";

export default function QuickRepliesTab({ channels }: { channels: WAChannel[] }) {
  const [items, setItems] = useState<WAQuickReply[]>([]);
  const [search, setSearch] = useState("");
  const [device, setDevice] = useState(0);
  const [edit, setEdit] = useState<WAQuickReply | "new" | null>(null);
  const [del, setDel] = useState<WAQuickReply | null>(null);
  const [copy, setCopy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const load = () => waApi.quickReplies().then(setItems).catch(() => setItems([]));
  useEffect(() => { void load(); }, []);

  const shown = useMemo(() => {
    const t = search.trim().toLocaleLowerCase("tr");
    return items
      .filter((x) => (!device || x.channelIds.includes(device)) && (!t || (x.shortcut + " " + x.title + " " + x.body).toLocaleLowerCase("tr").includes(t)))
      .sort((a, b) => a.shortcut.localeCompare(b.shortcut, "tr"));
  }, [items, search, device]);

  return (
    <Card title="Hazır yanıtlar" icon={Zap} actions={
      <span className="flex items-center gap-2">
        {channels.length > 1 && <Button variant="secondary" onClick={() => setCopy(true)}><Copy /> Cihaza kopyala</Button>}
        <Button onClick={() => setEdit("new")}><Plus /> Yeni yanıt</Button>
      </span>
    }>
      <p className="mb-4 text-sm text-muted-foreground">Mesaj yazarken <b>/</b> tuşuna basıp kısayolu yazınca hazır yanıt gelir. <code className="rounded bg-muted px-1 font-mono text-xs">{"{musteri}"}</code> müşterinin adıyla, <code className="rounded bg-muted px-1 font-mono text-xs">{"{ad}"}</code> sizin adınızla değişir.</p>
      <Toolbar className="mb-3">
        <span className="relative min-w-48 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Ara" className="h-9 w-full rounded-xl border border-border/60 bg-card pr-3 pl-9 text-sm outline-none focus:border-ring/50" />
        </span>
        {channels.length > 1 && (
          <select value={device} onChange={(e) => setDevice(Number(e.target.value))} className="h-9 rounded-xl border border-border/60 bg-card px-3 text-sm">
            <option value={0}>Tüm cihazlar</option>
            {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        )}
        {msg && <span className="text-xs text-muted-foreground">{msg}</span>}
      </Toolbar>
      {shown.length === 0 ? (
        <EmptyState icon={<Zap />} title={items.length ? "Aramaya uyan yanıt yok" : "Henüz hazır yanıt yok"} description={items.length ? undefined : "Sık yazdığınız cevapları buraya ekleyin, ekip aynı cümleyi her seferinde yazmasın."} />
      ) : (
        <div className="divide-y divide-border/50 overflow-hidden rounded-2xl ring-1 ring-border/60">
          {shown.map((q) => (
            <div key={q.id} className="group flex items-start gap-3 px-4 py-3 hover:bg-accent/30">
              <code className="mt-0.5 shrink-0 rounded-lg bg-primary/10 px-2 py-0.5 font-mono text-xs font-semibold text-primary">/{q.shortcut}</code>
              <div className="min-w-0 flex-1 space-y-1">
                {q.title && <p className="text-sm font-medium">{q.title}</p>}
                <p className="line-clamp-2 text-sm whitespace-pre-wrap text-muted-foreground">{waText(q.body)}</p>
                {channels.length > 1 && <DeviceChips channels={channels} ids={q.channelIds} />}
              </div>
              <span className="flex shrink-0 gap-1 opacity-60 transition-opacity group-hover:opacity-100">
                <button type="button" data-tip="Düzenle" onClick={() => setEdit(q)} className="flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-3.5" /></button>
                <button type="button" data-tip="Sil" onClick={() => setDel(q)} className="flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-3.5" /></button>
              </span>
            </div>
          ))}
        </div>
      )}
      {edit && <Form item={edit === "new" ? null : edit} channels={channels} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); void load(); }} />}
      {copy && <CopyDialog channels={channels} onClose={() => setCopy(false)} onDone={(n) => { setCopy(false); setMsg(`${n} yanıt kopyalandı.`); void load(); }} />}
      <ConfirmDialog open={!!del} title="Hazır yanıt silinsin mi?" description={`/${del?.shortcut} bütün cihazlardan kalkar.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteQuickReply(del.id).then(() => { setDel(null); void load(); }).catch((e) => { setMsg(e instanceof ApiError ? e.message : "Silinemedi."); setDel(null); })} />
    </Card>
  );
}

function Form({ item, channels, onClose, onSaved }: { item: WAQuickReply | null; channels: WAChannel[]; onClose: () => void; onSaved: () => void }) {
  const [shortcut, setShortcut] = useState(item?.shortcut ?? "");
  const [title, setTitle] = useState(item?.title ?? "");
  const [body, setBody] = useState(item?.body ?? "");
  const [ids, setIds] = useState<number[]>(item?.channelIds ?? channels.map((c) => c.id));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await waApi.saveQuickReply(item?.id ?? 0, { shortcut, title, body, channelIds: ids });
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={item ? "Hazır yanıtı düzenle" : "Yeni hazır yanıt"} footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !shortcut.trim() || !body.trim()}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
    </>}>
      <div className="space-y-4">
        <div className="grid grid-cols-[8rem_1fr] gap-3">
          <FormField label="Kısayol">
            <span className="relative block">
              <span className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 font-mono text-sm text-muted-foreground">/</span>
              <input className={cn(inputCls, "pl-6 font-mono")} value={shortcut} onChange={(e) => setShortcut(e.target.value.toLocaleLowerCase("tr").replace(/\s+/g, "-"))} placeholder="kargo" />
            </span>
          </FormField>
          <FormField label="Başlık (isteğe bağlı)"><input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Kargo takip bilgisi" /></FormField>
        </div>
        <FormField label="Metin"><textarea className={areaCls} rows={5} value={body} onChange={(e) => setBody(e.target.value)} placeholder="Merhaba {musteri}, kargonuzu şu linkten takip edebilirsiniz: ..." /></FormField>
        {channels.length > 1 && (
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Hangi cihazlarda kullanılsın</p>
            <DevicePicker channels={channels} value={ids} onChange={setIds} />
          </div>
        )}
      </div>
    </Modal>
  );
}

function CopyDialog({ channels, onClose, onDone }: { channels: WAChannel[]; onClose: () => void; onDone: (n: number) => void }) {
  const [from, setFrom] = useState(channels[0]?.id ?? 0);
  const [to, setTo] = useState(channels[1]?.id ?? 0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const go = async () => {
    setBusy(true);
    try {
      const r = await waApi.copyToChannel(from, to, "quick_replies");
      onDone(r.copied);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kopyalanamadı.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title="Hazır yanıtları başka cihaza kopyala" description="Kaynaktaki yanıtlar hedef cihazda da kullanılabilir olur. Hedefte zaten aynı kısayol varsa dokunulmaz." footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void go()} disabled={busy || from === to}>{busy ? "Kopyalanıyor..." : "Kopyala"}</Button>
    </>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <FormField label="Nereden"><select className={inputCls} value={from} onChange={(e) => setFrom(Number(e.target.value))}>{channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select></FormField>
        <FormField label="Nereye"><select className={inputCls} value={to} onChange={(e) => setTo(Number(e.target.value))}>{channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select></FormField>
      </div>
    </Modal>
  );
}
