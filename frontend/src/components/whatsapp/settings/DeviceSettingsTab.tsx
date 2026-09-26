// DeviceSettingsTab edits one number's own settings: read receipts, the
// greeting, distribution, the waiting list, working hours, the survey and
// the chatbot words. Each block is locked unless the person has the
// permission for it; the server checks the same.

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { BellRing, CalendarClock, CheckCheck, ClipboardCheck, Copy, Hand, Hourglass, MessageSquareOff, Plus, Save, Shuffle, Trash2, type LucideIcon } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Modal } from "@/components/ui";
import { IconChip } from "@/components/ui/rows";
import { areaCls, CopyField, FormField, inputCls, Switch, SwitchRow, Words } from "@/components/whatsapp/settings/parts";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WASettings, WATemplate } from "@/whatsapp/types";
import { normalizeSettings } from "@/whatsapp/util";
import { TimeInput } from "@/components/whatsapp/settings/TimeParts";

const DAYS = ["Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi", "Pazar"];

const SECTIONS: { key: string; label: string }[] = [
  { key: "readReceipts", label: "Okundu bilgisi" },
  { key: "greeting", label: "Karşılama mesajı" },
  { key: "distribution", label: "Dağıtım" },
  { key: "waiting", label: "Cevap bekleyenler süresi" },
  { key: "hours", label: "Mesai saatleri ve tatiller" },
  { key: "survey", label: "Memnuniyet anketi" },
  { key: "bot", label: "Chatbot kelimeleri ve süresi" },
  { key: "optout", label: "Mesaj almak istemeyenler" },
];

