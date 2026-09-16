// Ekip Performansı: every agent the viewer may see, with live status and
// today's call figures. The server scopes the list by permission
// (performance.view_all: everyone, performance.view_role: agents sharing a
// role with the viewer); the page only renders what it gets.

import { useEffect, useMemo, useState } from "react";
import { PhoneIncoming, PhoneOutgoing, Users } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { TeamRow, TeamStatus } from "../api/types";
import { Badge, Card, EmptyState, Skeleton } from "../components/ui";
import { displayNumber } from "../softphone/dial";
import { cn } from "../lib/utils";
import { formatClock } from "./callFormat";

const REFRESH_MS = 15000;

const STATUS: Record<TeamStatus, { label: string; tone: "green" | "amber" | "red" | "slate" | "blue" }> = {
  talking: { label: "Görüşmede", tone: "blue" },
  available: { label: "Boşta", tone: "green" },
  break: { label: "Molada", tone: "amber" },
  backoffice: { label: "Backoffice", tone: "amber" },
  dnd: { label: "Rahatsız etmeyin", tone: "red" },
  unregistered: { label: "Kayıtsız", tone: "slate" },
  off: { label: "Mesai dışı", tone: "slate" },
};

type SortKey = "long" | "answered" | "unanswered" | "inbound" | "outbound" | "talkSeconds" | "shift";

