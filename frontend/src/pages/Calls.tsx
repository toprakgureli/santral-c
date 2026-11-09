import { useEffect, useRef, useState } from "react";
import { ArrowLeftRight, Captions, Check, Copy, Download, History, PhoneIncoming, PhoneOutgoing, Play, RotateCcw, RotateCw, X } from "lucide-react";
import { ListRow, Toolbar } from "../components/ui/rows";
import { api, ApiError } from "../api/client";
import type { Call, TranscriptLine } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { displayNumber } from "../softphone/dial";
import { fetchCallTranscript } from "../softphone/liveTranscript";
import { cn } from "../lib/utils";
import { Button, Card, DateField, EmptyState, Input, Select, Skeleton, Spinner } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";
import { presetRange, type Preset } from "../lib/dateRange";

// datePreset resolves a named range to [from, to] Istanbul days; "all" has
// no bounds.
function datePreset(key: string): { from: string; to: string } {
  return key === "all" ? { from: "", to: "" } : presetRange(key as Preset);
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
  // Listening needs the recording permission; the server also limits
  // someone who cannot see every call to their own calls.
  const canRec = can(user, "call.record_access");
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
  const [playing, setPlaying] = useState<{
    uuid: string;
    label: string;
    peer: string;
    startedAt: string;
    hasAudio: boolean;
    openTranscript?: boolean;
  } | null>(null);
  const perPage = 20;

  // In extension mode the chosen extension is the filter; otherwise the free
  // number search applies. An extension's calls are looked up across everyone,
  // so it rides the "all" scope with the extension as the number match.
  const extMode = scope === "ext";

  // What is typed is applied to the list after a pause (or on Enter). The
  // list follows the applied values, so one change means one request.
  const [applied, setApplied] = useState({ number: "", ext: "" });
  // Enter on an unchanged search still asks again.
  const [again, setAgain] = useState(0);
  const term = extMode ? applied.ext.trim() : applied.number.trim();

  useEffect(() => {
    if (extMode && !term) {
      setCalls([]); setTotal(0); setTotalPages(1); setError(null); setLoading(false);
      return;
    }
    // A reply that arrives after the filters changed again is dropped, so
    // an older, slower request cannot overwrite a newer list.
    let current = true;
    setLoading(true);
    api
      .listCalls({
        direction: direction || undefined,
        number: term || undefined,
        scope: extMode ? "all" : scope,
        from: from || undefined,
        to: to || undefined,
        page,
        perPage,
      })
      .then((r) => {
        if (!current) return;
        setCalls(r.items);
        setTotal(r.total);
        setTotalPages(r.totalPages);
        setError(null);
      })
      .catch((e) => {
        if (!current) return;
        setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."); setCalls([]); setTotal(0); setTotalPages(1);
      })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [page, direction, scope, extMode, term, from, to, again]);

  // exportCsv downloads the current filter (all pages, bounded server-side) as CSV.
  async function exportCsv() {
    setExporting(true);
    try {
      await api.exportCalls({
        direction: direction || undefined,
        number: term || undefined,
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

  // applyNumber puts the typed number into effect. Looking up a number means
  // "find this caller", so the default "today" preset widens to all dates the
  // first time a number is applied.
  function applyNumber() {
    setPage(1);
    if (number.trim() && preset === "today") {
      setPreset("all");
      setFrom("");
      setTo("");
    }
    setApplied((cur) => (cur.number === number ? cur : { ...cur, number }));
  }

  // Debounce the extension filter so typing a dahili searches without Enter.
  useEffect(() => {
    if (!extMode || ext === applied.ext) return;
    const t = window.setTimeout(() => {
      setPage(1);
      setApplied((cur) => ({ ...cur, ext }));
    }, 350);
    return () => window.clearTimeout(t);
  }, [ext, extMode, applied.ext]);

  // Same for the number search.
  useEffect(() => {
    if (extMode || number === applied.number) return;
    const t = window.setTimeout(applyNumber, 350);
    return () => window.clearTimeout(t);
  }, [number, extMode, applied.number]); // eslint-disable-line react-hooks/exhaustive-deps -- applyNumber is new on every render; the wait restarts only when the number changes

  return (
    <>
      <Card
        title="Çağrılar"
        icon={History}
        actions={
          <div className="flex items-center gap-3">
            {total > 0 && <span className="text-xs tabular-nums text-muted-foreground">{total} kayıt</span>}
            {canExport && (
              <Button variant="secondary" onClick={exportCsv} disabled={exporting || loading} data-tip="Bu filtreyi CSV olarak indir">
                <Download />
                {exporting ? "Hazırlanıyor..." : "CSV"}
              </Button>
            )}
          </div>
        }
      >
        <Toolbar className="mb-4">
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
                onKeyDown={(e) => {
                  if (e.key !== "Enter") return;
                  applyNumber();
                  setAgain((n) => n + 1);
                }}
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
                <DateField value={from} max={to || undefined} onChange={(v) => { setFrom(v); setPage(1); }} className="w-40" data-tip="Başlangıç tarihi" />
                <span className="text-muted-foreground">-</span>
                <DateField value={to} min={from || undefined} onChange={(v) => { setTo(v); setPage(1); }} className="w-40" data-tip="Bitiş tarihi" />
              </div>
            )}
        </Toolbar>
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        <div className="relative">
          {loading && calls.length > 0 && (
            <div className="absolute inset-0 z-10 flex items-center justify-center">
              <span className="flex items-center gap-2 rounded-full bg-card/95 px-4 py-2 text-sm text-muted-foreground shadow-md ring-1 ring-border/60">
                <Spinner /> Yükleniyor…
              </span>
            </div>
          )}
          <div className={cn("space-y-1 transition-opacity", loading && calls.length > 0 && "pointer-events-none opacity-40")}>
            {loading && calls.length === 0 && Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-14 w-full rounded-2xl" />)}
            {calls.map((c) => {
              const label = `${displayNumber(c.fromNumber) || c.fromNumber} -> ${displayNumber(c.toNumber) || c.toNumber}`;
              const peer = c.direction === "inbound" ? c.fromNumber : c.toNumber;
              const isPlaying = playing?.uuid === c.uuid;
              const icon = c.direction === "inbound" ? PhoneIncoming : c.direction === "outbound" ? PhoneOutgoing : ArrowLeftRight;
              const tone = c.disposition === "answered" ? (c.direction === "inbound" ? "success" : "primary") : c.disposition === "in_progress" ? "primary" : "destructive";
              return (
                <ListRow
                  key={c.uuid}
                  icon={icon}
                  tone={tone}
                  active={isPlaying}
                  title={<span className="font-mono tabular-nums">{c.fromNumber} <span className="text-muted-foreground/60">→</span> {c.toNumber}</span>}
                  sub={<span className="flex items-center gap-2"><Direction value={c.direction} /><CallDisposition value={c.disposition} /><span>{formatStamp(c.startedAt)}</span></span>}
                  trailing={
                    <>
                      <span className="w-16 text-right font-mono text-sm tabular-nums">{c.disposition === "answered" || c.disposition === "in_progress" ? formatDuration(c.durationSeconds) : "—"}</span>
                      <span className="flex items-center gap-1">
                        {c.disposition === "answered" && (
                          <button
                            type="button"
                            onClick={() =>
                              setPlaying({
                                uuid: c.uuid,
                                label,
                                peer,
                                startedAt: c.startedAt,
                                hasAudio: Boolean(c.recording && canRec),
                                openTranscript: true,
                              })
                            }
                            aria-label="Transkript"
                            data-tip="Görüşme transkripti"
                            className={cn(
                              "flex size-8 items-center justify-center rounded-xl transition-colors",
                              isPlaying && playing?.openTranscript
                                ? "bg-primary text-primary-foreground"
                                : "bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground",
                            )}
                          >
                            <Captions className="size-3.5" />
                          </button>
                        )}
                        {c.recording && canRec ? (
                          <>
                            <button
                              type="button"
                              onClick={() =>
                                setPlaying({
                                  uuid: c.uuid,
                                  label,
                                  peer,
                                  startedAt: c.startedAt,
                                  hasAudio: true,
                                  openTranscript: playing?.uuid === c.uuid ? playing.openTranscript : false,
                                })
                              }
                              aria-label="Dinle"
                              data-tip="Dinle"
                              className={cn(
                                "flex size-8 items-center justify-center rounded-xl transition-colors",
                                isPlaying && playing?.hasAudio
                                  ? "bg-primary text-primary-foreground"
                                  : "bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground",
                              )}
                            >
                              <Play className="size-3.5" />
                            </button>
                            <a href={`/api/v1/calls/${encodeURIComponent(c.uuid)}/recording?download=1`} target="_blank" rel="noopener" aria-label="İndir" data-tip="İndir" className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
                              <Download className="size-3.5" />
                            </a>
                          </>
                        ) : (
                          <span className="w-[4.25rem]" />
                        )}
                      </span>
                    </>
                  }
                />
              );
            })}
            {calls.length === 0 && !error && !loading && (
              <EmptyState icon={<History />} title="Kayıt yok" description={(from || to) ? "Bu tarih aralığında kayıt yok." : "Bu filtrelerle eşleşen çağrı yok."} />
            )}
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

      {playing && (
        <RecordingBar
          uuid={playing.uuid}
          label={playing.label}
          peer={playing.peer}
          startedAt={playing.startedAt}
          hasAudio={playing.hasAudio}
          defaultTranscriptOpen={playing.openTranscript}
          onClose={() => setPlaying(null)}
        />
      )}
    </>
  );
}

const SPEEDS = [1, 1.25, 1.5, 2] as const;

function formatSec(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}

// RecordingBar is a wide, fixed player at the bottom of the screen for the
// selected recording and its transcript, with speed controls, seeking and download.
function RecordingBar({
  uuid,
  label,
  peer,
  startedAt,
  hasAudio,
  defaultTranscriptOpen,
  onClose,
}: {
  uuid: string;
  label: string;
  peer: string;
  startedAt: string;
  hasAudio: boolean;
  defaultTranscriptOpen?: boolean;
  onClose: () => void;
}) {
  const src = `/api/v1/calls/${encodeURIComponent(uuid)}/recording`;
  const audioRef = useRef<HTMLAudioElement>(null);
  const [speed, setSpeed] = useState<number>(1);
  const [currentSec, setCurrentSec] = useState(0);
  const [showTranscript, setShowTranscript] = useState(Boolean(defaultTranscriptOpen || !hasAudio));
  const [lines, setLines] = useState<TranscriptLine[]>([]);
  const [loadingTr, setLoadingTr] = useState(true);
  const [search, setSearch] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setShowTranscript(Boolean(defaultTranscriptOpen || !hasAudio));
  }, [uuid, defaultTranscriptOpen, hasAudio]);

  useEffect(() => {
    let active = true;
    setLoadingTr(true);
    void fetchCallTranscript({ callId: uuid, peer, startedAt })
      .then((res) => {
        if (active) setLines(res.lines ?? []);
      })
      .finally(() => {
        if (active) setLoadingTr(false);
      });
    return () => {
      active = false;
    };
  }, [uuid, peer, startedAt]);

  function changeSpeed(next: number) {
    setSpeed(next);
    if (audioRef.current) {
      audioRef.current.playbackRate = next;
    }
  }

  function skip(delta: number) {
    const el = audioRef.current;
    if (!el) return;
    el.currentTime = Math.max(0, el.currentTime + delta);
  }

  function seekTo(sec: number) {
    const el = audioRef.current;
    if (!el) return;
    el.currentTime = Math.max(0, sec);
    void el.play().catch(() => undefined);
  }

  function copyTranscript() {
    if (!lines.length) return;
    const text = lines
      .map((l) => `[${formatSec(l.atSec)}] ${l.speaker === "agent" ? "Temsilci" : "Müşteri"}: ${l.text}`)
      .join("\n");
    void navigator.clipboard?.writeText(text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1800);
  }

  const q = search.trim().toLocaleLowerCase("tr");
  const filtered = q ? lines.filter((l) => l.text.toLocaleLowerCase("tr").includes(q)) : lines;

  return (
    <div className="fixed inset-x-0 bottom-0 z-40 border-t border-border/70 bg-card/95 shadow-[0_-8px_24px_rgba(0,0,0,0.15)] backdrop-blur">
      {showTranscript && (
        <div className="border-b border-border/60 bg-background/80 px-4 py-3">
          <div className="mx-auto max-w-5xl">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Captions className="size-4 text-primary" />
                <span className="text-xs font-semibold">Görüşme Transkripti</span>
                {lines.length > 0 && (
                  <span className="rounded-full bg-primary/15 px-2 py-0.5 font-mono text-[11px] font-medium text-primary tabular-nums">
                    {lines.length} satır
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                {lines.length > 2 && (
                  <Input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Transkript içinde ara..."
                    className="h-7 w-48 text-xs"
                  />
                )}
                {lines.length > 0 && (
                  <button
                    type="button"
                    onClick={copyTranscript}
                    className="flex items-center gap-1 rounded-lg border border-border/70 bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
                  >
                    {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />}
                    {copied ? "Kopyalandı" : "Kopyala"}
                  </button>
                )}
              </div>
            </div>

            {loadingTr ? (
              <div className="flex h-24 items-center justify-center gap-2 text-xs text-muted-foreground">
                <Spinner /> Transkript yükleniyor...
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex h-20 items-center justify-center rounded-xl border border-dashed border-border/60 text-xs text-muted-foreground">
                {lines.length === 0
                  ? "Bu görüşme için kaydedilmiş transkript bulunmuyor."
                  : "Aramanızla eşleşen konuşma satırı bulunamadı."}
              </div>
            ) : (
              <div className="max-h-52 space-y-1.5 overflow-y-auto pr-1">
                {filtered.map((l, idx) => {
                  const isAgent = l.speaker === "agent";
                  const nextSec = filtered[idx + 1]?.atSec ?? Number.POSITIVE_INFINITY;
                  const active = hasAudio && currentSec >= l.atSec && currentSec < nextSec;
                  return (
                    <button
                      key={l.id || idx}
                      type="button"
                      onClick={() => hasAudio && seekTo(l.atSec)}
                      className={cn(
                        "flex w-full items-start gap-2.5 rounded-xl px-3 py-2 text-left text-xs transition",
                        active
                          ? "bg-primary/15 ring-1 ring-primary/40"
                          : "bg-card/80 ring-1 ring-border/40 hover:bg-accent/60",
                      )}
                    >
                      <span className="shrink-0 font-mono text-[11px] text-muted-foreground tabular-nums">
                        {formatSec(l.atSec)}
                      </span>
                      <span
                        className={cn(
                          "w-16 shrink-0 font-semibold",
                          isAgent ? "text-primary" : "text-amber-500",
                        )}
                      >
                        {isAgent ? "Temsilci" : "Müşteri"}
                      </span>
                      <span className="min-w-0 flex-1 break-words leading-relaxed text-foreground">
                        {l.text}
                      </span>
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      <div className="px-4 py-3">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-3">
          <div className="hidden min-w-0 shrink-0 sm:block">
            <div className="text-xs text-muted-foreground">{hasAudio ? "Çağrı kaydı" : "Çağrı detayı"}</div>
            <div className="truncate text-sm font-medium tabular-nums">{label}</div>
          </div>

          {hasAudio ? (
            <>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() => skip(-5)}
                  data-tip="5 saniye geri"
                  className="flex items-center gap-0.5 rounded-lg border border-border/70 px-2 py-1 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
                >
                  <RotateCcw className="size-3.5" /> 5s
                </button>
                <button
                  type="button"
                  onClick={() => skip(10)}
                  data-tip="10 saniye ileri"
                  className="flex items-center gap-0.5 rounded-lg border border-border/70 px-2 py-1 text-xs font-medium text-muted-foreground transition hover:bg-accent hover:text-foreground"
                >
                  10s <RotateCw className="size-3.5" />
                </button>
              </div>

              {/* Call recordings have no caption track; live transcript panel sits above. */}
              <audio
                ref={audioRef}
                key={uuid}
                controls
                autoPlay
                src={src}
                onPlay={(e) => {
                  e.currentTarget.playbackRate = speed;
                }}
                onTimeUpdate={(e) => setCurrentSec(e.currentTarget.currentTime)}
                className="h-10 min-w-[12rem] flex-1"
              />

              <div className="flex items-center gap-0.5 rounded-xl border border-border/70 bg-muted/40 p-0.5">
                {SPEEDS.map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => changeSpeed(s)}
                    className={cn(
                      "rounded-lg px-2 py-1 font-mono text-xs font-medium tabular-nums transition",
                      speed === s
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {s}x
                  </button>
                ))}
              </div>
            </>
          ) : (
            <div className="flex-1 text-xs text-muted-foreground">
              Ses kaydı olmadan görüşme transkripti görüntüleniyor.
            </div>
          )}

          <button
            type="button"
            onClick={() => setShowTranscript((v) => !v)}
            data-tip="Transkript panelini aç/kapat"
            className={cn(
              "flex items-center gap-1.5 rounded-xl border px-3 py-1.5 text-xs font-medium transition",
              showTranscript
                ? "border-primary/40 bg-primary/15 text-primary"
                : "border-border/70 bg-muted/50 text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            <Captions className="size-4" />
            Transkript {lines.length > 0 ? `(${lines.length})` : ""}
          </button>

          {hasAudio && (
            <a
              href={`${src}?download=1`}
              target="_blank"
              rel="noopener"
              aria-label="İndir"
              data-tip="İndir"
              className="shrink-0 rounded-lg p-2 text-muted-foreground transition hover:bg-accent hover:text-foreground"
            >
              <Download className="size-5" />
            </a>
          )}
          <button
            onClick={onClose}
            aria-label="Kapat"
            data-tip="Kapat"
            className="shrink-0 rounded-lg p-2 text-muted-foreground transition hover:bg-accent hover:text-foreground"
          >
            <X className="size-5" />
          </button>
        </div>
      </div>
    </div>
  );
}
