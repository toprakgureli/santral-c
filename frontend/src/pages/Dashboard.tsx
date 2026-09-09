import { useEffect, useRef, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Call, PBXExtension, PBXQueue } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { useSoftphoneContext } from "../softphone/SoftphoneContext";
import { tones } from "../softphone/tones";
import { Badge, Button, Card, Input } from "../components/ui";
import { ContextMenu, type MenuItem } from "../components/ContextMenu";
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

const agentStatus: Record<string, { label: string; tone: "green" | "amber" | "slate" | "red" }> = {
  AVAILABLE: { label: "Boşta", tone: "green" },
  TALKING: { label: "Görüşmede", tone: "amber" },
  UNREGISTERED: { label: "Kayıtsız", tone: "slate" },
  SS_DND: { label: "Rahatsız etmeyin", tone: "red" },
};

export function Dashboard() {
  const { user } = useAuth();
  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);
  const canTransfer = can(user, "call.transfer");

  const [exts, setExts] = useState<PBXExtension[]>([]);
  const [queues, setQueues] = useState<PBXQueue[]>([]);

  useEffect(() => {
    if (!canTransfer) return;
    let live = true;
    const loadExts = () => api.pbxExtensions().then((d) => live && setExts(d)).catch(() => undefined);
    loadExts();
    api.pbxQueues().then((d) => live && setQueues(d)).catch(() => undefined);
    const timer = window.setInterval(loadExts, 30000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [canTransfer]);

  const totals = {
    available: exts.filter((e) => e.status === "AVAILABLE").length,
    talking: exts.filter((e) => e.status === "TALKING").length,
    offline: exts.filter((e) => e.status === "UNREGISTERED").length,
  };

  return (
    <div className="space-y-4">
      <StatusBar totals={totals} showTotals={canTransfer} extension={user?.sipExtension} />
      <div className="grid gap-4 xl:grid-cols-[1fr_1.3fr_1fr]">
        {canSeeCalls ? <CallHistory /> : <div className="hidden xl:block" />}
        <Softphone hasExtension={!!user?.sipExtension} />
        {canTransfer ? <AgentsQueues exts={exts} queues={queues} /> : <div className="hidden xl:block" />}
      </div>
    </div>
  );
}

function StatusBar({ totals, showTotals, extension }: { totals: { available: number; talking: number; offline: number }; showTotals: boolean; extension?: string }) {
  const phone = useSoftphoneContext();
  return (
    <div className="flex flex-wrap items-center justify-between gap-4 rounded-2xl bg-card px-5 py-3 ring-1 ring-border/60">
      <div className="flex items-center gap-4">
        <div>
          <div className="text-xs text-muted-foreground">Dahili</div>
          <div className="text-lg font-semibold">{phone.extension ?? extension ?? "—"}</div>
        </div>
        <Badge tone={statusTone[phone.status]}>{statusLabel[phone.status]}</Badge>
      </div>
      {showTotals && (
        <div className="flex items-center gap-5 text-sm">
          <Stat dot="bg-success" label="Boşta" value={totals.available} />
          <Stat dot="bg-warning" label="Görüşmede" value={totals.talking} />
          <Stat dot="bg-muted-foreground/60" label="Çevrimdışı" value={totals.offline} />
        </div>
      )}
    </div>
  );
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

function Softphone({ hasExtension }: { hasExtension: boolean }) {
  const phone = useSoftphoneContext();
  const [target, setTarget] = useState("");
  const [xfer, setXfer] = useState("");
  const [showKeypad, setShowKeypad] = useState(false);

  const idle = phone.status === "registered" || phone.status === "error" || phone.status === "connecting";
  const outgoing = phone.status === "calling" || phone.status === "ringing";
  const active = phone.status === "in-call" || phone.status === "held";

  return (
    <Card title="Softphone">
      <div className="space-y-4">
        {!hasExtension ? (
          <p className="text-sm text-muted-foreground">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
        ) : phone.secondary ? (
          <p className="text-sm text-muted-foreground">Softphone başka bir sekmede açık. Çağrılar orada yönetiliyor.</p>
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

                <div className="flex gap-2 border-t border-border pt-3">
                  <Input placeholder="Numaraya aktar" value={xfer} onChange={(e) => setXfer(e.target.value)} />
                  <Button variant="secondary" onClick={() => xfer && phone.transfer(xfer).catch(() => undefined)} disabled={!xfer}>
                    Aktar
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">Dahili/kuyruğa aktarmak için sağdaki listeye sağ tıkla.</p>
              </div>
            )}

            {(outgoing || active || phone.status === "incoming") && (
              <Button variant="danger" className="w-full" onClick={() => phone.hangup().catch(() => undefined)}>
                Kapat
              </Button>
            )}
          </>
        )}
      </div>
    </Card>
  );
}

