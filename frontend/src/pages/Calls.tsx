import { useEffect, useState } from "react";
import { Download, Play, X } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Call } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { displayNumber } from "../softphone/dial";
import { Button, Card, Input, Select } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

export function Calls() {
  const { user } = useAuth();
  const canRec = can(user, "call.record_access") || can(user, "cdr.view_all");
  const canAll = can(user, "cdr.view_all") || can(user, "call.view_all");
  const [calls, setCalls] = useState<Call[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [page, setPage] = useState(1);
  const [direction, setDirection] = useState("");
  const [number, setNumber] = useState("");
  // Owners/managers default to everyone's calls; agents only ever see their own.
  const [scope, setScope] = useState<"own" | "all">(canAll ? "all" : "own");
  const [error, setError] = useState<string | null>(null);
  const [playing, setPlaying] = useState<{ uuid: string; label: string } | null>(null);
  const perPage = 20;

  function load() {
    api
      .listCalls({ direction: direction || undefined, number: number || undefined, scope, page, perPage })
      .then((r) => {
        setCalls(r.items);
        setTotal(r.total);
        setTotalPages(r.totalPages);
        setError(null);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."));
  }

  useEffect(load, [page, direction, scope]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <>
      <Card
        title={`Çağrılar${total ? ` (${total})` : ""}`}
        actions={
          <div className="flex flex-wrap gap-2">
            {canAll && (
              <Select value={scope} onChange={(e) => { setScope(e.target.value as "own" | "all"); setPage(1); }} className="w-40">
                <option value="all">Tüm çağrılar</option>
                <option value="own">Kendi çağrılarım</option>
              </Select>
            )}
            <Input
              placeholder="Numara ara"
              value={number}
              onChange={(e) => setNumber(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && (setPage(1), load())}
              className="w-40"
            />
            <Select value={direction} onChange={(e) => { setDirection(e.target.value); setPage(1); }} className="w-32">
              <option value="">Tüm yönler</option>
              <option value="inbound">Gelen</option>
              <option value="outbound">Giden</option>
              <option value="internal">Dahili</option>
            </Select>
          </div>
        }
      >
        {error && <p className="mb-3 text-sm text-destructive">{error}</p>}
        <div className="overflow-x-auto">
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
              {calls.length === 0 && !error && (
                <tr>
                  <td colSpan={7} className="py-6 text-center text-sm text-muted-foreground">Kayıt yok.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
          <Button variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>Önceki</Button>
          <span>Sayfa {page} / {Math.max(totalPages, 1)}</span>
          <Button variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>Sonraki</Button>
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
