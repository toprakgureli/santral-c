// Ekip Performansı: every agent the viewer may see as a card, with live
// status and today's figures. The server scopes the list by permission
// (performance.view_all: everyone, performance.view_role: agents sharing a
// role with the viewer); the page only renders what it gets.
//
// Each card mirrors the dashboard's two blocks: "Ulaşılanlar" are real
// conversations (answered, 30 seconds or longer) split by direction,
// "Ulaşılamayanlar" are calls that never connected plus the ones too short
// to count. No call appears in both.

import { useEffect, useMemo, useState } from "react";
import { ArrowUpDown, CalendarRange, ChevronDown, Clock, PhoneIncoming, PhoneOutgoing, Users } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { TeamRow, TeamStatus } from "../api/types";
import { Badge, Card, DateField, EmptyState, Select, Skeleton } from "../components/ui";
import { displayNumber } from "../softphone/dial";
import { cn } from "../lib/utils";
import UserAvatar from "../components/ui/UserAvatar";
import { Link } from "react-router-dom";
import { formatClock } from "./callFormat";

const REFRESH_MS = 15000;

const STATUS: Record<TeamStatus, { label: string; tone: "green" | "amber" | "red" | "slate" | "blue"; dot: string }> = {
  talking: { label: "Görüşmede", tone: "blue", dot: "bg-primary" },
  available: { label: "Boşta", tone: "green", dot: "bg-success" },
  break: { label: "Molada", tone: "amber", dot: "bg-warning" },
  backoffice: { label: "Backoffice", tone: "amber", dot: "bg-warning" },
  dnd: { label: "Rahatsız etmeyin", tone: "red", dot: "bg-destructive" },
  unregistered: { label: "Kayıtsız", tone: "slate", dot: "bg-muted-foreground/50" },
  off: { label: "Mesai dışı", tone: "slate", dot: "bg-muted-foreground/40" },
};

type SortKey = "long" | "talkSeconds" | "unanswered" | "shift" | "name";

const SORTS: { key: SortKey; label: string }[] = [
  { key: "long", label: "Gerçek çağrıya göre" },
  { key: "talkSeconds", label: "Görüşme süresine göre" },
  { key: "unanswered", label: "Cevapsıza göre" },
  { key: "shift", label: "Mesai süresine göre" },
  { key: "name", label: "İsme göre" },
];

