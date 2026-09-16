// WrapUpCard asks for an escalation after every answered call. The moment a
// conversation ends it comes to the front on whichever page the agent is on
// and stays until the escalation is saved or the agent says none was
// needed. Only calls that were actually answered count; missed, cancelled
// and unanswered calls never open it. Pending wrap-ups survive a reload and
// wait while a new call is in progress.

import { useEffect, useState } from "react";
import { PhoneIncoming, PhoneOutgoing, TriangleAlert } from "lucide-react";
import { api } from "@/api/client";
import type { EscalationCategory } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import { EscalationForm } from "@/components/escalation/EscalationForm";
import { Badge } from "@/components/ui";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { displayNumber } from "@/softphone/dial";
import { useSoftphoneContext, type EndedCall } from "@/softphone/SoftphoneContext";

const KEY = "santral.wrapup.pending";
const DONE_KEY = "santral.wrapup.done";

// markWrapUpDone records that an escalation was already entered for a call
// (from the dashboard, mid-call), so the wrap-up card does not ask again.
export function markWrapUpDone(callId: string) {
  try {
    const raw = window.sessionStorage.getItem(DONE_KEY);
    const ids = raw ? (JSON.parse(raw) as string[]) : [];
    if (!ids.includes(callId)) ids.push(callId);
    window.sessionStorage.setItem(DONE_KEY, JSON.stringify(ids.slice(-50)));
  } catch {
    // storage unavailable; the card may ask once more, which is harmless
  }
}

function isWrapUpDone(callId: string): boolean {
  try {
    const raw = window.sessionStorage.getItem(DONE_KEY);
    return raw ? (JSON.parse(raw) as string[]).includes(callId) : false;
  } catch {
    return false;
  }
}

function readPending(): EndedCall[] {
  try {
    const raw = window.sessionStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as EndedCall[]) : [];
  } catch {
    return [];
  }
}

function writePending(items: EndedCall[]) {
  try {
    window.sessionStorage.setItem(KEY, JSON.stringify(items));
  } catch {
    // storage unavailable; the list still lives in memory for this page
  }
}

function hhmm(ms: number) {
  return new Date(ms).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

function duration(seconds: number) {
  const s = Math.max(0, Math.floor(seconds));
  const m = Math.floor(s / 60);
  const sec = s % 60;
  if (m === 0) return `${sec} sn`;
  return sec > 0 ? `${m} dk ${sec} sn` : `${m} dk`;
}

export default function WrapUpCard() {
  const { user } = useAuth();
  const phone = useSoftphoneContext();
  const allowed = can(user, "escalation.view");
  const canSearch = can(user, "escalation.search");
  const [pending, setPending] = useState<EndedCall[]>(readPending);
  const [categories, setCategories] = useState<EscalationCategory[] | null>(null);
  const [historyCount, setHistoryCount] = useState(0);
  const [confirmSkip, setConfirmSkip] = useState(false);

  // Queue every answered call as it ends.
  useEffect(() => {
    const c = phone.lastEnded;
    if (!c || !allowed || isWrapUpDone(c.id)) return;
    setPending((list) => {
      if (list.some((p) => p.id === c.id)) return list;
      const next = [...list, c];
      writePending(next);
      return next;
    });
  }, [phone.lastEnded, allowed]);

  useEffect(() => {
    if (!allowed) return;
    api.escalationCategories().then(setCategories).catch(() => setCategories([]));
  }, [allowed]);

  const current = pending[0];
  const busy = ["calling", "ringing", "incoming", "in-call", "held"].includes(phone.status);
  const show = allowed && !!current && !busy && categories !== null && categories.length > 0;

  useEffect(() => {
    setConfirmSkip(false);
    setHistoryCount(0);
  }, [current?.id]);

  if (!show || !current) return null;

  function done() {
    setPending((list) => {
      const next = list.filter((p) => p.id !== current!.id);
      writePending(next);
      return next;
    });
  }

  const number = displayNumber(current.peer) || current.peer;
  const inbound = current.direction === "inbound";
  const rest = pending.length - 1;

  return (
    <div className="fixed inset-0 z-[55] flex items-center justify-center bg-background/70 p-4 backdrop-blur-md">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="wrapup-title"
        className="animate-in fade-in zoom-in-95 flex max-h-[92vh] w-full max-w-lg flex-col overflow-hidden rounded-3xl border-2 border-primary/40 bg-card shadow-2xl shadow-primary/10 duration-300"
      >
        <div className="relative px-7 pt-6 pb-5">
          <div className="pointer-events-none absolute -right-10 -top-10 size-40 rounded-full bg-primary/15 blur-3xl" />
          <div className="flex items-center gap-3">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/15 text-primary">
              <TriangleAlert className="size-5" />
            </span>
            <div className="min-w-0">
              <h2 id="wrapup-title" className="text-lg font-semibold leading-tight">Görüşme bitti, eskalasyonu gir</h2>
              <p className="text-xs text-muted-foreground">Her cevaplanan çağrının sonucu kaydedilir. Kayıt olmadan bu kart kapanmaz.</p>
            </div>
            {rest > 0 && <Badge tone="amber">+{rest} bekliyor</Badge>}
          </div>

          <div className="mt-5 flex items-center justify-between gap-3 rounded-2xl bg-primary/10 px-4 py-3 ring-1 ring-primary/30">
            <div className="min-w-0">
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                {inbound ? <PhoneIncoming className="size-3.5 text-success" /> : <PhoneOutgoing className="size-3.5 text-primary" />}
                {inbound ? "Gelen çağrı" : "Giden çağrı"}
                <span>·</span>
                <span className="font-mono tabular-nums">{hhmm(current.answeredAt)} - {hhmm(current.endedAt)}</span>
                <span>·</span>
                <span className="font-mono tabular-nums">{duration((current.endedAt - current.answeredAt) / 1000)}</span>
              </div>
              <div className="mt-0.5 text-2xl font-bold tabular-nums tracking-wide">{number}</div>
            </div>
            {canSearch && <Badge tone={historyCount ? "amber" : "slate"}>{historyCount} geçmiş kayıt</Badge>}
          </div>
        </div>

        <div className="flex-1 overflow-y-auto border-t border-border/60 px-7 py-5">
          <EscalationForm
            categories={categories!}
            number={number}
            canSearch={canSearch}
            callUuid={current.id}
            onHistory={(items) => setHistoryCount(items.length)}
            onSaved={done}
            aside={
              confirmSkip ? (
                <span className="flex items-center gap-2 text-xs">
                  <span className="text-muted-foreground">Kayıt girilmeyecek, emin misin?</span>
                  <button type="button" onClick={done} className="font-medium text-destructive hover:underline">Evet, geç</button>
                  <button type="button" onClick={() => setConfirmSkip(false)} className="text-muted-foreground hover:underline">Vazgeç</button>
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => setConfirmSkip(true)}
                  className={cn("text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline")}
                >
                  Eskalasyon gerektirmiyor
                </button>
              )
            }
          />
        </div>
      </div>
    </div>
  );
}
