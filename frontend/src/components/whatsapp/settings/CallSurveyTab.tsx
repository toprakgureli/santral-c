// CallSurveyTab sets up the survey that goes out on WhatsApp after a phone
// call: which number sends it, which approved template, what each answer
// button is worth, and which calls get one.

import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, CircleAlert, PhoneCall, Save } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card } from "@/components/ui";
import { areaCls, CopyField, FormField, inputCls, SwitchRow } from "@/components/whatsapp/settings/parts";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WACallSurveySettings, WAChannel, WATemplate } from "@/whatsapp/types";

const VARS = [
  { key: "{musteri}", label: "müşterinin adı" },
  { key: "{temsilci}", label: "görüşen temsilci" },
  { key: "{tarih}", label: "görüşme tarihi" },
  { key: "{link}", label: "anket linki" },
];

function countVars(text?: string) {
  let n = 0;
  for (const m of (text ?? "").matchAll(/\{\{(\d+)\}\}/g)) n = Math.max(n, Number(m[1]));
  return n;
}

export default function CallSurveyTab({ channels }: { channels: WAChannel[] }) {
  const [s, setS] = useState<WACallSurveySettings | null>(null);
  const [templates, setTemplates] = useState<WATemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    waApi.callSurvey().then((v) => setS({ ...v, channelId: v.channelId || channels[0]?.id || 0 })).catch((e) => setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Ayarlar alınamadı." }));
  }, [channels]);
  useEffect(() => {
    if (!s?.channelId) return;
    waApi.templates(s.channelId).then((l) => setTemplates(l.filter((t) => t.status === "APPROVED"))).catch(() => setTemplates([]));
  }, [s?.channelId]);

  const tpl = useMemo(() => templates.find((t) => t.name === s?.template && t.language === s?.templateLang), [templates, s?.template, s?.templateLang]);
  const body = tpl?.components.find((c) => c.type === "BODY")?.text ?? "";
  const quick = (tpl?.components.find((c) => c.type === "BUTTONS")?.buttons ?? []).filter((b) => b.type === "QUICK_REPLY");
  const urlButton = (tpl?.components.find((c) => c.type === "BUTTONS")?.buttons ?? []).find((b) => b.type === "URL");
  const blanks = countVars(body);

  if (channels.length === 0) return <p className="rounded-2xl bg-card p-8 text-center text-sm text-muted-foreground ring-1 ring-border/60">Önce bir WhatsApp numarası ekleyin.</p>;
  if (!s) return <div className="h-64 animate-pulse rounded-2xl bg-muted/40" />;

  const channel = channels.find((c) => c.id === s.channelId);
  const up = (p: Partial<WACallSurveySettings>) => setS((cur) => (cur ? { ...cur, ...p } : cur));
  const setParam = (i: number, v: string) => up({ params: Array.from({ length: Math.max(blanks, s.params.length) }, (_, j) => (j === i ? v : s.params[j] ?? "")) });
  const preview = body.replace(/\{\{(\d+)\}\}/g, (all, k) => {
    const v = s.params[Number(k) - 1];
    if (!v) return all;
    return v.replace("{musteri}", "Ayşe").replace("{temsilci}", "Toprak").replace("{tarih}", new Date().toLocaleDateString("tr-TR")).replace("{link}", "https://tally.so/r/...");
  });

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const out = await waApi.saveCallSurvey({ ...s, params: s.params.slice(0, blanks), buttonScores: quick.length ? quick.map((_, i) => s.buttonScores[i] ?? 5) : s.buttonScores });
      setS(out);
      setMsg({ ok: true, text: out.enabled ? "Kaydedildi. Bundan sonra biten görüşmelere anket gidecek." : "Kaydedildi." });
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card title="Çağrı sonrası anket" icon={PhoneCall} actions={<Button onClick={() => void save()} disabled={busy}><Save /> {busy ? "Kaydediliyor..." : "Kaydet"}</Button>}>
      <p className="mb-4 text-sm text-muted-foreground">Telefon görüşmesi bittikten sonra müşteriye WhatsApp'tan kısa bir anket gider. Müşterinin cevabı o görüşmeye ve temsilciye puan olarak yazılır; temsilcilerin önüne yeni bir sohbet olarak düşmez. Sabit hatlara gitmez.</p>
      {msg && (
        <p className={cn("mb-4 flex items-start gap-2 rounded-xl px-3 py-2 text-sm", msg.ok ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive")}>
          {msg.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <CircleAlert className="mt-0.5 size-4 shrink-0" />} {msg.text}
        </p>
      )}
      <div className="grid gap-6 xl:grid-cols-[1fr_22rem]">
        <div className="space-y-5">
          <SwitchRow title="Görüşmeden sonra anket gönder" sub="Kapattığınız anda sıradaki anketler de gitmez." on={s.enabled} onChange={(v) => up({ enabled: v })} />
          <p className="rounded-xl bg-muted/40 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
            Anket, sadece rolünde <b className="text-foreground">"Bu kişinin telefon görüşmelerinden sonra müşteriye WhatsApp anketi gider"</b> yetkisi olan kişilerin görüşmelerinden sonra gider. Bir ekip ya da kişi için kapatmak isterseniz Roller sayfasından bu yetkiyi kaldırın.
          </p>

          <div className="grid gap-3 sm:grid-cols-2">
            <FormField label="Hangi numaradan gitsin">
              <select className={inputCls} value={s.channelId} onChange={(e) => up({ channelId: Number(e.target.value), template: "", templateLang: "" })}>
                {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
            </FormField>
            <FormField label="Şablon" hint={templates.length === 0 ? "Bu numarada onaylı şablon yok. Şablonlar sekmesinden oluşturun." : "Hizmet kategorisinde, cevap düğmeli bir şablon önerilir."}>
              <select className={inputCls} value={s.template ? `${s.template}|${s.templateLang}` : ""} onChange={(e) => { const [n, l] = e.target.value.split("|"); up({ template: n ?? "", templateLang: l ?? "" }); }}>
                <option value="">Şablon seçin</option>
                {templates.map((t) => <option key={t.id} value={`${t.name}|${t.language}`}>{t.name} ({t.language})</option>)}
              </select>
            </FormField>
          </div>

          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Müşteri nasıl cevap versin</p>
            <div className="grid gap-1.5 sm:grid-cols-2">
              <button type="button" onClick={() => up({ mode: "buttons" })} className={cn("rounded-xl px-3 py-2.5 text-left ring-1", s.mode === "buttons" ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
                <span className="block text-sm font-medium">Şablondaki düğmelerle</span>
                <span className="block text-[0.7rem] text-muted-foreground">Tek dokunuşla cevap. Örn. Çok iyi / İyi / Kötü.</span>
              </button>
              <button type="button" onClick={() => up({ mode: "link" })} className={cn("rounded-xl px-3 py-2.5 text-left ring-1", s.mode === "link" ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
                <span className="block text-sm font-medium">Tally formuyla</span>
                <span className="block text-[0.7rem] text-muted-foreground">Linke tıklar, formu doldurur. Puanla birlikte yorum da alınır.</span>
              </button>
            </div>
          </div>

          {s.mode === "buttons" && tpl && (
            quick.length === 0 ? (
              <p className="rounded-xl bg-warning/10 px-3 py-2 text-sm text-warning">Bu şablonda cevap düğmesi yok. Hızlı cevap düğmeli bir şablon seçin ya da Tally formunu kullanın.</p>
            ) : (
              <div className="space-y-1.5">
                <p className="text-xs font-medium text-muted-foreground">Her düğme kaç puan sayılsın</p>
                {quick.map((b, i) => (
                  <div key={i} className="flex items-center gap-3 rounded-xl bg-muted/30 px-3 py-2">
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{b.text}</span>
                    <div className="flex gap-1">
                      {[1, 2, 3, 4, 5].map((n) => (
                        <button key={n} type="button" onClick={() => up({ buttonScores: quick.map((_, j) => (j === i ? n : s.buttonScores[j] ?? 5)) })} className={cn("size-8 rounded-lg text-sm font-semibold tabular-nums", (s.buttonScores[i] ?? 5) === n ? "bg-primary text-primary-foreground" : "bg-card text-muted-foreground ring-1 ring-border/60 hover:bg-accent")}>{n}</button>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )
          )}

          {s.mode === "link" && (
            <div className="space-y-3">
              <FormField label="Tally form linki" hint="Formda şu gizli alanları (hidden fields) açın: call, agent, token. Sistem bunları doldurur.">
                <input className={inputCls} value={s.linkUrl} onChange={(e) => up({ linkUrl: e.target.value })} placeholder="https://tally.so/r/..." />
              </FormField>
              {channel?.surveyHookPath && <CopyField label="Tally > Integrations > Webhooks'a girilecek adres" value={window.location.origin + channel.surveyHookPath} />}
              <p className="text-[0.7rem] text-muted-foreground">Formun imza anahtarı, seçtiğiniz numaranın Cihaz ayarları &gt; Memnuniyet anketi bölümündeki anahtarla aynı olmalı. Link şablona iki yoldan gider: şablonda değişkenli bir link düğmesi varsa oraya, yoksa aşağıdaki boşluklardan birine {"{link}"} yazın.</p>
            </div>
          )}

          {blanks > 0 && (
            <div className="space-y-2 rounded-2xl bg-muted/30 p-3">
              <p className="text-xs font-medium text-muted-foreground">Şablondaki boşluklar</p>
              {Array.from({ length: blanks }).map((_, i) => (
                <label key={i} className="flex items-center gap-2">
                  <code className="w-10 shrink-0 font-mono text-xs text-muted-foreground">{`{{${i + 1}}}`}</code>
                  <input className={cn(inputCls, "h-9")} value={s.params[i] ?? ""} onChange={(e) => setParam(i, e.target.value)} placeholder={i === 0 ? "{musteri}" : "{temsilci}"} />
                </label>
              ))}
              <p className="text-[0.7rem] text-muted-foreground">
                Kullanabilecekleriniz: {VARS.map((v, i) => <span key={v.key}>{i > 0 && ", "}<code className="rounded bg-muted px-1 font-mono">{v.key}</code> {v.label}</span>)}.
              </p>
            </div>
          )}

          <div className="space-y-3 rounded-2xl bg-muted/30 p-3">
            <p className="text-xs font-medium text-muted-foreground">Hangi görüşmelerden sonra</p>
            <div className="flex flex-wrap gap-1 rounded-xl bg-muted/60 p-1">
              {([["both", "Gelen ve giden"], ["inbound", "Sadece gelen"], ["outbound", "Sadece giden"]] as const).map(([k, l]) => (
                <button key={k} type="button" onClick={() => up({ directions: k })} className={cn("flex-1 rounded-lg px-2 py-1.5 text-xs font-medium", s.directions === k ? "bg-card shadow-sm" : "text-muted-foreground")}>{l}</button>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <FormField label="En az kaç saniye sürmüş olsun"><input type="number" min={0} className={inputCls} value={s.minSeconds} onChange={(e) => up({ minSeconds: Math.max(0, Number(e.target.value) || 0) })} /></FormField>
              <FormField label="Görüşmeden kaç dakika sonra"><input type="number" min={0} className={inputCls} value={s.delayMinutes} onChange={(e) => up({ delayMinutes: Math.max(0, Number(e.target.value) || 0) })} /></FormField>
              <FormField label="Aynı kişiye kaç günde bir"><input type="number" min={0} className={inputCls} value={s.quietDays} onChange={(e) => up({ quietDays: Math.max(0, Number(e.target.value) || 0) })} /></FormField>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-[1fr_12rem]">
            <FormField label="Cevaptan sonra teşekkür mesajı" hint="Boş bırakırsanız gönderilmez.">
              <textarea className={areaCls} rows={2} value={s.thankYou} onChange={(e) => up({ thankYou: e.target.value })} />
            </FormField>
            <FormField label="Bu puan ve altında yöneticilere haber ver" hint="0 yazarsanız haber verilmez.">
              <input type="number" min={0} max={5} className={inputCls} value={s.alertBelow} onChange={(e) => up({ alertBelow: Math.min(5, Math.max(0, Number(e.target.value) || 0)) })} />
            </FormField>
          </div>
        </div>

        <aside className="space-y-2 xl:sticky xl:top-4 xl:self-start">
          <p className="text-xs font-medium text-muted-foreground">Müşterinin göreceği</p>
          <div className="rounded-3xl bg-[#efe7dd] p-4 dark:bg-[#0b141a]">
            {tpl ? (
              <div className="max-w-[18rem] overflow-hidden rounded-2xl rounded-tl-sm bg-white text-slate-900 shadow-sm dark:bg-[#202c33] dark:text-slate-100">
                <p className="px-3 pt-2 pb-1.5 text-sm leading-snug whitespace-pre-wrap">{waText(preview)}</p>
                {quick.map((b, i) => <p key={i} className="border-t border-slate-200 py-2 text-center text-sm font-medium text-sky-600 dark:border-slate-600 dark:text-sky-400">{b.text}</p>)}
                {urlButton && <p className="border-t border-slate-200 py-2 text-center text-sm font-medium text-sky-600 dark:border-slate-600 dark:text-sky-400">{urlButton.text}</p>}
              </div>
            ) : (
              <p className="py-8 text-center text-xs text-slate-500">Şablon seçince burada görünür.</p>
            )}
          </div>
        </aside>
      </div>
    </Card>
  );
}
