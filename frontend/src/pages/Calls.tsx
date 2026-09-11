import { useEffect, useState } from "react";
import { Download, Play } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Call } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Button, Card, Input, Select } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

export function Calls() {
  const { user } = useAuth();
  const canRec = can(user, "call.record_access") || can(user, "cdr.view_all");
  const [calls, setCalls] = useState<Call[]>([]);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [page, setPage] = useState(1);
  const [direction, setDirection] = useState("");
  const [number, setNumber] = useState("");
  const [error, setError] = useState<string | null>(null);
  const perPage = 20;

  function load() {
    api
      .listCalls({ direction: direction || undefined, number: number || undefined, page, perPage })
      .then((r) => {
        setCalls(r.items);
        setTotal(r.total);
        setTotalPages(r.totalPages);
        setError(null);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."));
  }

  useEffect(load, [page, direction]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <Card
      title={`Çağrılar${total ? ` (${total})` : ""}`}
      actions={
        <div className="flex gap-2">
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
      <table className="w-full text-sm">
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
          {calls.map((c) => (
            <tr key={c.uuid} className="border-t border-border/60">
              <td className="py-2"><Direction value={c.direction} /></td>
              <td className="py-2">{c.fromNumber}</td>
              <td className="py-2">{c.toNumber}</td>
              <td className="py-2"><CallDisposition value={c.disposition} /></td>
              <td className="py-2">{formatDuration(c.durationSeconds)}</td>
              <td className="py-2">{c.recording && canRec ? <RecordingCell uuid={c.uuid} /> : <span className="text-muted-foreground">—</span>}</td>
              <td className="py-2 text-muted-foreground">{formatStamp(c.startedAt)}</td>
            </tr>
          ))}
          {calls.length === 0 && !error && (
            <tr>
              <td colSpan={7} className="py-6 text-center text-sm text-muted-foreground">Kayıt yok.</td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
        <Button variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>Önceki</Button>
        <span>Sayfa {page} / {Math.max(totalPages, 1)}</span>
        <Button variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>Sonraki</Button>
      </div>
    </Card>
  );
}

// RecordingCell lazily loads the recording audio (streamed through our backend)
// only when the agent chooses to listen; a download link is always available.
function RecordingCell({ uuid }: { uuid: string }) {
  const [open, setOpen] = useState(false);
  const src = `/api/v1/calls/${encodeURIComponent(uuid)}/recording`;
  return (
    <div className="flex items-center gap-2">
      {open ? (
        <audio controls autoPlay src={src} className="h-8 w-48" />
      ) : (
        <Button variant="ghost" className="h-8 gap-1.5 px-2" onClick={() => setOpen(true)}>
          <Play className="size-3.5" /> Dinle
        </Button>
      )}
      <a href={`${src}?download=1`} title="İndir" className="text-muted-foreground transition hover:text-foreground">
        <Download className="size-4" />
      </a>
    </div>
  );
}
