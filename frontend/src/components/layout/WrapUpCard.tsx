// WrapUpCard asks for an escalation after every answered call. The moment a
// conversation ends it comes to the front on whichever page the agent is on
// and stays until the escalation is saved or the agent says none was
// needed. Only real conversations count: missed, cancelled and unanswered
// calls never open it, and neither does a call the PBX "answered" only to
// play a rejection or busy announcement, which is why answered calls under
// MIN_SECONDS (the panel's own valid-call threshold) are skipped too.
// Pending wrap-ups survive a reload and wait while a new call is in progress.

import { useEffect, useState } from "react";
import { PhoneIncoming, PhoneOutgoing, TriangleAlert } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { EscalationCategory, EscalationRecord } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import { EscalationForm } from "@/components/escalation/EscalationForm";
import { Badge, Button } from "@/components/ui";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { displayNumber } from "@/softphone/dial";
import { useSoftphoneContext, type EndedCall } from "@/softphone/SoftphoneContext";

const KEY = "santral.wrapup.pending";
// A call shorter than this is not a conversation (same 30 s rule as Geçerli çağrı).
const MIN_SECONDS = 30;
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
  const [history, setHistory] = useState<EscalationRecord[]>([]);
  const historyCount = history.length;
  const [confirmSkip, setConfirmSkip] = useState(false);
  const [skipping, setSkipping] = useState(false);
  const [skipError, setSkipError] = useState<string | null>(null);

  // Queue every answered call as it ends.
  useEffect(() => {
    const c = phone.lastEnded;
    if (!c || !allowed || isWrapUpDone(c.id)) return;
    if ((c.endedAt - c.answeredAt) / 1000 < MIN_SECONDS) return;
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
    setSkipError(null);
    setHistory([]);
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

  // "No escalation needed" is a record too: the customer's history shows who
  // looked at the call and decided so.
  async function skip() {
    setSkipping(true);
    setSkipError(null);
    try {
      await api.logNoEscalation({ number, callUuid: current!.id });
      done();
    } catch (e) {
      setSkipError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setSkipping(false);
    }
  }

  return (
    <div className="fixed inset-0 z-[55] flex items-center justify-center bg-background/70 p-4 backdrop-blur-md">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="wrapup-title"
        className="animate-in fade-in zoom-in-95 flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-3xl border-2 border-violet-500/40 bg-card shadow-2xl shadow-violet-500/10 duration-300"
      >
        <div className="relative flex items-center gap-3 border-b border-border/60 px-7 py-5">
          <div className="pointer-events-none absolute -right-10 -top-10 size-40 rounded-full bg-violet-500/15 blur-3xl" />
          <span className="flex size-11 items-center justify-center rounded-2xl bg-violet-500/15 text-violet-500">
            <TriangleAlert className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <h2 id="wrapup-title" className="text-lg font-semibold leading-tight">Görüşme bitti, eskalasyonu gir</h2>
            <p className="text-xs text-muted-foreground">Her geçerli çağrının (30 sn ve üstü) sonucu kaydedilir. Kayıt olmadan bu kart kapanmaz.</p>
          </div>
          {rest > 0 && <Badge tone="amber">+{rest} bekliyor</Badge>}
        </div>

        <div className="grid min-h-0 flex-1 overflow-hidden md:grid-cols-[19rem_1fr]">
          {/* Call facts and the customer's past escalations */}
          <div className="flex min-h-0 flex-col border-b border-border/60 bg-violet-500/5 px-6 py-5 md:border-b-0 md:border-r">
            <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Müşteri</div>
            <div className="mt-1 text-3xl font-bold tabular-nums tracking-wide">{number}</div>
            <ul className="mt-4 space-y-2.5 text-sm">
              <Fact label="Yön" value={inbound ? "Gelen çağrı" : "Giden çağrı"} icon={inbound ? <PhoneIncoming className="size-3.5 text-success" /> : <PhoneOutgoing className="size-3.5 text-violet-500" />} />
              <Fact label="Başlangıç" value={hhmm(current.answeredAt)} mono />
              <Fact label="Bitiş" value={hhmm(current.endedAt)} mono />
              <Fact label="Süre" value={duration((current.endedAt - current.answeredAt) / 1000)} mono />
              {canSearch && <Fact label="Geçmiş kayıt" value={historyCount === 0 ? "Yok" : `${historyCount} kayıt`} tone={historyCount ? "amber" : undefined} />}
            </ul>
            {canSearch && history.length > 0 && (
              <div className="mt-4 flex min-h-0 flex-1 flex-col">
                <div className="mb-1.5 text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Geçmiş görüşmeler</div>
                <ul className="min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1">
                  {history.map((h) => (
                    <li key={h.id} className="rounded-lg bg-card px-3 py-2 text-xs ring-1 ring-border/50">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate font-medium">{h.agentName}</span>
                        <span className="shrink-0 text-muted-foreground">{h.createdAt}</span>
                      </div>
                      <div className="mt-0.5 truncate">
                        <span className="text-warning">{h.categoryName}</span>
                        <span className="mx-1 text-muted-foreground">›</span>
                        <span className="text-muted-foreground">{h.reasonName}</span>
                      </div>
                      {h.note && <p className="mt-0.5 line-clamp-2 text-foreground/80">{h.note}</p>}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>

          {/* Form */}
          <div className="overflow-y-auto px-7 py-5">
            <EscalationForm
              categories={categories!}
              number={number}
              canSearch={canSearch}
              callUuid={current.id}
              onHistory={setHistory}
              onSaved={done}
              showHistory={false}
              pickerHeight={280}
              aside={
                <button
                  type="button"
                  onClick={() => setConfirmSkip(true)}
                  className={cn("text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline")}
                >
                  Eskalasyon gerekli değil
                </button>
              }
            />
          </div>
        </div>
      </div>

      {/* Are you sure? Skipping is a record too, so it deserves a stop. */}
      {confirmSkip && (
        <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/50 p-4" onClick={() => !skipping && setConfirmSkip(false)}>
          <div
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="skip-title"
            onClick={(e) => e.stopPropagation()}
            className="animate-in fade-in zoom-in-95 w-full max-w-sm rounded-2xl border border-border bg-card p-6 shadow-2xl duration-200"
          >
            <div className="flex items-start gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-warning/15 text-warning">
                <TriangleAlert className="size-5" />
              </span>
              <div className="space-y-1">
                <h3 id="skip-title" className="text-base font-semibold leading-tight">Emin misin?</h3>
                <p className="text-sm leading-relaxed text-muted-foreground">
                  <span className="font-mono tabular-nums text-foreground">{number}</span> numaralı müşteri için bu görüşme
                  {" "}<span className="font-medium text-foreground">"eskalasyon gerekli değil"</span> olarak senin adınla kaydedilecek.
                </p>
              </div>
            </div>
            {skipError && <p className="mt-3 text-xs text-destructive">{skipError}</p>}
            <div className="mt-5 flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setConfirmSkip(false)} disabled={skipping}>Vazgeç</Button>
              <Button onClick={skip} disabled={skipping} className="bg-warning text-black hover:bg-warning/90">
                {skipping ? "Kaydediliyor..." : "Evet, öyle işaretle"}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// Fact is one labelled line in the call facts list.
function Fact({ label, value, icon, mono, tone }: { label: string; value: string; icon?: React.ReactNode; mono?: boolean; tone?: "amber" }) {
  return (
    <li className="flex items-center justify-between gap-3">
      <span className="flex items-center gap-1.5 text-muted-foreground">
        {icon}
        {label}
      </span>
      <span className={cn("font-medium", mono && "font-mono tabular-nums", tone === "amber" && "text-warning")}>{value}</span>
    </li>
  );
}