export default function DeviceSettingsTab({ channels, reload }: { channels: WAChannel[]; reload: () => void }) {
  const { user } = useAuth();
  const [channelId, setChannelId] = useState<number>(channels[0]?.id ?? 0);
  const channel = channels.find((c) => c.id === channelId) ?? channels[0];
  const [s, setS] = useState<WASettings | null>(null);
  const [secret, setSecret] = useState("");
  const [templates, setTemplates] = useState<WATemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [copying, setCopying] = useState(false);

  useEffect(() => {
    if (!channel) return;
    setS(normalizeSettings(channel.settings));
    setSecret("");
    setMsg(null);
    waApi.templates(channel.id).then((t) => setTemplates(t.filter((x) => x.status === "APPROVED"))).catch(() => setTemplates([]));
  }, [channel]);

  const dirty = useMemo(() => !!s && !!channel && JSON.stringify(s) !== JSON.stringify(normalizeSettings(channel.settings)), [s, channel]);

  if (!channel || !s) return <p className="rounded-2xl bg-card p-8 text-center text-sm text-muted-foreground ring-1 ring-border/60">Önce Cihazlar sekmesinden bir numara ekleyin.</p>;

  const pRead = can(user, "whatsapp.setting_read_receipts");
  const pGreet = can(user, "whatsapp.setting_greeting");
  const pDist = can(user, "whatsapp.setting_distribution");
  const pGen = can(user, "whatsapp.setting_general");
  const up = (fn: (d: WASettings) => void) => setS((cur) => {
    if (!cur) return cur;
    const next = structuredClone(cur);
    fn(next);
    return next;
  });

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await waApi.saveSettings(channel.id, s, secret);
      setSecret("");
      setMsg({ ok: true, text: "Kaydedildi. Yeni ayarlar hemen geçerli." });
      reload();
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setBusy(false);
    }
  };

  const tplOptions = (
    <>
      <option value="">Şablon yok</option>
      {templates.map((t) => <option key={t.id} value={t.name + "|" + t.language}>{t.name} ({t.language})</option>)}
    </>
  );

  return (
    <div className="space-y-4">
      <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center gap-2 rounded-2xl bg-background/85 px-1 py-2 backdrop-blur">
        <div className="flex flex-wrap gap-1 rounded-2xl bg-muted/50 p-1">
          {channels.map((c) => (
            <button key={c.id} type="button" onClick={() => setChannelId(c.id)} className={cn("rounded-xl px-3 py-1.5 text-sm font-medium transition-colors", c.id === channel.id ? "bg-card text-foreground shadow-sm ring-1 ring-border/60" : "text-muted-foreground hover:text-foreground")}>{c.name}</button>
          ))}
        </div>
        <span className="ml-auto flex items-center gap-2">
          {msg && <span className={cn("text-xs", msg.ok ? "text-success" : "text-destructive")}>{msg.text}</span>}
          {channels.length > 1 && pGen && <Button variant="secondary" onClick={() => setCopying(true)}><Copy /> Başka cihazdan kopyala</Button>}
          <Button onClick={() => void save()} disabled={busy || (!dirty && !secret)}><Save /> {busy ? "Kaydediliyor..." : dirty || secret ? "Değişiklikleri kaydet" : "Kaydedildi"}</Button>
        </span>
      </div>

      <div className="grid items-start gap-4 xl:grid-cols-2">
        <Block icon={CheckCheck} title="Okundu bilgisi" locked={!pRead} sub="Biz sohbeti açınca müşteri mavi tik görsün mü?">
          <SwitchRow title="Mavi tik gönder" sub="Kapalıyken müşteri mesajını okuduğumuzu görmez, sadece iletildiğini görür." on={s.readReceipts} onChange={(v) => up((d) => { d.readReceipts = v; })} disabled={!pRead} />
        </Block>

        <Block icon={Hand} title="Karşılama mesajı" locked={!pGreet} sub={'Temsilci "Karşıla" düğmesine bastığında müşteriye giden kısa tanıtım.'}>
          <SwitchRow title="Karşılama mesajı gönderilsin" on={s.greeting.enabled} onChange={(v) => up((d) => { d.greeting.enabled = v; })} disabled={!pGreet} />
          <div className={cn("space-y-3", !s.greeting.enabled && "opacity-60")}>
            <FormField label="Mesaj" hint={<>Kullanabileceğiniz alanlar: <Var>{"{ad}"}</Var> temsilcinin adı, <Var>{"{adsoyad}"}</Var> adı soyadı, <Var>{"{unvan}"}</Var> profilindeki unvan ya da rolü, <Var>{"{musteri}"}</Var> müşterinin adı.</>}>
              <textarea className={areaCls} rows={3} value={s.greeting.text} onChange={(e) => up((d) => { d.greeting.text = e.target.value; })} disabled={!pGreet} />
            </FormField>
            <Preview text={s.greeting.text.replace("{ad}", user?.name?.split(" ")[0] ?? "Toprak").replace("{adsoyad}", user?.name ?? "").replace("{unvan}", "teknik destek uzmanınız").replace("{musteri}", "Ayşe")} />
            <FormField label="24 saat geçtiyse bu şablonla gönder" hint="Müşteri son 24 saatte yazmadıysa düz mesaj gönderilemez. Şablonun değişkenleri sırayla ad, unvan, müşteri olarak doldurulur.">
              <select className={inputCls} value={s.greeting.template ? s.greeting.template + "|" + s.greeting.templateLang : ""} onChange={(e) => up((d) => { const [n, l] = e.target.value.split("|"); d.greeting.template = n ?? ""; d.greeting.templateLang = l ?? ""; })} disabled={!pGreet}>{tplOptions}</select>
            </FormField>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" className="size-4 accent-primary" checked={s.greeting.forHelpers} onChange={(e) => up((d) => { d.greeting.forHelpers = e.target.checked; })} disabled={!pGreet} />
              Sohbete sonradan katılan kişi de kendini tanıtsın
            </label>
          </div>
        </Block>

        <Block icon={Shuffle} title="Dağıtım" locked={!pDist} sub="Yeni gelen sohbet kime düşsün?">
          <SwitchRow title="Sohbetleri otomatik dağıt" sub="Açıkken sohbet, şu an müsait olan ve paneli açık kişilere sırayla verilir; en uzun süredir sohbet almamış olan önce alır. Kapalıyken sohbetler havuzda bekler, isteyen alır." on={s.distribution.enabled} onChange={(v) => up((d) => { d.distribution.enabled = v; })} disabled={!pDist} />
          <FormField label="Bir kişide aynı anda en fazla kaç açık sohbet olsun" hint="Sınıra ulaşan kişiye yeni sohbet verilmez. 0 sınırsız demektir.">
            <input type="number" min={0} max={500} className={cn(inputCls, "w-32")} value={s.distribution.maxOpen} onChange={(e) => up((d) => { d.distribution.maxOpen = Math.max(0, Number(e.target.value) || 0); })} disabled={!pDist || !s.distribution.enabled} />
          </FormField>
        </Block>

        <Block icon={Hourglass} title="Cevap bekleyenler" locked={!pGen} sub="Uzun süre cevapsız kalan müşteri herkesin önüne düşer.">
          <FormField label="Kaç dakika cevapsız kalınca herkese gösterilsin" hint="Süre mesai saatleri içinde sayılır. Mesai dışında saat durur. 0 yazarsanız bu liste kapanır.">
            <div className="flex items-center gap-2">
              <input type="number" min={0} max={1440} className={cn(inputCls, "w-28")} value={s.waitingMinutes} onChange={(e) => up((d) => { d.waitingMinutes = Math.max(0, Number(e.target.value) || 0); })} disabled={!pGen} />
              <span className="text-sm text-muted-foreground">dakika</span>
            </div>
          </FormField>
        </Block>

        <Block icon={CalendarClock} title="Mesai saatleri" locked={!pGen} sub="Türkiye saatiyle. Mesai dışı mesajlar, bekleme süresi ve chatbot'lardaki mesai ayarları bunu kullanır." wide>
          <SwitchRow title="Mesai saatleri uygulansın" sub="Kapalıyken her an mesai içi sayılır." on={s.hours.enabled} onChange={(v) => up((d) => { d.hours.enabled = v; })} disabled={!pGen} />
          <div className={cn("grid gap-1.5 sm:grid-cols-2", !s.hours.enabled && "pointer-events-none opacity-50")}>
            {s.hours.days.map((day, i) => (
              <div key={i} className="flex items-center gap-2 rounded-xl bg-muted/30 px-3 py-2">
                <Switch on={day.open} onChange={(v) => up((d) => { d.hours.days[i].open = v; })} disabled={!pGen} label={DAYS[i]} />
                <span className="w-20 text-sm font-medium">{DAYS[i]}</span>
                {day.open ? (
                  <span className="ml-auto flex items-center gap-1.5">
                    <TimeInput className="h-8" label={`${DAYS[i]} açılış`} value={day.from} onChange={(v) => up((d) => { d.hours.days[i].from = v; })} disabled={!pGen} />
                    <span className="text-muted-foreground">–</span>
                    <TimeInput className="h-8" label={`${DAYS[i]} kapanış`} value={day.to} onChange={(v) => up((d) => { d.hours.days[i].to = v; })} disabled={!pGen} />
                  </span>
                ) : <span className="ml-auto text-xs text-muted-foreground">Kapalı</span>}
              </div>
            ))}
          </div>
          <Holidays values={s.hours.holidays} onChange={(v) => up((d) => { d.hours.holidays = v; })} disabled={!pGen || !s.hours.enabled} />
        </Block>

        <Block icon={ClipboardCheck} title="Memnuniyet anketi" locked={!pGen} sub="Sohbet çözülünce müşteriye sorulur.">
          <div className="flex flex-wrap gap-1 rounded-2xl bg-muted/50 p-1">
            {([["off", "Anket yok"], ["native", "WhatsApp içinde 1-5 puan"], ["tally", "Tally formu"]] as const).map(([k, l]) => (
              <button key={k} type="button" disabled={!pGen} onClick={() => up((d) => { d.survey.mode = k; })} className={cn("flex-1 rounded-xl px-3 py-1.5 text-xs font-medium transition-colors", s.survey.mode === k ? "bg-card text-foreground shadow-sm ring-1 ring-border/60" : "text-muted-foreground hover:text-foreground")}>{l}</button>
            ))}
          </div>
          {s.survey.mode !== "off" && (
            <div className="space-y-3">
              <FormField label="Müşteriye giden mesaj" hint={s.survey.mode === "tally" ? <>Link mesajın sonuna eklenir, başka bir yere koymak için <Var>{"{link}"}</Var> yazın. <Var>{"{musteri}"}</Var> ve <Var>{"{temsilci}"}</Var> kullanılabilir.</> : <>Altına 1'den 5'e puan listesi eklenir. <Var>{"{musteri}"}</Var> ve <Var>{"{temsilci}"}</Var> kullanılabilir.</>}>
                <textarea className={areaCls} rows={2} value={s.survey.text} onChange={(e) => up((d) => { d.survey.text = e.target.value; })} disabled={!pGen} placeholder="Görüşmemizi değerlendirir misiniz?" />
              </FormField>
              {s.survey.mode === "tally" && (
                <>
                  <FormField label="Tally form linki" hint="Formda şu gizli alanları (hidden fields) açın: ticket, number, agent, channel, token. Sistem bunları doldurur.">
                    <input className={inputCls} value={s.survey.url} onChange={(e) => up((d) => { d.survey.url = e.target.value; })} disabled={!pGen} placeholder="https://tally.so/r/..." />
                  </FormField>
                  {channel.surveyHookPath && <CopyField label="Tally > Integrations > Webhooks'a girilecek adres" value={window.location.origin + channel.surveyHookPath} />}
                  <FormField label="Tally imza anahtarı (Signing secret)" hint={channel.hasSurveySecret ? "Kayıtlı. Değiştirmek için yenisini yazın, silmek için tek bir - yazın." : "Webhook ayarında oluşturduğunuz anahtar. Cevapların gerçekten Tally'den geldiğini doğrular."}>
                    <input type="password" autoComplete="off" className={cn(inputCls, "font-mono")} value={secret} onChange={(e) => setSecret(e.target.value)} disabled={!pGen} placeholder={channel.hasSurveySecret ? "••••••••" : ""} />
                  </FormField>
                </>
              )}
              <FormField label="24 saat geçtiyse bu şablonla gönder">
                <select className={inputCls} value={s.survey.template ? s.survey.template + "|" + s.survey.templateLang : ""} onChange={(e) => up((d) => { const [n, l] = e.target.value.split("|"); d.survey.template = n ?? ""; d.survey.templateLang = l ?? ""; })} disabled={!pGen}>{tplOptions}</select>
              </FormField>
              <FormField label="Bu puanın altında yöneticilere haber ver" hint="0 yazarsanız haber verilmez.">
                <input type="number" min={0} max={5} className={cn(inputCls, "w-24")} value={s.survey.alertBelow} onChange={(e) => up((d) => { d.survey.alertBelow = Math.min(5, Math.max(0, Number(e.target.value) || 0)); })} disabled={!pGen} />
              </FormField>
            </div>
          )}
        </Block>

        <Block icon={BellRing} title="Chatbot" locked={!pGen} sub="Bu numaradaki bütün chatbot'lar için geçerli.">
          <FormField label="Müşteri bu kelimelerden birini yazarsa chatbot'u bırakıp temsilciye aktar" hint="Enter ile ekleyin.">
            <Words values={s.humanKeywords} onChange={(v) => up((d) => { d.humanKeywords = v; })} disabled={!pGen} placeholder="temsilci, insan, yetkili" />
          </FormField>
          <FormField label="Müşteri bu kadar dakika cevap vermezse chatbot sohbeti kapatsın">
            <div className="flex items-center gap-2">
              <input type="number" min={1} className={cn(inputCls, "w-28")} value={s.botTimeoutMinutes} onChange={(e) => up((d) => { d.botTimeoutMinutes = Math.max(1, Number(e.target.value) || 30); })} disabled={!pGen} />
              <span className="text-sm text-muted-foreground">dakika</span>
            </div>
          </FormField>
        </Block>

        <Block icon={MessageSquareOff} title="Mesaj almak istemeyenler" locked={!pGen} sub="Müşteri bu kelimelerden birini yazınca ona toplu ve otomatik mesaj gönderilmez.">
          <FormField label="Kelimeler">
            <Words values={s.optOutKeywords} onChange={(v) => up((d) => { d.optOutKeywords = v; })} disabled={!pGen} placeholder="DUR, iptal" />
          </FormField>
          <FormField label="Müşteriye verilecek cevap" hint="Boş bırakırsanız cevap verilmez.">
            <textarea className={areaCls} rows={2} value={s.optOutReply} onChange={(e) => up((d) => { d.optOutReply = e.target.value; })} disabled={!pGen} />
          </FormField>
        </Block>
      </div>

      {copying && <CopyDialog target={channel} channels={channels.filter((c) => c.id !== channel.id)} onClose={() => setCopying(false)} onDone={() => { setCopying(false); reload(); }} />}
    </div>
  );
}

