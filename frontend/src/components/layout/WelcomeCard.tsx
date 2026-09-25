// WelcomeCard greets an agent who has not started a shift yet, in the same
// shape as the break card but in the panel's blue. Unlike the break card it
// can be closed: without a shift the dialer stays locked anyway, and the
// agent may only be here to look at calls and figures. It shows on every
// page load while no shift is open, stays closed once dismissed, and does not
// come back after a shift has been started (so ending the day is quiet).

import { useEffect, useState } from "react";
import { Sunrise, X } from "lucide-react";
import { Button } from "@/components/ui";
import { useAuth } from "@/auth/AuthContext";
import { usePresence } from "@/presence/PresenceContext";
import { useShift } from "@/shift/ShiftContext";

function greeting(d: Date) {
  const h = d.getHours();
  if (h < 6) return "İyi geceler";
  if (h < 12) return "Günaydın";
  if (h < 18) return "İyi günler";
  return "İyi akşamlar";
}

export default function WelcomeCard() {
  const { user } = useAuth();
  const shift = useShift();
  const presence = usePresence();
  const [dismissed, setDismissed] = useState(false);
  const [now, setNow] = useState(() => new Date());

  // A started shift closes the card until the next page load, so ending the
  // shift in the evening does not bring the greeting back.
  useEffect(() => {
    if (shift.active && !dismissed) setDismissed(true);
  }, [shift.active, dismissed]);

  const show = !shift.loading && !shift.active && presence.hasExtension && !dismissed;

  useEffect(() => {
    if (!show) return;
    const t = window.setInterval(() => setNow(new Date()), 30000);
    return () => window.clearInterval(t);
  }, [show]);

  if (!show) return null;

  const firstName = (user?.name ?? "").trim().split(/\s+/)[0] || "";

  function close() {
    setDismissed(true);
  }

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-background/70 p-4 backdrop-blur-md">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="welcome-title"
        className="animate-in fade-in zoom-in-95 relative w-full max-w-md overflow-hidden rounded-3xl border-2 border-primary/30 bg-card shadow-2xl shadow-primary/10 duration-300"
      >
        <button
          type="button"
          onClick={close}
          aria-label="Kapat"
          className="absolute right-4 top-4 z-10 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" data-tip="Kapat">
          <X className="size-4" />
        </button>

        <div className="relative px-7 pt-7 pb-6">
          <div className="pointer-events-none absolute -right-10 -top-10 size-40 rounded-full bg-primary/15 blur-3xl" />
          <div className="flex items-center gap-3">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/15 text-primary">
              <Sunrise className="size-5" />
            </span>
            <div>
              <h2 id="welcome-title" className="text-lg font-semibold leading-tight">
                {greeting(now)}{firstName ? `, ${firstName}` : ""}
              </h2>
              <p className="text-xs text-muted-foreground">Henüz mesaini başlatmadın.</p>
            </div>
          </div>

          <div className="mt-6 text-center">
            <div className="font-mono text-5xl font-semibold tabular-nums tracking-tight">
              {now.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}
            </div>
            <div className="mt-2 text-sm text-muted-foreground">
              {now.toLocaleDateString("tr-TR", { weekday: "long", day: "numeric", month: "long" })}
            </div>
          </div>
        </div>

        <div className="grid grid-cols-2 divide-x divide-border/60 border-y border-border/60 bg-muted/30">
          <div className="px-6 py-3.5">
            <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Dahili</div>
            <div className="mt-0.5 font-mono text-lg font-semibold tabular-nums">{user?.sipExtension ?? "—"}</div>
          </div>
          <div className="px-6 py-3.5">
            <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Çağrı alma</div>
            <div className="mt-0.5 text-lg font-semibold text-muted-foreground">Kapalı</div>
          </div>
        </div>

        <div className="px-7 py-4 text-sm leading-relaxed text-muted-foreground">
          Mesai başlamadan çağrı alıp arayamazsın, ama çağrı geçmişine ve sürelere bakabilirsin. Çalışmaya başlayınca mesaiyi başlat, süreler o andan itibaren sayılır.
        </div>

        {shift.error && <p className="px-7 pb-2 text-xs text-destructive">{shift.error}</p>}

        <div className="flex gap-2 px-7 pb-7">
          <Button variant="secondary" onClick={close} className="h-12 flex-1 text-base">
            Şimdilik kapat
          </Button>
          <Button onClick={() => shift.start()} disabled={shift.busy} className="h-12 flex-[1.4] text-base shadow-md">
            {shift.busy ? "Başlatılıyor..." : "Mesaiyi başlat"}
          </Button>
        </div>
      </div>
    </div>
  );
}
