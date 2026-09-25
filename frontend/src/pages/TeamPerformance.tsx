// Ekip Performansı: every agent the viewer may see as a card, with live
// status and the figures of the chosen days. The server scopes the list by
// permission (performance.view_all: everyone, performance.view_role: agents
// sharing a role with the viewer); the page only renders what it gets.
//
// Every card has the same rows in the same places, so the eye finds a
// figure by position: who and what now, the shift, reached against
// unreached, then the four rates. "Ulaşılan" are real conversations
// (answered, 30 seconds or longer), "Ulaşılamayan" are calls that never
// connected plus the ones too short to count. No call appears in both.

import { useEffect, useMemo, useState } from "react";
import { ArrowUpDown, ChevronDown, Clock, PhoneIncoming, PhoneOutgoing, Users } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { TeamRow, TeamStatus } from "../api/types";
import { Badge, Card, EmptyState, Select, Skeleton } from "../components/ui";
import RangePicker, { useRange } from "../components/RangePicker";
import { rangeLabel, ymd } from "../lib/dateRange";
import { displayNumber } from "../softphone/dial";
import { cn } from "../lib/utils";
import UserAvatar from "../components/ui/UserAvatar";
import { Link } from "react-router-dom";
import { formatClock } from "./callFormat";

const REFRESH_MS = 15000;

// One colour per status, used the same way everywhere on the card: the
// dot on the photo, the ring around it, the header tint and the badge.
const STATUS: Record<TeamStatus, { label: string; tone: "green" | "amber" | "red" | "slate" | "blue"; dot: string; ring: string; tint: string }> = {
  talking: { label: "Görüşmede", tone: "blue", dot: "bg-primary", ring: "ring-primary/60", tint: "from-primary/12" },
  available: { label: "Boşta", tone: "green", dot: "bg-success", ring: "ring-success/60", tint: "from-success/12" },
  break: { label: "Molada", tone: "amber", dot: "bg-warning", ring: "ring-warning/60", tint: "from-warning/14" },
  backoffice: { label: "Backoffice", tone: "amber", dot: "bg-amber-600", ring: "ring-amber-600/60", tint: "from-amber-600/12" },
  dnd: { label: "Rahatsız etmeyin", tone: "red", dot: "bg-destructive", ring: "ring-destructive/60", tint: "from-destructive/12" },
  unregistered: { label: "Kayıtsız", tone: "slate", dot: "bg-muted-foreground/50", ring: "ring-muted-foreground/30", tint: "from-muted/60" },
  off: { label: "Mesai dışı", tone: "slate", dot: "bg-muted-foreground/40", ring: "ring-border", tint: "from-muted/40" },
};

type SortKey = "long" | "reach" | "talkSeconds" | "occupancy" | "unanswered" | "escalations" | "shift" | "name";

const SORTS: { key: SortKey; label: string }[] = [
  { key: "long", label: "Gerçek çağrıya göre" },
  { key: "reach", label: "Ulaşma oranına göre" },
  { key: "talkSeconds", label: "Görüşme süresine göre" },
  { key: "occupancy", label: "Yoğunluğa göre" },
  { key: "unanswered", label: "Ulaşılamayana göre" },
  { key: "escalations", label: "Eskalasyona göre" },
  { key: "shift", label: "Mesai süresine göre" },
  { key: "name", label: "İsme göre" },
];

// Derived rates, computed once per row.
function unreachedOf(r: TeamRow) {
  return r.calls.unanswered + r.calls.short;
}
function reachOf(r: TeamRow) {
  const attempts = r.calls.long + unreachedOf(r);
  return attempts > 0 ? r.calls.long / attempts : -1;
}
function occupancyOf(r: TeamRow) {
  return r.shift.seconds > 0 ? Math.min(1, r.calls.talkSeconds / r.shift.seconds) : -1;
}
function pct(v: number) {
  return v < 0 ? "—" : `%${Math.round(v * 100)}`;
}

// forHow says how long a state has held: "12 dk", "1 sa 05 dk", "2 gün".
function forHow(iso: string | undefined, now: number): string | null {
  if (!iso) return null;
  const s = Math.max(0, Math.floor((now - Date.parse(iso)) / 1000));
  if (Number.isNaN(s)) return null;
  const m = Math.floor(s / 60);
  if (m < 1) return "az önce";
  if (m < 60) return `${m} dk`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} sa ${String(m % 60).padStart(2, "0")} dk`;
  return `${Math.floor(h / 24)} gün`;
}

function hhmm(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

// stamp is hh:mm, prefixed with the day when the range spans several days.
function stamp(iso: string, withDay: boolean) {
  const d = new Date(iso);
  const t = d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  return withDay ? `${d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" })} ${t}` : t;
}

