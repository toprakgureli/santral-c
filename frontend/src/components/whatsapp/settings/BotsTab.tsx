// BotsTab lists the chatbots. Each bot is chosen for the numbers it should
// answer on; a new number starts with none. A bot can be copied to start
// another from it.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Bot, Copy, Moon, Pencil, Plus, Sparkles, Tag, Trash2, Workflow } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { DeviceChips, DevicePicker, FormField, inputCls, Switch, Words } from "@/components/whatsapp/settings/parts";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WABot, WAChannel } from "@/whatsapp/types";
import { since } from "@/whatsapp/util";

export const BOT_TRIGGER: Record<string, { label: string; sub: string; icon: typeof Bot }> = {
  entry: { label: "Her yeni sohbette", sub: "Müşteri yazınca önce bu chatbot karşılar.", icon: Sparkles },
  after_hours: { label: "Sadece mesai dışında", sub: "Mesai saatleri dışında gelen sohbetleri karşılar.", icon: Moon },
  keyword: { label: "Belirli kelimelerle", sub: "Müşteri şu kelimelerden birini yazınca başlar.", icon: Tag },
};

export default function BotsTab({ channels }: { channels: WAChannel[] }) {
  const { user } = useAuth();
  const navigate = useNavigate();
  const manage = can(user, "whatsapp.bot_manage");
  const publish = can(user, "whatsapp.bot_publish");
  const [bots, setBots] = useState<WABot[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [settings, setSettings] = useState<WABot | null>(null);
  const [copying, setCopying] = useState<WABot | null>(null);
  const [del, setDel] = useState<WABot | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const now = Date.now();
  const load = () => waApi.bots().then(setBots).catch(() => setBots([])).finally(() => setLoading(false));
  useEffect(() => { void load(); }, []);

  const toggle = async (b: WABot) => {
    setMsg(null);
    try {
      await waApi.updateBot(b.id, { name: b.name, description: b.description, trigger: b.trigger, keywords: b.keywords, channelIds: b.channelIds, active: !b.active });
      void load();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Değiştirilemedi.");
    }
  };

  return (
    <Card title="Chatbot'lar" icon={Bot} actions={manage && <Button onClick={() => setCreating(true)}><Plus /> Yeni chatbot</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Chatbot müşteriyi karşılar, menüden seçtirir, bilgi toplar ve gerektiğinde bir temsilciye aktarır. Her chatbot sadece seçtiğiniz cihazlarda çalışır. Bir cihazda aynı anda tek bir "her yeni sohbette" ve tek bir "mesai dışında" chatbot'u açık olabilir.</p>
      {msg && <p className="mb-3 rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{msg}</p>}
      {loading ? (
        <div className="grid gap-3 md:grid-cols-2">{[0, 1].map((i) => <div key={i} className="h-40 animate-pulse rounded-2xl bg-muted/50" />)}</div>
      ) : bots.length === 0 ? (
        <EmptyState icon={<Bot />} title="Henüz chatbot yok" description={manage ? "İlk chatbot'u oluşturun. Hazır bir karşılama akışıyla başlar, üzerinde istediğiniz gibi değiştirirsiniz." : undefined} />
      ) : (
        <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-3">
          {bots.map((b) => {
            const T = BOT_TRIGGER[b.trigger] ?? BOT_TRIGGER.entry;
            const live = b.publishedVersion > 0;
            return (
              <div key={b.id} className={cn("flex flex-col gap-3 rounded-2xl p-4 ring-1 transition-colors", b.active ? "bg-card ring-border/60" : "bg-muted/25 ring-border/40")}>
                <div className="flex items-start gap-3">
                  <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-2xl", b.active ? "bg-gradient-to-br from-emerald-500/20 to-primary/20 text-emerald-600 dark:text-emerald-400" : "bg-muted text-muted-foreground")}><Bot className="size-5" /></span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold">{b.name}</p>
                    <p className="line-clamp-2 text-xs text-muted-foreground">{b.description || T.sub}</p>
                  </div>
                  {publish && <span data-tip={b.active ? "Çalışıyor. Kapatmak için tıklayın." : live ? "Kapalı. Açmak için tıklayın." : "Önce yayınlayın"}><Switch on={b.active} onChange={() => void toggle(b)} disabled={!live && !b.active} label="Açık" /></span>}
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-[0.7rem]">
                  <span className="flex items-center gap-1 rounded-full bg-sky-500/10 px-2 py-0.5 font-medium text-sky-700 dark:text-sky-400"><T.icon className="size-3" /> {T.label}</span>
                  {b.trigger === "keyword" && b.keywords.slice(0, 4).map((k) => <span key={k} className="rounded-full bg-muted px-2 py-0.5">{k}</span>)}
                  {live ? <span className="rounded-full bg-success/10 px-2 py-0.5 font-medium text-success">Sürüm {b.publishedVersion} yayında</span> : <span className="rounded-full bg-warning/12 px-2 py-0.5 font-medium text-warning">Henüz yayınlanmadı</span>}
                  {b.draftChanged && live && <span className="rounded-full bg-warning/12 px-2 py-0.5 font-medium text-warning" data-tip="Taslakta yayınlanmamış değişiklikler var">Taslakta değişiklik var</span>}
                </div>
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1"><DeviceChips channels={channels} ids={b.channelIds} /></span>
                  <span className="shrink-0 text-[0.68rem] text-muted-foreground">{since(b.updatedAt, now)} önce</span>
                </div>
                <div className="mt-auto flex items-center gap-1 border-t border-border/50 pt-3">
                  <Button className="h-8 px-3 text-xs" onClick={() => navigate(`/whatsapp/bots/${b.id}`)}><Workflow /> Akışı aç</Button>
                  {manage && <Button variant="ghost" className="h-8 px-2.5 text-xs" onClick={() => setSettings(b)}><Pencil /> Ayarlar</Button>}
                  {manage && <Button variant="ghost" className="h-8 px-2.5 text-xs" onClick={() => setCopying(b)}><Copy /> Kopyala</Button>}
                  {manage && <button type="button" data-tip="Sil" onClick={() => setDel(b)} className="ml-auto flex size-8 items-center justify-center rounded-xl text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-3.5" /></button>}
                </div>
              </div>
            );
          })}
        </div>
      )}
      {creating && <CreateDialog onClose={() => setCreating(false)} onCreated={(b) => navigate(`/whatsapp/bots/${b.id}`)} />}
      {settings && <BotSettingsDialog bot={settings} channels={channels} canPublish={publish} onClose={() => setSettings(null)} onSaved={() => { setSettings(null); void load(); }} />}
      {copying && <CopyDialog bot={copying} channels={channels} onClose={() => setCopying(null)} onDone={() => { setCopying(null); void load(); }} />}
      <ConfirmDialog open={!!del} title="Chatbot silinsin mi?" description={`${del?.name} ve bütün sürümleri silinir. Şu an bu chatbot'la konuşan müşteriler temsilciye aktarılır.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteBot(del.id).then(() => { setDel(null); void load(); }).catch((e) => { setMsg(e instanceof ApiError ? e.message : "Silinemedi."); setDel(null); })} />
    </Card>
  );
}

function TriggerChoice({ value, onChange }: { value: string; onChange: (v: WABot["trigger"]) => void }) {
  return (
    <div className="grid gap-1.5 sm:grid-cols-3">
      {Object.entries(BOT_TRIGGER).map(([k, t]) => (
        <button key={k} type="button" onClick={() => onChange(k as WABot["trigger"])} className={cn("flex flex-col gap-1 rounded-xl px-3 py-2.5 text-left ring-1 transition-colors", value === k ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
          <t.icon className={cn("size-4", value === k ? "text-primary" : "text-muted-foreground")} />
          <span className="text-sm font-medium">{t.label}</span>
          <span className="text-[0.68rem] leading-snug text-muted-foreground">{t.sub}</span>
        </button>
      ))}
    </div>
  );
}

function CreateDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (b: WABot) => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [trigger, setTrigger] = useState<WABot["trigger"]>("entry");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <Modal open onClose={onClose} title="Yeni chatbot" description="Hazır bir karşılama akışıyla başlar. Cihazları ve açılışı akışı bitirince seçersiniz." size="lg" footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button disabled={busy || !name.trim()} onClick={() => { setBusy(true); waApi.createBot({ name, description, trigger }).then(onCreated).catch((e) => setError(e instanceof ApiError ? e.message : "Oluşturulamadı.")).finally(() => setBusy(false)); }}>{busy ? "Oluşturuluyor..." : "Oluştur ve akışı aç"}</Button>
    </>}>
      <div className="space-y-4">
        <FormField label="Adı"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} placeholder="Destek karşılama" autoFocus /></FormField>
        <FormField label="Kısa açıklama (isteğe bağlı)"><input className={inputCls} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Müşteriyi karşılar, konuyu sorar, doğru ekibe aktarır" /></FormField>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Ne zaman başlasın</p>
          <TriggerChoice value={trigger} onChange={setTrigger} />
        </div>
      </div>
    </Modal>
  );
}

export function BotSettingsDialog({ bot, channels, canPublish, onClose, onSaved }: { bot: WABot; channels: WAChannel[]; canPublish: boolean; onClose: () => void; onSaved: (b: WABot) => void }) {
  const [name, setName] = useState(bot.name);
  const [description, setDescription] = useState(bot.description);
  const [trigger, setTrigger] = useState(bot.trigger);
  const [keywords, setKeywords] = useState(bot.keywords);
  const [ids, setIds] = useState(bot.channelIds);
  const [active, setActive] = useState(bot.active);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const b = await waApi.updateBot(bot.id, { name, description, trigger, keywords, channelIds: ids, active });
      onSaved(b);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={`${bot.name} · ayarlar`} size="lg" footer={<>
      {error && <span className="mr-auto max-w-md text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void save()} disabled={busy || !name.trim()}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
    </>}>
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <FormField label="Adı"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} /></FormField>
          <FormField label="Kısa açıklama"><input className={inputCls} value={description} onChange={(e) => setDescription(e.target.value)} /></FormField>
        </div>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Ne zaman başlasın</p>
          <TriggerChoice value={trigger} onChange={setTrigger} />
        </div>
        {trigger === "keyword" && <FormField label="Başlatan kelimeler" hint="Müşterinin mesajı bu kelimelerden birini içerirse başlar. Enter ile ekleyin."><Words values={keywords} onChange={setKeywords} placeholder="kampanya, sipariş" /></FormField>}
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Bu chatbot hangi cihazlarda kullanılsın</p>
          <DevicePicker channels={channels} value={ids} onChange={setIds} />
        </div>
        <label className={cn("flex items-center gap-2 text-sm", (!canPublish || bot.publishedVersion === 0) && "opacity-60")}>
          <input type="checkbox" className="size-4 accent-primary" checked={active} disabled={!canPublish || bot.publishedVersion === 0} onChange={(e) => setActive(e.target.checked)} />
          Açık {bot.publishedVersion === 0 ? "(önce akışı yayınlayın)" : !canPublish ? "(açıp kapatma yetkiniz yok)" : ""}
        </label>
      </div>
    </Modal>
  );
}

function CopyDialog({ bot, channels, onClose, onDone }: { bot: WABot; channels: WAChannel[]; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState(`${bot.name} (kopya)`);
  const [ids, setIds] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <Modal open onClose={onClose} title={`${bot.name} kopyalansın`} description="Akışın tamamı yeni bir chatbot'a kopyalanır. Kopya kapalı başlar; iki chatbot bundan sonra birbirinden bağımsızdır." footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button disabled={busy || !name.trim()} onClick={() => { setBusy(true); waApi.copyBot(bot.id, name, ids).then(onDone).catch((e) => setError(e instanceof ApiError ? e.message : "Kopyalanamadı.")).finally(() => setBusy(false)); }}>{busy ? "Kopyalanıyor..." : "Kopyala"}</Button>
    </>}>
      <div className="space-y-4">
        <FormField label="Yeni adı"><input className={inputCls} value={name} onChange={(e) => setName(e.target.value)} /></FormField>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Kopya hangi cihazlarda kullanılsın</p>
          <DevicePicker channels={channels} value={ids} onChange={setIds} />
        </div>
      </div>
    </Modal>
  );
}
