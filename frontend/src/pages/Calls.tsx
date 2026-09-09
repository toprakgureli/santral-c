import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Call, CallDetail } from "../api/types";
import { Badge, Button, Card, Input, Select } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatTime } from "./callFormat";

export function Calls() {
  const [calls, setCalls] = useState<Call[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [direction, setDirection] = useState("");
  const [number, setNumber] = useState("");
  const [selected, setSelected] = useState<CallDetail | null>(null);
  const perPage = 15;

  function load() {
    api
      .listCalls({ direction: direction || undefined, number: number || undefined, page, perPage })
      .then((r) => {
        setCalls(r.items);
        setTotal(r.total);
      })
      .catch(() => setCalls([]));
  }

  useEffect(load, [page, direction]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
      <Card
        title={`Çağrılar (${total})`}
        actions={
          <div className="flex gap-2">
            <Input placeholder="Numara ara" value={number} onChange={(e) => setNumber(e.target.value)} onKeyDown={(e) => e.key === "Enter" && (setPage(1), load())} className="w-40" />
            <Select value={direction} onChange={(e) => { setDirection(e.target.value); setPage(1); }} className="w-32">
              <option value="">Tüm yönler</option>
              <option value="inbound">Gelen</option>
              <option value="outbound">Giden</option>
              <option value="internal">Dahili</option>
            </Select>
          </div>
        }
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-slate-400">
              <th className="pb-2">Yön</th>
              <th className="pb-2">Kimden</th>
              <th className="pb-2">Kime</th>
              <th className="pb-2">Durum</th>
              <th className="pb-2">Süre</th>
              <th className="pb-2">Zaman</th>
            </tr>
          </thead>
          <tbody>
            {calls.map((c) => (
              <tr
                key={c.id}
                className="cursor-pointer border-t border-slate-100 hover:bg-slate-50"
                onClick={() => api.getCall(c.id).then(setSelected).catch(() => undefined)}
              >
                <td className="py-2"><Direction value={c.direction} /></td>
                <td className="py-2">{c.fromNumber}</td>
                <td className="py-2">{c.toNumber}</td>
                <td className="py-2"><CallDisposition value={c.disposition} /></td>
                <td className="py-2">{formatDuration(c.talkSeconds)}</td>
                <td className="py-2 text-slate-400">{formatTime(c.startedAt)}</td>
              </tr>
            ))}
            {calls.length === 0 && (
              <tr>
                <td colSpan={6} className="py-6 text-center text-sm text-slate-400">Kayıt yok.</td>
              </tr>
            )}
          </tbody>
        </table>
        <div className="mt-4 flex items-center justify-between text-sm text-slate-500">
          <Button variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>Önceki</Button>
          <span>Sayfa {page}</span>
          <Button variant="secondary" disabled={page * perPage >= total} onClick={() => setPage((p) => p + 1)}>Sonraki</Button>
        </div>
      </Card>

      <div>
        {selected ? <CallDetailPanel call={selected} onClose={() => setSelected(null)} /> : (
          <Card title="Çağrı detayı">
            <p className="text-sm text-slate-400">Detay için bir çağrı seçin.</p>
          </Card>
        )}
      </div>
    </div>
  );
}

function CallDetailPanel({ call, onClose }: { call: CallDetail; onClose: () => void }) {
  return (
    <Card title={`Çağrı #${call.id}`} actions={<Button variant="ghost" onClick={onClose}>Kapat</Button>}>
      <dl className="space-y-1 text-sm">
        <Row k="Yön"><Direction value={call.direction} /></Row>
        <Row k="Durum"><CallDisposition value={call.disposition} /></Row>
        <Row k="Kimden">{call.fromNumber}</Row>
        <Row k="Kime">{call.toNumber}</Row>
        <Row k="Çalma">{formatDuration(call.ringSeconds)}</Row>
        <Row k="Görüşme">{formatDuration(call.talkSeconds)}</Row>
        {call.hangupCauseCode !== undefined && (
          <Row k="Kapanış">{`Q.850 ${call.hangupCauseCode} ${call.hangupCauseText ?? ""}`}</Row>
        )}
      </dl>

      <h3 className="mb-2 mt-4 text-xs font-semibold uppercase text-slate-400">Zaman çizelgesi</h3>
      <ol className="space-y-1">
        {call.events.map((e) => (
          <li key={e.id} className="flex items-center gap-2 text-sm">
            <Badge tone="slate">{e.type}</Badge>
            <span className="text-slate-400">{formatTime(e.at)}</span>
          </li>
        ))}
        {call.events.length === 0 && <li className="text-sm text-slate-400">Olay yok.</li>}
      </ol>

      {call.quality.length > 0 && (
        <>
          <h3 className="mb-2 mt-4 text-xs font-semibold uppercase text-slate-400">Ses kalitesi</h3>
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-slate-400">
                <th>Bacak</th><th>Jitter</th><th>Kayıp</th><th>RTT</th><th>MOS</th>
              </tr>
            </thead>
            <tbody>
              {call.quality.map((q) => (
                <tr key={q.id} className="border-t border-slate-100">
                  <td>{q.leg}</td>
                  <td>{q.jitterMs?.toFixed(1) ?? "—"}</td>
                  <td>{q.lossPct?.toFixed(1) ?? "—"}%</td>
                  <td>{q.rttMs?.toFixed(0) ?? "—"}</td>
                  <td>{q.mos?.toFixed(2) ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Card>
  );
}

function Row({ k, children }: { k: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between">
      <dt className="text-slate-400">{k}</dt>
      <dd className="font-medium">{children}</dd>
    </div>
  );
}
