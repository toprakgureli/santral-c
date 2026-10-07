// LiveAlerts: for whoever leads the team (performance.live_alerts), what
// needs a look right now: a conversation running long, someone on break past
// the day's limit, incoming calls left unanswered, customers nobody called
// back, planned call backs past their time. It asks every 30 seconds; an
// alert that was not there before comes up once as a toast, and the list
// under the icon always shows what is going on now.

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Siren } from "lucide-react";
import { api } from "@/api/client";
import type { LiveAlert } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import { clockTime } from "@/lib/time";
import { cn } from "@/lib/utils";
import { pushToast } from "./Toasts";

const POLL_MS = 30000;

export default function LiveAlerts() {
  const { can } = useAuth();
  const allowed = can("performance.live_alerts");
  const navigate = useNavigate();
  const [items, setItems] = useState<LiveAlert[]>([]);
  const [open, setOpen] = useState(false);
  // Keys seen in the last read: one missing then and present now is news.
  const seen = useRef<Set<string> | null>(null);
  const box = useRef<HTMLDivElement>(null);

  const poll = useCallback(async () => {
    try {
      const list = await api.liveAlerts();
      if (seen.current !== null) {
        for (const a of list) {
          if (!seen.current.has(a.key)) pushToast({ id: `alert-${a.key}`, text: a.text, link: a.link, tone: "alert" });
        }
      }
      seen.current = new Set(list.map((a) => a.key));
      setItems(list);
    } catch {
      // the next poll tries again
    }
  }, []);

  useEffect(() => {
    if (!allowed) return;
    void poll();
    const t = window.setInterval(() => void poll(), POLL_MS);
    return () => window.clearInterval(t);
  }, [allowed, poll]);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  if (!allowed) return null;
  const warnings = items.filter((a) => a.level === "warning").length;

  return (
    <div className="relative" ref={box}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={items.length ? `Canlı uyarılar, ${items.length} uyarı` : "Canlı uyarılar"}
        data-tip="Canlı uyarılar"
        className={cn("relative flex size-10 items-center justify-center rounded-xl transition-colors hover:bg-accent", items.length ? "text-warning" : "text-muted-foreground hover:text-foreground")}
      >
        <Siren className="size-[1.15rem]" />
        {items.length > 0 && (
          <span className={cn("absolute top-1.5 right-1.5 flex min-w-4 items-center justify-center rounded-full px-1 text-[0.6rem] leading-4 font-bold text-white tabular-nums", warnings ? "bg-destructive" : "bg-warning")}>
            {items.length}
          </span>
        )}
      </button>

      {open && (
        <div className="animate-in fade-in slide-in-from-top-1 absolute right-0 mt-2 w-[22rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-lg duration-150">
          <div className="border-b border-border/60 px-3 py-2">
            <span className="text-sm font-semibold">Canlı uyarılar</span>
            <span className="ml-2 text-xs text-muted-foreground">30 saniyede bir yenilenir</span>
          </div>
          <div className="max-h-96 overflow-y-auto p-1">
            {items.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">Şu an dikkat gerektiren bir şey yok.</p>
            ) : (
              items.map((a) => (
                <button
                  key={a.key}
                  type="button"
                  onClick={() => {
                    setOpen(false);
                    if (a.link) navigate(a.link);
                  }}
                  className="flex w-full gap-2.5 rounded-lg px-3 py-2.5 text-left hover:bg-accent"
                >
                  <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", a.level === "warning" ? "bg-destructive" : "bg-warning")} />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm leading-snug">{a.text}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">Başladı: {clockTime(a.since)}</span>
                  </span>
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
