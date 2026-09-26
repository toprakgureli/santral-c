// AITab sets up the reply assistant: the key, how strong a model, and what
// it should know about the company. Nothing is ever sent by it; it only
// writes a draft into the agent's box.

import { useEffect, useState } from "react";
import { CheckCircle2, CircleAlert, KeyRound, Save, Sparkles } from "lucide-react";
import { ApiError } from "@/api/client";
import { Button, Card } from "@/components/ui";
import { areaCls, FormField, inputCls, SwitchRow } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAAISettings } from "@/whatsapp/types";

export default function AITab() {
  const [s, setS] = useState<WAAISettings | null>(null);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => {
    waApi.ai().then(setS).catch((e) => setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Ayarlar alınamadı." }));
  }, []);

  if (!s) return <div className="h-64 animate-pulse rounded-2xl bg-muted/40" />;

  const save = async (apiKey = key) => {
    setBusy(true);
    setMsg(null);
    try {
      const out = await waApi.saveAI({ enabled: s.enabled, model: s.model, instructions: s.instructions, useQuickReplies: s.useQuickReplies, apiKey });
      setS(out);
      setKey("");
      setMsg({ ok: true, text: "Kaydedildi." });
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setBusy(true);
    setMsg(null);
    try {
      setMsg({ ok: true, text: (await waApi.testAI()).message });
    } catch (e) {
      setMsg({ ok: false, text: e instanceof ApiError ? e.message : "Denenemedi." });
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card title="Yapay zekâ ile cevap önerisi" icon={Sparkles} actions={
      <span className="flex items-center gap-2">
        {s.hasKey && <Button variant="secondary" onClick={() => void test()} disabled={busy}>Bağlantıyı dene</Button>}
        <Button onClick={() => void save()} disabled={busy}><Save /> {busy ? "Kaydediliyor..." : "Kaydet"}</Button>
      </span>
    }>
      <div className="grid gap-6 lg:grid-cols-[1fr_20rem]">
        <div className="space-y-4">
          {msg && (
            <p className={cn("flex items-start gap-2 rounded-xl px-3 py-2 text-sm", msg.ok ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive")}>
              {msg.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <CircleAlert className="mt-0.5 size-4 shrink-0" />} {msg.text}
            </p>
          )}
          <SwitchRow title="Temsilciler öneri alabilsin" sub="Açıkken mesaj kutusunda mor yıldız düğmesi çıkar. Yetkisi olmayanlar görmez." on={s.enabled} onChange={(v) => setS({ ...s, enabled: v })} disabled={!s.hasKey && !key} />
          <FormField label="Anthropic API anahtarı" hint={s.hasKey ? "Kayıtlı. Değiştirmek için yenisini yapıştırın. Anahtar şifreli saklanır ve bir daha gösterilmez." : "console.anthropic.com adresindeki hesabınızdan alınan, sk-ant- ile başlayan anahtar."}>
            <span className="flex gap-2">
              <span className="relative flex-1">
                <KeyRound className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
                <input type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder={s.hasKey ? "••••••••••••" : "sk-ant-..."} className={cn(inputCls, "pl-9 font-mono")} />
              </span>
              {s.hasKey && <Button variant="ghost" onClick={() => void save("-")} disabled={busy}>Anahtarı sil</Button>}
            </span>
          </FormField>
          <div className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground">Model</p>
            <div className="grid gap-1.5 sm:grid-cols-3">
              {s.models.map((m) => (
                <button key={m.id} type="button" onClick={() => setS({ ...s, model: m.id })} className={cn("rounded-xl px-3 py-2 text-left ring-1 transition-colors", s.model === m.id ? "bg-primary/10 ring-primary/40" : "ring-border/60 hover:bg-accent/60")}>
                  <span className="block text-sm font-medium">{m.label}</span>
                  <span className="block font-mono text-[0.65rem] text-muted-foreground">{m.id}</span>
                </button>
              ))}
            </div>
          </div>
          <FormField label="Şirketiniz hakkında bilmesi gerekenler" hint="Ne satıyorsunuz, çalışma saatleri, iade ve kargo kuralları, nasıl hitap edilmeli. Burada yazmayan bir bilgiyi uydurmaz; boşluk bırakır.">
            <textarea className={areaCls} rows={9} value={s.instructions} onChange={(e) => setS({ ...s, instructions: e.target.value })} placeholder={"Örnek:\nFirmamız internet ve telefon hizmeti veriyor.\nArıza kayıtları 24 saat içinde çözülür.\nMüşterilere \"siz\" diye hitap edilir, emoji kullanılmaz."} />
          </FormField>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" className="size-4 accent-primary" checked={s.useQuickReplies} onChange={(e) => setS({ ...s, useQuickReplies: e.target.checked })} />
            Hazır yanıtları da bilgi kaynağı olarak kullansın
          </label>
        </div>
        <aside className="space-y-3 self-start rounded-2xl bg-muted/30 p-4 text-sm">
          <p className="font-semibold">Nasıl çalışır?</p>
          <ol className="list-decimal space-y-2 pl-4 text-muted-foreground">
            <li>Temsilci sohbette mor yıldıza basar.</li>
            <li>Kutu boşsa, son mesajlara bakıp bir cevap taslağı yazar. Kutuda yazı varsa onu düzeltir.</li>
            <li>Taslak mesaj kutusuna düşer. Temsilci okur, gerekirse değiştirir, kendisi gönderir.</li>
          </ol>
          <p className="text-xs text-muted-foreground">Bilmediği yerleri [köşeli parantez] içinde bırakır; bunlar doldurulmadan mesaj gönderilemez.</p>
          <p className="rounded-xl bg-card px-3 py-2 text-xs text-muted-foreground ring-1 ring-border/60">Öneri için sohbetin son 30 mesajı, bu sayfadaki şirket bilgisi ve hazır yanıtlar Anthropic'e gönderilir. İç notlar gönderilmez.</p>
        </aside>
      </div>
    </Card>
  );
}
