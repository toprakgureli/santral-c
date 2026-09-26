// Ekip Performansı: everyone the viewer may see, with live status and the
// figures of the chosen days. The server scopes the list by permission
// (performance.view_all: everyone, performance.view_role: agents sharing a
// role with the viewer); the page only renders what it gets.
//
// The page is built to be read at a glance. By default it is a list: one
// line per person with the same five columns, so people are compared
// straight down a column. A click on a line opens everything else about
// that person on the right. Cards stay available as a second view, each
// with one big number and three small ones. Colour is kept for the status
// and for a figure that crosses a line (reach under half); every other
// number stays neutral.
//
// "Gerçek çağrı" is an answered call of 30 seconds or longer. "Ulaşma" is
// real calls over all attempts (real, too short, unanswered).

import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ChevronRight, Clock, Coffee, Headset, LayoutGrid, List, Mic, MicOff, Phone, PhoneIncoming, PhoneMissed, PhoneOff, PhoneOutgoing, Share2, Star, TriangleAlert, Users, X, type LucideIcon } from "lucide-react";
import { Link } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { TeamRow, TeamStatus } from "../api/types";
import WhatsAppIcon from "../components/icons/WhatsAppIcon";
import AgentCallsDialog from "../components/performance/AgentCallsDialog";
import ShareDialog from "../components/performance/ShareDialog";
import RangePicker, { useRange } from "../components/RangePicker";
import { Card, EmptyState, Skeleton } from "../components/ui";
import UserAvatar from "../components/ui/UserAvatar";
import { rangeLabel, ymd } from "../lib/dateRange";
import { STATUS_COLOR } from "../lib/status";
import { cn } from "../lib/utils";
import { displayNumber } from "../softphone/dial";
import { formatClock } from "./callFormat";

const REFRESH_MS = 15000;
const VIEW_KEY = "santral.perf-view";

// One colour per status, used the same way everywhere: the bar on a card's
// edge, the dot on the photo and the status chip.
const STATUS: Record<TeamStatus, { label: string; icon: LucideIcon; dot: string; bar: string; chip: string }> = {
  talking: { ...STATUS_COLOR.talking, icon: Phone },
  available: { ...STATUS_COLOR.available, label: "Boşta", icon: Headset },
  break: { ...STATUS_COLOR.break, icon: Coffee },
  backoffice: { ...STATUS_COLOR.backoffice, icon: MicOff },
  dnd: { ...STATUS_COLOR.dnd, icon: PhoneOff },
  unregistered: { ...STATUS_COLOR.unregistered, icon: Mic },
  off: { ...STATUS_COLOR.off, icon: Clock },
};

type SortKey = "long" | "talkSeconds" | "reach" | "occupancy" | "name";

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
const lowReach = (v: number) => v >= 0 && v < 0.5;

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

// short is a compact duration: "42 dk", "3 sa 05 dk".
function short(seconds: number) {
  const m = Math.round(seconds / 60);
  if (m < 60) return `${m} dk`;
  return `${Math.floor(m / 60)} sa ${String(m % 60).padStart(2, "0")} dk`;
}

// liveLine is the sentence under a person's name: what they are doing now.
function liveLine(r: TeamRow, live: boolean) {
  if (!live) return "Seçilen tarihlerin rakamları";
  switch (r.status) {
    case "off": return "Mesai kapalı, çağrı yönlendirilmiyor";
    case "available": return "Hatta, sıradaki çağrıyı bekliyor";
    case "break": return "Molada, dönünce çağrı almaya devam edecek";
    case "backoffice": return "Backoffice işinde, çağrılar diğer temsilcilere düşüyor";
    case "dnd": return "Rahatsız etmeyin açık, çağrılar diğer temsilcilere düşüyor";
    case "unregistered": return "Telefonu bağlı değil, çağrı düşmüyor";
    default: return "Görüşmede";
  }
}

// stateSince is when the current state began: the call's start while
// talking, the last shift end when off, the presence change otherwise.
function stateSince(r: TeamRow) {
  return r.status === "talking" ? r.call?.startedAt : r.status === "off" ? r.shift.lastEnd : r.since;
}

