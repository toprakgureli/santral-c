import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeftRight,
  ChevronDown,
  Copy,
  Delete,
  Grid3x3,
  Mic,
  MicOff,
  Pause,
  Phone,
  PhoneOff,
  Play,
  Search,
} from "lucide-react";
import { api, ApiError } from "../api/client";
import type { Call, EscalationCategory, EscalationRecord, PBXExtension, PBXQueue, PBXStats } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { useSoftphoneContext } from "../softphone/SoftphoneContext";
import { displayNumber, normalizeDial } from "../softphone/dial";
import { tones } from "../softphone/tones";
import { Badge, Button, Card, Select } from "../components/ui";
import { cn } from "../lib/utils";
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
  const canEscalate = can(user, "escalation.view");
  const phone = useSoftphoneContext();

  const [exts, setExts] = useState<PBXExtension[]>([]);
  const [queues, setQueues] = useState<PBXQueue[]>([]);
  const [stats, setStats] = useState<PBXStats | null>(null);
  const [categories, setCategories] = useState<EscalationCategory[]>([]);

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

  useEffect(() => {
    let live = true;
    const load = () => api.pbxStats().then((d) => live && setStats(d)).catch(() => undefined);
    load();
    const timer = window.setInterval(load, 60000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    if (!canEscalate) return;
    api.escalationCategories().then(setCategories).catch(() => undefined);
  }, [canEscalate]);

  const totals = {
    available: exts.filter((e) => e.status === "AVAILABLE").length,
    talking: exts.filter((e) => e.status === "TALKING").length,
    offline: exts.filter((e) => e.status === "UNREGISTERED").length,
  };

  return (
    <div className="space-y-4">
      <StatusBar totals={totals} showTotals={canTransfer} extension={user?.sipExtension} hasExtension={!!user?.sipExtension} stats={stats} />
      <div className="grid gap-4 xl:grid-cols-[1fr_1.3fr_1fr]">
        {canSeeCalls ? <CallHistory /> : <div className="hidden xl:block" />}
        <div className="space-y-4">
          <Softphone hasExtension={!!user?.sipExtension} />
          {canEscalate && <Escalation categories={categories} activePeer={phone.peer ?? undefined} />}
        </div>
        {canTransfer ? <AgentsQueues exts={exts} queues={queues} /> : <div className="hidden xl:block" />}
      </div>
    </div>
  );
}

const AGENT_STATE_KEY = "santral.agentStatus";
const agentStates: Record<string, { label: string; tone: "green" | "amber" | "red" }> = {
  available: { label: "Müsait", tone: "green" },
  break: { label: "Molada", tone: "amber" },
  backoffice: { label: "Backoffice", tone: "amber" },
  dnd: { label: "Rahatsız Etmeyin", tone: "red" },
};

