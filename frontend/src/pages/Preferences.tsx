// Preferences ("Ayarlarım"): each person's own settings. They change
// nothing for anybody else. For now WhatsApp: sounds, desktop notices, a
// mute for everything for a while, and the conversations muted or pinned
// one by one.

import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ArrowLeft, Bell, BellOff, MessageCircle, Pin, SlidersHorizontal } from "lucide-react";
import { ApiError } from "@/api/client";
import { Card, EmptyState } from "@/components/ui";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import { MUTES } from "@/components/whatsapp/ConversationList";
import { SwitchRow } from "@/components/whatsapp/settings/parts";
import { cn } from "@/lib/utils";
import type { WAMute } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";

function until(iso?: string) {
  if (!iso) return "";
  const d = new Date(iso);
  if (d.getFullYear() > 9000) return "siz açana kadar";
  const today = new Date();
  const sameDay = d.toDateString() === today.toDateString();
  return `${sameDay ? "bugün" : d.toLocaleDateString("tr-TR", { day: "numeric", month: "long" })} ${d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}'e kadar`;
}

export function Preferences() {
  const wa = useWhatsApp();
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const [permission, setPermission] = useState(() => (typeof Notification === "undefined" ? "unsupported" : Notification.permission));
  const [, tick] = useState(0);

  useEffect(() => {
    const t = window.setInterval(() => tick((x) => x + 1), 30000);
    return () => window.clearInterval(t);
  }, []);

  const run = async (fn: () => Promise<void>) => {
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    }
  };

  const muted = wa.prefs.conversations.filter((c) => c.mutedUntil && Date.parse(c.mutedUntil) > Date.now());
  const pinned = wa.prefs.conversations.filter((c) => c.pinnedAt);

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex items-center gap-3">
        <button type="button" onClick={() => (window.history.length > 1 ? navigate(-1) : navigate("/"))} aria-label="Geri" data-tip="Geri" className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border/70 bg-card text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground">
          <ArrowLeft className="size-4" />
        </button>
        <span className="flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary"><SlidersHorizontal className="size-5" /></span>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Ayarlarım</h1>
          <p className="text-xs text-muted-foreground">Buradaki ayarlar sadece sizin için geçerli, başka kimseyi etkilemez.</p>
        </div>
      </div>
      {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

      {!wa.enabled ? (
        <Card><EmptyState icon={<SlidersHorizontal />} title="Şimdilik değiştirilecek bir ayar yok" /></Card>
      ) : (
        <>
          <Card title="WhatsApp bildirimleri" icon={Bell}>
            <div className="space-y-2">
              <SwitchRow title="Yeni mesajda ses çal" sub="Size düşen, havuzdaki ve cevap bekleyen sohbetlerde." on={wa.prefs.sound} onChange={(v) => void run(() => wa.setPrefs({ sound: v }))} />
              <SwitchRow
                title="Masaüstü bildirimi göster"
                sub={permission === "denied" ? "Tarayıcı bu site için bildirimleri engellemiş. Adres çubuğundaki kilit simgesinden izin verebilirsiniz." : "Panel arka plandayken ekranın köşesinde çıkar."}
                on={wa.prefs.desktop}
                onChange={(v) => void run(async () => {
                  if (v && typeof Notification !== "undefined" && Notification.permission === "default") setPermission(await Notification.requestPermission());
                  await wa.setPrefs({ desktop: v });
                })}
              />
            </div>
          </Card>

          <Card title="Tüm WhatsApp taleplerini sessize al" icon={BellOff}>
            {wa.mutedAll ? (
              <div className="flex flex-wrap items-center gap-3 rounded-2xl bg-warning/10 px-4 py-3">
                <BellOff className="size-5 shrink-0 text-warning" />
                <p className="min-w-0 flex-1 text-sm">Sessizde, <b>{until(wa.prefs.mutedUntil)}</b>. Mesajlar gelmeye devam eder ama ses ve bildirim çıkmaz.</p>
                <button type="button" onClick={() => void run(() => wa.setPrefs({ mute: "off" }))} className="rounded-full bg-wa-accent px-4 py-1.5 text-sm font-semibold text-wa-on-accent shadow-sm">Sesi aç</button>
              </div>
            ) : (
              <>
                <p className="mb-3 text-sm text-muted-foreground">Toplantıdayken ya da başka bir işe odaklanırken seçin. Mesajlar gelmeye devam eder, sadece ses ve bildirim çıkmaz.</p>
                <div className="flex flex-wrap gap-2">
                  {MUTES.map((m) => (
                    <button key={m.key} type="button" onClick={() => void run(() => wa.setPrefs({ mute: m.key as WAMute }))} className="rounded-full bg-muted/70 px-4 py-2 text-sm font-medium transition-colors hover:bg-accent">{m.label}</button>
                  ))}
                </div>
              </>
            )}
          </Card>

          <Card title="Sessize aldığınız sohbetler" icon={MessageCircle}>
            {muted.length === 0 ? (
              <p className="text-sm text-muted-foreground">Yok. Bir sohbete sağ tıklayıp "Sessize al" diyebilirsiniz; o sohbet ses çıkarmaz ve sayaçlara katılmaz.</p>
            ) : (
              <div className="divide-y divide-border/50">
                {muted.map((m) => <ConvLine key={m.id} id={m.id} note={until(m.mutedUntil)} action="Sesi aç" onAction={() => void run(() => wa.setConvPref(m.id, { mute: "off" }))} />)}
              </div>
            )}
          </Card>

          {pinned.length > 0 && (
            <Card title="Sabitlediğiniz sohbetler" icon={Pin}>
              <div className="divide-y divide-border/50">
                {pinned.map((p) => <ConvLine key={p.id} id={p.id} note="listenin en üstünde" action="Kaldır" onAction={() => void run(() => wa.setConvPref(p.id, { pin: false }))} />)}
              </div>
            </Card>
          )}
        </>
      )}
    </div>
  );
}

function ConvLine({ id, note, action, onAction }: { id: number; note: string; action: string; onAction: () => void }) {
  const wa = useWhatsApp();
  const c = wa.byId(id);
  return (
    <div className="flex items-center gap-3 py-2.5">
      {c ? <ContactAvatar name={c.contact.display} seed={c.contact.waId} className="size-10" /> : <span className="size-10 rounded-full bg-muted" />}
      <Link to={`/whatsapp/${id}`} className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium hover:underline">{c ? c.contact.display : `Sohbet #${id}`}</span>
        <span className="block truncate text-xs text-muted-foreground">{[c?.channelName, note].filter(Boolean).join(" · ")}</span>
      </Link>
      <button type="button" onClick={onAction} className={cn("rounded-full px-3 py-1.5 text-xs font-semibold text-wa-accent hover:bg-wa-accent/10")}>{action}</button>
    </div>
  );
}
