import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Call, Webphone } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { canAny } from "../lib/permissions";
import { Card } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

export function Dashboard() {
  const { user } = useAuth();
  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);

  return (
    <div className="grid gap-6 lg:grid-cols-[320px_1fr]">
      <Softphone hasExtension={!!user?.sipExtension} />
      <div>
        {canSeeCalls ? (
          <RecentCalls />
        ) : (
          <Card title="Hoş geldiniz">
            <p className="text-sm text-slate-600">Sol taraftaki softphone ile çağrı yapabilirsiniz.</p>
          </Card>
        )}
      </div>
    </div>
  );
}

function Softphone({ hasExtension }: { hasExtension: boolean }) {
  const [phone, setPhone] = useState<Webphone | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!hasExtension) {
      setLoading(false);
      return;
    }
    api
      .webphone()
      .then(setPhone)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Softphone yüklenemedi."))
      .finally(() => setLoading(false));
  }, [hasExtension]);

  return (
    <Card title="Softphone">
      {!hasExtension ? (
        <p className="text-sm text-slate-500">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
      ) : loading ? (
        <p className="text-sm text-slate-400">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-red-600">{error}</p>
      ) : phone ? (
        <iframe
          title="Bulutsantralim Web Telefonu"
          src={phone.url}
          allow="microphone"
          className="h-[640px] w-full rounded-lg border border-slate-200"
        />
      ) : null}
    </Card>
  );
}

function RecentCalls() {
  const [calls, setCalls] = useState<Call[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .listCalls({ perPage: 10 })
      .then((r) => setCalls(r.items))
      .catch((e) => setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."))
      .finally(() => setLoading(false));
  }, []);

  return (
    <Card title="Son çağrılar">
      {loading ? (
        <p className="text-sm text-slate-400">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-red-600">{error}</p>
      ) : calls.length === 0 ? (
        <p className="text-sm text-slate-400">Henüz çağrı kaydı yok.</p>
      ) : (
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
              <tr key={c.uuid} className="border-t border-slate-100">
                <td className="py-2"><Direction value={c.direction} /></td>
                <td className="py-2">{c.fromNumber}</td>
                <td className="py-2">{c.toNumber}</td>
                <td className="py-2"><CallDisposition value={c.disposition} /></td>
                <td className="py-2">{formatDuration(c.durationSeconds)}</td>
                <td className="py-2 text-slate-400">{formatStamp(c.startedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}