function StatusBar({ totals, showTotals, extension, hasExtension, stats }: { totals: { available: number; talking: number; offline: number }; showTotals: boolean; extension?: string; hasExtension: boolean; stats: PBXStats | null }) {
  const phone = useSoftphoneContext();
  const [agentState, setAgentState] = useState<string>(() => {
    try {
      return localStorage.getItem(AGENT_STATE_KEY) ?? "available";
    } catch {
      return "available";
    }
  });
  const busy = phone.status === "in-call" || phone.status === "held" || phone.status === "ringing" || phone.status === "calling" || phone.status === "incoming";
  const state = agentStates[agentState] ?? agentStates.available;

  function changeState(v: string) {
    setAgentState(v);
    try {
      localStorage.setItem(AGENT_STATE_KEY, v);
    } catch {
      // ignore
    }
    api.setAgentStatus(v !== "available").catch(() => undefined);
  }

  // While in a call the live call status wins; otherwise the presence badge
  // reflects the agent's chosen state (so "Molada" no longer shows green).
  const badgeTone = busy ? statusTone[phone.status] : state.tone;
  const badgeLabel = busy ? statusLabel[phone.status] : hasExtension ? state.label : statusLabel[phone.status];
  const dotColor = badgeTone === "green" ? "bg-success" : badgeTone === "red" ? "bg-destructive" : badgeTone === "blue" ? "bg-primary" : badgeTone === "amber" ? "bg-warning" : "bg-muted-foreground/50";

  return (
    <div className="flex flex-wrap items-center justify-between gap-4 rounded-2xl bg-card px-5 py-3 ring-1 ring-border/60">
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2.5">
          <span className={cn("size-2.5 rounded-full", dotColor, !busy && state.tone === "green" && "animate-pulse")} />
          <div>
            <div className="text-xs text-muted-foreground">Dahili</div>
            <div className="text-lg font-semibold leading-tight">{phone.extension ?? extension ?? "—"}</div>
          </div>
        </div>
        <Badge tone={badgeTone}>{badgeLabel}</Badge>
        {hasExtension && (
          <Select value={agentState} onChange={(e) => changeState(e.target.value)} className="h-9 w-40">
            {Object.entries(agentStates).map(([v, s]) => (
              <option key={v} value={v}>
                {s.label}
              </option>
            ))}
          </Select>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-5 text-sm">
        {stats && (
          <>
            <Stat dot="bg-primary" label="Bugün" value={stats.total} />
            <Stat dot="bg-destructive" label="Cevapsız" value={stats.missed} />
          </>
        )}
        {showTotals && (
          <>
            <Stat dot="bg-success" label="Boşta" value={totals.available} />
            <Stat dot="bg-warning" label="Görüşmede" value={totals.talking} />
            <Stat dot="bg-muted-foreground/60" label="Çevrimdışı" value={totals.offline} />
          </>
        )}
      </div>
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

function Round({ onClick, tone = "muted", title, disabled, size = "md", children }: { onClick?: () => void; tone?: "muted" | "on" | "call" | "hang"; title?: string; disabled?: boolean; size?: "md" | "lg"; children: React.ReactNode }) {
  const toneClass: Record<string, string> = {
    muted: "bg-muted text-foreground hover:bg-accent",
    on: "bg-primary text-primary-foreground",
    call: "bg-success text-white hover:opacity-90",
    hang: "bg-destructive text-white hover:opacity-90",
  };
  const sizeClass = size === "lg" ? "size-16 [&_svg]:size-6" : "size-12 [&_svg]:size-5";
  return (
    <button onClick={onClick} disabled={disabled} title={title} className={cn("flex items-center justify-center rounded-full shadow-sm transition active:scale-95 disabled:opacity-40", sizeClass, toneClass[tone])}>
      {children}
    </button>
  );
}

function Softphone({ hasExtension }: { hasExtension: boolean }) {
  const phone = useSoftphoneContext();
  const [target, setTarget] = useState("");
  const [showKeypad, setShowKeypad] = useState(false);
  const [dur, setDur] = useState(0);

  const idle = phone.status === "registered" || phone.status === "error" || phone.status === "connecting";
  const outgoing = phone.status === "calling" || phone.status === "ringing";
  const active = phone.status === "in-call" || phone.status === "held";

  useEffect(() => {
    if (!active) { setDur(0); return; }
    const t = window.setInterval(() => setDur((d) => d + 1), 1000);
    return () => window.clearInterval(t);
  }, [active]);

  function callNow() {
    const n = normalizeDial(target);
    if (n) phone.call(n).catch(() => undefined);
  }

  return (
    <Card title="Softphone">
      {!hasExtension ? (
        <p className="text-sm text-muted-foreground">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
      ) : phone.secondary ? (
        <p className="text-sm text-muted-foreground">Softphone başka bir sekmede açık. Çağrılar orada yönetiliyor.</p>
      ) : (
        <div className="space-y-4">
          {phone.error && <p className="text-sm text-destructive">{phone.error}</p>}

          {/* Idle: number entry + dialpad */}
          {idle && (
            <>
              <div className="space-y-1">
                <input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && callNow()}
                  placeholder="Numara veya dahili"
                  inputMode="tel"
                  className="w-full bg-transparent text-center text-3xl font-semibold tracking-wide text-foreground outline-none placeholder:text-muted-foreground/40"
                />
                <p className="h-4 text-center text-xs text-muted-foreground">{phone.endReason ? `Son çağrı: ${phone.endReason}` : ""}</p>
              </div>
              <div className="mx-auto grid max-w-[15rem] grid-cols-3 gap-2">
                {keypadKeys.map((k) => (
                  <button
                    key={k}
                    onClick={() => { setTarget((t) => t + k); tones.dtmf(k); }}
                    className="h-12 rounded-xl bg-muted text-lg font-semibold text-foreground transition active:scale-95 hover:bg-accent"
                  >
                    {k}
                  </button>
                ))}
              </div>
              <div className="mx-auto flex max-w-[15rem] items-center justify-between">
                <span className="size-12" />
                <Round tone="call" size="lg" title="Ara" onClick={callNow} disabled={phone.status !== "registered" || !target}>
                  <Phone />
                </Round>
                <Round title="Sil" onClick={() => setTarget((t) => t.slice(0, -1))} disabled={!target}>
                  <Delete />
                </Round>
              </div>
            </>
          )}

          {/* Ringing / incoming / in-call: a centred call face */}
          {!idle && (
            <div className="flex flex-col items-center gap-5 py-2">
              <div className="text-center">
                <div className="text-2xl font-semibold tracking-wide">{displayNumber(phone.peer || "") || phone.peer || "—"}</div>
                <div className="mt-1 text-sm text-muted-foreground">
                  {phone.status === "incoming" ? "Gelen çağrı" : outgoing ? "Aranıyor..." : phone.held ? "Beklemede" : "Görüşme"}
                </div>
              </div>

              {active && (
                <div className="font-mono text-5xl font-semibold tabular-nums tracking-tight">{formatDuration(dur)}</div>
              )}

              {phone.status === "incoming" && (
                <div className="flex items-center justify-center gap-12 pt-1">
                  <Round tone="call" size="lg" title="Cevapla" onClick={() => phone.answer().catch(() => undefined)}><Phone /></Round>
                  <Round tone="hang" size="lg" title="Reddet" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
                </div>
              )}

              {active && (
                <div className="flex flex-col items-center gap-3">
                  <div className="flex items-center justify-center gap-3">
                    <Round tone={phone.muted ? "on" : "muted"} title="Sustur" onClick={phone.toggleMute}>{phone.muted ? <MicOff /> : <Mic />}</Round>
                    <Round tone={phone.held ? "on" : "muted"} title="Beklet" onClick={() => phone.toggleHold().catch(() => undefined)}>{phone.held ? <Play /> : <Pause />}</Round>
                    <Round tone={showKeypad ? "on" : "muted"} title="Tuşlar" onClick={() => setShowKeypad((v) => !v)}><Grid3x3 /></Round>
                    <Round tone="hang" size="lg" title="Kapat" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
                  </div>
                  {showKeypad && (
                    <div className="mx-auto grid max-w-[15rem] grid-cols-3 gap-2">
                      {keypadKeys.map((k) => (
                        <button key={k} onClick={() => { tones.dtmf(k); phone.sendDtmf(k); }} className="h-11 rounded-xl bg-muted text-lg font-semibold transition active:scale-95 hover:bg-accent">
                          {k}
                        </button>
                      ))}
                    </div>
                  )}
                  <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                    <ArrowLeftRight className="size-3.5" /> Aktarmak için sağdaki listeye sağ tıkla
                  </p>
                </div>
              )}

              {outgoing && (
                <Round tone="hang" size="lg" title="Kapat" onClick={() => phone.hangup().catch(() => undefined)}><PhoneOff /></Round>
              )}
            </div>
          )}
        </div>
      )}
    </Card>
  );
}

function Escalation({ categories, activePeer }: { categories: EscalationCategory[]; activePeer?: string }) {
  const [number, setNumber] = useState("");
  const [catId, setCatId] = useState<number | "">("");
  const [reasonId, setReasonId] = useState<number | "">("");
  const [note, setNote] = useState("");
  const [history, setHistory] = useState<EscalationRecord[]>([]);
  const [status, setStatus] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  // When a call is live, follow its number so the agent logs against the right
  // customer without retyping.
  useEffect(() => {
    if (activePeer) setNumber(displayNumber(activePeer));
  }, [activePeer]);

  const loadHistory = useCallback((n: string) => {
    const key = n.trim();
    if (!key) { setHistory([]); return; }
    api.escalationHistory(key).then(setHistory).catch(() => setHistory([]));
  }, []);

  useEffect(() => {
    const t = window.setTimeout(() => loadHistory(number), 400);
    return () => window.clearTimeout(t);
  }, [number, loadHistory]);

  const reasons = useMemo(() => categories.find((c) => c.id === catId)?.reasons ?? [], [categories, catId]);

  async function save() {
    if (!number.trim() || reasonId === "") { setStatus("Numara ve durum seçin."); return; }
    setSaving(true);
    setStatus(null);
    try {
      await api.logEscalation({ number: number.trim(), reasonId: Number(reasonId), note: note.trim() || undefined });
      setNote("");
      setStatus("Eskalasyon kaydedildi.");
      loadHistory(number);
    } catch (e) {
      setStatus(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card title="Eskalasyon / Müşteri Ara">
      <div className="space-y-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            value={number}
            onChange={(e) => setNumber(e.target.value)}
            placeholder="Müşteri numarası"
            inputMode="tel"
            className="h-10 w-full rounded-xl border border-border/70 bg-muted/40 pl-9 pr-3 text-sm outline-none focus-visible:border-ring/60 focus-visible:bg-card"
          />
        </div>

        {categories.length > 0 ? (
          <div className="grid grid-cols-2 gap-2">
            <Select value={catId} onChange={(e) => { setCatId(e.target.value ? Number(e.target.value) : ""); setReasonId(""); }} className="h-9">
              <option value="">Kategori</option>
              {categories.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </Select>
            <Select value={reasonId} onChange={(e) => setReasonId(e.target.value ? Number(e.target.value) : "")} className="h-9" disabled={catId === ""}>
              <option value="">Durum</option>
              {reasons.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
            </Select>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">Henüz eskalasyon durumu tanımlı değil. Yönetici, Eskalasyon menüsünden ekleyebilir.</p>
        )}

        <textarea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Not (opsiyonel)"
          rows={2}
          className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3 py-2 text-sm outline-none focus-visible:border-ring/60 focus-visible:bg-card"
        />
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">{status}</span>
          <Button onClick={save} disabled={saving || reasonId === "" || !number.trim()}>{saving ? "Kaydediliyor..." : "Kaydet"}</Button>
        </div>

        {history.length > 0 && (
          <div className="space-y-1.5 border-t border-border/60 pt-3">
            <p className="text-xs font-medium text-muted-foreground">Geçmiş eskalasyonlar</p>
            <ul className="max-h-40 space-y-1.5 overflow-y-auto">
              {history.map((h) => (
                <li key={h.id} className="rounded-lg bg-muted/50 px-3 py-2 text-xs">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium text-foreground">{h.agentName} görüştü</span>
                    <span className="text-muted-foreground">{h.createdAt}</span>
                  </div>
                  <div className="mt-0.5 text-muted-foreground">
                    {h.categoryName} · {h.reasonName}
                  </div>
                  {h.note && <div className="mt-0.5 text-foreground/80">Not: {h.note}</div>}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </Card>
  );
}

function AgentsQueues({ exts, queues }: { exts: PBXExtension[]; queues: PBXQueue[] }) {
  const phone = useSoftphoneContext();
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [confirmExt, setConfirmExt] = useState<string | null>(null);
  const inCall = phone.status === "in-call" || phone.status === "held";
  const canDial = phone.status === "registered";

  const sorted = [...exts].sort((a, b) => (a.status === "UNREGISTERED" ? 1 : 0) - (b.status === "UNREGISTERED" ? 1 : 0));

  function agentMenu(e: React.MouseEvent, ext: string) {
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `Çağrıyı ${ext} dahilisine aktar`, onClick: () => phone.transfer(ext).catch(() => undefined), disabled: !inCall },
        { label: "Çağrıyı dinle (yakında)", onClick: () => undefined, disabled: true },
      ],
    });
  }
  function queueMenu(e: React.MouseEvent, num: string) {
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [{ label: `Çağrıyı ${num} kuyruğuna aktar`, onClick: () => phone.transfer(num).catch(() => undefined), disabled: !inCall }],
    });
  }

  return (
    <div className="space-y-4">
      <Card title="Temsilciler">
        <ul className="max-h-72 space-y-0.5 overflow-y-auto">
          {sorted.map((e) => {
            const s = agentStatus[e.status] ?? { label: e.status, tone: "slate" as const };
            const online = e.status !== "UNREGISTERED";
            return (
              <li
                key={e.extension}
                onContextMenu={(ev) => agentMenu(ev, e.extension)}
                onClick={() => canDial && !inCall && setConfirmExt(e.extension)}
                title="Sol tık: ara · Sağ tık: aktar"
                className="flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 hover:bg-accent"
              >
                <span className="flex items-center gap-2 text-sm">
                  <span className={cn("size-2 rounded-full", s.tone === "green" ? "bg-success" : s.tone === "amber" ? "bg-warning" : s.tone === "red" ? "bg-destructive" : "bg-muted-foreground/50")} />
                  <span className="font-medium">{e.extension}</span>
                  <span className="text-xs text-muted-foreground">({online ? "hatta" : "çıkmış"})</span>
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

      {confirmExt && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={() => setConfirmExt(null)}>
          <div className="w-full max-w-xs rounded-2xl border border-border bg-card p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <p className="text-sm">{confirmExt} dahilisini aramak ister misiniz?</p>
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setConfirmExt(null)}>İptal</Button>
              <Button onClick={() => { phone.call(confirmExt).catch(() => undefined); setConfirmExt(null); }}>Ara</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function CallHistory() {
  const phone = useSoftphoneContext();
  const [calls, setCalls] = useState<Call[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const canDial = phone.status === "registered";
  const inCall = phone.status === "in-call" || phone.status === "held";
  const copyTimer = useRef<number | null>(null);

  useEffect(() => {
    let live = true;
    let warmTries = 0;
    const load = () => {
      api
        .listCalls({ perPage: 20 })
        .then((r) => {
          if (!live) return;
          setCalls(r.items);
          setError(null);
          // The backend serves an empty page (200) while its snapshot warms up;
          // retry a few times before settling on the empty state.
          if (r.items.length === 0 && warmTries < 5) {
            warmTries += 1;
            window.setTimeout(load, 4000);
            return;
          }
          setLoading(false);
        })
        .catch((e) => {
          if (!live) return;
          setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi.");
          setLoading(false);
        });
    };
    load();
    const timer = window.setInterval(() => { warmTries = 5; load(); }, 30000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, []);

  const copy = useCallback((raw: string) => {
    const val = displayNumber(raw);
    navigator.clipboard?.writeText(val).catch(() => undefined);
    setCopied(val);
    if (copyTimer.current) window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopied(null), 1200);
  }, []);

  function rowMenu(e: React.MouseEvent, number: string) {
    e.preventDefault();
    if (!number) return;
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `${displayNumber(number)} ara`, onClick: () => phone.call(normalizeDial(number)).catch(() => undefined), disabled: !canDial },
        { label: "Numarayı kopyala", onClick: () => copy(number) },
        { label: "Görüşmeye aktar", onClick: () => phone.transfer(normalizeDial(number)).catch(() => undefined), disabled: !inCall },
      ],
    });
  }

  return (
    <Card title="Çağrı Geçmişi">
      {loading ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : calls.length === 0 ? (
        <p className="text-sm text-muted-foreground">Henüz çağrı kaydı yok.</p>
      ) : (
        <ul className="max-h-[34rem] space-y-1 overflow-y-auto">
          {calls.map((c) => {
            const counterpart = c.direction === "outbound" ? c.toNumber : c.fromNumber;
            const isOpen = open === c.uuid;
            return (
              <li key={c.uuid} className="rounded-lg ring-1 ring-transparent hover:ring-border/60">
                <button
                  onClick={() => setOpen(isOpen ? null : c.uuid)}
                  onContextMenu={(e) => rowMenu(e, counterpart)}
                  className="flex w-full items-center justify-between gap-2 rounded-lg px-2 py-2 text-left hover:bg-accent"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", isOpen && "rotate-180")} />
                    <Direction value={c.direction} />
                    <span className="truncate text-sm font-medium">{displayNumber(counterpart) || "—"}</span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    <CallDisposition value={c.disposition} />
                    <span className="hidden text-xs text-muted-foreground sm:inline">{formatStamp(c.startedAt)}</span>
                  </span>
                </button>

                {isOpen && (
                  <div className="space-y-2 px-3 pb-3 pt-1 text-sm">
                    <CopyRow label="Arayan" number={c.fromNumber} copied={copied} onCopy={copy} />
                    <CopyRow label="Aranan" number={c.toNumber} copied={copied} onCopy={copy} />
                    <div className="flex items-center gap-4 text-xs text-muted-foreground">
                      <span>Süre: {formatDuration(c.durationSeconds)}</span>
                      <span>{formatStamp(c.startedAt)}</span>
                    </div>
                    <div className="flex gap-2 pt-1">
                      <Button variant="secondary" className="h-8 px-3" onClick={() => phone.call(normalizeDial(counterpart)).catch(() => undefined)} disabled={!canDial || !counterpart}>
                        <Phone className="size-3.5" /> Ara
                      </Button>
                    </div>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
    </Card>
  );
}

function CopyRow({ label, number, copied, onCopy }: { label: string; number: string; copied: string | null; onCopy: (n: string) => void }) {
  const shown = displayNumber(number);
  const isCopied = copied === shown && !!shown;
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <button
        onClick={() => number && onCopy(number)}
        title="Kopyala"
        className="flex items-center gap-1.5 rounded-md px-2 py-1 font-medium tabular-nums transition hover:bg-accent"
      >
        {shown || "—"}
        {number && <Copy className="size-3.5 text-muted-foreground" />}
        {isCopied && <span className="text-xs text-success">kopyalandı</span>}
      </button>
    </div>
  );
}
