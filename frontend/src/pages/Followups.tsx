// Followups ("Geri Dönüşler"): the customers the team still owes a call. A
// call out that did not get through puts the number here; the row closes
// by itself once anyone has a real conversation with that number, a
// colleague calling it or the customer calling in. "Ben arıyorum" holds a
// row for half an hour so two people never call the same customer at once.

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, Clock3, Phone, PhoneMissed, PhoneOutgoing, RotateCcw, UserRound, XCircle } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button } from "@/components/ui";
import { IconChip } from "@/components/ui/rows";
import { followApi, REASON_LABEL, talkLabel, type Unreached } from "@/followups/api";
import { clockTime, isToday, shortMonthDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { useShift } from "@/shift/ShiftContext";
import { displayNumber, normalizeDial } from "@/softphone/dial";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const REFRESH_MS = 30000;

function when(iso: string): string {
  return isToday(iso) ? `Bugün ${clockTime(iso)}` : `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

export function Followups() {
  const { can } = useAuth();
  const seesAll = can("call.view_all") || can("cdr.view_all");
  const [all, setAll] = useState(false);
  const [closed, setClosed] = useState(false);
  const [rows, setRows] = useState<Unreached[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<number | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await followApi.unreached(all, closed));
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Liste alınamadı.");
    }
  }, [all, closed]);

  useEffect(() => {
    setRows(null);
    void load();
    const t = window.setInterval(() => void load(), REFRESH_MS);
    return () => window.clearInterval(t);
  }, [load]);

  const act = async (id: number, fn: () => Promise<void>) => {
    setBusy(id);
    setError(null);
    try {
      await fn();
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
      await load();
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <IconChip icon={PhoneMissed} tone="destructive" size="lg" />
        <div className="min-w-0 flex-1">
          <h1 className="text-lg font-semibold tracking-tight">Geri Dönüşler</h1>
          <p className="text-xs text-muted-foreground">Ulaşılamayan müşteriler. Biri o numarayla görüşünce (arayarak ya da müşteri arayınca) kayıt kendiliğinden kapanır ve ulaşamayan kişiye haber verilir.</p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {seesAll && (
          <Segment value={all ? "all" : "mine"} onChange={(v) => setAll(v === "all")} options={[["mine", "Benim"], ["all", "Ekip"]]} />
        )}
        <Segment value={closed ? "closed" : "open"} onChange={(v) => setClosed(v === "closed")} options={[["open", "Bekleyenler"], ["closed", "Son 7 günde kapananlar"]]} />
        {rows && <span className="ml-auto text-xs text-muted-foreground">{rows.length} kayıt</span>}
      </div>

      {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}

      {rows === null ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : rows.length === 0 ? (
        <div className="rounded-2xl bg-card px-6 py-14 text-center ring-1 ring-border/60">
          <CheckCircle2 className="mx-auto mb-2 size-8 text-success/60" />
          <p className="text-sm font-medium">{closed ? "Son 7 günde kapanan kayıt yok" : "Geri dönülecek müşteri yok"}</p>
          {!closed && <p className="mt-1 text-xs text-muted-foreground">Ulaşamadığın bir arama olursa burada görünür; biri ulaşınca kendiliğinden kapanır.</p>}
        </div>
      ) : (
        <div className="space-y-2">
          {rows.map((r) => <Row key={r.id} r={r} team={all} busy={busy === r.id} act={act} />)}
        </div>
      )}
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

function Row({ r, team, busy, act }: { r: Unreached; team: boolean; busy: boolean; act: (id: number, fn: () => Promise<void>) => Promise<void> }) {
  const { can } = useAuth();
  const phone = useSoftphoneContext();
  const shift = useShift();
  const canDial = can("call.originate") && shift.active && phone.status === "registered";
  const open = r.status === "open";
  const heldByOther = r.claim && !r.claim.mine;

  const callNow = () =>
    void act(r.id, async () => {
      // Calling holds the row so nobody else calls at the same time.
      await followApi.claim(r.id).catch(() => undefined);
      await phone.call(normalizeDial(r.number));
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
          {can("call.originate") && (
            <Button onClick={callNow} disabled={!canDial || busy || !!heldByOther} data-tip={!shift.active ? "Aramak için mesaini başlat" : heldByOther ? `${r.claim?.by.name} arıyor` : undefined}>
              <Phone /> Ara
            </Button>
          )}
          {r.claim?.mine ? (
            <Button variant="secondary" onClick={() => void act(r.id, () => followApi.unclaim(r.id))} disabled={busy}><RotateCcw /> Bırak</Button>
          ) : (
            <Button variant="secondary" onClick={() => void act(r.id, () => followApi.claim(r.id))} disabled={busy || !!heldByOther}><PhoneOutgoing /> Ben arıyorum</Button>
          )}
          <Button variant="ghost" onClick={() => void act(r.id, () => followApi.drop(r.id))} disabled={busy} data-tip="Geri aramaya gerek yok; kayıt listeden kalkar">Gerek kalmadı</Button>
        </div>
      )}
    </div>
  );
}