// ymd formats a Date as a local YYYY-MM-DD.
function ymd(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

type Preset = "today" | "yesterday" | "last7" | "last30" | "month" | "day" | "custom";

const PRESETS: { key: Preset; label: string }[] = [
  { key: "today", label: "Bugün" },
  { key: "yesterday", label: "Dün" },
  { key: "last7", label: "Son 7 gün" },
  { key: "last30", label: "Son 30 gün" },
  { key: "month", label: "Bu ay" },
  { key: "day", label: "Belirli gün" },
  { key: "custom", label: "Tarih aralığı" },
];

// presetRange resolves a preset to an inclusive local [from, to].
function presetRange(key: Preset): { from: string; to: string } {
  const now = new Date();
  const today = ymd(now);
  const shift = (days: number) => ymd(new Date(now.getFullYear(), now.getMonth(), now.getDate() - days));
  switch (key) {
    case "yesterday": return { from: shift(1), to: shift(1) };
    case "last7": return { from: shift(6), to: today };
    case "last30": return { from: shift(29), to: today };
    case "month": return { from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: today };
    default: return { from: today, to: today };
  }
}

function dmy(iso: string) {
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
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

export function TeamPerformance() {
  const [rows, setRows] = useState<TeamRow[]>([]);
  const [scope, setScope] = useState<"all" | "role">("role");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<SortKey>("long");
  const [now, setNow] = useState(() => Date.now());
  const [preset, setPreset] = useState<Preset>("today");
  const [from, setFrom] = useState(() => ymd(new Date()));
  const [to, setTo] = useState(() => ymd(new Date()));
  const isToday = from === ymd(new Date()) && to === from;

  function choosePreset(key: Preset) {
    setPreset(key);
    if (key !== "day" && key !== "custom") {
      const r = presetRange(key);
      setFrom(r.from);
      setTo(r.to);
    } else if (key === "day") {
      setTo(from);
    }
  }

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
    const value = (r: TeamRow) => (sort === "shift" ? r.shift.seconds : sort === "name" ? 0 : r.calls[sort]);
    return [...rows].sort((a, b) => {
      const oa = a.status !== "off" ? 0 : 1;
      const ob = b.status !== "off" ? 0 : 1;
      if (oa !== ob) return oa - ob;
      return value(b) - value(a) || a.name.localeCompare(b.name, "tr");
    });
  }, [rows, sort]);

  const totals = useMemo(() => {
    const t = { onShift: 0, talking: 0, real: 0, inReal: 0, outReal: 0, unanswered: 0, short: 0 };
    for (const r of rows) {
      if (r.status !== "off") t.onShift += 1;
      if (r.status === "talking") t.talking += 1;
      t.real += r.calls.long;
      t.inReal += r.calls.inboundReal;
      t.outReal += r.calls.outboundReal;
      t.unanswered += r.calls.unanswered;
      t.short += r.calls.short;
    }
    return t;
  }, [rows]);

  return (
    <div className="space-y-4">
      {/* Team strip, same shape as the dashboard's status bar. */}
      <div className="rounded-2xl bg-card px-5 py-3 ring-1 ring-border/60">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex flex-wrap items-center gap-5 text-sm">
            <Stat dot="bg-success" label="Mesaide" value={totals.onShift} />
            <Stat dot="bg-primary" label="Görüşmede" value={totals.talking} />
            <span className="hidden h-5 w-px bg-border sm:block" />
            <Stat dot="bg-success" label="Gerçek çağrı" value={totals.real} hint="Ekip toplamı, 30 saniye ve üstü görüşmeler" />
            <Stat dot="bg-success/60" label="Gelen" value={totals.inReal} hint="Gerçek çağrı olan gelenler" />
            <Stat dot="bg-success/60" label="Giden" value={totals.outReal} hint="Gerçek çağrı olan gidenler" />
            <span className="hidden h-5 w-px bg-border sm:block" />
            <Stat dot="bg-destructive" label="Cevapsız" value={totals.unanswered} hint="Hiç bağlanmayan çağrılar" />
            <Stat dot="bg-warning" label="Geçersiz" value={totals.short} hint="Bağlanıp 30 saniye dolmayanlar" />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="hidden text-xs text-muted-foreground md:inline">
              {scope === "all" ? "Tüm ekip" : "Kendi rolündekiler"} · {isToday ? "bugün" : from === to ? dmy(from) : `${dmy(from)} - ${dmy(to)}`}
            </span>
            <label className="relative" title="Tarih">
              <CalendarRange className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Select value={preset} onChange={(e) => choosePreset(e.target.value as Preset)} className="h-9 w-40 pl-9">
                {PRESETS.map((p) => (
                  <option key={p.key} value={p.key}>{p.label}</option>
                ))}
              </Select>
            </label>
            {preset === "day" && (
              <DateField value={from} max={ymd(new Date())} onChange={(v) => { setFrom(v); setTo(v); }} className="w-40 [&>input]:h-9" title="Gün" />
            )}
            {preset === "custom" && (
              <div className="flex items-center gap-1">
                <DateField value={from} max={to || undefined} onChange={setFrom} className="w-40 [&>input]:h-9" title="Başlangıç" />
                <span className="text-muted-foreground">-</span>
                <DateField value={to} min={from || undefined} max={ymd(new Date())} onChange={setTo} className="w-40 [&>input]:h-9" title="Bitiş" />
              </div>
            )}
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
        <p className="text-xs text-muted-foreground">Durumlar ve mesai başlangıcı anlık, çağrı rakamları seçilen tarihleri kapsıyor.</p>
      )}

      {loading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <Skeleton key={i} className="h-56 w-full rounded-2xl" />
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
  const [showMissed, setShowMissed] = useState(false);
  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;
  const unreached = r.calls.unanswered + r.calls.short;
  // How long the current state has held: the call's start while talking,
  // the last shift end when off, the presence change otherwise.
  const stateSince = r.status === "talking" ? r.call?.startedAt : off ? r.shift.lastEnd : r.since;
  const held = live ? forHow(stateSince, now) : null;

  return (
    <section className={cn("flex flex-col rounded-2xl bg-card shadow-sm ring-1 ring-border/60 transition", off && "opacity-60")}>
      {/* Header: who, and what they are doing right now */}
      <header className="flex items-start justify-between gap-3 border-b border-border/60 px-5 py-4">
        <div className="flex min-w-0 items-center gap-3">
          <UserAvatar userId={r.userId} name={r.name} className="size-10" fallbackClassName="bg-primary/10 text-sm text-primary">
            <span className={cn("absolute -right-0.5 -bottom-0.5 size-3 rounded-full ring-2 ring-card", s.dot, r.status === "available" && "animate-pulse")} />
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

      <div className="space-y-3 px-5 py-4">
        {/* Live line: current call or how long in the current state */}
        {r.call && (
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="flex min-w-0 items-center gap-1.5 text-foreground">
              {r.call.direction === "inbound" ? <PhoneIncoming className="size-3.5 text-success" /> : <PhoneOutgoing className="size-3.5 text-primary" />}
              <span className="truncate font-mono">{displayNumber(r.call.peer) || r.call.peer}</span>
              {r.call.peerName && <span className="truncate text-muted-foreground">{r.call.peerName}</span>}
              <span className="font-mono tabular-nums text-muted-foreground">{formatClock(callFor)}</span>
            </span>
          </div>
        )}

        {/* Shift strip: start, end and total inside the range */}
        <div className="flex items-center gap-3 rounded-xl border border-border/60 bg-muted/20 px-3 py-2">
          <Clock className="size-3.5 shrink-0 text-muted-foreground" />
          {r.shift.firstStart ? (
            <div className="grid flex-1 grid-cols-3 gap-2">
              <ShiftCell label={multiDay ? "İlk başlangıç" : "Başlangıç"} value={stamp(r.shift.firstStart, multiDay)} />
              <ShiftCell
                label={multiDay ? "Son bitiş" : "Bitiş"}
                value={r.shift.open ? "devam ediyor" : r.shift.lastEnd ? stamp(r.shift.lastEnd, multiDay) : "—"}
                tone={r.shift.open ? "live" : undefined}
              />
              <ShiftCell label={multiDay ? "Toplam mesai" : "Mesai süresi"} value={r.shift.seconds > 0 ? formatClock(r.shift.seconds) : "—"} strong />
            </div>
          ) : (
            <span className="text-xs text-muted-foreground">{live ? "Mesai başlatılmadı" : "Seçilen tarihlerde mesai kaydı yok"}</span>
          )}
        </div>

        {/* Reached: real conversations */}
        <div className="rounded-xl border border-success/30 bg-success/5 p-2">
          <div className="mb-1.5 px-1 text-[0.7rem] font-semibold text-success">Ulaşılanlar</div>
          <div className="grid grid-cols-3 gap-2">
            <Tile label="Gerçek çağrı" sub="30 sn ve üstü" value={r.calls.long} tone="green" big />
            <Tile label="Gelen" sub="gerçek" value={r.calls.inboundReal} tone="green" />
            <Tile label="Giden" sub="gerçek" value={r.calls.outboundReal} tone="green" />
          </div>
        </div>

        {/* Unreached: never connected or too short */}
        <div className="rounded-xl border border-border/60 bg-muted/20 p-2">
          <button
            type="button"
            onClick={() => setShowMissed((v) => !v)}
            className="flex w-full items-center justify-between px-1 text-[0.7rem] font-semibold text-muted-foreground hover:text-foreground"
          >
            <span>
              Ulaşılamayanlar · {unreached}
              <span className="ml-1 font-normal">({r.calls.unanswered} cevapsız, {r.calls.short} geçersiz)</span>
            </span>
            <ChevronDown className={cn("size-3.5 transition-transform", showMissed && "rotate-180")} />
          </button>
          {showMissed && (
            <div className="mt-1.5 grid grid-cols-4 gap-2">
              <Tile label="Cevapsız" sub="bağlanmadı" value={r.calls.unanswered} tone="slate" />
              <Tile label="Gelen" sub="cevapsız" value={r.calls.inboundMissed} tone="slate" />
              <Tile label="Giden" sub="cevapsız" value={r.calls.outboundMissed} tone="slate" />
              <Tile label="Geçersiz" sub="30 sn altı" value={r.calls.short} tone="slate" />
            </div>
          )}
        </div>

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>Toplam görüşme</span>
          <span className="font-mono font-semibold tabular-nums text-foreground">{formatClock(r.calls.talkSeconds)}</span>
        </div>
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>Ortalama görüşme <span className="text-muted-foreground/60">(gerçek çağrı)</span></span>
          <span className="font-mono font-semibold tabular-nums text-foreground">{r.calls.avgTalkSeconds > 0 ? formatClock(r.calls.avgTalkSeconds) : "—"}</span>
        </div>
      </div>
    </section>
  );
}