function hhmm(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

export function TeamPerformance() {
  const [rows, setRows] = useState<TeamRow[]>([]);
  const [scope, setScope] = useState<"all" | "role">("role");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<SortKey>("long");
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    let live = true;
    const load = () =>
      api
        .performanceToday()
        .then((r) => {
          if (!live) return;
          setRows(r.items);
          setScope(r.scope);
          setError(null);
        })
        .catch((e) => live && setError(e instanceof ApiError ? e.message : "Ekip verisi alınamadı."))
        .finally(() => live && setLoading(false));
    load();
    const timer = window.setInterval(load, REFRESH_MS);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, []);

  // A one-second clock so the call and shift timers move between refreshes.
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  const sorted = useMemo(() => {
    const value = (r: TeamRow) => (sort === "shift" ? r.shift.seconds : r.calls[sort]);
    return [...rows].sort((a, b) => {
      const oa = a.status !== "off" ? 0 : 1;
      const ob = b.status !== "off" ? 0 : 1;
      if (oa !== ob) return oa - ob;
      return value(b) - value(a) || a.name.localeCompare(b.name, "tr");
    });
  }, [rows, sort]);

  const totals = useMemo(() => {
    const t = { onShift: 0, talking: 0, valid: 0, answered: 0, unanswered: 0, inbound: 0, outbound: 0 };
    for (const r of rows) {
      if (r.status !== "off") t.onShift += 1;
      if (r.status === "talking") t.talking += 1;
      t.valid += r.calls.long;
      t.answered += r.calls.answered;
      t.unanswered += r.calls.unanswered;
      t.inbound += r.calls.inbound;
      t.outbound += r.calls.outbound;
    }
    return t;
  }, [rows]);

  const header = (key: SortKey, label: string, hint?: string) => (
    <th className="pb-2 pr-3 text-right">
      <button
        type="button"
        onClick={() => setSort(key)}
        title={hint}
        className={cn("font-medium hover:text-foreground", sort === key ? "text-foreground" : "text-muted-foreground")}
      >
        {label}
      </button>
    </th>
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-2xl bg-card px-5 py-3 text-sm ring-1 ring-border/60">
        <Stat label="Mesaide" value={totals.onShift} dot="bg-success" />
        <Stat label="Görüşmede" value={totals.talking} dot="bg-primary" />
        <Stat label="Gerçek toplam" value={totals.valid} dot="bg-primary" />
        <Stat label="Görüşülen" value={totals.answered} dot="bg-success" />
        <Stat label="Cevapsız" value={totals.unanswered} dot="bg-destructive" />
        <Stat label="Gelen" value={totals.inbound} dot="bg-muted-foreground/60" />
        <Stat label="Giden" value={totals.outbound} dot="bg-muted-foreground/60" />
        <span className="ml-auto text-xs text-muted-foreground">
          {scope === "all" ? "Tüm ekip" : "Kendi rolündekiler"} · bugün, 00:00'dan beri · 15 sn'de bir yenilenir
        </span>
      </div>

      <Card title="Ekip Performansı">
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-9 w-full" />
            ))}
          </div>
        ) : rows.length === 0 ? (
          <EmptyState icon={<Users />} title="Görüntülenecek temsilci yok" description="Dahilisi olan aktif kullanıcı bulunamadı ya da rolünüzle eşleşen kimse yok." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[64rem] text-sm">
              <thead>
                <tr className="text-left text-xs text-muted-foreground">
                  <th className="pb-2 pr-3">Temsilci</th>
                  <th className="pb-2 pr-3">Durum</th>
                  {header("shift", "Mesai", "Bugün mesaide geçen süre")}
                  {header("long", "Gerçek toplam", "Geçerli çağrılar: 30 saniye ve üstü görüşmeler")}
                  {header("answered", "Görüşülen", "Cevaplanan çağrılar, geçersiz dahil")}
                  <th className="pb-2 pr-3 text-right text-muted-foreground" title="Geçersiz: 30 saniyeden kısa / Geçerli: 30 saniye ve üstü">Geçersiz / Geçerli</th>
                  {header("unanswered", "Cevapsız")}
                  {header("inbound", "Gelen")}
                  {header("outbound", "Giden")}
                  {header("talkSeconds", "Görüşme", "Toplam görüşme süresi")}
                </tr>
              </thead>
              <tbody>
                {sorted.map((r) => {
                  const s = STATUS[r.status] ?? STATUS.off;
                  const off = r.status === "off";
                  const callFor = r.call ? Math.max(0, Math.floor((now - Date.parse(r.call.startedAt)) / 1000)) : 0;
                  const shiftLive = r.shift.seconds;
                  return (
                    <tr key={r.userId} className={cn("border-t border-border/60", off && "text-muted-foreground/70")}>
                      <td className="py-2 pr-3">
                        <div className="font-medium">{r.name}</div>
                        <div className="text-xs text-muted-foreground">
                          {r.extension}
                          {r.roles.length > 0 && <span> · {r.roles.join(", ")}</span>}
                        </div>
                      </td>
                      <td className="py-2 pr-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone={s.tone}>{s.label}</Badge>
                          {r.call && (
                            <span className="flex items-center gap-1 text-xs text-muted-foreground">
                              {r.call.direction === "inbound" ? <PhoneIncoming className="size-3.5" /> : <PhoneOutgoing className="size-3.5" />}
                              <span className="font-mono">{displayNumber(r.call.peer) || r.call.peer}</span>
                              {r.call.peerName && <span>({r.call.peerName})</span>}
                              <span className="font-mono tabular-nums">{formatClock(callFor)}</span>
                            </span>
                          )}
                          {!r.call && r.since && !off && r.status !== "available" && (
                            <span className="text-xs text-muted-foreground">{hhmm(r.since)}&apos;den beri</span>
                          )}
                        </div>
                      </td>
                      <td className="py-2 pr-3 text-right font-mono tabular-nums" title={r.shift.startedAt ? `Mesai ${hhmm(r.shift.startedAt)} başladı` : "Mesai başlatılmadı"}>
                        {shiftLive > 0 ? formatClock(shiftLive) : "—"}
                      </td>
                      <td className="py-2 pr-3 text-right font-semibold tabular-nums">{r.calls.long}</td>
                      <td className="py-2 pr-3 text-right tabular-nums text-success">{r.calls.answered}</td>
                      <td className="py-2 pr-3 text-right tabular-nums text-muted-foreground">
                        {r.calls.short} / {r.calls.long}
                      </td>
                      <td className="py-2 pr-3 text-right tabular-nums text-destructive">{r.calls.unanswered}</td>
                      <td className="py-2 pr-3 text-right tabular-nums">{r.calls.inbound}<Minus n={r.calls.inboundMissed} /></td>
                      <td className="py-2 pr-3 text-right tabular-nums">{r.calls.outbound}<Minus n={r.calls.outboundMissed} /></td>
                      <td className="py-2 pr-3 text-right font-mono tabular-nums">{formatClock(r.calls.talkSeconds)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}

// Minus is the unanswered share of a direction count, small and red.
function Minus({ n }: { n: number }) {
  if (!n) return null;
  return <span className="ml-1 text-xs font-semibold text-destructive" title="Bağlanmayan">-{n}</span>;
}

function Stat({ dot, label, value }: { dot: string; label: string; value: number }) {
  return (
    <span className="flex items-center gap-2">
      <span className={`size-2 rounded-full ${dot}`} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
    </span>
  );
}
