import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Call } from "../api/types";
import { Badge, Button, Card, Input, Select } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

export function Calls() {
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
      {error && <p className="mb-3 text-sm text-red-600">{error}</p>}
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-slate-400">
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
            <tr key={c.uuid} className="border-t border-slate-100">
              <td className="py-2"><Direction value={c.direction} /></td>
              <td className="py-2">{c.fromNumber}</td>
              <td className="py-2">{c.toNumber}</td>
              <td className="py-2"><CallDisposition value={c.disposition} /></td>
              <td className="py-2">{formatDuration(c.durationSeconds)}</td>
              <td className="py-2">{c.recording ? <Badge tone="blue">var</Badge> : <span className="text-slate-300">—</span>}</td>
              <td className="py-2 text-slate-400">{formatStamp(c.startedAt)}</td>
            </tr>
          ))}
          {calls.length === 0 && !error && (
            <tr>
              <td colSpan={7} className="py-6 text-center text-sm text-slate-400">Kayıt yok.</td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="mt-4 flex items-center justify-between text-sm text-slate-500">
        <Button variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>Önceki</Button>
        <span>Sayfa {page} / {Math.max(totalPages, 1)}</span>
        <Button variant="secondary" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>Sonraki</Button>
      </div>
    </Card>
  );
}
