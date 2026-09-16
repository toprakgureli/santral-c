// BreakOverlay covers the panel while the agent is on a break. A break is easy
// to forget, so it stays in front on every page until "Molayı bitir" is
// pressed. It shows the running break, when it started, today's total break
// time and the earlier breaks of the day.

import { useEffect, useState } from "react";
import { Coffee } from "lucide-react";
import { Button } from "@/components/ui";
import { usePresence } from "@/presence/PresenceContext";
import { useShift } from "@/shift/ShiftContext";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { cn } from "@/lib/utils";

function hhmm(iso: string) {
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

// clock renders a running duration as HH:MM:SS.
function clock(seconds: number) {
  const s = Math.max(0, Math.floor(seconds));
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`;
}

// brief renders a duration in words: "45 sn", "12 dk", "1 sa 05 dk".
function brief(seconds: number) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s} sn`;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h === 0) return `${m} dk`;
  return `${h} sa ${String(m).padStart(2, "0")} dk`;
}

export default function BreakOverlay() {
  const presence = usePresence();
  const shift = useShift();
  const phone = useSoftphoneContext();
  const [now, setNow] = useState(() => Date.now());

  const data = presence.data;
  const onBreak = shift.active && presence.hasExtension && data?.state === "break";
  // A call in progress wins over the overlay (placing one ends the break anyway).
  const busy = phone.status === "in-call" || phone.status === "held" || phone.status === "ringing" || phone.status === "calling" || phone.status === "incoming";
  const show = onBreak && !busy;

  useEffect(() => {
    if (!show) return;
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [show]);

  const [ending, setEnding] = useState(false);
  useEffect(() => {
    if (!show) setEnding(false);
  }, [show]);

  if (!show || !data) return null;

  const startedAt = data.since ? Date.parse(data.since) : presence.fetchedAt;
  const current = (now - startedAt) / 1000;
  const liveDelta = Math.max(0, (now - presence.fetchedAt) / 1000);
  const todayTotal = (data.totals?.break ?? 0) + liveDelta;
  const pauses = (data.pauses ?? []).filter((p) => p.state === "break");
  const earlier = pauses.filter((p) => p.endedAt);
  const count = earlier.length + 1;

  async function end() {
    setEnding(true);
    try {
      await presence.change("available");
    } catch {
      setEnding(false);
    }
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/70 p-4 backdrop-blur-md">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="break-title"
        className="animate-in fade-in zoom-in-95 w-full max-w-md overflow-hidden rounded-3xl border border-warning/30 bg-card shadow-2xl shadow-warning/10 duration-300"
      >
        {/* Header */}
        <div className="relative px-7 pt-7 pb-6">
          <div className="pointer-events-none absolute -right-10 -top-10 size-40 rounded-full bg-warning/15 blur-3xl" />
          <div className="flex items-center gap-3">
            <span className="relative flex size-11 items-center justify-center rounded-2xl bg-warning/15 text-warning">
              <Coffee className="size-5" />
              <span className="absolute -right-0.5 -top-0.5 size-2.5 rounded-full bg-warning ring-2 ring-card animate-pulse" />
            </span>
            <div>
              <h2 id="break-title" className="text-lg font-semibold leading-tight">Moladasın</h2>
              <p className="text-xs text-muted-foreground">Bu sırada sana çağrı düşmez.</p>
            </div>
          </div>

          <div className="mt-6 text-center">
            <div className="font-mono text-5xl font-semibold tabular-nums tracking-tight">{clock(current)}</div>
            <div className="mt-2 text-sm text-muted-foreground">
              <span className="font-mono tabular-nums text-foreground">{hhmm(new Date(startedAt).toISOString())}</span>
              {"'de başladı"}
            </div>
          </div>
        </div>

        {/* Today */}
        <div className="grid grid-cols-2 divide-x divide-border/60 border-y border-border/60 bg-muted/30">
          <div className="px-6 py-3.5">
            <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Bugün toplam mola</div>
            <div className="mt-0.5 font-mono text-lg font-semibold tabular-nums">{brief(todayTotal)}</div>
          </div>
          <div className="px-6 py-3.5">
            <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Mola sayısı</div>
            <div className="mt-0.5 font-mono text-lg font-semibold tabular-nums">{count}</div>
          </div>
        </div>

        {/* Breaks of the day */}
        <div className="px-7 py-4">
          <div className="mb-2 text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Bugünkü molalar</div>
          <ul className="max-h-40 space-y-1 overflow-y-auto text-sm">
            {earlier.map((p) => (
              <li key={p.startedAt} className="flex items-center justify-between rounded-lg px-2 py-1 text-muted-foreground">
                <span className="font-mono tabular-nums">{hhmm(p.startedAt)} - {hhmm(p.endedAt!)}</span>
                <span className="font-mono tabular-nums">{brief((Date.parse(p.endedAt!) - Date.parse(p.startedAt)) / 1000)}</span>
              </li>
            ))}
            <li className={cn("flex items-center justify-between rounded-lg bg-warning/10 px-2 py-1 font-medium")}>
              <span className="font-mono tabular-nums">{hhmm(new Date(startedAt).toISOString())} - <span className="font-sans text-warning">devam ediyor</span></span>
              <span className="font-mono tabular-nums">{brief(current)}</span>
            </li>
          </ul>
        </div>

        <div className="px-7 pb-7">
          <Button onClick={end} disabled={ending} className="h-12 w-full bg-warning text-base text-black shadow-md hover:bg-warning/90">
            {ending ? "Bitiriliyor..." : "Molayı bitir"}
          </Button>
        </div>
      </div>
    </div>
  );
}
