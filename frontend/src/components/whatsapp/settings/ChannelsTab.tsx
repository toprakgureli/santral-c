// ChannelsTab adds and manages WhatsApp numbers (devices): the Meta
// details, the connection test, the webhook address to give Meta, and who
// works on each number.

import { useEffect, useState } from "react";
import { Activity, CheckCircle2, CircleAlert, Pencil, Plus, Smartphone, Trash2, Users } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Card, ConfirmDialog, EmptyState, Modal } from "@/components/ui";
import { ListRow } from "@/components/ui/rows";
import { CopyField, FormField, inputCls, PeoplePicker } from "@/components/whatsapp/settings/parts";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAgent, WAChannel, WAChannelCheck } from "@/whatsapp/types";
import { since } from "@/whatsapp/util";

const QUALITY: Record<string, { label: string; tone: string }> = {
  GREEN: { label: "Kalite yüksek", tone: "bg-success/12 text-success" },
  YELLOW: { label: "Kalite orta", tone: "bg-warning/12 text-warning" },
  RED: { label: "Kalite düşük", tone: "bg-destructive/10 text-destructive" },
};

export default function ChannelsTab({ channels, reload }: { channels: WAChannel[]; reload: () => void }) {
  const { user } = useAuth();
  const manage = can(user, "whatsapp.channel_manage");
  const members = can(user, "whatsapp.team_manage");
  const [edit, setEdit] = useState<WAChannel | "new" | null>(null);
  const [setup, setSetup] = useState<WAChannel | null>(null);
  const [people, setPeople] = useState<WAChannel | null>(null);
  const [del, setDel] = useState<WAChannel | null>(null);
  const [check, setCheck] = useState<Record<number, WAChannelCheck | "busy">>({});
  const [error, setError] = useState<string | null>(null);
  const now = Date.now();

  const test = async (c: WAChannel) => {
    setCheck((cur) => ({ ...cur, [c.id]: "busy" }));
    try {
      const r = await waApi.testChannel(c.id);
      setCheck((cur) => ({ ...cur, [c.id]: r }));
      reload();
    } catch (e) {
      setCheck((cur) => ({ ...cur, [c.id]: { ok: false, message: e instanceof ApiError ? e.message : "Test yapılamadı.", subscribed: false, webhookSeen: false } }));
    }
  };

  return (
    <Card title="Cihazlar" icon={Smartphone} actions={manage && <Button onClick={() => setEdit("new")}><Plus className="size-4" /> Numara ekle</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Her WhatsApp numarası ayrı bir cihazdır. Yeni eklenen cihazın ayarları sıfırdan başlar; başka bir cihazın chatbot'u, hazır yanıtı ya da kuralı ona kendiliğinden geçmez.</p>
      {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
      {channels.length === 0 ? (
        <EmptyState icon={<Smartphone />} title="Henüz numara yok" description={manage ? "Meta'dan aldığınız numara kimliği ve erişim anahtarıyla ilk numarayı ekleyin." : "Numara eklemek için yetkiniz yok."} />
      ) : (
        <div className="space-y-2">
          {channels.map((c) => {
            const q = QUALITY[c.qualityRating];
            const ck = check[c.id];
            return (
              <div key={c.id} className="rounded-2xl ring-1 ring-border/60">
                <ListRow
                  icon={Smartphone}
                  tone={c.active ? "success" : "muted"}
                  title={<span className="flex items-center gap-2">{c.name}{!c.active && <span className="rounded-full bg-muted px-2 py-0.5 text-[0.6rem] font-semibold text-muted-foreground">Kapalı</span>}</span>}
                  sub={
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      <span className="font-mono tabular-nums">{c.displayPhone || "numara bilgisi bekleniyor"}</span>
                      {c.verifiedName && <span>· {c.verifiedName}</span>}
                      {q && <span className={cn("rounded-full px-2 py-px text-[0.6rem] font-semibold", q.tone)}>{q.label}</span>}
                      {c.messagingLimit && <span className="text-[0.65rem]">· sınır {c.messagingLimit.replace("TIER_", "").replace("K", " bin").toLowerCase()}</span>}
                    </span>
                  }
                  trailing={
                    <>
                      <span className={cn("hidden items-center gap-1 text-[0.7rem] md:flex", c.lastWebhookAt ? "text-muted-foreground" : "text-warning")} data-tip="Meta'dan en son ne zaman bildirim geldi">
                        <Activity className="size-3.5" /> {c.lastWebhookAt ? `${since(c.lastWebhookAt, now)} önce` : "Bildirim gelmedi"}
                      </span>
                      {manage && <Chip onClick={() => setSetup(c)} tip="Webhook adresi ve kurulum adımları">Kurulum</Chip>}
                      {manage && <Chip onClick={() => void test(c)} tip="Meta'ya bağlanıp bilgileri kontrol et">{ck === "busy" ? "Deneniyor..." : "Bağlantıyı test et"}</Chip>}
                      {members && <IconChipBtn tip="Bu numarada kim çalışır" onClick={() => setPeople(c)}><Users className="size-3.5" /><span className="text-[0.65rem] tabular-nums">{c.memberIds.length}</span></IconChipBtn>}
                      {manage && <IconChipBtn tip="Düzenle" onClick={() => setEdit(c)}><Pencil className="size-3.5" /></IconChipBtn>}
                      {manage && <IconChipBtn tip="Sil" danger onClick={() => setDel(c)}><Trash2 className="size-3.5" /></IconChipBtn>}
                    </>
                  }
                />
                {manage && (
                  <div className="flex gap-2 px-2.5 pb-2.5 sm:hidden">
                    <Chip mobile onClick={() => setSetup(c)} tip="Webhook adresi ve kurulum adımları">Kurulum</Chip>
                    <Chip mobile onClick={() => void test(c)} tip="Meta'ya bağlanıp bilgileri kontrol et">{ck === "busy" ? "Deneniyor..." : "Bağlantıyı test et"}</Chip>
                  </div>
                )}
                {ck && ck !== "busy" && (
                  <div className={cn("mx-2.5 mb-2.5 flex items-start gap-2 rounded-xl px-3 py-2 text-xs", ck.ok ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive")}>
                    {ck.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <CircleAlert className="mt-0.5 size-4 shrink-0" />}
                    <span className="min-w-0 flex-1 leading-relaxed">{ck.message}</span>
                    {ck.ok && !ck.subscribed && <Button className="h-7 px-2.5 text-xs" onClick={() => void waApi.subscribe(c.id).then(() => test(c)).catch((e) => setError(e instanceof ApiError ? e.message : "Olmadı."))}>Mesajları almaya başla</Button>}
                  </div>
                )}
                {c.lastError && (
                  <p className="mx-2.5 mb-2.5 rounded-xl bg-warning/10 px-3 py-2 text-xs text-warning">Son sorun{c.lastErrorAt ? ` (${since(c.lastErrorAt, now)} önce)` : ""}: {c.lastError}</p>
                )}
              </div>
            );
          })}
        </div>
      )}
      {edit && <ChannelForm channel={edit === "new" ? null : edit} onClose={() => setEdit(null)} onSaved={(c) => { setEdit(null); reload(); if (edit === "new") setSetup(c); }} />}
      {setup && <SetupDialog channel={setup} onClose={() => setSetup(null)} />}
      {people && <MembersDialog channel={people} onClose={() => setPeople(null)} onSaved={() => { setPeople(null); reload(); }} />}
      <ConfirmDialog open={!!del} title="Numara silinsin mi?" description={`${del?.name} ve üzerindeki bütün sohbetler, mesajlar ve kayıtlar silinir. Bu geri alınamaz.`} confirmLabel="Sil" onCancel={() => setDel(null)}
        onConfirm={() => del && void waApi.deleteChannel(del.id).then(() => { setDel(null); reload(); }).catch((e) => setError(e instanceof ApiError ? e.message : "Silinemedi."))} />
    </Card>
  );
}

function Chip({ onClick, tip, mobile, children }: { onClick: () => void; tip: string; mobile?: boolean; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} data-tip={tip} className={cn("h-8 items-center rounded-full px-3 text-xs font-medium text-muted-foreground ring-1 ring-border/60 transition-colors hover:bg-accent hover:text-foreground", mobile ? "flex" : "hidden sm:flex")}>{children}</button>;
}

function IconChipBtn({ tip, onClick, danger, children }: { tip: string; onClick: () => void; danger?: boolean; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className={cn("flex h-8 min-w-8 items-center justify-center gap-1 rounded-xl bg-muted/70 px-2 text-muted-foreground transition-colors", danger ? "hover:bg-destructive/10 hover:text-destructive" : "hover:bg-accent hover:text-foreground")}>{children}</button>;
}

function ChannelForm({ channel, onClose, onSaved }: { channel: WAChannel | null; onClose: () => void; onSaved: (c: WAChannel) => void }) {
  const [f, setF] = useState({
    name: channel?.name ?? "",
    displayPhone: channel?.displayPhone ?? "",
    phoneNumberId: channel?.phoneNumberId ?? "",
    wabaId: channel?.wabaId ?? "",
    appId: channel?.appId ?? "",
    graphVersion: channel?.graphVersion ?? "",
    accessToken: "",
    appSecret: "",
    active: channel?.active ?? true,
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const set = (k: keyof typeof f, v: string | boolean) => setF((cur) => ({ ...cur, [k]: v }));

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const c = channel ? await waApi.updateChannel(channel.id, f) : await waApi.createChannel(f);
      onSaved(c);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={channel ? `${channel.name} numarasını düzenle` : "WhatsApp numarası ekle"} description="Bu bilgiler Meta Business panelindeki WhatsApp > API Kurulumu sayfasında yazar. Anahtarlar şifreli saklanır ve bir daha gösterilmez." size="lg"
      footer={<>
        {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
        <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
        <Button onClick={() => void save()} disabled={busy}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
      </>}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField label="Panelde görünecek ad" hint="Örn. Destek Hattı, Satış Hattı"><input className={inputCls} value={f.name} onChange={(e) => set("name", e.target.value)} /></FormField>
        <FormField label="Numara (isteğe bağlı)" hint="Boş bırakırsanız Meta'dan okunur."><input className={inputCls} value={f.displayPhone} onChange={(e) => set("displayPhone", e.target.value)} placeholder="+90 850 ..." /></FormField>
        <FormField label="Numara kimliği (Phone number ID)"><input className={cn(inputCls, "font-mono")} value={f.phoneNumberId} onChange={(e) => set("phoneNumberId", e.target.value)} /></FormField>
        <FormField label="İşletme hesabı kimliği (WABA ID)"><input className={cn(inputCls, "font-mono")} value={f.wabaId} onChange={(e) => set("wabaId", e.target.value)} /></FormField>
        <FormField label="Uygulama kimliği (App ID)" hint="Görselli şablon oluşturmak için gerekir."><input className={cn(inputCls, "font-mono")} value={f.appId} onChange={(e) => set("appId", e.target.value)} /></FormField>
        <FormField label="Graph API sürümü" hint="Boş bırakılırsa güncel varsayılan kullanılır."><input className={cn(inputCls, "font-mono")} value={f.graphVersion} onChange={(e) => set("graphVersion", e.target.value)} placeholder="v23.0" /></FormField>
        <FormField label="Kalıcı erişim anahtarı (System user token)" hint={channel?.hasToken ? "Kayıtlı. Değiştirmek için yenisini yapıştırın." : "Meta Business ayarlarında sistem kullanıcısı için oluşturulan kalıcı anahtar."}>
          <input className={cn(inputCls, "font-mono")} type="password" autoComplete="off" value={f.accessToken} onChange={(e) => set("accessToken", e.target.value)} placeholder={channel?.hasToken ? "••••••••" : ""} />
        </FormField>
        <FormField label="Uygulama gizli anahtarı (App secret)" hint={channel?.hasAppSecret ? "Kayıtlı. Değiştirmek için yenisini yapıştırın." : "Meta'dan gelen bildirimlerin gerçekten Meta'dan geldiğini doğrulamak için."}>
          <input className={cn(inputCls, "font-mono")} type="password" autoComplete="off" value={f.appSecret} onChange={(e) => set("appSecret", e.target.value)} placeholder={channel?.hasAppSecret ? "••••••••" : ""} />
        </FormField>
        {channel && (
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input type="checkbox" checked={f.active} onChange={(e) => set("active", e.target.checked)} className="size-4 accent-primary" />
            Numara açık (kapatılırsa mesaj gönderilmez, gelenler yine kaydedilir)
          </label>
        )}
      </div>
    </Modal>
  );
}

function SetupDialog({ channel, onClose }: { channel: WAChannel; onClose: () => void }) {
  const origin = window.location.origin;
  return (
    <Modal open onClose={onClose} title={`${channel.name} · kurulum`} description="Meta'nın mesajları bize bildirmesi için bu adımları bir kez yapmanız yeterli." size="lg" footer={<Button onClick={onClose}>Tamam</Button>}>
      <ol className="space-y-4 text-sm">
        <Step n={1} title="Webhook adresini Meta'ya girin">
          <p className="text-muted-foreground">Meta uygulama panelinde WhatsApp &gt; Yapılandırma &gt; Webhook bölümünde "Düzenle"ye basın ve aşağıdaki iki değeri yapıştırın.</p>
          <div className="mt-2 space-y-2">
            <CopyField label="Geri çağırma adresi (Callback URL)" value={origin + (channel.hookPath ?? "")} />
            <CopyField label="Doğrulama anahtarı (Verify token)" value={channel.verifyToken ?? ""} secret />
          </div>
        </Step>
        <Step n={2} title="Hangi bildirimlerin geleceğini seçin">
          <p className="text-muted-foreground">Aynı sayfadaki listeden şunlara abone olun: <b>messages</b>, <b>message_template_status_update</b>, <b>message_template_quality_update</b>, <b>phone_number_quality_update</b>, <b>account_update</b>.</p>
        </Step>
        <Step n={3} title="Mesajları almaya başlayın">
          <p className="text-muted-foreground">Cihaz listesinde "Bağlantıyı test et"e basın. Meta henüz bu hesabın mesajlarını göndermiyorsa orada "Mesajları almaya başla" düğmesi çıkar.</p>
        </Step>
        <Step n={4} title="Kimin bakacağını seçin">
          <p className="text-muted-foreground">Listede kişi simgesine basıp bu numarada çalışacak kişileri ekleyin. Ayarlar &gt; Cihaz ayarları'ndan karşılama mesajı, mesai saatleri ve dağıtımı ayarlayın.</p>
        </Step>
      </ol>
    </Modal>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground">{n}</span>
      <div className="min-w-0 flex-1 pt-0.5">
        <p className="mb-1 font-semibold">{title}</p>
        {children}
      </div>
    </li>
  );
}

function MembersDialog({ channel, onClose, onSaved }: { channel: WAChannel; onClose: () => void; onSaved: () => void }) {
  const [people, setPeople] = useState<WAAgent[]>([]);
  const [ids, setIds] = useState<number[]>(channel.memberIds);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    waApi.agents().then(setPeople).catch(() => setPeople([]));
  }, []);
  const save = async () => {
    setBusy(true);
    try {
      await waApi.setMembers(channel.id, ids);
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={`${channel.name} · kim çalışır`} description="Seçilen kişiler bu numaranın sohbetlerini görür ve otomatik dağıtıma girer. Tümünü görme yetkisi olanlar her numarayı zaten görür." size="lg"
      footer={<>
        {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
        <span className="mr-auto text-xs text-muted-foreground">{ids.length} kişi seçili</span>
        <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
        <Button onClick={() => void save()} disabled={busy}>{busy ? "Kaydediliyor..." : "Kaydet"}</Button>
      </>}
    >
      <PeoplePicker people={people} value={ids} onChange={setIds} />
    </Modal>
  );
}
