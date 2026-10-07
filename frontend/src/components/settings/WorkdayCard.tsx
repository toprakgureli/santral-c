// WorkdayCard sets when the working day ends and each role's daily target of
// real calls. The end drives the shift reminder, the day's summary five
// minutes before it and the automatic close fifty minutes after; the target
// shows on the dashboard and in the day's summary.

import { useEffect, useState } from "react";
import { CalendarClock, Loader2 } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { Workday } from "@/api/types";
import { Badge, Button, Card, Input, Skeleton } from "@/components/ui";
import { rememberWorkday } from "@/lib/workday";

// shifted moves an "HH:MM" time by some minutes: the summary five minutes
// before the end, the automatic close fifty after.
function shifted(end: string, minutes: number): string {
  const [h, m] = end.split(":").map(Number);
  if (Number.isNaN(h) || Number.isNaN(m)) return "";
  const t = h * 60 + m + minutes;
  return `${String(Math.floor(t / 60)).padStart(2, "0")}:${String(t % 60).padStart(2, "0")}`;
}

export default function WorkdayCard() {
  const [saved, setSaved] = useState<Workday | null>(null);
  const [end, setEnd] = useState("");
  const [targets, setTargets] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const fill = (w: Workday) => {
    setSaved(w);
    setEnd(w.shiftEnd);
    setTargets(Object.fromEntries(w.roles.map((r) => [r.id, r.target ? String(r.target) : ""])));
  };

  useEffect(() => {
    api.workday().then(fill).catch((e) => setError(e instanceof ApiError ? e.message : "Ayar okunamadı."));
  }, []);

  const parsed = Object.fromEntries(Object.entries(targets).map(([id, v]) => [id, v.trim() === "" ? 0 : Number(v)]));
  const valid = /^\d{2}:\d{2}$/.test(end) && end >= "12:00" && end <= "23:00" && Object.values(parsed).every((n) => Number.isInteger(n) && n >= 0 && n <= 500);
  const dirty = !!saved && (end !== saved.shiftEnd || saved.roles.some((r) => (parsed[r.id] ?? 0) !== r.target));

  const save = async () => {
    if (!valid) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const w = await api.updateWorkday({ shiftEnd: end, targets: parsed });
      fill(w);
      rememberWorkday(w);
      setNotice("Kaydedildi. Mesai saatleri ve hedefler herkeste hemen geçerli.");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Ayar kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card title="Mesai ve Hedefler" icon={CalendarClock} actions={saved ? <Badge tone="blue">Bitiş {saved.shiftEnd}</Badge> : <Skeleton className="h-5 w-24" />}>
      {!saved ? (
        error ? <p className="text-sm text-destructive">{error}</p> : <Skeleton className="h-24 w-full" />
      ) : (
        <div className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-[12rem_1fr] sm:items-start">
            <label className="space-y-1.5">
              <span className="text-sm font-medium">Mesai bitişi</span>
              <Input type="time" value={end} min="12:00" max="23:00" onChange={(e) => setEnd(e.target.value)} />
            </label>
            <p className="text-xs leading-relaxed text-muted-foreground sm:pt-7">
              {valid ? (
                <>Saat <b className="text-foreground">{shifted(end, -5)}</b> olunca mesaisi açık herkese günün özeti çıkar, <b className="text-foreground">{end}</b> mesai bitişidir; <b className="text-foreground">{shifted(end, 50)}</b> olunca hâlâ açık mesailer kendiliğinden kapanır.</>
              ) : (
                "12:00 ile 23:00 arasında bir saat seç."
              )}
            </p>
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium">Günlük gerçek çağrı hedefi</p>
            <p className="text-xs text-muted-foreground">Ana sayfada ve günün özetinde ilerleme olarak görünür. Birden fazla rolü olan kişiye en yüksek hedef uygulanır. Boş bırakılan rolde hedef gösterilmez.</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {saved.roles.map((r) => (
                <label key={r.id} className="flex items-center justify-between gap-3 rounded-xl border border-border/60 px-3 py-2">
                  <span className="truncate text-sm">{r.name}</span>
                  <Input
                    inputMode="numeric"
                    value={targets[r.id] ?? ""}
                    onChange={(e) => setTargets((cur) => ({ ...cur, [r.id]: e.target.value.replace(/\D/g, "") }))}
                    placeholder="Yok"
                    className="h-8 w-20 text-right tabular-nums"
                  />
                </label>
              ))}
            </div>
          </div>

          {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
          {notice && <p className="rounded-xl bg-success/10 px-3 py-2 text-sm text-success">{notice}</p>}
          <div className="flex justify-end">
            <Button onClick={() => void save()} disabled={busy || !dirty || !valid}>
              {busy && <Loader2 className="animate-spin" />} Kaydet
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
}
