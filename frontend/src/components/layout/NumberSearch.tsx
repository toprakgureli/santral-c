// NumberSearch: the box in the top bar that answers "who spoke with this
// number". Type a number, and a card lists the agents who handled it with
// their call counts and talk time, then the calls themselves, newest
// first. Agents who may only see their own calls get their own.

import { useEffect, useRef, useState } from "react";
import { PhoneIncoming, PhoneOutgoing, Search, X } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { CallLookup } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import UserAvatar from "@/components/ui/UserAvatar";
import { canAny } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { CallDisposition, formatClock } from "@/pages/callFormat";
import { displayNumber } from "@/softphone/dial";

function stamp(iso: string) {
  const d = new Date(iso);
  return `${d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" })} ${d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}`;
}

function minutes(seconds: number) {
  if (seconds < 60) return `${seconds} sn`;
  const m = Math.round(seconds / 60);
  return m < 60 ? `${m} dk` : `${Math.floor(m / 60)} sa ${String(m % 60).padStart(2, "0")} dk`;
}

export default function NumberSearch() {
  const { user } = useAuth();
  const allowed = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own", "call.originate"]);
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<CallLookup | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  // Search a moment after typing stops.
  useEffect(() => {
    const digits = q.replace(/\D/g, "");
    if (digits.length < 3) {
      setResult(null);
      setError(null);
      return;
    }
    let live = true;
    setBusy(true);
    const t = window.setTimeout(() => {
      api
        .callLookup(q)
        .then((r) => {
          if (!live) return;
          setResult(r);
          setError(null);
        })
        .catch((e) => live && setError(e instanceof ApiError ? e.message : "Arama yapılamadı."))
        .finally(() => live && setBusy(false));
    }, 350);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [q]);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (!allowed) return null;
  const showCard = open && q.replace(/\D/g, "").length >= 3;

  return (
    <div ref={box} className="relative hidden md:block">
      <div className={cn("flex h-9 w-56 items-center gap-2 rounded-xl border bg-muted/40 px-3 transition-[width,box-shadow,border-color] focus-within:w-72 focus-within:border-ring/60 focus-within:bg-card focus-within:ring-4 focus-within:ring-ring/15", open ? "border-ring/40" : "border-border/60")}>
        <Search className="size-4 shrink-0 text-muted-foreground" />
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => setOpen(true)}
          placeholder="Numara ara"
          inputMode="tel"
          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground/60"
        />
        {q && (
          <button type="button" onClick={() => { setQ(""); setResult(null); }} aria-label="Temizle" className="rounded-md p-0.5 text-muted-foreground hover:text-foreground">
            <X className="size-3.5" />
          </button>
        )}
      </div>

      {showCard && (
        <div className="animate-in fade-in slide-in-from-top-1 absolute right-0 mt-2 w-[min(26rem,calc(100vw-2rem))] overflow-hidden rounded-2xl border border-border bg-popover text-popover-foreground shadow-xl duration-150">
          {error ? (
            <p className="px-4 py-3 text-sm text-destructive">{error}</p>
          ) : !result ? (
            <p className="px-4 py-3 text-sm text-muted-foreground">{busy ? "Aranıyor..." : "Bekleniyor..."}</p>
          ) : result.total === 0 ? (
            <p className="px-4 py-3 text-sm text-muted-foreground">Son {result.days} günde bu numarayla {result.scope === "own" ? "görüşmen" : "görüşme"} yok.</p>
          ) : (
            <>
              <div className="border-b border-border/60 bg-muted/30 px-4 py-2.5">
                <p className="font-mono text-sm font-semibold">{displayNumber(result.number) || result.number}</p>
                <p className="text-xs text-muted-foreground">
                  Son {result.days} gün · {result.total} çağrı · {result.answered} görüşme · {minutes(result.talkSeconds)} konuşma
                  {result.scope === "own" && " · yalnızca senin çağrıların"}
                </p>
              </div>
              <ul className="border-b border-border/60 px-2 py-1.5">
                {result.agents.map((a) => (
                  <li key={a.id} className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm">
                    <UserAvatar userId={a.id} name={a.name} className="size-7" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
                    <span className="min-w-0 flex-1 truncate font-medium">{a.name}</span>
                    <span className="text-xs tabular-nums text-muted-foreground">{a.answered}/{a.calls} görüşme · {minutes(a.talkSeconds)}</span>
                  </li>
                ))}
              </ul>
              <ul className="max-h-72 overflow-y-auto px-2 py-1.5">
                {result.items.map((c) => (
                  <li key={c.uuid} className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-xs">
                    {c.direction === "inbound" ? <PhoneIncoming className="size-3.5 shrink-0 text-success" /> : <PhoneOutgoing className="size-3.5 shrink-0 text-primary" />}
                    <span className="w-24 shrink-0 tabular-nums text-muted-foreground">{stamp(c.startedAt)}</span>
                    <span className="min-w-0 flex-1 truncate font-medium">{c.agentName}</span>
                    <CallDisposition value={c.disposition} />
                    <span className="w-12 shrink-0 text-right font-mono tabular-nums">{c.disposition === "answered" ? formatClock(c.durationSeconds) : "—"}</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </div>
      )}
    </div>
  );
}
