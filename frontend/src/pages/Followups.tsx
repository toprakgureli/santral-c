// Followups ("Geri Dönüşler"): the customers the team still owes a call, in
// two lists. Ulaşılamayanlar: a call out that did not get through puts the
// number here; the row closes by itself once anyone has a real conversation
// with that number, a colleague calling it or the customer calling in, and
// "Ben arıyorum" holds a row for half an hour so two people never call the
// same customer at once. Planlı aramalar: call backs promised for a time,
// which remind their owner on every page when due and close the same way.

import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { AlarmClock, Ban, Check, CheckCircle2, Clock3, Phone, PhoneMissed, PhoneOutgoing, Plus, RotateCcw, UserRound, XCircle } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button } from "@/components/ui";
import { IconChip } from "@/components/ui/rows";
import { announceFollowupsChanged, FOLLOWUPS_CHANGED, followApi, REASON_LABEL, talkLabel, type Reminder, type Unreached } from "@/followups/api";
import ScheduleCallback from "@/followups/ScheduleCallback";
import { clockTime, isToday, shortMonthDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { useShift } from "@/shift/ShiftContext";
import { displayNumber, normalizeDial } from "@/softphone/dial";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const REFRESH_MS = 30000;

function when(iso: string): string {
  return isToday(iso) ? `Bugün ${clockTime(iso)}` : `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

type Act = (id: number, fn: () => Promise<unknown>) => Promise<void>;

// useList reads one list now and every half minute, and runs actions on
// its rows with the list read again after each.
function useList<T>(read: () => Promise<T[]>) {
  const [rows, setRows] = useState<T[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const load = useCallback(async () => {
    try {
      setRows(await read());
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Liste alınamadı.");
    }
  }, [read]);
  useEffect(() => {
    setRows(null);
    void load();
    const t = window.setInterval(() => void load(), REFRESH_MS);
    const changed = () => void load();
    window.addEventListener(FOLLOWUPS_CHANGED, changed);
    return () => {
      window.clearInterval(t);
      window.removeEventListener(FOLLOWUPS_CHANGED, changed);
    };
  }, [load]);
  const act: Act = async (id, fn) => {
    setBusy(id);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
    } finally {
      setBusy(null);
      announceFollowupsChanged();
    }
  };
  return { rows, error, busy, act };
}

export function Followups() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "reminders" ? "reminders" : "unreached";
  const { can } = useAuth();
  const seesAll = can("call.view_all") || can("cdr.view_all");
  const [all, setAll] = useState(false);
  const [closed, setClosed] = useState(false);

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <IconChip icon={PhoneMissed} tone="destructive" size="lg" />
        <div className="min-w-0 flex-1">
          <h1 className="text-lg font-semibold tracking-tight">Geri Dönüşler</h1>
          <p className="text-xs text-muted-foreground">Geri dönülecek müşteriler. O numarayla kim görüşürse görüşsün (arayarak ya da müşteri arayınca) kayıt kendiliğinden kapanır ve ilgili kişiye haber verilir.</p>
        </div>
      </div>

      <div className="flex flex-wrap gap-1 border-b border-border/60">
        {([["unreached", "Ulaşılamayanlar", PhoneMissed], ["reminders", "Planlı aramalar", AlarmClock]] as const).map(([k, l, Icon]) => (
          <button key={k} type="button" onClick={() => setParams(k === "reminders" ? { tab: k } : {})}
            className={cn("-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors", tab === k ? "border-primary text-foreground" : "border-transparent text-muted-foreground hover:text-foreground")}>
            <Icon className="size-4" /> {l}
          </button>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {seesAll && <Segment value={all ? "all" : "mine"} onChange={(v) => setAll(v === "all")} options={[["mine", "Benim"], ["all", "Ekip"]]} />}
        <Segment value={closed ? "closed" : "open"} onChange={(v) => setClosed(v === "closed")} options={[["open", "Bekleyenler"], ["closed", "Son 7 günde kapananlar"]]} />
      </div>

      {tab === "unreached" ? <UnreachedList all={all} closed={closed} /> : <ReminderList all={all} done={closed} />}
    </div>
  );
}

function Segment({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: [string, string][] }) {
  return (
    <span className="flex rounded-xl bg-muted/60 p-0.5">
      {options.map(([k, l]) => (
        <button key={k} type="button" onClick={() => onChange(k)} className={cn("rounded-lg px-3 py-1.5 text-xs font-medium transition-colors", value === k ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>{l}</button>
      ))}
    </span>
  );
}

function Empty({ title, sub }: { title: string; sub?: string }) {
  return (
    <div className="rounded-2xl bg-card px-6 py-14 text-center ring-1 ring-border/60">
      <CheckCircle2 className="mx-auto mb-2 size-8 text-success/60" />
      <p className="text-sm font-medium">{title}</p>
      {sub && <p className="mt-1 text-xs text-muted-foreground">{sub}</p>}
    </div>
  );
}

// useDial says whether a call can be placed now, and places one.
function useDial() {
  const { can } = useAuth();
  const phone = useSoftphoneContext();
  const shift = useShift();
  return {
    allowed: can("call.originate"),
    ready: can("call.originate") && shift.active && phone.status === "registered",
    tip: shift.active ? undefined : "Aramak için mesaini başlat",
    call: (n: string) => phone.call(normalizeDial(n)),
  };
}

function UnreachedList({ all, closed }: { all: boolean; closed: boolean }) {
  const read = useCallback(() => followApi.unreached(all, closed), [all, closed]);
  const { rows, error, busy, act } = useList(read);
  const [plan, setPlan] = useState<string | null>(null);
  return (
    <>
      {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      {rows === null ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : rows.length === 0 ? (
        <Empty title={closed ? "Son 7 günde kapanan kayıt yok" : "Geri dönülecek müşteri yok"} sub={closed ? undefined : "Ulaşamadığın bir arama olursa burada görünür; biri ulaşınca kendiliğinden kapanır."} />
      ) : (
        <div className="space-y-2">
          <p className="px-1 text-xs text-muted-foreground">{rows.length} kayıt</p>
          {rows.map((r) => <UnreachedRow key={r.id} r={r} team={all} busy={busy === r.id} act={act} onPlan={() => setPlan(r.number)} />)}
        </div>
      )}
      {plan && <ScheduleCallback number={plan} onClose={() => setPlan(null)} />}
    </>
  );
}

function UnreachedRow({ r, team, busy, act, onPlan }: { r: Unreached; team: boolean; busy: boolean; act: Act; onPlan: () => void }) {
  const dial = useDial();
  const open = r.status === "open";
  const heldByOther = r.claim && !r.claim.mine;
  const callNow = () =>
    void act(r.id, async () => {
      // Calling holds the row so nobody else calls at the same time.
      await followApi.claim(r.id).catch(() => undefined);
      await dial.call(r.number);
    });

  return (
    <div className={cn("flex flex-wrap items-center gap-3 rounded-2xl bg-card p-3.5 ring-1 ring-border/60", busy && "opacity-60")}>
      <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl", open ? "bg-destructive/10 text-destructive" : r.status === "reached" ? "bg-success/12 text-success" : "bg-muted text-muted-foreground")}>
        {open ? <PhoneMissed className="size-5" /> : r.status === "reached" ? <CheckCircle2 className="size-5" /> : <XCircle className="size-5" />}
      </span>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="font-mono text-sm font-semibold tabular-nums">{displayNumber(r.number) || r.number}</span>
          {team && <span className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[0.7rem] text-muted-foreground"><UserRound className="size-3" />{r.mine ? "Sen" : r.user.name}</span>}
          <span className="rounded-full bg-muted px-2 py-0.5 text-[0.7rem] text-muted-foreground">{r.attempts} deneme · {REASON_LABEL[r.reason] ?? "Ulaşılamadı"}</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Son deneme {when(r.lastAt)}
          {r.attempts > 1 && ` · ilk deneme ${when(r.firstAt)}`}
        </p>
        {r.claim && open && (
          <p className={cn("flex items-center gap-1 text-xs font-medium", r.claim.mine ? "text-primary" : "text-warning")}>
            <PhoneOutgoing className="size-3.5" />
            {r.claim.mine ? "Sen arıyorsun" : `${r.claim.by.name} arıyor`} · {clockTime(r.claim.until)} olana kadar
          </p>
        )}
        {r.reached && (
          <p className="flex items-center gap-1 text-xs font-medium text-success">
            <CheckCircle2 className="size-3.5" />
            {r.reached.direction === "inbound" ? `Müşteri geri aradı, ${r.reached.by.name} ile` : `${r.reached.by.name} ulaştı,`} {talkLabel(r.reached.seconds)} görüştü · {when(r.reached.at)}
          </p>
        )}
        {r.status === "dropped" && (
          <p className="flex items-center gap-1 text-xs text-muted-foreground">
            <Clock3 className="size-3.5" />
            {r.droppedBy?.name ?? "Biri"} "gerek kalmadı" dedi{r.droppedAt ? ` · ${when(r.droppedAt)}` : ""}
          </p>
        )}
      </div>
      {open && (
        <div className="flex flex-wrap items-center gap-2">
          {dial.allowed && (
            <Button onClick={callNow} disabled={!dial.ready || busy || !!heldByOther} data-tip={heldByOther ? `${r.claim?.by.name} arıyor` : dial.tip}>
              <Phone /> Ara
            </Button>
          )}
          {r.claim?.mine ? (
            <Button variant="secondary" onClick={() => void act(r.id, () => followApi.unclaim(r.id))} disabled={busy}><RotateCcw /> Bırak</Button>
          ) : (
            <Button variant="secondary" onClick={() => void act(r.id, () => followApi.claim(r.id))} disabled={busy || !!heldByOther}><PhoneOutgoing /> Ben arıyorum</Button>
          )}
          <Button variant="secondary" onClick={onPlan} disabled={busy} data-tip="Belirli bir saatte hatırlatılsın"><AlarmClock /> Hatırlat</Button>
          <Button variant="ghost" onClick={() => void act(r.id, () => followApi.drop(r.id))} disabled={busy} data-tip="Geri aramaya gerek yok; kayıt listeden kalkar">Gerek kalmadı</Button>
        </div>
      )}
    </div>
  );
}

const DONE_LABEL: Record<string, string> = {
  done: "Arandı olarak kapatıldı",
  canceled: "Vazgeçildi",
  reached: "Görüşüldü",
};

function ReminderList({ all, done }: { all: boolean; done: boolean }) {
  const read = useCallback(() => followApi.reminders(all, done), [all, done]);
  const { rows, error, busy, act } = useList(read);
  const [adding, setAdding] = useState(false);
  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <p className="px-1 text-xs text-muted-foreground">{rows ? `${rows.length} kayıt` : ""}</p>
        <Button onClick={() => setAdding(true)}><Plus /> Geri arama planla</Button>
      </div>
      {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
      {rows === null ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : rows.length === 0 ? (
        <Empty title={done ? "Son 7 günde kapanan geri arama yok" : "Planlanmış geri arama yok"} sub={done ? undefined : "Çağrı sonrası kartından, çağrı geçmişinden ya da buradan bir müşteriye geri arama planlayabilirsin."} />
      ) : (
        <div className="space-y-2">
          {rows.map((r) => <ReminderRow key={r.id} r={r} team={all} busy={busy === r.id} act={act} />)}
        </div>
      )}
      {adding && <ScheduleCallback onClose={() => setAdding(false)} />}
    </>
  );
}

function ReminderRow({ r, team, busy, act }: { r: Reminder; team: boolean; busy: boolean; act: Act }) {
  const dial = useDial();
  const open = !r.doneAt;
  const late = open && Date.parse(r.dueAt) <= Date.now();
  return (
    <div className={cn("flex flex-wrap items-center gap-3 rounded-2xl bg-card p-3.5 ring-1", late ? "ring-warning/50" : "ring-border/60", busy && "opacity-60")}>
      <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-xl", !open ? "bg-success/12 text-success" : late ? "bg-warning/14 text-warning" : "bg-primary/10 text-primary")}>
        {open ? <AlarmClock className="size-5" /> : <CheckCircle2 className="size-5" />}
      </span>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="font-mono text-sm font-semibold tabular-nums">{displayNumber(r.number) || r.number}</span>
          {team && <span className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[0.7rem] text-muted-foreground"><UserRound className="size-3" />{r.mine ? "Sen" : r.user.name}</span>}
          <span className={cn("rounded-full px-2 py-0.5 text-[0.7rem] font-medium", late ? "bg-warning/14 text-warning" : "bg-muted text-muted-foreground")}>{late ? "Zamanı geldi · " : ""}{when(r.dueAt)}</span>
          {r.snoozes > 0 && <span className="text-[0.7rem] text-muted-foreground">{r.snoozes} kez ertelendi</span>}
        </div>
        {r.note && <p className="text-sm text-foreground/80">{r.note}</p>}
        {!open && (
          <p className="flex items-center gap-1 text-xs text-muted-foreground">
            <Check className="size-3.5" />
            {DONE_LABEL[r.doneReason ?? ""] ?? "Kapandı"}{r.doneBy ? ` · ${r.doneBy.name}` : ""}{r.doneAt ? ` · ${when(r.doneAt)}` : ""}
          </p>
        )}
      </div>
      {open && (
        <div className="flex flex-wrap items-center gap-2">
          {dial.allowed && (
            <Button onClick={() => void act(r.id, () => dial.call(r.number))} disabled={!dial.ready || busy} data-tip={dial.tip}><Phone /> Ara</Button>
          )}
          <Button variant="secondary" onClick={() => void act(r.id, () => followApi.doneReminder(r.id))} disabled={busy}><Check /> Tamamlandı</Button>
          <Button variant="ghost" onClick={() => void act(r.id, () => followApi.cancelReminder(r.id))} disabled={busy}><Ban /> Vazgeç</Button>
        </div>
      )}
    </div>
  );
}