export function TeamPerformance() {
  const [rows, setRows] = useState<TeamRow[]>([]);
  const [scope, setScope] = useState<"all" | "role">("role");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<SortKey>("long");
  const [share, setShare] = useState(false);
  const [openId, setOpenId] = useState<number | null>(null);
  const [view, setView] = useState<"list" | "cards">(() => {
    try {
      return localStorage.getItem(VIEW_KEY) === "cards" ? "cards" : "list";
    } catch {
      return "list";
    }
  });
  const [now, setNow] = useState(() => Date.now());
  const { preset, range, choose, setFrom, setTo } = useRange("today");
  const { from, to } = range;
  const isToday = from === ymd(new Date()) && to === from;
  const multiDay = from !== to;

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

  const chooseView = (v: "list" | "cards") => {
    setView(v);
    try {
      localStorage.setItem(VIEW_KEY, v);
    } catch {
      // storage unavailable
    }
  };

  const sorted = useMemo(() => {
    const value = (r: TeamRow) => {
      switch (sort) {
        case "name": return 0;
        case "reach": return reachOf(r);
        case "occupancy": return occupancyOf(r);
        case "talkSeconds": return r.calls.talkSeconds;
        default: return r.calls.long;
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
  const teamAttempts = totals.real + totals.unanswered + totals.short;
  const teamReach = teamAttempts > 0 ? totals.real / teamAttempts : -1;
  const maxReal = Math.max(1, ...rows.map((r) => r.calls.long));
  const hasWA = rows.some((r) => r.wa);
  const open = rows.find((r) => r.userId === openId) ?? null;

  return (
    <div className="space-y-4">
      {/* Four figures for the whole team, then the controls. */}
      <div className="flex flex-wrap items-stretch gap-3">
        <div className="grid min-w-0 flex-1 grid-cols-2 gap-3 md:grid-cols-4">
          <Figure label="Mesaide" value={String(totals.onShift)} note={`${rows.length} kişiden`} />
          <Figure label="Görüşmede" value={String(totals.talking)} note="şu an telefonda" dot={totals.talking > 0 ? "bg-destructive animate-pulse" : undefined} />
          <Figure label="Gerçek çağrı" value={String(totals.real)} note={`${totals.inReal} gelen · ${totals.outReal} giden`} hint="Cevaplanan, 30 saniye ve üstü çağrılar" />
          <Figure label="Ulaşma" value={pct(teamReach)} note={`${totals.unanswered + totals.short} ulaşılamayan`} hint="Gerçek çağrı / tüm denemeler" warn={lowReach(teamReach)} />
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-2xl bg-muted/40 p-2">
        <RangePicker preset={preset} range={range} onPreset={choose} onFrom={setFrom} onTo={setTo} />
        <span className="hidden text-xs text-muted-foreground lg:inline">{scope === "all" ? "Tüm ekip" : "Kendi rolündekiler"} · {rangeLabel(from, to)}</span>
        <span className="ml-auto flex items-center gap-2">
          <div className="flex rounded-xl bg-card p-0.5 ring-1 ring-border/60">
            <ViewBtn on={view === "list"} onClick={() => chooseView("list")} icon={List} label="Liste" />
            <ViewBtn on={view === "cards"} onClick={() => chooseView("cards")} icon={LayoutGrid} label="Kartlar" />
          </div>
          <button type="button" onClick={() => setShare(true)} disabled={rows.length === 0} data-tip="Performans görseli oluştur" className="flex h-9 items-center gap-2 rounded-xl border border-border/60 bg-card px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50">
            <Share2 className="size-4" /> Paylaş
          </button>
        </span>
      </div>

      {error && <p className="text-sm text-destructive">{error}</p>}
      {!isToday && !loading && <p className="text-xs text-muted-foreground">Durumlar anlık, çağrı ve mesai rakamları seçilen tarihleri kapsıyor.</p>}

      {loading && rows.length === 0 ? (
        <div className="space-y-2">{[0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-16 w-full rounded-2xl" />)}</div>
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState icon={<Users />} title="Görüntülenecek temsilci yok" description="Dahilisi olan aktif kullanıcı bulunamadı ya da rolünüzle eşleşen kimse yok." />
        </Card>
      ) : view === "list" ? (
        <section className="overflow-hidden rounded-2xl bg-card shadow-sm ring-1 ring-border/60">
          <div className={cn("hidden items-center gap-4 border-b border-border/60 px-4 py-2.5 text-[0.7rem] font-medium text-muted-foreground md:grid", hasWA ? "md:grid-cols-[minmax(14rem,1.6fr)_1.2fr_1fr_1fr_1fr_1fr_1.25rem]" : "md:grid-cols-[minmax(14rem,1.6fr)_1.2fr_1fr_1fr_1fr_1.25rem]")}>
            <SortHead on={sort === "name"} onClick={() => setSort("name")}>Kişi</SortHead>
            <SortHead on={sort === "long"} onClick={() => setSort("long")} hint="Cevaplanan, 30 saniye ve üstü">Gerçek çağrı</SortHead>
            <SortHead on={sort === "talkSeconds"} onClick={() => setSort("talkSeconds")} hint="Cevaplanan çağrıların toplam süresi" right>Görüşme süresi</SortHead>
            <SortHead on={sort === "reach"} onClick={() => setSort("reach")} hint="Gerçek çağrı / tüm denemeler" right>Ulaşma</SortHead>
            <SortHead on={sort === "occupancy"} onClick={() => setSort("occupancy")} hint="Görüşme süresi / mesai süresi" right>Yoğunluk</SortHead>
            {hasWA && <span className="text-right" data-tip="WhatsApp'ta çözdüğü / aldığı sohbet">WhatsApp</span>}
            <span />
          </div>
          <ul className="divide-y divide-border/50">
            {sorted.map((r) => (
              <ListLine key={r.userId} r={r} now={now} live={isToday} maxReal={maxReal} hasWA={hasWA} active={r.userId === openId} onOpen={() => setOpenId(r.userId)} />
            ))}
          </ul>
        </section>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {sorted.map((r) => (
            <AgentCard key={r.userId} row={r} now={now} live={isToday} multiDay={multiDay} from={from} to={to} onOpen={() => setOpenId(r.userId)} />
          ))}
        </div>
      )}
      {open && <Detail r={open} now={now} live={isToday} multiDay={multiDay} from={from} to={to} onClose={() => setOpenId(null)} />}
      <ShareDialog open={share} rows={rows} rangeLabel={rangeLabel(from, to)} scopeLabel={scope === "all" ? "Tüm ekip" : "Kendi rolündekiler"} onClose={() => setShare(false)} />
    </div>
  );
}

function Figure({ label, value, note, hint, warn, dot }: { label: string; value: string; note: string; hint?: string; warn?: boolean; dot?: string }) {
  return (
    <div className="rounded-2xl bg-card px-4 py-3 shadow-sm ring-1 ring-border/60" data-tip={hint}>
      <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">{dot && <span className={cn("size-2 rounded-full", dot)} />}{label}</p>
      <p className={cn("mt-0.5 text-2xl font-semibold tracking-tight tabular-nums", warn && "text-destructive")}>{value}</p>
      <p className="truncate text-[0.7rem] text-muted-foreground">{note}</p>
    </div>
  );
}

function ViewBtn({ on, onClick, icon: Icon, label }: { on: boolean; onClick: () => void; icon: LucideIcon; label: string }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium transition-colors", on ? "bg-primary/10 text-primary" : "text-muted-foreground hover:text-foreground")}>
      <Icon className="size-3.5" /> {label}
    </button>
  );
}

function SortHead({ on, onClick, hint, right, children }: { on: boolean; onClick: () => void; hint?: string; right?: boolean; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} data-tip={hint} className={cn("flex items-center gap-1 transition-colors hover:text-foreground", right && "justify-end", on && "text-foreground")}>
      {children}
      {on && <ArrowDown className="size-3" />}
    </button>
  );
}

function Who({ r, now, live, size = "md" }: { r: TeamRow; now: number; live: boolean; size?: "md" | "lg" }) {
  const s = STATUS[r.status] ?? STATUS.off;
  const since = stateSince(r);
  const held = live ? forHow(since, now) : null;
  return (
    <span className="flex min-w-0 items-center gap-3">
      <UserAvatar userId={r.userId} name={r.name} className={size === "lg" ? "size-12" : "size-9"} fallbackClassName="bg-primary/10 text-xs text-primary">
        <span className={cn("absolute -right-0.5 -bottom-0.5 size-3 rounded-full ring-2 ring-card", s.dot, r.status === "talking" && "animate-pulse")} />
      </UserAvatar>
      <span className="min-w-0">
        <span className={cn("block truncate font-semibold leading-tight", size === "lg" ? "text-base" : "text-sm")}>{r.name}</span>
        <span className={cn("mt-0.5 inline-flex max-w-full items-center gap-1 rounded-md px-1.5 py-px text-[0.68rem] font-medium", s.chip)} data-tip={held && since ? `${hhmm(since)}'den beri` : undefined}>
          <s.icon className="size-3 shrink-0" />
          <span className="truncate">{s.label}{held ? ` · ${held}` : ""}</span>
        </span>
      </span>
    </span>
  );
}

function ListLine({ r, now, live, maxReal, hasWA, active, onOpen }: { r: TeamRow; now: number; live: boolean; maxReal: number; hasWA: boolean; active: boolean; onOpen: () => void }) {
  const reach = reachOf(r);
  const occupancy = occupancyOf(r);
  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className={cn(
          "grid w-full grid-cols-[1fr_auto_auto] items-center gap-4 px-4 py-2.5 text-left transition-colors hover:bg-accent/40",
          hasWA ? "md:grid-cols-[minmax(14rem,1.6fr)_1.2fr_1fr_1fr_1fr_1fr_1.25rem]" : "md:grid-cols-[minmax(14rem,1.6fr)_1.2fr_1fr_1fr_1fr_1.25rem]",
          active && "bg-primary/5",
          r.status === "off" && "opacity-60",
        )}
      >
        <Who r={r} now={now} live={live} />
        <span className="flex items-center gap-3">
          <span className="w-9 text-right text-lg font-semibold tabular-nums">{r.calls.long}</span>
          <span className="hidden h-1.5 flex-1 overflow-hidden rounded-full bg-muted md:block">
            <span className="block h-full rounded-full bg-foreground/40" style={{ width: `${(r.calls.long / maxReal) * 100}%` }} />
          </span>
        </span>
        <span className="hidden text-right text-sm tabular-nums md:block">{r.calls.talkSeconds + callFor > 0 ? short(r.calls.talkSeconds + callFor) : "—"}</span>
        <span className={cn("text-right text-sm font-medium tabular-nums", lowReach(reach) && "text-destructive")}>{pct(reach)}</span>
        <span className="hidden text-right text-sm tabular-nums md:block">{pct(occupancy)}</span>
        {hasWA && (
          <span className="hidden text-right text-sm tabular-nums md:block">{r.wa ? <>{r.wa.resolved}<span className="text-muted-foreground">/{r.wa.owned}</span></> : "—"}</span>
        )}
        <ChevronRight className="hidden size-4 text-muted-foreground md:block" />
      </button>
    </li>
  );
}

// AgentCard is the second view: one big number, three small ones.
export function AgentCard({ row: r, now, live, onOpen }: { row: TeamRow; now: number; live: boolean; multiDay?: boolean; from?: string; to?: string; onOpen?: () => void }) {
  const s = STATUS[r.status] ?? STATUS.off;
  const reach = reachOf(r);
  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;
  return (
    <section className={cn("relative flex flex-col overflow-hidden rounded-2xl bg-card shadow-sm ring-1 ring-border/60", r.status === "off" && "opacity-60")}>
      <span aria-hidden className={cn("absolute inset-y-3 left-0 w-1 rounded-r-full", s.bar)} />
      <div className="px-5 pt-4">
        <Who r={r} now={now} live={live} />
        <p className="mt-2 truncate text-xs text-muted-foreground">
          {r.call ? <span className="font-mono text-foreground">{displayNumber(r.call.peer) || r.call.peer} · {formatClock(callFor)}</span> : liveLine(r, live)}
        </p>
      </div>
      <div className="px-5 pt-3 pb-3">
        <p className="text-[0.7rem] font-medium text-muted-foreground">Gerçek çağrı</p>
        <p className="text-4xl font-semibold tracking-tight tabular-nums">{r.calls.long}</p>
        <p className="text-[0.7rem] text-muted-foreground">{r.calls.inboundReal} gelen · {r.calls.outboundReal} giden</p>
      </div>
      <div className="grid grid-cols-3 gap-1.5 px-3">
        <Small label="Görüşme" value={r.calls.talkSeconds + callFor > 0 ? short(r.calls.talkSeconds + callFor) : "—"} hint="Cevaplanan çağrıların toplam süresi" />
        <Small label="Ulaşma" value={pct(reach)} hint="Gerçek çağrı / tüm denemeler" warn={lowReach(reach)} />
        <Small label="Yoğunluk" value={pct(occupancyOf(r))} hint="Görüşme süresi / mesai süresi" />
      </div>
      {onOpen && (
        <button type="button" onClick={onOpen} className="mx-3 mt-2 mb-3 flex h-9 items-center justify-center gap-1 rounded-xl text-xs font-medium text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground">
          Detay <ChevronRight className="size-3.5" />
        </button>
      )}
      {!onOpen && <span className="h-3" />}
    </section>
  );
}

function Small({ label, value, hint, warn }: { label: string; value: string; hint: string; warn?: boolean }) {
  return (
    <div className="rounded-xl bg-muted/50 px-2 py-1.5 text-center" data-tip={hint}>
      <div className={cn("truncate text-sm font-semibold tabular-nums", warn && "text-destructive")}>{value}</div>
      <div className="text-[0.62rem] text-muted-foreground">{label}</div>
    </div>
  );
}

// Detail is everything about one person, opened from a line or a card.
function Detail({ r, now, live, multiDay, from, to, onClose }: { r: TeamRow; now: number; live: boolean; multiDay: boolean; from: string; to: string; onClose: () => void }) {
  const [calls, setCalls] = useState(false);
  const reach = reachOf(r);
  const occupancy = occupancyOf(r);
  const unreached = unreachedOf(r);
  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/30" onClick={onClose}>
      <aside onClick={(e) => e.stopPropagation()} className="animate-in slide-in-from-right-4 fade-in flex h-full w-full max-w-md flex-col overflow-y-auto bg-card shadow-2xl duration-200">
        <header className="sticky top-0 z-10 flex items-start gap-3 border-b border-border/60 bg-card/95 px-5 py-4 backdrop-blur">
          <div className="min-w-0 flex-1">
            <Who r={r} now={now} live={live} size="lg" />
            <p className="mt-2 text-xs text-muted-foreground">{r.extension}{r.roles.length > 0 && ` · ${r.roles.join(", ")}`}</p>
          </div>
          <button type="button" onClick={onClose} aria-label="Kapat" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-4" /></button>
        </header>

        <div className="space-y-5 px-5 py-4">
          <p className="flex items-center gap-2 rounded-xl bg-muted/40 px-3 py-2 text-sm">
            {r.call ? (r.call.direction === "inbound" ? <PhoneIncoming className="size-4 shrink-0 text-success" /> : <PhoneOutgoing className="size-4 shrink-0 text-primary" />) : <Headset className="size-4 shrink-0 text-muted-foreground" />}
            {r.call ? (
              <span className="min-w-0 flex-1 truncate"><span className="font-mono">{displayNumber(r.call.peer) || r.call.peer}</span>{r.call.peerName && ` · ${r.call.peerName}`}<span className="float-right font-mono tabular-nums">{formatClock(callFor)}</span></span>
            ) : (
              <span className="text-muted-foreground">{liveLine(r, live)}</span>
            )}
          </p>

          <Block title="Mesai">
            <Row label="Başlangıç ve bitiş" value={r.shift.firstStart ? <span className="font-mono">{stamp(r.shift.firstStart, multiDay)} → {r.shift.open ? <span className="font-sans text-success">devam ediyor</span> : r.shift.lastEnd ? stamp(r.shift.lastEnd, multiDay) : "—"}</span> : live ? "Başlatılmadı" : "Kayıt yok"} />
            <Row label="Mesai süresi" value={r.shift.seconds > 0 ? short(r.shift.seconds) : "—"} />
            <Row label="Mola" value={r.breakSeconds > 0 ? short(r.breakSeconds) : "—"} />
          </Block>

          <Block title="Çağrılar">
            <Row icon={PhoneIncoming} tone="success" label="Ulaşılan (gerçek çağrı)" note={`${r.calls.inboundReal} gelen · ${r.calls.outboundReal} giden · 30 sn ve üstü`} value={<b className="text-lg">{r.calls.long}</b>} />
            <Row icon={PhoneMissed} tone="destructive" label="Ulaşılamayan" note={`${r.calls.unanswered} cevapsız · ${r.calls.short} geçersiz`} value={<b className="text-lg">{unreached}</b>} />
            <div className="flex items-center gap-2 px-1 pt-1 text-xs" data-tip="Gerçek çağrı / tüm denemeler">
              <span className="w-16 text-muted-foreground">Ulaşma</span>
              <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted"><span className={cn("block h-full rounded-full", lowReach(reach) ? "bg-destructive" : "bg-success")} style={{ width: `${reach < 0 ? 0 : reach * 100}%` }} /></span>
              <b className={cn("w-10 text-right tabular-nums", lowReach(reach) && "text-destructive")}>{pct(reach)}</b>
            </div>
          </Block>

          <Block title="Süreler">
            <Row label="Toplam görüşme" note="Süren çağrı dahil" value={r.calls.talkSeconds + callFor > 0 ? short(r.calls.talkSeconds + callFor) : "—"} />
            <Row label="Ortalama görüşme" note="Gerçek çağrılarda" value={r.calls.avgTalkSeconds > 0 ? formatClock(r.calls.avgTalkSeconds) : "—"} />
            <Row label="En uzun görüşme" value={r.calls.longestSeconds > 0 ? formatClock(r.calls.longestSeconds) : "—"} />
            <Row label="Yoğunluk" note="Mesainin görüşmede geçen payı" value={pct(occupancy)} />
            <Row icon={r.escalations > 0 ? TriangleAlert : undefined} label="Eskalasyon" value={String(r.escalations)} />
          </Block>

          {r.survey && (
            <Block title="Görüşme sonrası anket">
              <Row icon={Star} label="Ortalama puan" note={`${r.survey.answered} müşteri cevapladı`} value={<b className={cn(r.survey.average < 3 && "text-destructive")}>{r.survey.average.toFixed(1)} / 5</b>} />
            </Block>
          )}

          {r.wa && (
            <Block title="WhatsApp">
              <Row icon={WhatsAppIcon as unknown as LucideIcon} label="Aldığı sohbet" value={String(r.wa.owned)} />
              <Row label="Çözdüğü" value={String(r.wa.resolved)} />
              <Row label="Gönderdiği mesaj" value={String(r.wa.messages)} />
              <Row label="İlk cevap süresi" note="Ortalama" value={r.wa.avgFirstReplySec > 0 ? short(Math.max(60, r.wa.avgFirstReplySec)) : "—"} />
              {r.wa.ratings > 0 && <Row label="Müşteri puanı" note={`${r.wa.ratings} değerlendirme`} value={`${r.wa.avgRating.toFixed(1)} / 5`} />}
            </Block>
          )}

          <Block title="Son görüşmeler">
            {r.recent.length === 0 ? (
              <p className="px-1 text-sm text-muted-foreground">Kayıt yok.</p>
            ) : (
              <ul className="space-y-1">
                {r.recent.map((c, i) => (
                  <li key={i} className="flex items-center gap-2.5 rounded-lg px-1 py-1 text-xs">
                    {c.direction === "inbound" ? <PhoneIncoming className="size-3.5 shrink-0 text-success" /> : <PhoneOutgoing className="size-3.5 shrink-0 text-primary" />}
                    <span className="w-20 shrink-0 tabular-nums text-muted-foreground">{stamp(c.startedAt, true)}</span>
                    <span className="min-w-0 flex-1 truncate"><span className="font-mono">{displayNumber(c.peer) || c.peer}</span>{c.peerName && <span className="text-muted-foreground"> · {c.peerName}</span>}</span>
                    <span className="font-mono tabular-nums">{c.disposition === "answered" ? formatClock(c.durationSeconds) : "—"}</span>
                  </li>
                ))}
              </ul>
            )}
            <button type="button" onClick={() => setCalls(true)} className="mt-2 flex h-9 w-full items-center justify-center gap-1 rounded-xl bg-muted/50 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground">
              Seçilen tarihlerin bütün görüşmeleri <ChevronRight className="size-3.5" />
            </button>
          </Block>

          <Link to={`/profile/${r.userId}`} className="flex h-10 items-center justify-center rounded-xl border border-border/60 text-sm font-medium hover:bg-accent">Profili aç</Link>
        </div>
      </aside>
      {calls && <div onClick={(e) => e.stopPropagation()}><AgentCallsDialog row={r} from={from} to={to} onClose={() => setCalls(false)} /></div>}
    </div>
  );
}

function Block({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section>
      <p className="mb-1.5 px-1 text-[0.68rem] font-semibold tracking-wide text-muted-foreground uppercase">{title}</p>
      <div className="space-y-0.5 rounded-2xl bg-muted/25 p-2 ring-1 ring-border/50">{children}</div>
    </section>
  );
}

function Row({ icon: Icon, tone, label, note, value }: { icon?: LucideIcon; tone?: "success" | "destructive"; label: string; note?: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2.5 rounded-lg px-1.5 py-1.5">
      {Icon && (
        <span className={cn("flex size-7 shrink-0 items-center justify-center rounded-lg", tone === "success" ? "bg-success/10 text-success" : tone === "destructive" ? "bg-destructive/10 text-destructive" : "bg-muted/70 text-muted-foreground")}>
          <Icon className="size-3.5" />
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm">{label}</span>
        {note && <span className="block truncate text-[0.68rem] text-muted-foreground">{note}</span>}
      </span>
      <span className="shrink-0 text-sm tabular-nums">{value}</span>
    </div>
  );
}