function AgentsQueues({ exts, queues }: { exts: PBXExtension[]; queues: PBXQueue[] }) {
  const phone = useSoftphoneContext();
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const inCall = phone.status === "in-call" || phone.status === "held";
  const canDial = phone.status === "registered";

  const sorted = [...exts].sort((a, b) => (a.status === "UNREGISTERED" ? 1 : 0) - (b.status === "UNREGISTERED" ? 1 : 0));
  const online = exts.filter((e) => e.status !== "UNREGISTERED").length;

  function agentMenu(e: React.MouseEvent, ext: string) {
    e.preventDefault();
    const items: MenuItem[] = [];
    if (inCall) items.push({ label: `📞 ${ext} dahilisine aktar`, onClick: () => phone.transfer(ext).catch(() => undefined) });
    if (canDial) items.push({ label: `📞 ${ext} numarasını ara`, onClick: () => phone.call(ext).catch(() => undefined) });
    if (items.length === 0) items.push({ label: "Aktarmak için görüşmede olun", onClick: () => undefined, disabled: true });
    setMenu({ x: e.clientX, y: e.clientY, items });
  }
  function queueMenu(e: React.MouseEvent, num: string) {
    e.preventDefault();
    const items: MenuItem[] = inCall
      ? [{ label: `📞 ${num} kuyruğuna aktar`, onClick: () => phone.transfer(num).catch(() => undefined) }]
      : [{ label: "Aktarmak için görüşmede olun", onClick: () => undefined, disabled: true }];
    setMenu({ x: e.clientX, y: e.clientY, items });
  }

  return (
    <div className="space-y-4">
      <Card title={`Hatta Olanlar (${online})`}>
        <ul className="max-h-72 space-y-0.5 overflow-y-auto">
          {sorted.map((e) => {
            const s = agentStatus[e.status] ?? { label: e.status, tone: "slate" as const };
            return (
              <li
                key={e.extension}
                onContextMenu={(ev) => agentMenu(ev, e.extension)}
                onClick={() => canDial && phone.call(e.extension).catch(() => undefined)}
                title="Sağ tık: aktar/ara"
                className="flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 hover:bg-accent"
              >
                <span className="flex items-center gap-2 text-sm">
                  <span className={`size-2 rounded-full ${s.tone === "green" ? "bg-success" : s.tone === "amber" ? "bg-warning" : s.tone === "red" ? "bg-destructive" : "bg-muted-foreground/50"}`} />
                  <span className="font-medium">{e.extension}</span>
                </span>
                <Badge tone={s.tone}>{s.label}</Badge>
              </li>
            );
          })}
          {exts.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Liste alınamadı.</li>}
        </ul>
      </Card>

      <Card title="Kuyruklar">
        <ul className="max-h-56 space-y-0.5 overflow-y-auto">
          {queues.map((q) => (
            <li
              key={q.number}
              onContextMenu={(ev) => queueMenu(ev, q.number)}
              title="Sağ tık: kuyruğa aktar"
              className="flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 hover:bg-accent"
            >
              <span className="text-sm"><span className="font-medium">{q.number}</span> <span className="text-muted-foreground">{q.name}</span></span>
            </li>
          ))}
          {queues.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Kuyruk yok.</li>}
        </ul>
      </Card>

      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
    </div>
  );
}

function CallHistory() {
  const phone = useSoftphoneContext();
  const [calls, setCalls] = useState<Call[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const canDial = phone.status === "registered";
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current) return;
    loaded.current = true;
    api
      .listCalls({ perPage: 20 })
      .then((r) => setCalls(r.items))
      .catch((e) => setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi."))
      .finally(() => setLoading(false));
  }, []);

  return (
    <Card title="Çağrı Geçmişi">
      {loading ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : calls.length === 0 ? (
        <p className="text-sm text-muted-foreground">Henüz çağrı kaydı yok.</p>
      ) : (
        <ul className="max-h-[32rem] space-y-0.5 overflow-y-auto">
          {calls.map((c) => {
            const counterpart = c.direction === "outbound" ? c.toNumber : c.fromNumber;
            return (
              <li
                key={c.uuid}
                onClick={() => canDial && counterpart && phone.call(counterpart).catch(() => undefined)}
                title={canDial ? "Ara" : undefined}
                className="flex cursor-pointer items-center justify-between gap-2 rounded-lg px-2 py-2 hover:bg-accent"
              >
                <span className="flex items-center gap-2 min-w-0">
                  <Direction value={c.direction} />
                  <span className="truncate text-sm font-medium">{counterpart || "—"}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <CallDisposition value={c.disposition} />
                  <span className="text-xs text-muted-foreground">{formatDuration(c.durationSeconds)}</span>
                  <span className="hidden text-xs text-muted-foreground sm:inline">{formatStamp(c.startedAt)}</span>
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}
