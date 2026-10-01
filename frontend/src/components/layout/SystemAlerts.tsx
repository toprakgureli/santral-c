// SystemAlerts shows the server's system warnings (a filling disk, a late
// backup, WhatsApp queues backing up) as cards under the top bar, to people
// holding system.health. Each card says what is wrong and what to do; the
// cross closes it until the warning changes.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowRight, ChevronDown, ChevronUp, OctagonAlert, TriangleAlert, X } from "lucide-react";
import { api, type SystemWarning } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import {
  keepCurrent,
  linkFor,
  openWarnings,
  readDismissed,
  SYSTEM_HEALTH_PERMISSION,
  SYSTEM_HEALTH_POLL_MS,
  writeDismissed,
} from "@/lib/systemHealth";

// How many cards show before the rest fold under "N uyarı daha".
const FOLDED = 2;

export default function SystemAlerts() {
  const { user } = useAuth();
  const allowed = can(user, SYSTEM_HEALTH_PERMISSION);
  const [warnings, setWarnings] = useState<SystemWarning[]>([]);
  const [dismissed, setDismissed] = useState<Set<string>>(readDismissed);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!allowed) {
      setWarnings([]);
      return;
    }
    let stopped = false;
    const load = () => {
      api
        .systemHealth()
        .then((r) => {
          if (stopped) return;
          setWarnings(r.warnings);
          setDismissed((prev) => {
            const next = keepCurrent(prev, r.warnings);
            if (next.size !== prev.size) writeDismissed(next);
            return next;
          });
        })
        // A failed ask keeps the last list; the next minute tries again.
        .catch(() => undefined);
    };
    load();
    const timer = window.setInterval(load, SYSTEM_HEALTH_POLL_MS);
    // Coming back to the tab asks at once instead of waiting a minute.
    const onVisible = () => {
      if (document.visibilityState === "visible") load();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      stopped = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [allowed]);

  const dismiss = useCallback((fingerprint: string) => {
    setDismissed((prev) => {
      const next = new Set(prev).add(fingerprint);
      writeDismissed(next);
      return next;
    });
  }, []);

  const open = useMemo(() => openWarnings(warnings, dismissed), [warnings, dismissed]);
  if (!allowed || open.length === 0) return null;
  const shown = expanded ? open : open.slice(0, FOLDED);
  const hidden = open.length - shown.length;

  return (
    <section
      aria-label="Sistem uyarıları"
      className="pointer-events-none fixed top-[4.5rem] right-4 z-50 flex max-h-[calc(100svh-6rem)] w-[min(24rem,calc(100vw-2rem))] flex-col gap-2 overflow-y-auto"
    >
      {shown.map((w) => (
        <WarningCard key={w.fingerprint} warning={w} onDismiss={() => dismiss(w.fingerprint)} />
      ))}
      {(hidden > 0 || (expanded && open.length > FOLDED)) && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="pointer-events-auto inline-flex items-center gap-1 self-end rounded-full border border-border bg-card/95 px-3 py-1 text-xs font-medium text-muted-foreground shadow-sm backdrop-blur hover:text-foreground"
        >
          {expanded ? (
            <>
              <ChevronUp className="size-3.5" /> Daha az göster
            </>
          ) : (
            <>
              <ChevronDown className="size-3.5" /> {hidden} uyarı daha
            </>
          )}
        </button>
      )}
    </section>
  );
}

function WarningCard({ warning: w, onDismiss }: { warning: SystemWarning; onDismiss: () => void }) {
  const navigate = useNavigate();
  const { user } = useAuth();
  const link = linkFor(user, w);
  const critical = w.level === "critical";
  const Icon = critical ? OctagonAlert : TriangleAlert;
  return (
    <div
      role={critical ? "alert" : "status"}
      className={cn(
        "pointer-events-auto animate-in slide-in-from-right-4 fade-in relative overflow-hidden rounded-2xl border bg-card shadow-2xl duration-300",
        critical ? "border-destructive/40" : "border-warning/50",
      )}
    >
      <span className={cn("absolute inset-y-0 left-0 w-1", critical ? "bg-destructive" : "bg-warning")} />
      <div className="flex items-start gap-3 py-3 pr-3 pl-4">
        <span
          className={cn(
            "flex size-9 shrink-0 items-center justify-center rounded-xl",
            critical ? "bg-destructive/12 text-destructive" : "bg-warning/15 text-warning",
          )}
        >
          <Icon className="size-[1.125rem]" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm leading-tight font-semibold">{w.title}</p>
          <p className="mt-1 text-sm leading-snug">{w.text}</p>
          <p className="mt-1.5 text-xs leading-snug text-muted-foreground">
            <span className="font-medium text-foreground/80">Ne yapmalı: </span>
            {w.action}
          </p>
          {link && (
            <button
              type="button"
              onClick={() => navigate(link)}
              className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
            >
              Sayfayı aç <ArrowRight className="size-3.5" />
            </button>
          )}
        </div>
        <button
          type="button"
          onClick={onDismiss}
          aria-label="Bu uyarıyı gizle"
          data-tip="Gizle; durum değişirse yine gösterilir"
          className="-mt-1 -mr-1 rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <X className="size-4" />
        </button>
      </div>
    </div>
  );
}
