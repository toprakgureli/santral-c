import { useEffect, useRef, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Call, PBXExtension, PBXQueue } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { canAny } from "../lib/permissions";
import { useSoftphone } from "../softphone/useSoftphone";
import { tones } from "../softphone/tones";
import { Badge, Button, Card, Input, Select } from "../components/ui";
import { CallDisposition, Direction, formatDuration, formatStamp } from "./callFormat";

const statusLabel: Record<string, string> = {
  connecting: "Bağlanıyor...",
  registered: "Hazır",
  calling: "Aranıyor...",
  ringing: "Çalıyor...",
  incoming: "Gelen çağrı",
  "in-call": "Görüşmede",
  held: "Beklemede",
  error: "Hata",
  disabled: "Softphone yok",
};

const statusTone: Record<string, "slate" | "green" | "amber" | "red" | "blue"> = {
  registered: "green",
  "in-call": "blue",
  held: "amber",
  calling: "amber",
  ringing: "amber",
  incoming: "amber",
  connecting: "amber",
  error: "red",
  disabled: "slate",
};

const keypadKeys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"];

export function Dashboard() {
  const { user } = useAuth();
  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);

  return (
    <div className="grid gap-6 lg:grid-cols-[340px_1fr]">
      <Softphone hasExtension={!!user?.sipExtension} />
      <div>
        {canSeeCalls ? (
          <RecentCalls />
        ) : (
          <Card title="Hoş geldiniz">
            <p className="text-sm text-muted-foreground">Sol taraftaki softphone ile çağrı yapabilirsiniz.</p>
          </Card>
        )}
      </div>
    </div>
  );
}

function Softphone({ hasExtension }: { hasExtension: boolean }) {
  const phone = useSoftphone(hasExtension);
  const [target, setTarget] = useState("");
  const [xfer, setXfer] = useState("");
  const [showKeypad, setShowKeypad] = useState(false);
  const [exts, setExts] = useState<PBXExtension[]>([]);
  const [queues, setQueues] = useState<PBXQueue[]>([]);
  const [selExt, setSelExt] = useState("");
  const [selQueue, setSelQueue] = useState("");
  const dirLoaded = useRef(false);

  const idle = phone.status === "registered" || phone.status === "error" || phone.status === "connecting";
  const outgoing = phone.status === "calling" || phone.status === "ringing";
  const active = phone.status === "in-call" || phone.status === "held";

  useEffect(() => {
    if (active && !dirLoaded.current) {
      dirLoaded.current = true;
      api.pbxExtensions().then(setExts).catch(() => undefined);
      api.pbxQueues().then(setQueues).catch(() => undefined);
    }
  }, [active]);

  return (
    <Card title="Softphone">
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <div className="text-xs text-muted-foreground">Dahili</div>
            <div className="text-lg font-semibold">{phone.extension ?? "—"}</div>
          </div>
          <Badge tone={statusTone[phone.status]}>{statusLabel[phone.status]}</Badge>
        </div>

        {!hasExtension ? (
          <p className="text-sm text-muted-foreground">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
        ) : (
          <>
            {phone.error && <p className="text-sm text-destructive">{phone.error}</p>}

            {idle && (
              <>
                {phone.endReason && <p className="text-xs text-muted-foreground">Son çağrı: {phone.endReason}</p>}
                <div className="flex gap-2">
                  <Input
                    placeholder="Numara veya dahili"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && target && phone.call(target).catch(() => undefined)}
                  />
                  <Button onClick={() => target && phone.call(target).catch(() => undefined)} disabled={phone.status !== "registered" || !target}>
                    Ara
                  </Button>
                </div>
              </>
            )}

            {(outgoing || active) && (
              <div className="rounded-lg bg-muted/60 px-3 py-2 text-sm text-foreground">
                {outgoing ? "Aranıyor: " : "Görüşme: "}
                <span className="font-medium">{phone.peer}</span>
              </div>
            )}

            {phone.status === "incoming" && (
              <div className="flex items-center justify-between rounded-lg bg-warning/10 px-3 py-2">
                <span className="text-sm text-warning">Gelen çağrı: {phone.peer}</span>
                <div className="flex gap-2">
                  <Button onClick={() => phone.answer().catch(() => undefined)}>Cevapla</Button>
                  <Button variant="danger" onClick={() => phone.hangup().catch(() => undefined)}>Reddet</Button>
                </div>
              </div>
            )}

            {active && (
              <div className="space-y-3">
                <div className="grid grid-cols-3 gap-2">
                  <Button variant="secondary" onClick={phone.toggleMute}>{phone.muted ? "Susturmayı aç" : "Sustur"}</Button>
                  <Button variant="secondary" onClick={() => phone.toggleHold().catch(() => undefined)}>{phone.held ? "Devam et" : "Beklet"}</Button>
                  <Button variant="secondary" onClick={() => setShowKeypad((v) => !v)}>Tuşlar</Button>
                </div>

                {showKeypad && (
                  <div className="grid grid-cols-3 gap-2">
                    {keypadKeys.map((k) => (
                      <Button key={k} variant="secondary" onClick={() => { tones.dtmf(k); phone.sendDtmf(k); }}>{k}</Button>
                    ))}
                  </div>
                )}

                <div className="space-y-2 border-t border-border pt-3">
                  <div className="flex gap-2">
                    <Input placeholder="Numaraya aktar" value={xfer} onChange={(e) => setXfer(e.target.value)} />
                    <Button variant="secondary" onClick={() => xfer && phone.transfer(xfer).catch(() => undefined)} disabled={!xfer}>
                      Aktar
                    </Button>
                  </div>
                  <div className="flex gap-2">
                    <Select value={selExt} onChange={(e) => setSelExt(e.target.value)}>
                      <option value="">Dahiliye aktar...</option>
                      {exts.map((x) => (
                        <option key={x.extension} value={x.extension}>
                          {x.extension} ({x.status})
                        </option>
                      ))}
                    </Select>
                    <Button variant="secondary" disabled={!selExt} onClick={() => phone.transfer(selExt).catch(() => undefined)}>
                      Aktar
                    </Button>
                  </div>
                  <div className="flex gap-2">
                    <Select value={selQueue} onChange={(e) => setSelQueue(e.target.value)}>
                      <option value="">Kuyruğa aktar...</option>
                      {queues.map((q) => (
                        <option key={q.number} value={q.number}>
                          {q.number} - {q.name}
                        </option>
                      ))}
                    </Select>
                    <Button variant="secondary" disabled={!selQueue} onClick={() => phone.transfer(selQueue).catch(() => undefined)}>
                      Aktar
                    </Button>
                  </div>
                </div>
              </div>
            )}

            {(outgoing || active || phone.status === "incoming") && (
              <Button variant="danger" className="w-full" onClick={() => phone.hangup().catch(() => undefined)}>
                Kapat
              </Button>
            )}
          </>
        )}
        <audio ref={phone.audioRef} autoPlay />
      </div>
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
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : calls.length === 0 ? (
        <p className="text-sm text-muted-foreground">Henüz çağrı kaydı yok.</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
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
              <tr key={c.uuid} className="border-t border-border/60">
                <td className="py-2"><Direction value={c.direction} /></td>
                <td className="py-2">{c.fromNumber}</td>
                <td className="py-2">{c.toNumber}</td>
                <td className="py-2"><CallDisposition value={c.disposition} /></td>
                <td className="py-2">{formatDuration(c.durationSeconds)}</td>
                <td className="py-2 text-muted-foreground">{formatStamp(c.startedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}