// ShiftCell is one labelled value in the card's shift strip.
function ShiftCell({ label, value, tone, strong }: { label: string; value: string; tone?: "live"; strong?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="text-[0.65rem] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className={cn("truncate text-xs tabular-nums", tone === "live" ? "font-medium text-success" : "font-mono", strong && "font-semibold text-foreground")}>{value}</div>
    </div>
  );
}

function Tile({ label, sub, value, tone, big }: { label: string; sub?: string; value: number; tone: "green" | "slate"; big?: boolean }) {
  return (
    <div className="rounded-lg bg-card/70 px-2 py-1.5 text-center ring-1 ring-border/40">
      <div className={cn("font-bold tabular-nums leading-none", big ? "text-2xl" : "text-lg", tone === "green" ? "text-success" : "text-muted-foreground")}>{value}</div>
      <div className="mt-1 text-[0.65rem] font-medium text-foreground/80">{label}</div>
      {sub && <div className="text-[0.6rem] text-muted-foreground">{sub}</div>}
    </div>
  );
}

function Stat({ dot, label, value, hint }: { dot: string; label: string; value: number; hint?: string }) {
  return (
    <span className="flex items-center gap-2" title={hint}>
      <span className={`size-2 rounded-full ${dot}`} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
    </span>
  );
}
