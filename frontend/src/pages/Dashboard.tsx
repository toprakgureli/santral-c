import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { Call } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { canAny } from "../lib/permissions";
import { useSoftphone } from "../softphone/useSoftphone";
import { Badge, Button, Card, Input } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatTime } from "./callFormat";

const statusLabel: Record<string, string> = {
  idle: "Hazır değil",
  connecting: "Bağlanıyor...",
  registered: "Hazır",
  ringing: "Çalıyor...",
  "in-call": "Görüşmede",
  error: "Hata",
  disabled: "Softphone yok",
};

const statusTone: Record<string, "slate" | "green" | "amber" | "red" | "blue"> = {
  registered: "green",
  "in-call": "blue",
  ringing: "amber",
  error: "red",
  connecting: "amber",
  idle: "slate",
  disabled: "slate",
};

export function Dashboard() {
  const { user } = useAuth();
  const hasPhone = !!user?.sipExtension;
  const phone = useSoftphone(hasPhone);
  const [target, setTarget] = useState("");

  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);

  return (
    <div className="grid gap-6 md:grid-cols-[360px_1fr]">
      <div className="space-y-4">
        <Card title="Softphone">
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-sm text-slate-500">Dahili</div>
                <div className="text-lg font-semibold">{phone.extension ?? user?.sipExtension ?? "—"}</div>
              </div>
              <Badge tone={statusTone[phone.status]}>{statusLabel[phone.status]}</Badge>
            </div>

            {phone.error && <p className="text-sm text-red-600">{phone.error}</p>}

            {hasPhone ? (
              <>
                <div className="flex gap-2">
                  <Input
                    placeholder="Numara veya dahili"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && target) phone.call(target).catch(() => undefined);
                    }}
                  />
                  <Button
                    onClick={() => target && phone.call(target).catch(() => undefined)}
                    disabled={phone.status !== "registered" || !target}
                  >
                    Ara
                  </Button>
                </div>

                {phone.incomingFrom && (
                  <div className="flex items-center justify-between rounded-lg bg-amber-50 px-3 py-2">
                    <span className="text-sm text-amber-700">{phone.incomingFrom}</span>
                    <Button variant="primary" onClick={() => phone.answer().catch(() => undefined)}>
                      Cevapla
                    </Button>
                  </div>
                )}

                {(phone.status === "in-call" || phone.status === "ringing") && (
                  <Button variant="danger" className="w-full" onClick={() => phone.hangup().catch(() => undefined)}>
                    Kapat
                  </Button>
                )}
              </>
            ) : (
              <p className="text-sm text-slate-500">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
            )}
            <audio ref={phone.audioRef} autoPlay />
          </div>
        </Card>
      </div>

      <div className="space-y-4">
        {canSeeCalls ? <RecentCalls /> : (
          <Card title="Hoş geldiniz">
            <p className="text-sm text-slate-600">Sol taraftaki softphone ile çağrı yapabilirsiniz.</p>
          </Card>
        )}
      </div>
    </div>
  );
}

function RecentCalls() {
  const [calls, setCalls] = useState<Call[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .listCalls({ perPage: 8 })
      .then((r) => setCalls(r.items))
      .catch(() => setCalls([]))
      .finally(() => setLoading(false));
  }, []);

  return (
    <Card title="Son çağrılar">
      {loading ? (
        <p className="text-sm text-slate-400">Yükleniyor...</p>
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
              <tr key={c.id} className="border-t border-slate-100">
                <td className="py-2"><Direction value={c.direction} /></td>
                <td className="py-2">{c.fromNumber}</td>
                <td className="py-2">{c.toNumber}</td>
                <td className="py-2"><CallDisposition value={c.disposition} /></td>
                <td className="py-2">{formatDuration(c.talkSeconds)}</td>
                <td className="py-2 text-slate-400">{formatTime(c.startedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}
