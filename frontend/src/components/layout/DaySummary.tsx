// DaySummary: the person's day, shown when they end their shift and five
// minutes before the working day ends. Today's real calls are set against
// their own daily target and their own last week, never against colleagues,
// with the day's follow-ups so nothing promised is forgotten overnight.

import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { AlarmClock, Clock3, Coffee, LogOut, MessageSquareText, PhoneIncoming, PhoneMissed, PhoneOutgoing, Sparkles, Target, Timer, TrendingUp } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { DaySummary as Summary } from "@/api/types";
import { Button, Modal } from "@/components/ui";
import { longDate } from "@/lib/time";
import { cn } from "@/lib/utils";

function hm(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h === 0) return `${m} dk`;
  return m === 0 ? `${h} sa` : `${h} sa ${m} dk`;
}

function ms(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m === 0) return `${s} sn`;
  return s === 0 ? `${m} dk` : `${m} dk ${s} sn`;
}

// weekLine says how today compares with the person's own last week.
function weekLine(s: Summary): string {
  if (s.weekDays === 0) return "Geçen hafta çağrın olmadığı için karşılaştırma yok.";
  const avg = s.weekAverage;
  const a = avg.toLocaleString("tr-TR", { maximumFractionDigits: 1 });
  if (s.today.real >= s.weekBest && s.today.real > 0) return `Son 7 günün en iyi günü: ${s.today.real} gerçek çağrı. Ortalaman ${a}.`;
  if (s.today.real >= avg * 1.1) return `Son 7 günlük ortalamanın (${a}) üstündesin. En iyi günün ${s.weekBest}.`;
  if (s.today.real >= avg * 0.9) return `Son 7 günlük ortalamanla (${a}) aynı çizgidesin. En iyi günün ${s.weekBest}.`;
  return `Son 7 günlük ortalaman ${a}, en iyi günün ${s.weekBest}. Yarın yeni bir gün.`;
}

export default function DaySummary({ ending, onClose, onEnd }: { ending: boolean; onClose: () => void; onEnd: () => Promise<void> }) {
  const [s, setS] = useState<Summary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.daySummary().then(setS).catch((e) => setError(e instanceof ApiError ? e.message : "Günün özeti alınamadı."));
  }, []);

  const end = async () => {
    setBusy(true);
    try {
      await onEnd();
      onClose();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Mesai bitirilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const t = s?.today;
  const done = !!s && s.target > 0 && s.today.real >= s.target;
  const pct = s && s.target > 0 ? Math.min(100, Math.round((s.today.real / s.target) * 100)) : 0;

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title="Günün özeti"
      description={longDate(new Date())}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>{ending ? "Vazgeç" : "Kapat"}</Button>
          <Button variant={ending ? "danger" : "primary"} onClick={() => void end()} disabled={busy}>
            <LogOut /> Mesaiyi bitir
          </Button>
        </>
      }
    >
      {error && <p className="mb-3 rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      {!s || !t ? (
        !error && <p className="text-sm text-muted-foreground">Hazırlanıyor...</p>
      ) : (
        <div className="space-y-4">
          {/* Today's real calls against the target */}
          <div className={cn("rounded-2xl p-4 ring-1", done ? "bg-success/8 ring-success/25" : "bg-primary/5 ring-primary/15")}>
            <div className="flex items-end justify-between gap-3">
              <div>
                <p className="text-xs font-medium text-muted-foreground">Gerçek çağrı ({s.realSeconds} sn ve üstü)</p>
                <p className="text-4xl font-bold tabular-nums tracking-tight">
                  {t.real}
                  {s.target > 0 && <span className="text-xl font-semibold text-muted-foreground"> / {s.target}</span>}
                </p>
              </div>
              {done ? (
                <span className="flex items-center gap-1.5 rounded-full bg-success/15 px-3 py-1 text-sm font-semibold text-success"><Sparkles className="size-4" /> Hedef tamam, tebrikler</span>
              ) : s.target > 0 ? (
                <span className="flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-sm font-medium text-primary"><Target className="size-4" /> Hedefe {s.target - t.real} çağrı kaldı</span>
              ) : null}
            </div>
            {s.target > 0 && (
              <div className="mt-3 h-2.5 overflow-hidden rounded-full bg-muted">
                <div className={cn("h-full rounded-full transition-all", done ? "bg-success" : "bg-primary")} style={{ width: `${pct}%` }} />
              </div>
            )}
            <p className="mt-3 flex items-center gap-1.5 text-sm text-muted-foreground"><TrendingUp className="size-4 shrink-0" /> {weekLine(s)}</p>
          </div>

          {/* The day in figures */}
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            <Fig icon={PhoneIncoming} label="Gelen gerçek" value={String(t.inboundReal)} />
            <Fig icon={PhoneOutgoing} label="Giden gerçek" value={String(t.outboundReal)} />
            <Fig icon={PhoneMissed} label="Ulaşılamayan" value={String(t.unanswered + t.short)} sub={`${t.unanswered} cevapsız, ${t.short} kısa`} />
            <Fig icon={Timer} label="Toplam konuşma" value={hm(t.talkSeconds)} sub={t.real ? `ortalama ${ms(t.avgTalkSeconds)}` : undefined} />
            <Fig icon={Clock3} label="En uzun görüşme" value={t.longestSeconds ? ms(t.longestSeconds) : "–"} />
            <Fig icon={MessageSquareText} label="Eskalasyon" value={String(t.escalations)} />
            <Fig icon={Clock3} label="Mesai" value={hm(t.shiftSeconds)} />
            <Fig icon={Coffee} label="Mola" value={hm(t.breakSeconds)} />
          </div>

          {/* What is still owed */}
          {(s.followups.unreached > 0 || s.followups.remindersOpen > 0) && (
            <div className="rounded-2xl bg-warning/8 p-3.5 text-sm ring-1 ring-warning/25">
              <p className="flex items-start gap-2">
                <AlarmClock className="mt-0.5 size-4 shrink-0 text-warning" />
                <span>
                  {s.followups.unreached > 0 && `Bugün ${s.followups.unreached} numaraya ulaşamadın${s.followups.reached > 0 ? `, ${s.followups.reached} tanesine sonradan ulaşıldı` : ""}. `}
                  {s.followups.remindersOpen > 0 && `${s.followups.remindersOpen} planlı geri arama bekliyor. `}
                  <Link to="/followups" onClick={onClose} className="font-medium text-primary hover:underline">Geri Dönüşler'e bak</Link>
                </span>
              </p>
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}

function Fig({ icon: Icon, label, value, sub }: { icon: typeof Clock3; label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-xl bg-card p-3 ring-1 ring-border/60">
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><Icon className="size-3.5" /> {label}</p>
      <p className="mt-1 text-lg font-semibold tabular-nums">{value}</p>
      {sub && <p className="text-[0.7rem] text-muted-foreground">{sub}</p>}
    </div>
  );
}
