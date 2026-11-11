// MissedCallTab sets up automatic WhatsApp template dispatch after missed calls.

import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowRight, CheckCircle2, CircleAlert, PhoneMissed, Save, Sparkles } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card } from "@/components/ui";
import { FormField, inputCls, SwitchRow } from "@/components/whatsapp/settings/parts";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAMissedCallRecoverySettings, WAChannel, WATemplate } from "@/whatsapp/types";

const VARS = [
  { key: "{musteri}", label: "müşterinin adı" },
  { key: "{tarih}", label: "çağrı tarihi" },
  { key: "{saat}", label: "çağrı saati" },
];

function countVars(text?: string) {
  let n = 0;
  for (const m of (text ?? "").matchAll(/\{\{(\d+)\}\}/g)) n = Math.max(n, Number(m[1]));
  return n;
}

export default function MissedCallTab({ channels }: { channels: WAChannel[] }) {
  const [s, setS] = useState<WAMissedCallRecoverySettings | null>(null);
  const [templates, setTemplates] = useState<WATemplate[]>([]);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    waApi
      .missedCallRecovery()
      .then((v) => setS({ ...v, channelId: v.channelId || channels[0]?.id || 0 }))
      .catch((e) => setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Ayarlar alınamadı." }));
  }, [channels]);

  useEffect(() => {
    if (!s?.channelId) return;
    waApi
      .templates(s.channelId)
      .then((l) => setTemplates(l.filter((t) => t.status === "APPROVED")))
      .catch(() => setTemplates([]));
  }, [s?.channelId]);

  const tpl = useMemo(
    () => templates.find((t) => t.name === s?.template && t.language === s?.templateLang),
    [templates, s?.template, s?.templateLang],
  );
  const body = tpl?.components.find((c) => c.type === "BODY")?.text ?? "";
  const blanks = countVars(body);

  if (channels.length === 0) {
    return <p className="rounded-2xl bg-card p-8 text-center text-sm text-muted-foreground ring-1 ring-border/60">Önce bir WhatsApp numarası ekle.</p>;
  }
  if (!s) {
    return <div className="h-64 animate-pulse rounded-2xl bg-muted/40" />;
  }

  const up = (p: Partial<WAMissedCallRecoverySettings>) => setS((cur) => (cur ? { ...cur, ...p } : cur));
  const setParam = (i: number, v: string) =>
    up({
      params: Array.from({ length: Math.max(blanks, s.params.length) }, (_, j) => (j === i ? v : s.params[j] ?? "")),
    });

  const preview = body.replace(/\{\{(\d+)\}\}/g, (all, k) => {
    const v = s.params[Number(k) - 1];
    if (!v) return all;
    return v
      .replace("{musteri}", "Ahmet Yılmaz")
      .replace("{tarih}", "11.11.2026")
      .replace("{saat}", "14:35");
  });

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const out = await waApi.saveMissedCallRecovery({
        ...s,
        params: s.params.slice(0, blanks),
      });
      setS(out);
      setMsg({
        ok: true,
        text: out.enabled
          ? "Kaydedildi. Santralde cevapsız kalan çağrılara otomatik kurtarma şablonu gidecek."
          : "Kaydedildi.",
      });
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card
      title="Kaçan çağrı kurtarma"
      icon={PhoneMissed}
      actions={
        <Button onClick={() => void save()} disabled={busy}>
          <Save /> {busy ? "Kaydediliyor..." : "Kaydet"}
        </Button>
      }
    >
      <p className="mb-4 text-sm text-muted-foreground">
        Santralde tüm temsilciler meşgulken veya mesai dışındayken bir arama cevapsız kapandığında, arayan cep numarasına WhatsApp'tan otomatik bir şablon mesajı gönderilir. Müşteri yanıt verdiğinde WhatsApp gelen kutusunda yeni bir talep açılır. Temsilcilerin XLite veya masaüstü softphone kullanması fark etmeksizin santralin yanıt durumuna göre çalışır. Sabit hatlar filtrelenir.
      </p>

      {msg && (
        <p
          className={cn(
            "mb-4 flex items-start gap-2 rounded-xl px-3 py-2 text-sm",
            msg.ok ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive",
          )}
        >
          {msg.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <CircleAlert className="mt-0.5 size-4 shrink-0" />} {msg.text}
        </p>
      )}

      <div className="space-y-5">
        <div className="flex items-center justify-between rounded-2xl bg-muted/30 px-4 py-3 ring-1 ring-border/40">
          <div className="space-y-0.5">
            <p className="text-xs font-semibold">Kaçan Çağrılar Listesi</p>
            <p className="text-[11px] text-muted-foreground">
              Onay bekleyen veya geçmişte gönderilen tüm kaçan çağrıları canlı takip et.
            </p>
          </div>
          <Link
            to="/whatsapp/missed-calls"
            className="flex items-center gap-1.5 rounded-xl bg-card px-3 py-1.5 text-xs font-semibold text-primary shadow-sm ring-1 ring-border/60 hover:bg-accent"
          >
            Listeyi Aç <ArrowRight className="size-3.5" />
          </Link>
        </div>

        <SwitchRow
          on={s.enabled}
          onChange={(v) => up({ enabled: v })}
          title="Kaçan çağrılarda kurtarma özelliğini etkinleştir"
          sub="Cevapsız kalan gelen çağrılarda müşteriye WhatsApp şablonu iletilir veya onay listesine alınır."
        />

        {s.enabled && (
          <FormField label="Kurtarma Modu" hint="Kaçan çağrının nasıl kurtarılacağını seç">
            <div className="grid gap-3 sm:grid-cols-2">
              <button
                type="button"
                onClick={() => up({ mode: "auto" })}
                className={cn(
                  "flex flex-col items-start gap-1 rounded-2xl p-3.5 text-left ring-1 transition-colors",
                  (s.mode ?? "auto") === "auto"
                    ? "bg-primary/10 ring-primary/40 text-foreground"
                    : "bg-muted/20 ring-border/50 text-muted-foreground hover:bg-accent/50",
                )}
              >
                <span className="text-xs font-semibold text-foreground">⚡ Otomatik Mod (Anında Gönder)</span>
                <span className="text-xs leading-relaxed text-muted-foreground">
                  Santralde çağrı cevapsız kapandığı anda seçili şablon mesajı beklemeden otomatik müşteriye gönderilir.
                </span>
              </button>

              <button
                type="button"
                onClick={() => up({ mode: "manual" })}
                className={cn(
                  "flex flex-col items-start gap-1 rounded-2xl p-3.5 text-left ring-1 transition-colors",
                  s.mode === "manual"
                    ? "bg-primary/10 ring-primary/40 text-foreground"
                    : "bg-muted/20 ring-border/50 text-muted-foreground hover:bg-accent/50",
                )}
              >
                <span className="text-xs font-semibold text-foreground">📋 Onaylı Liste Modu (Temsilci Kontrollü)</span>
                <span className="text-xs leading-relaxed text-muted-foreground">
                  Kaçan çağrılar 'Kurtarma Bekleyenler' listesine eklenir; temsilci tek tıkla şablonu gönderir veya müşteriyi geri arar.
                </span>
              </button>
            </div>
          </FormField>
        )}

        <div className="grid gap-4 md:grid-cols-2">
          <FormField label="Gönderen WhatsApp numarası" hint="Şablonun çıkış yapacağı hat">
            <select
              value={s.channelId}
              onChange={(e) => up({ channelId: Number(e.target.value) })}
              className={inputCls}
            >
              {channels.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name ? `${c.name} (${c.displayPhone})` : c.displayPhone}
                </option>
              ))}
            </select>
          </FormField>

          <FormField label="Aynı numaraya tekrar gönderme aralığı" hint="Aynı kişi tekrar aradığında spam olmaması için bekleme süresi">
            <div className="flex items-center gap-2">
              <input
                type="number"
                min={1}
                max={72}
                value={s.quietHours}
                onChange={(e) => up({ quietHours: Math.max(1, Number(e.target.value) || 1) })}
                className={inputCls}
              />
              <span className="shrink-0 text-sm text-muted-foreground">saat</span>
            </div>
          </FormField>
        </div>

        <FormField label="Gönderilecek şablon" hint="Meta tarafından onaylanmış WhatsApp şablonu">
          <select
            value={s.template ? `${s.template}:${s.templateLang}` : ""}
            onChange={(e) => {
              const [name, lang] = e.target.value.split(":");
              up({ template: name ?? "", templateLang: lang ?? "tr" });
            }}
            className={inputCls}
          >
            <option value="">Bir şablon seç...</option>
            {templates.map((t) => (
              <option key={`${t.name}:${t.language}`} value={`${t.name}:${t.language}`}>
                {t.name} ({t.language}) {t.category === "MARKETING" ? "· Pazarlama" : "· Hizmet"}
              </option>
            ))}
          </select>
        </FormField>

        {blanks > 0 && (
          <div className="space-y-3 rounded-2xl bg-muted/30 p-4 ring-1 ring-border/40">
            <div className="flex items-center gap-2">
              <Sparkles className="size-4 text-primary" />
              <h3 className="text-sm font-semibold">Şablon değişkenleri ({blanks} adet)</h3>
            </div>
            <p className="text-xs text-muted-foreground">
              Şablondaki boşluklara ne yazılacağını seç. Kullanılabilir etiketler: {VARS.map((v) => v.key).join(", ")}.
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              {Array.from({ length: blanks }).map((_, i) => (
                <div key={i} className="space-y-1">
                  <label className="text-xs font-medium text-muted-foreground">
                    {"{{"}
                    {i + 1}
                    {"}}"} boşluğu
                  </label>
                  <input
                    value={s.params[i] ?? ""}
                    onChange={(e) => setParam(i, e.target.value)}
                    placeholder="örn: {musteri}"
                    className={inputCls}
                  />
                  <div className="flex flex-wrap gap-1 pt-1">
                    {VARS.map((v) => (
                      <button
                        key={v.key}
                        type="button"
                        onClick={() => setParam(i, v.key)}
                        className="rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-mono text-muted-foreground hover:bg-accent hover:text-foreground"
                      >
                        {v.key}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {tpl && (
          <div className="rounded-2xl bg-muted/20 p-4 ring-1 ring-border/40">
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Müşterinin göreceği mesaj önizlemesi
            </h3>
            <div className="max-w-md rounded-2xl rounded-tl-sm bg-[#DCF8C6] p-3 text-sm text-black shadow-sm dark:bg-[#005c4b] dark:text-white">
              <div className="whitespace-pre-wrap">{waText(preview)}</div>
              <div className="mt-1.5 text-right font-mono text-[10px] text-muted-foreground/80 dark:text-white/60">
                14:35
              </div>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}