// short is a compact duration for the small cells: "42 dk", "3 sa 05 dk".
function short(seconds: number) {
  const m = Math.round(seconds / 60);
  if (m < 60) return `${m} dk`;
  return `${Math.floor(m / 60)} sa ${String(m % 60).padStart(2, "0")} dk`;
}

export function TeamPerformance() {
  const [rows, setRows] = useState<TeamRow[]>([]);
  const [scope, setScope] = useState<"all" | "role">("role");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<SortKey>("long");
  const [now, setNow] = useState(() => Date.now());
  const { preset, range, choose, setFrom, setTo } = useRange("today");
  const { from, to } = range;
  const isToday = from === ymd(new Date()) && to === from;

  useEffect(() => {
    if (!from || !to || to < from) return;
    let live = true;
    const load = () =>
      api
        .performanceToday({ from, to })
        .then((r) => {
          if (!live) return;
          setRows(r.items);
          setScope(r.scope);
          setError(null);
        })
        .catch((e) => live && setError(e instanceof ApiError ? e.message : "Ekip verisi alınamadı."))
        .finally(() => live && setLoading(false));
    setLoading(true);
    load();
    const timer = window.setInterval(load, REFRESH_MS);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [from, to]);

  // A one-second clock so the call timers move between refreshes.
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  const sorted = useMemo(() => {
    const value = (r: TeamRow) => {
      switch (sort) {
        case "shift": return r.shift.seconds;
        case "name": return 0;
        case "reach": return reachOf(r);
        case "occupancy": return occupancyOf(r);
        case "unanswered": return unreachedOf(r);
        case "escalations": return r.escalations;
        default: return r.calls[sort];
      }
    };
    return [...rows].sort((a, b) => {
      const oa = a.status !== "off" ? 0 : 1;
      const ob = b.status !== "off" ? 0 : 1;
      if (oa !== ob) return oa - ob;
      return value(b) - value(a) || a.name.localeCompare(b.name, "tr");
    });
  }, [rows, sort]);

  const totals = useMemo(() => {
    const t = { onShift: 0, talking: 0, real: 0, inReal: 0, outReal: 0, unanswered: 0, short: 0, talk: 0, shift: 0, escalations: 0 };
    for (const r of rows) {
      if (r.status !== "off") t.onShift += 1;
      if (r.status === "talking") t.talking += 1;
      t.real += r.calls.long;
      t.inReal += r.calls.inboundReal;
      t.outReal += r.calls.outboundReal;
      t.unanswered += r.calls.unanswered;
      t.short += r.calls.short;
      t.talk += r.calls.talkSeconds;
      t.shift += r.shift.seconds;
      t.escalations += r.escalations;
    }
    return t;
  }, [rows]);
  const teamAttempts = totals.real + totals.unanswered + totals.short;

  return (
    <div className="space-y-4">
      {/* Team strip, same shape as the dashboard's status bar. */}
      <div className="rounded-2xl bg-card px-5 py-3 ring-1 ring-border/60">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex flex-wrap items-center gap-5 text-sm">
            <Stat dot="bg-success" label="Mesaide" value={String(totals.onShift)} />
            <Stat dot="bg-primary" label="Görüşmede" value={String(totals.talking)} />
            <span className="hidden h-5 w-px bg-border sm:block" />
            <Stat dot="bg-success" label="Gerçek çağrı" value={String(totals.real)} hint={`${totals.inReal} gelen · ${totals.outReal} giden · 30 saniye ve üstü`} />
            <Stat dot="bg-destructive" label="Ulaşılamayan" value={String(totals.unanswered + totals.short)} hint={`${totals.unanswered} cevapsız · ${totals.short} geçersiz`} />
            <Stat dot="bg-primary/60" label="Ulaşma" value={teamAttempts > 0 ? `%${Math.round((totals.real / teamAttempts) * 100)}` : "—"} hint="Gerçek çağrı / tüm denemeler" />
            <span className="hidden h-5 w-px bg-border sm:block" />
            <Stat dot="bg-violet-500" label="Yoğunluk" value={totals.shift > 0 ? `%${Math.round((totals.talk / totals.shift) * 100)}` : "—"} hint="Görüşme süresi / mesai süresi" />
            <Stat dot="bg-warning" label="Eskalasyon" value={String(totals.escalations)} hint="Ekibin kaydettiği eskalasyonlar" />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="hidden text-xs text-muted-foreground md:inline">
              {scope === "all" ? "Tüm ekip" : "Kendi rolündekiler"} · {rangeLabel(from, to)}
            </span>
            <RangePicker preset={preset} range={range} onPreset={choose} onFrom={setFrom} onTo={setTo} />
            <label className="relative" title="Sıralama">
              <ArrowUpDown className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Select value={sort} onChange={(e) => setSort(e.target.value as SortKey)} className="h-9 w-52 pl-9">
                {SORTS.map((s) => (
                  <option key={s.key} value={s.key}>{s.label}</option>
                ))}
              </Select>
            </label>
          </div>
        </div>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}
      {!isToday && !loading && (
        <p className="text-xs text-muted-foreground">Durumlar anlık, çağrı ve mesai rakamları seçilen tarihleri kapsıyor.</p>
      )}

      {loading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <Skeleton key={i} className="h-72 w-full rounded-2xl" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState icon={<Users />} title="Görüntülenecek temsilci yok" description="Dahilisi olan aktif kullanıcı bulunamadı ya da rolünüzle eşleşen kimse yok." />
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {sorted.map((r) => (
            <AgentCard key={r.userId} row={r} now={now} live={isToday} multiDay={from !== to} />
          ))}
        </div>
      )}
    </div>
  );
}