function Block({ icon, title, sub, locked, wide, children }: { icon: LucideIcon; title: string; sub?: string; locked?: boolean; wide?: boolean; children: ReactNode }) {
  return (
    <section className={cn("space-y-3 rounded-2xl bg-card p-4 shadow-sm ring-1 ring-border/60", wide && "xl:col-span-2")}>
      <header className="flex items-start gap-3">
        <IconChip icon={icon} tone="primary" />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{title}</h3>
          {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
        </div>
        {locked && <span className="rounded-full bg-muted px-2 py-0.5 text-[0.65rem] font-medium text-muted-foreground" data-tip="Bu bölümü değiştirme yetkiniz yok">Sadece görüntüleme</span>}
      </header>
      {children}
    </section>
  );
}

function Var({ children }: { children: ReactNode }) {
  return <code className="rounded bg-muted px-1 py-px font-mono text-[0.68rem] text-foreground">{children}</code>;
}

function Preview({ text }: { text: string }) {
  return (
    <div className="rounded-2xl bg-[#e7f7e2] p-3 dark:bg-emerald-950/40">
      <p className="mb-1 text-[0.65rem] font-semibold tracking-wide text-emerald-800/70 uppercase dark:text-emerald-300/70">Müşterinin göreceği</p>
      <p className="max-w-[85%] rounded-xl rounded-tr-sm bg-[#d9fdd3] px-3 py-2 text-sm whitespace-pre-wrap text-slate-900 shadow-sm dark:bg-emerald-800/60 dark:text-emerald-50">{text || "…"}</p>
    </div>
  );
}

function Holidays({ values, onChange, disabled }: { values: string[]; onChange: (v: string[]) => void; disabled?: boolean }) {
  const [day, setDay] = useState("");
  const sorted = [...(values ?? [])].sort();
  const fmt = (d: string) => new Date(d + "T12:00:00").toLocaleDateString("tr-TR", { day: "numeric", month: "long", year: "numeric", weekday: "short" });
  return (
    <div className={cn("space-y-2", disabled && "opacity-60")}>
      <p className="text-xs font-medium text-muted-foreground">Tatil günleri (bu günler tüm gün kapalı sayılır)</p>
      <div className="flex flex-wrap items-center gap-1.5">
        {sorted.map((d) => (
          <span key={d} className="flex items-center gap-1 rounded-full bg-warning/12 px-2.5 py-1 text-xs font-medium text-warning">
            {fmt(d)}
            {!disabled && <button type="button" onClick={() => onChange(values.filter((x) => x !== d))} aria-label="Kaldır"><Trash2 className="size-3" /></button>}
          </span>
        ))}
        {!disabled && (
          <span className="flex items-center gap-1">
            <input type="date" value={day} onChange={(e) => setDay(e.target.value)} className="h-8 rounded-lg border border-border/60 bg-card px-2 text-sm" />
            <button type="button" disabled={!day} onClick={() => { if (day && !values.includes(day)) onChange([...values, day]); setDay(""); }} className="flex h-8 items-center gap-1 rounded-lg px-2 text-xs font-semibold text-primary hover:bg-primary/10 disabled:opacity-40"><Plus className="size-3.5" /> Ekle</button>
          </span>
        )}
        {sorted.length === 0 && disabled && <span className="text-xs text-muted-foreground">Tatil günü yok.</span>}
      </div>
    </div>
  );
}

function CopyDialog({ target, channels, onClose, onDone }: { target: WAChannel; channels: WAChannel[]; onClose: () => void; onDone: () => void }) {
  const [from, setFrom] = useState(channels[0]?.id ?? 0);
  const [sections, setSections] = useState<string[]>(SECTIONS.map((x) => x.key));
  const [extra, setExtra] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const go = async () => {
    setBusy(true);
    setError(null);
    try {
      if (sections.length) await waApi.copySettings(target.id, from, sections);
      for (const w of extra) await waApi.copyToChannel(from, target.id, w as "quick_replies" | "rules");
      onDone();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kopyalanamadı.");
    } finally {
      setBusy(false);
    }
  };
  const toggle = (list: string[], set: (v: string[]) => void, k: string) => set(list.includes(k) ? list.filter((x) => x !== k) : [...list, k]);
  return (
    <Modal open onClose={onClose} title={`${target.name} için ayarları kopyala`} description="Seçtiğiniz bölümler diğer cihazdakiyle aynı olur. Seçmedikleriniz değişmez." footer={<>
      {error && <span className="mr-auto text-xs text-destructive">{error}</span>}
      <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
      <Button onClick={() => void go()} disabled={busy || (!sections.length && !extra.length)}>{busy ? "Kopyalanıyor..." : "Kopyala"}</Button>
    </>}>
      <div className="space-y-4">
        <FormField label="Nereden">
          <select className={inputCls} value={from} onChange={(e) => setFrom(Number(e.target.value))}>
            {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </FormField>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Ayarlar</p>
          {SECTIONS.map((x) => (
            <label key={x.key} className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={sections.includes(x.key)} onChange={() => toggle(sections, setSections, x.key)} /> {x.label}</label>
          ))}
        </div>
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Ayrıca bu cihaza da ekle</p>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={extra.includes("quick_replies")} onChange={() => toggle(extra, setExtra, "quick_replies")} /> Hazır yanıtlar</label>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="size-4 accent-primary" checked={extra.includes("rules")} onChange={() => toggle(extra, setExtra, "rules")} /> Otomatik mesaj kuralları</label>
          <p className="text-[0.7rem] text-muted-foreground">Chatbot'ları Chatbot'lar sekmesinden cihaz seçerek ya da kopyalayarak paylaşabilirsiniz.</p>
        </div>
      </div>
    </Modal>
  );
}
