import { useEffect, useRef, useState } from "react";
import { Download, Play, X } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Call } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { displayNumber } from "../softphone/dial";
import { Button, Card, Input, Select, Spinner, TableSkeleton } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

// ymd formats a Date as a local YYYY-MM-DD (not UTC, so it matches the panel's day).
function ymd(d: Date): string {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

// datePreset resolves a named range to [from, to] local dates.
function datePreset(key: string): { from: string; to: string } {
  const now = new Date();
  const today = ymd(now);
  const shift = (days: number) => ymd(new Date(now.getFullYear(), now.getMonth(), now.getDate() - days));
  switch (key) {
    case "today": return { from: today, to: today };
    case "yesterday": return { from: shift(1), to: shift(1) };
    case "last7": return { from: shift(6), to: today };
    case "last30": return { from: shift(29), to: today };
    case "month": return { from: ymd(new Date(now.getFullYear(), now.getMonth(), 1)), to: today };
    default: return { from: "", to: "" }; // "all"
  }
}

const datePresetLabels: { key: string; label: string }[] = [
  { key: "today", label: "Bugün" },
  { key: "yesterday", label: "Dün" },
  { key: "last7", label: "Son 7 gün" },
  { key: "last30", label: "Son 30 gün" },
  { key: "month", label: "Bu ay" },
  { key: "all", label: "Tüm tarihler" },
  { key: "custom", label: "Özel aralık" },
];

export function Calls() {
  const { user } = useAuth();
  const canRec = can(user, "call.record_access") || can(user, "cdr.view_all");
  const canAll = can(user, "cdr.view_all") || can(user, "call.view_all");
  const canExport = can(user, "cdr.export");
  const [exporting, setExporting] = useState(false);
  const [calls, setCalls] = useState<Call[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [page, setPage] = useState(1);
  const [direction, setDirection] = useState("");
  const [number, setNumber] = useState("");
  // Owners/managers default to everyone's calls; agents only ever see their own.
  // "ext" narrows to a single extension's calls (managers only).
  const [scope, setScope] = useState<"own" | "all" | "ext">(canAll ? "all" : "own");
  const [ext, setExt] = useState("");
  // Default to today's calls, as requested.
  const [preset, setPreset] = useState("today");
  const [from, setFrom] = useState(() => datePreset("today").from);
  const [to, setTo] = useState(() => datePreset("today").to);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [playing, setPlaying] = useState<{ uuid: string; label: string } | null>(null);
  const perPage = 20;

  // In extension mode the chosen extension is the filter; otherwise the free
  // number search applies. An extension's calls are looked up across everyone,
  // so it rides the "all" scope with the extension as the number match.
  const extMode = scope === "ext";

  function load() {
    const q = extMode ? ext.trim() : number.trim();
    if (extMode && !q) { setCalls([]); setTotal(0); setTotalPages(1); setError(null); setLoading(false); return; }
    setLoading(true);
    api
      .listCalls({
        direction: direction || undefined,
        number: q || undefined,
        scope: extMode ? "all" : scope,
        from: from || undefined,
        to: to || undefined,
        page,
        perPage,
      })
      .then((r) => {
        setCalls(r.items);
        setTotal(r.total);
        setTotalPages(r.totalPages);
        setError(null);
      })
      .catch((e) => { setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."); setCalls([]); setTotal(0); setTotalPages(1); })
      .finally(() => setLoading(false));
  }

  useEffect(load, [page, direction, scope, from, to]); // eslint-disable-line react-hooks/exhaustive-deps

  // exportCsv downloads the current filter (all pages, bounded server-side) as CSV.
  async function exportCsv() {
    const q = extMode ? ext.trim() : number.trim();
    setExporting(true);
    try {
      await api.exportCalls({
        direction: direction || undefined,
        number: q || undefined,
        scope: extMode ? "all" : scope,
        from: from || undefined,
        to: to || undefined,
      });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Dışa aktarma başarısız oldu.");
    } finally {
      setExporting(false);
    }
  }

  // Debounce the extension filter so typing a dahili searches without Enter.
  useEffect(() => {
    if (!extMode) return;
    const t = window.setTimeout(() => { setPage(1); load(); }, 350);
    return () => window.clearTimeout(t);
  }, [ext, extMode]); // eslint-disable-line react-hooks/exhaustive-deps

  // Same for the number search. Looking up a number means "find this caller",
  // so the default "today" preset widens to all dates the first time a number
  // is typed; the date change itself triggers the load.
  const searched = useRef("");
  useEffect(() => {
    if (extMode || number === searched.current) return;
    const t = window.setTimeout(() => {
      searched.current = number;
      setPage(1);
      if (number.trim() && preset === "today") {
        setPreset("all");
        setFrom("");
        setTo("");
        return;
      }
      load();
    }, 350);
    return () => window.clearTimeout(t);
  }, [number, extMode]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <>
      <Card
        title={`Çağrılar${total ? ` (${total})` : ""}`}
        actions={
          <div className="flex flex-wrap gap-2">
            {canAll && (
              <Select value={scope} onChange={(e) => { setScope(e.target.value as "own" | "all" | "ext"); setPage(1); }} className="w-44">
                <option value="all">Tüm çağrılar</option>
                <option value="own">Kendi çağrılarım</option>
                <option value="ext">Belirli dahili</option>
              </Select>
            )}
            {extMode ? (
              <Input
                placeholder="Dahili (örn. 1014)"
                value={ext}
                onChange={(e) => setExt(e.target.value)}
                inputMode="numeric"
                className="w-40"
              />
            ) : (
              <Input
                placeholder="Numara / dahili ara"
                value={number}
                onChange={(e) => setNumber(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && (setPage(1), load())}
                className="w-44"
              />
            )}
            <Select value={direction} onChange={(e) => { setDirection(e.target.value); setPage(1); }} className="w-32">
              <option value="">Tüm yönler</option>
              <option value="inbound">Gelen</option>
              <option value="outbound">Giden</option>
              <option value="internal">Dahili</option>
            </Select>
            <Select
              value={preset}
              onChange={(e) => {
                const key = e.target.value;
                setPreset(key);
                if (key !== "custom") {
                  const r = datePreset(key);
                  setFrom(r.from);
                  setTo(r.to);
                }
                setPage(1);
              }}
              className="w-36"
            >
              {datePresetLabels.map((p) => <option key={p.key} value={p.key}>{p.label}</option>)}
            </Select>
            {preset === "custom" && (
              <div className="flex items-center gap-1">
                <Input type="date" value={from} max={to || undefined} onChange={(e) => { setFrom(e.target.value); setPage(1); }} className="w-36" title="Başlangıç tarihi" />
                <span className="text-muted-foreground">–</span>
                <Input type="date" value={to} min={from || undefined} onChange={(e) => { setTo(e.target.value); setPage(1); }} className="w-36" title="Bitiş tarihi" />
              </div>
            )}
            {canExport && (
              <Button variant="secondary" onClick={exportCsv} disabled={exporting || loading} title="Bu filtreyi CSV olarak indir">
                <Download />
                {exporting ? "Hazırlanıyor..." : "CSV"}
              </Button>
            )}
          </div>
        }
      >
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        <div className="relative">
          {loading && calls.length > 0 && (
            <div className="absolute inset-0 z-10 flex items-center justify-center">
              <span className="flex items-center gap-2 rounded-full bg-card/95 px-4 py-2 text-sm text-muted-foreground shadow-md ring-1 ring-border/60">
                <Spinner /> Yükleniyor…
              </span>
            </div>
          )}
          <div className={`overflow-x-auto transition-opacity ${loading && calls.length > 0 ? "pointer-events-none opacity-40" : ""}`}>
          <table className="w-full min-w-[52rem] text-sm">
            <thead>
              <tr className="text-left text-xs text-muted-foreground">
                <th className="pb-2">Yön</th>
                <th className="pb-2">Kimden</th>
                <th className="pb-2">Kime</th>
                <th className="pb-2">Durum</th>
                <th className="pb-2">Süre</th>
                <th className="pb-2">Kayıt</th>
                <th className="pb-2">Zaman</th>
              </tr>
            </thead>
            <tbody>
              {loading && calls.length === 0 && <TableSkeleton rows={8} cols={7} />}
              {calls.map((c) => {
                const label = `${displayNumber(c.fromNumber) || c.fromNumber} → ${displayNumber(c.toNumber) || c.toNumber}`;
                const isPlaying = playing?.uuid === c.uuid;
                return (
                  <tr key={c.uuid} className={"border-t border-border/60" + (isPlaying ? " bg-accent/40" : "")}>
                    <td className="py-2"><Direction value={c.direction} /></td>
                    <td className="py-2">{c.fromNumber}</td>
                    <td className="py-2">{c.toNumber}</td>
                    <td className="py-2"><CallDisposition value={c.disposition} /></td>
                    <td className="py-2">{formatDuration(c.durationSeconds)}</td>
                    <td className="py-2">
                      {c.recording && canRec ? (
                        <div className="flex items-center gap-1">
                          <Button variant={isPlaying ? "primary" : "ghost"} className="h-8 gap-1.5 px-2" onClick={() => setPlaying({ uuid: c.uuid, label })}>
                            <Play className="size-3.5" /> Dinle
                          </Button>
                          <a href={`/api/v1/calls/${encodeURIComponent(c.uuid)}/recording?download=1`} title="İndir" className="text-muted-foreground transition hover:text-foreground">
                            <Download className="size-4" />
                          </a>
                        </div>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </td>
                    <td className="py-2 text-muted-foreground">{formatStamp(c.startedAt)}</td>
                  </tr>
                );
              })}
              {calls.length === 0 && !error && !loading && (
                <tr>
                  <td colSpan={7} className="py-6 text-center text-sm text-muted-foreground">
                    {(from || to) ? "Bu tarih aralığında kayıt yok." : "Kayıt yok."}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          </div>
        </div>
        <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
          <Button variant="secondary" disabled={loading || page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>Önceki</Button>
          <span className="tabular-nums">
            Sayfa {page} / {Math.max(totalPages, 1)}
            {total > 0 && <span className="ml-2 text-muted-foreground/70">· {total} kayıt</span>}
          </span>
          <Button
            variant="secondary"
            disabled={loading || (page >= totalPages && calls.length < perPage)}
            onClick={() => setPage((p) => p + 1)}
          >
            Sonraki
          </Button>
        </div>
      </Card>

      {playing && <RecordingBar uuid={playing.uuid} label={playing.label} onClose={() => setPlaying(null)} />}
    </>
  );
}

// RecordingBar is a wide, fixed player at the bottom of the screen for the
// selected recording, with seeking (backend serves Range requests) and download.
function RecordingBar({ uuid, label, onClose }: { uuid: string; label: string; onClose: () => void }) {
  const src = `/api/v1/calls/${encodeURIComponent(uuid)}/recording`;
  return (
    <div className="fixed inset-x-0 bottom-0 z-40 border-t border-border/70 bg-card/95 px-4 py-3 shadow-[0_-8px_24px_rgba(0,0,0,0.15)] backdrop-blur">
      <div className="mx-auto flex max-w-5xl items-center gap-4">
        <div className="hidden min-w-0 shrink-0 sm:block">
          <div className="text-xs text-muted-foreground">Çağrı kaydı</div>
          <div className="truncate text-sm font-medium tabular-nums">{label}</div>
        </div>
        {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
        <audio key={uuid} controls autoPlay src={src} className="h-10 flex-1" />
        <a href={`${src}?download=1`} title="İndir" className="shrink-0 rounded-lg p-2 text-muted-foreground transition hover:bg-accent hover:text-foreground">
          <Download className="size-5" />
        </a>
        <button onClick={onClose} title="Kapat" className="shrink-0 rounded-lg p-2 text-muted-foreground transition hover:bg-accent hover:text-foreground">
          <X className="size-5" />
        </button>
      </div>
    </div>
  );
}