function AgentCard({ row: r, now, live, multiDay }: { row: TeamRow; now: number; live: boolean; multiDay: boolean }) {
  const s = STATUS[r.status] ?? STATUS.off;
  const off = r.status === "off";
  const [more, setMore] = useState(false);
  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;
  const unreached = unreachedOf(r);
  const reach = reachOf(r);
  const occupancy = occupancyOf(r);
  // How long the current state has held: the call's start while talking,
  // the last shift end when off, the presence change otherwise.
  const stateSince = r.status === "talking" ? r.call?.startedAt : off ? r.shift.lastEnd : r.since;
  const held = live ? forHow(stateSince, now) : null;

  return (
    <section className={cn("flex flex-col rounded-2xl bg-card shadow-sm ring-1 ring-border/60 transition", off && "opacity-60")}>
      {/* 1. Who, and what they are doing right now */}
      <header className={cn("flex items-start justify-between gap-3 rounded-t-2xl border-b border-border/60 bg-gradient-to-r to-transparent px-5 py-3.5", s.tint)}>
        <div className="flex min-w-0 items-center gap-3">
          <UserAvatar userId={r.userId} name={r.name} className={cn("size-10 rounded-full ring-2 ring-offset-2 ring-offset-card", s.ring)} fallbackClassName="bg-primary/10 text-sm text-primary">
            <span className={cn("absolute -right-0.5 -bottom-0.5 size-3 rounded-full ring-2 ring-card", s.dot, (r.status === "available" || r.status === "talking") && "animate-pulse")} />
          </UserAvatar>
          <div className="min-w-0">
            <Link to={`/profile/${r.userId}`} className="block truncate font-semibold leading-tight hover:underline" title="Profili aç">{r.name}</Link>
            <div className="truncate text-xs text-muted-foreground">
              {r.extension}
              {r.roles.length > 0 && <span> · {r.roles.join(", ")}</span>}
            </div>
          </div>
        </div>
        <span className="flex shrink-0 flex-col items-end gap-0.5">
          <Badge tone={s.tone}>{s.label}{held ? ` · ${held}` : ""}</Badge>
          {held && stateSince && <span className="text-[0.65rem] tabular-nums text-muted-foreground/70">{hhmm(stateSince)}&apos;den beri</span>}
        </span>
      </header>

      <div className="space-y-2.5 px-5 py-3.5">
        {/* 2. Live line, always the same height: the call, or what the state means */}
        <div className="flex h-5 items-center gap-1.5 text-xs text-muted-foreground">
          {r.call ? (
            <>
              {r.call.direction === "inbound" ? <PhoneIncoming className="size-3.5 text-success" /> : <PhoneOutgoing className="size-3.5 text-primary" />}
              <span className="truncate font-mono text-foreground">{displayNumber(r.call.peer) || r.call.peer}</span>
              {r.call.peerName && <span className="truncate">{r.call.peerName}</span>}
              <span className="ml-auto font-mono tabular-nums text-foreground">{formatClock(callFor)}</span>
            </>
          ) : (
            <span className="truncate">
              {!live ? "Seçilen tarihlerin rakamları" : off ? "Mesaide değil" : r.status === "available" ? "Çağrı bekliyor" : r.status === "break" ? "Molada, çağrı almıyor" : r.status === "unregistered" ? "Telefon bağlı değil" : "Çağrı almıyor"}
            </span>
          )}
        </div>

        {/* 3. Shift strip: start, end, total, break */}
        <div className="flex items-center gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-2">
          <Clock className="size-3.5 shrink-0 text-muted-foreground" />
          {r.shift.firstStart ? (
            <div className="grid flex-1 grid-cols-4 gap-2">
              <ShiftCell label={multiDay ? "İlk giriş" : "Giriş"} value={stamp(r.shift.firstStart, multiDay)} />
              <ShiftCell
                label={multiDay ? "Son çıkış" : "Çıkış"}
                value={r.shift.open ? "devam" : r.shift.lastEnd ? stamp(r.shift.lastEnd, multiDay) : "—"}
                tone={r.shift.open ? "live" : undefined}
              />
              <ShiftCell label="Mesai" value={r.shift.seconds > 0 ? short(r.shift.seconds) : "—"} strong />
              <ShiftCell label="Mola" value={r.breakSeconds > 0 ? short(r.breakSeconds) : "—"} tone={r.breakSeconds > 0 ? "warn" : undefined} />
            </div>
          ) : (
            <span className="text-xs text-muted-foreground">{live ? "Mesai başlatılmadı" : "Seçilen tarihlerde mesai kaydı yok"}</span>
          )}
        </div>

        {/* 4. Reached against unreached: two equal blocks, big number left, breakdown right */}
        <div className="grid grid-cols-2 gap-2">
          <Block tone="green" label="Ulaşılan" value={r.calls.long} sub="30 sn ve üstü">
            <Line icon={<PhoneIncoming className="size-3" />} label="Gelen" value={r.calls.inboundReal} />
            <Line icon={<PhoneOutgoing className="size-3" />} label="Giden" value={r.calls.outboundReal} />
          </Block>
          <Block tone="red" label="Ulaşılamayan" value={unreached} sub="bağlanmayan + kısa">
            <Line label="Cevapsız" value={r.calls.unanswered} hint={`${r.calls.inboundMissed} gelen · ${r.calls.outboundMissed} giden`} />
            <Line label="Geçersiz" value={r.calls.short} hint="30 sn altı" />
          </Block>
        </div>

        {/* 5. Reach bar: the share of attempts that became a conversation */}
        <div className="flex items-center gap-2 text-[0.7rem]" title="Gerçek çağrı / tüm denemeler">
          <span className="w-16 shrink-0 text-muted-foreground">Ulaşma</span>
          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-destructive/20">
            <div className="h-full rounded-full bg-success transition-[width]" style={{ width: `${reach < 0 ? 0 : reach * 100}%` }} />
          </div>
          <span className="w-10 shrink-0 text-right font-semibold tabular-nums">{pct(reach)}</span>
        </div>

        {/* 6. Four rates, always in this order */}
        <div className="grid grid-cols-4 gap-2">
          <Rate label="Görüşme" value={r.calls.talkSeconds > 0 ? short(r.calls.talkSeconds) : "—"} hint="Cevaplanan çağrıların toplam süresi" />
          <Rate label="Ortalama" value={r.calls.avgTalkSeconds > 0 ? formatClock(r.calls.avgTalkSeconds) : "—"} hint={`Gerçek çağrı ortalaması · en uzun ${r.calls.longestSeconds > 0 ? formatClock(r.calls.longestSeconds) : "—"}`} />
          <Rate label="Yoğunluk" value={pct(occupancy)} hint="Görüşme süresi / mesai süresi" tone={occupancy >= 0.5 ? "text-violet-500" : undefined} />
          <Rate label="Eskalasyon" value={String(r.escalations)} hint="Kaydettiği eskalasyonlar" tone={r.escalations > 0 ? "text-warning" : undefined} />
        </div>

        {/* 7. The fold: the latest calls, closed until asked */}
        <div className="rounded-xl border border-border/60">
          <button
            type="button"
            onClick={() => setMore((v) => !v)}
            className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-xs font-medium text-muted-foreground hover:bg-accent/60 hover:text-foreground"
          >
            <span>Son görüşmeler{r.recent.length > 0 && !more && r.recent[0] ? <span className="ml-1.5 font-normal">· en son {displayNumber(r.recent[0].peer) || r.recent[0].peer}{r.recent[0].peerName ? ` (${r.recent[0].peerName})` : ""}, {stamp(r.recent[0].startedAt, true)}</span> : null}</span>
            <ChevronDown className={cn("size-3.5 shrink-0 transition-transform", more && "rotate-180")} />
          </button>
          {more && (
            <ul className="border-t border-border/60 px-2 py-1.5">
              {r.recent.length === 0 && <li className="px-1 py-1.5 text-xs text-muted-foreground">Seçilen tarihlerde çağrı yok.</li>}
              {r.recent.map((c, i) => (
                <li key={i} className="flex items-center gap-2 rounded-lg px-1 py-1 text-xs">
                  {c.direction === "inbound" ? <PhoneIncoming className="size-3.5 shrink-0 text-success" /> : <PhoneOutgoing className="size-3.5 shrink-0 text-primary" />}
                  <span className="w-[4.5rem] shrink-0 tabular-nums text-muted-foreground">{stamp(c.startedAt, true)}</span>
                  <span className="min-w-0 flex-1 truncate">
                    <span className="font-mono">{displayNumber(c.peer) || c.peer}</span>
                    {c.peerName && <span className="text-muted-foreground"> · {c.peerName}</span>}
                  </span>
                  <span className={cn("w-12 shrink-0 text-right font-mono tabular-nums", c.disposition === "answered" ? (c.durationSeconds >= 30 ? "text-success" : "text-warning") : "text-destructive")}>
                    {c.disposition === "answered" ? formatClock(c.durationSeconds) : "cevapsız"}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </section>
  );
}

// ShiftCell is one labelled value in the card's shift strip.
function ShiftCell({ label, value, tone, strong }: { label: string; value: string; tone?: "live" | "warn"; strong?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="text-[0.6rem] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className={cn("truncate text-xs tabular-nums", tone === "live" ? "font-medium text-success" : tone === "warn" ? "font-medium text-warning" : "font-mono", strong && "font-semibold text-foreground")}>{value}</div>
    </div>
  );
}

// Block is one of the two call blocks: a big total with its breakdown.
function Block({ tone, label, value, sub, children }: { tone: "green" | "red"; label: string; value: number; sub: string; children: React.ReactNode }) {
  return (
    <div className={cn("rounded-xl border p-2.5", tone === "green" ? "border-success/30 bg-success/5" : "border-destructive/25 bg-destructive/5")}>
      <div className="flex items-baseline justify-between gap-2">
        <span className={cn("text-[0.7rem] font-semibold", tone === "green" ? "text-success" : "text-destructive")}>{label}</span>
        <span className={cn("text-2xl font-bold leading-none tabular-nums", tone === "green" ? "text-success" : "text-destructive")}>{value}</span>
      </div>
      <div className="text-[0.6rem] text-muted-foreground">{sub}</div>
      <div className="mt-1.5 space-y-0.5 border-t border-border/40 pt-1.5">{children}</div>
    </div>
  );
}

function Line({ icon, label, value, hint }: { icon?: React.ReactNode; label: string; value: number; hint?: string }) {
  return (
    <div className="flex items-center gap-1.5 text-[0.7rem]" title={hint}>
      {icon && <span className="text-muted-foreground">{icon}</span>}
      <span className="text-muted-foreground">{label}</span>
      <span className="ml-auto font-semibold tabular-nums">{value}</span>
    </div>
  );
}

function Rate({ label, value, hint, tone }: { label: string; value: string; hint: string; tone?: string }) {
  return (
    <div className="rounded-lg bg-muted/25 px-2 py-1.5 text-center ring-1 ring-border/40" title={hint}>
      <div className={cn("truncate text-sm font-semibold tabular-nums", tone)}>{value}</div>
      <div className="text-[0.6rem] text-muted-foreground">{label}</div>
    </div>
  );
}

function Stat({ dot, label, value, hint }: { dot: string; label: string; value: string; hint?: string }) {
  return (
    <span className="flex items-center gap-2" title={hint}>
      <span className={`size-2 rounded-full ${dot}`} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
    </span>
  );
}
