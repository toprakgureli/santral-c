// ReminderWatcher brings a planned call back to the front once it is due,
// on whichever page is open: call now, snooze a quarter of an hour, or mark
// it done. Calling also snoozes it, so if the customer does not answer it
// comes back; a real conversation with the number closes it on the server.
// A call back that fell due while the panel was closed shows on opening.

import { useCallback, useEffect, useState } from "react";
import { AlarmClock, Check, Clock3, Phone } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import { Button } from "@/components/ui";
import { announceFollowupsChanged, FOLLOWUPS_CHANGED, followApi, type Reminder } from "@/followups/api";
import { clockTime, isToday, shortMonthDate } from "@/lib/time";
import { useShift } from "@/shift/ShiftContext";
import { displayNumber, normalizeDial } from "@/softphone/dial";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const POLL_MS = 30000;
const SNOOZE_MIN = 15;

function when(iso: string): string {
  return isToday(iso) ? clockTime(iso) : `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

export default function ReminderWatcher() {
  const { can } = useAuth();
  const allowed = ["call.originate", "call.view_own", "cdr.view_own", "call.view_all", "cdr.view_all"].some((p) => can(p));
  const [items, setItems] = useState<Reminder[]>([]);
  const [now, setNow] = useState(() => Date.now());
  const [busy, setBusy] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      setItems(await followApi.reminders(false, false));
    } catch {
      // the next poll tries again
    }
  }, []);

  useEffect(() => {
    if (!allowed) return;
    void load();
    const poll = window.setInterval(() => void load(), POLL_MS);
    const tick = window.setInterval(() => setNow(Date.now()), 15000);
    const changed = () => void load();
    window.addEventListener(FOLLOWUPS_CHANGED, changed);
    return () => {
      window.clearInterval(poll);
      window.clearInterval(tick);
      window.removeEventListener(FOLLOWUPS_CHANGED, changed);
    };
  }, [allowed, load]);

  if (!allowed) return null;
  const due = items.filter((r) => Date.parse(r.dueAt) <= now).slice(0, 3);
  if (due.length === 0) return null;

  const act = async (id: number, fn: () => Promise<unknown>) => {
    setBusy(id);
    try {
      await fn();
    } catch {
      // the list read below shows what is true
    } finally {
      setBusy(null);
      announceFollowupsChanged();
    }
  };

  return (
    <div className="fixed bottom-5 left-5 z-[60] flex w-[22rem] max-w-[calc(100vw-2.5rem)] flex-col gap-2">
      {due.map((r) => <Card key={r.id} r={r} busy={busy === r.id} act={act} />)}
    </div>
  );
}

function Card({ r, busy, act }: { r: Reminder; busy: boolean; act: (id: number, fn: () => Promise<unknown>) => Promise<void> }) {
  const { can } = useAuth();
  const phone = useSoftphoneContext();
  const shift = useShift();
  const canDial = can("call.originate") && shift.active && phone.status === "registered";
  return (
    <div role="alert" className="animate-in fade-in slide-in-from-bottom-2 rounded-2xl border border-warning/40 bg-popover p-3.5 text-popover-foreground shadow-xl duration-200">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-warning/14 text-warning"><AlarmClock className="size-5" /></span>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium text-warning">Geri arama zamanı · {when(r.dueAt)}</p>
          <p className="font-mono text-base font-semibold tabular-nums">{displayNumber(r.number) || r.number}</p>
          {r.note && <p className="mt-0.5 line-clamp-2 text-sm text-foreground/80">{r.note}</p>}
          {r.snoozes > 0 && <p className="mt-0.5 text-xs text-muted-foreground">{r.snoozes} kez ertelendi</p>}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {can("call.originate") && (
          <Button className="h-9 flex-1" disabled={!canDial || busy} onClick={() => void act(r.id, async () => {
            await followApi.snooze(r.id, SNOOZE_MIN);
            await phone.call(normalizeDial(r.number));
          })} data-tip={!shift.active ? "Aramak için mesaini başlat" : undefined}>
            <Phone /> Ara
          </Button>
        )}
        <Button variant="secondary" className="h-9" disabled={busy} onClick={() => void act(r.id, () => followApi.snooze(r.id, SNOOZE_MIN))}><Clock3 /> 15 dk</Button>
        <Button variant="secondary" className="h-9" disabled={busy} onClick={() => void act(r.id, () => followApi.doneReminder(r.id))}><Check /> Tamamlandı</Button>
      </div>
    </div>
  );
}
