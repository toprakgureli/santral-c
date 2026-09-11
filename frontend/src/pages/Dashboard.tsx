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
  PhoneIncoming,
  PhoneOff,
  PhoneOutgoing,
  Play,
  Search,
  TriangleAlert,
} from "lucide-react";
import { api, ApiError } from "../api/client";
import type { AgentPresenceState, Call, EscalationCategory, EscalationRecord, PBXExtension, PBXQueue, PBXStats } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { useSoftphoneContext } from "../softphone/SoftphoneContext";
import { displayNumber, normalizeDial } from "../softphone/dial";
import { tones } from "../softphone/tones";
import { Badge, Button, Card, Select } from "../components/ui";
import { cn } from "../lib/utils";
import { ContextMenu, type MenuItem } from "../components/ContextMenu";
import { SearchableSelect } from "../components/SearchableSelect";
import { callQuality, formatClock, formatDuration, formatStamp } from "./callFormat";

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
  BREAK: { label: "Molada", tone: "amber" },
  BACKOFFICE: { label: "Backoffice", tone: "amber" },
  SS_DND: { label: "Rahatsız etmeyin", tone: "red" },
};

export function Dashboard() {
  const { user } = useAuth();
  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);
  const canTransfer = can(user, "call.transfer");
  const canEscalate = can(user, "escalation.view");
  const canCall = can(user, "call.originate");
  const canSearchEsc = can(user, "escalation.search");
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

    // Live agent statuses over SSE: presence changes appear instantly and the
    // hosted-PBX refreshes are pushed as soon as they arrive. A slow poll backs
    // it up if the stream cannot connect (e.g. a proxy that buffers it).
    let backup = 0;
    const startBackup = () => {
      if (backup) return;
      backup = window.setInterval(loadExts, 30000);
    };
    let es: EventSource | null = null;
    try {
      es = new EventSource("/api/v1/pbx/stream", { withCredentials: true });
      es.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          if (msg?.type === "extensions" && Array.isArray(msg.items) && live) setExts(msg.items);
        } catch {
          // ignore malformed frames
        }
      };
      es.onerror = () => startBackup();
    } catch {
      startBackup();
    }
    return () => {
      live = false;
      es?.close();
      if (backup) window.clearInterval(backup);
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
        {canSeeCalls ? <CallHistory canCall={canCall} /> : <div className="hidden xl:block" />}
        {/* Softphone with the escalation panel directly below it. */}
        <div className="space-y-4">
          <Softphone hasExtension={!!user?.sipExtension} canCall={canCall} />
          {canEscalate && <Escalation categories={categories} activePeer={phone.peer ?? undefined} canSearch={canSearchEsc} />}
        </div>
        {canTransfer ? <AgentsQueues exts={exts} queues={queues} canCall={canCall} /> : <div className="hidden xl:block" />}
      </div>
    </div>
  );
}

const agentStates: Record<AgentPresenceState, { label: string; tone: "green" | "amber" | "red" }> = {
  available: { label: "Müsait", tone: "green" },
  break: { label: "Molada", tone: "amber" },
  backoffice: { label: "Backoffice", tone: "amber" },
  dnd: { label: "Rahatsız Etmeyin", tone: "red" },
};

function StatusBar({ totals, showTotals, extension, hasExtension, stats }: { totals: { available: number; talking: number; offline: number }; showTotals: boolean; extension?: string; hasExtension: boolean; stats: PBXStats | null }) {
  const phone = useSoftphoneContext();
  const [agentState, setAgentState] = useState<AgentPresenceState>("available");
  const [since, setSince] = useState<number>(() => Date.now());
  const [nowTick, setNowTick] = useState<number>(() => Date.now());
  const [presenceTotals, setPresenceTotals] = useState<Record<string, number>>({});
  const [talk, setTalk] = useState(0);
  const callStartRef = useRef<number>(0);
  const busy = phone.status === "in-call" || phone.status === "held" || phone.status === "ringing" || phone.status === "calling" || phone.status === "incoming";
  const onCall = phone.status === "in-call" || phone.status === "held";
  const state = agentStates[agentState] ?? agentStates.available;

  // Presence is stored server-side, so it survives reloads and shows in the
  // agent list; load the current value, when it started, and today's totals.
  useEffect(() => {
    if (!hasExtension) return;
    let live = true;
    const load = () => api.getAgentStatus().then((s) => {
      if (!live) return;
      setAgentState(s.state);
      setSince(s.since ? Date.parse(s.since) : Date.now());
      setPresenceTotals(s.totals ?? {});
      setTalk(s.talk ?? 0);
    }).catch(() => undefined);
    load();
    const timer = window.setInterval(load, 20000);
    return () => { live = false; window.clearInterval(timer); };
  }, [hasExtension]);

  // A live clock so the "how long in this state / on this call" timer ticks.
  useEffect(() => {
    const t = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  useEffect(() => {
    if (onCall && !callStartRef.current) callStartRef.current = Date.now();
    if (!onCall) callStartRef.current = 0;
  }, [onCall]);

  function changeState(v: AgentPresenceState) {
    setAgentState(v);
    setSince(Date.now());
    api.setAgentStatus(v).catch(() => undefined);
  }

  // While in a call the live call status wins; otherwise the presence badge
  // reflects the agent's chosen state (so "Molada" no longer shows green).
  const badgeTone = busy ? statusTone[phone.status] : state.tone;
  const badgeLabel = busy ? statusLabel[phone.status] : hasExtension ? state.label : statusLabel[phone.status];
  const dotColor = badgeTone === "green" ? "bg-success" : badgeTone === "red" ? "bg-destructive" : badgeTone === "blue" ? "bg-primary" : badgeTone === "amber" ? "bg-warning" : "bg-muted-foreground/50";
  const timerSeconds = onCall
    ? Math.max(0, Math.floor((nowTick - (callStartRef.current || nowTick)) / 1000))
    : Math.max(0, Math.floor((nowTick - since) / 1000));

  return (
    <div className="rounded-2xl bg-card px-5 py-3 ring-1 ring-border/60">
      <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2.5">
          <span className={cn("size-2.5 rounded-full", dotColor, !busy && state.tone === "green" && "animate-pulse")} />
          <div>
            <div className="text-xs text-muted-foreground">Dahili</div>
            <div className="text-lg font-semibold leading-tight">{phone.extension ?? extension ?? "—"}</div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge tone={badgeTone}>{badgeLabel}</Badge>
          {hasExtension && <span className="font-mono text-sm tabular-nums text-muted-foreground" title={onCall ? "Görüşme süresi" : "Bu durumdaki süre"}>{formatClock(timerSeconds)}</span>}
        </div>
        {hasExtension && (
          <Select value={agentState} onChange={(e) => changeState(e.target.value as AgentPresenceState)} className="h-9 w-40">
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

      {hasExtension && (
        <div className="mt-2.5 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-border/50 pt-2.5 text-xs">
          <span className="font-semibold text-muted-foreground">Bugün toplam:</span>
          <Dur label="Müsait" seconds={presenceTotals.available ?? 0} dot="bg-success" />
          <Dur label="Mola" seconds={presenceTotals.break ?? 0} dot="bg-warning" />
          <Dur label="Backoffice" seconds={presenceTotals.backoffice ?? 0} dot="bg-warning" />
          {(presenceTotals.dnd ?? 0) > 0 && <Dur label="Rahatsız Etmeyin" seconds={presenceTotals.dnd} dot="bg-destructive" />}
          <Dur label="Görüşme" seconds={talk} dot="bg-primary" />
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

function Dur({ dot, label, seconds }: { dot: string; label: string; seconds: number }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className={`size-2 rounded-full ${dot}`} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono font-semibold tabular-nums">{formatClock(Math.max(0, Math.round(seconds)))}</span>
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

function Softphone({ hasExtension, canCall }: { hasExtension: boolean; canCall: boolean }) {
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
    if (!canCall) return;
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

          {/* Idle: number entry + dialpad (only for agents allowed to place calls) */}
          {idle && !canCall && (
            <div className="py-8 text-center">
              <p className="text-sm text-muted-foreground">Giden çağrı yetkiniz yok.</p>
              <p className="mt-1 text-xs text-muted-foreground/70">Gelen çağrıları cevaplayabilirsiniz.</p>
            </div>
          )}
          {idle && canCall && (
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

function Escalation({ categories, activePeer, canSearch }: { categories: EscalationCategory[]; activePeer?: string; canSearch: boolean }) {
  const [customer, setCustomer] = useState("");
  const [catId, setCatId] = useState<number | null>(null);
  const [reasonId, setReasonId] = useState<number | null>(null);
  const [note, setNote] = useState("");
  const [history, setHistory] = useState<EscalationRecord[]>([]);
  const [status, setStatus] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const onActiveCall = !!activePeer;

  // Follow the live call's number; keep it after the call ends so the agent can
  // still wrap up. There is no manual number entry here anymore.
  useEffect(() => {
    if (activePeer) setCustomer(displayNumber(activePeer));
  }, [activePeer]);

  const loadHistory = useCallback((n: string) => {
    if (!canSearch) { setHistory([]); return; }
    const key = n.trim();
    if (!key) { setHistory([]); return; }
    api.escalationHistory(key).then(setHistory).catch(() => setHistory([]));
  }, [canSearch]);

  useEffect(() => { loadHistory(customer); }, [customer, loadHistory]);

  const reasons = useMemo(() => categories.find((c) => c.id === catId)?.reasons ?? [], [categories, catId]);
  const catOptions = useMemo(() => categories.map((c) => ({ id: c.id, label: c.name })), [categories]);
  const reasonOptions = useMemo(() => reasons.map((r) => ({ id: r.id, label: r.name })), [reasons]);

  async function save() {
    if (!customer.trim() || reasonId === null) { setStatus({ kind: "err", text: "Durum seçin." }); return; }
    setSaving(true);
    setStatus(null);
    try {
      await api.logEscalation({ number: customer.trim(), reasonId, note: note.trim() || undefined });
      setNote("");
      setReasonId(null);
      setCatId(null);
      setStatus({ kind: "ok", text: "Eskalasyon kaydedildi." });
      loadHistory(customer);
    } catch (e) {
      setStatus({ kind: "err", text: e instanceof ApiError ? e.message : "Kaydedilemedi." });
    } finally {
      setSaving(false);
    }
  }

  if (categories.length === 0) {
    return (
      <EscalationFrame>
        <p className="rounded-xl bg-muted/40 px-4 py-6 text-center text-sm text-muted-foreground">
          Henüz eskalasyon durumu tanımlı değil. Yönetici, <span className="font-medium">Eskalasyon</span> menüsünden kategori ve durum ekleyebilir.
        </p>
      </EscalationFrame>
    );
  }

  if (!customer.trim()) {
    return (
      <EscalationFrame>
        <p className="rounded-xl bg-muted/40 px-4 py-8 text-center text-sm text-muted-foreground">
          Bir çağrı başladığında müşteri bilgisi burada belirir ve eskalasyon girebilirsiniz.
        </p>
      </EscalationFrame>
    );
  }

  return (
    <EscalationFrame active={onActiveCall}>
      <div className="space-y-4">
        {/* Prominent customer header — highlighted during a live call */}
        <div className={cn("rounded-2xl px-4 py-3 transition", onActiveCall ? "bg-primary/10 ring-1 ring-primary/30" : "bg-muted/40")}>
          <div className="flex items-center justify-between gap-2">
            <div>
              <div className="text-xs text-muted-foreground">{onActiveCall ? "Görüşülen müşteri" : "Son müşteri"}</div>
              <div className="text-2xl font-bold tabular-nums tracking-wide">{customer}</div>
            </div>
            {canSearch && (
              <Badge tone={history.length ? "amber" : "slate"}>{history.length} geçmiş kayıt</Badge>
            )}
          </div>
        </div>

        {/* Customer history opens prominently as soon as a call is answered */}
        {canSearch && history.length > 0 && (
          <div className="rounded-xl bg-muted/30 p-3">
            <p className="mb-2 text-xs font-semibold text-muted-foreground">Geçmiş görüşmeler</p>
            <ul className="max-h-44 space-y-2 overflow-y-auto">
              {history.map((h) => (
                <li key={h.id} className="rounded-lg bg-card px-3 py-2 text-sm ring-1 ring-border/50">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium">{h.agentName} görüştü</span>
                    <span className="text-xs text-muted-foreground">{h.createdAt}</span>
                  </div>
                  <div className="mt-1"><Badge tone="amber">{h.categoryName}</Badge> <span className="text-muted-foreground">{h.reasonName}</span></div>
                  {h.note && <p className="mt-1 text-foreground/80">{h.note}</p>}
                </li>
              ))}
            </ul>
          </div>
        )}

        {/* Step 1: big category combobox */}
        <div className="space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">1 · Kategori</span>
          <SearchableSelect
            size="lg"
            value={catId}
            onChange={(id) => { setCatId(id); setReasonId(null); }}
            options={catOptions}
            placeholder="Kategori seçin"
            searchPlaceholder="Kategori ara..."
          />
        </div>

        {/* Step 2: durum appears once a category is chosen */}
        {catId !== null && (
          <div className="space-y-1.5 animate-in fade-in slide-in-from-top-1 duration-200">
            <span className="text-xs font-medium text-muted-foreground">2 · Durum</span>
            <SearchableSelect
              size="lg"
              value={reasonId}
              onChange={setReasonId}
              options={reasonOptions}
              placeholder="Durum seçin"
              searchPlaceholder="Durum ara..."
            />
          </div>
        )}

        {/* Step 3: note appears once a durum is chosen */}
        {reasonId !== null && (
          <div className="space-y-1.5 animate-in fade-in slide-in-from-top-1 duration-200">
            <span className="text-xs font-medium text-muted-foreground">3 · Not</span>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Görüşme notu (opsiyonel)"
              rows={3}
              className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm outline-none focus-visible:border-ring/60 focus-visible:bg-card"
            />
          </div>
        )}

        <div className="flex items-center justify-between gap-3">
          {status ? <span className={cn("text-sm", status.kind === "ok" ? "text-success" : "text-destructive")}>{status.text}</span> : <span />}
          <Button className="h-11 px-6" onClick={save} disabled={saving || reasonId === null}>
            {saving ? "Kaydediliyor..." : "Eskalasyonu Kaydet"}
          </Button>
        </div>
      </div>
    </EscalationFrame>
  );
}

// EscalationFrame is a deliberately prominent card: a coloured header with an
// icon and a strong ring so the escalation area stands out during a call.
function EscalationFrame({ active, children }: { active?: boolean; children: React.ReactNode }) {
  return (
    <section className={cn("overflow-hidden rounded-2xl bg-card shadow-md ring-2 transition", active ? "ring-primary/50" : "ring-primary/20")}>
      <header className="flex items-center gap-3 border-b border-primary/15 bg-gradient-to-r from-primary/10 to-transparent px-5 py-3.5">
        <span className="flex size-9 items-center justify-center rounded-xl bg-primary/15 text-primary [&_svg]:size-5">
          <TriangleAlert />
        </span>
        <div>
          <h2 className="text-base font-bold leading-tight tracking-tight">Eskalasyon</h2>
          <p className="text-xs text-muted-foreground">Görüşme sonucunu kaydet</p>
        </div>
        {active && <span className="ml-auto flex items-center gap-1.5 rounded-full bg-primary/15 px-2.5 py-1 text-xs font-medium text-primary"><span className="size-1.5 animate-pulse rounded-full bg-primary" /> Canlı çağrı</span>}
      </header>
      <div className="p-5">{children}</div>
    </section>
  );
}

function AgentsQueues({ exts, queues, canCall }: { exts: PBXExtension[]; queues: PBXQueue[]; canCall: boolean }) {
  const phone = useSoftphoneContext();
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [confirmExt, setConfirmExt] = useState<string | null>(null);
  const inCall = phone.status === "in-call" || phone.status === "held";
  const canDial = phone.status === "registered" && canCall;

  const sorted = [...exts].sort((a, b) => (a.status === "UNREGISTERED" ? 1 : 0) - (b.status === "UNREGISTERED" ? 1 : 0));

  function agentMenu(e: React.MouseEvent, ext: string) {
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `${ext} dahilisini ara`, onClick: () => phone.call(ext).catch(() => undefined), disabled: !canDial || inCall },
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

function CallHistory({ canCall }: { canCall: boolean }) {
  const phone = useSoftphoneContext();
  const [calls, setCalls] = useState<Call[]>([]);
  const [counts, setCounts] = useState({ short: 0, long: 0, unanswered: 0 });
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const canDial = phone.status === "registered" && canCall;
  const inCall = phone.status === "in-call" || phone.status === "held";
  const copyTimer = useRef<number | null>(null);

  useEffect(() => {
    let live = true;
    const load = () => {
      api
        .recentCalls()
        .then((r) => {
          if (!live) return;
          setCalls(r.items);
          setCounts({ short: r.short, long: r.long, unanswered: r.unanswered });
          setError(null);
          setLoading(false);
        })
        .catch((e) => {
          if (!live) return;
          setError(e instanceof ApiError ? e.message : "Çağrılar yüklenemedi.");
          setLoading(false);
        });
    };
    load();
    // Our own store is authoritative and cheap; refresh often so a just-ended
    // call appears right away. It also resets at local midnight (server-side).
    const timer = window.setInterval(load, 10000);
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

  const real = counts.short + counts.long;
  const total = real + counts.unanswered;
  const term = query.trim();
  const filtered = term ? calls.filter((c) => displayNumber(c.direction === "outbound" ? c.toNumber : c.fromNumber).includes(displayNumber(term))) : calls;

  return (
    <Card title="Çağrı Geçmişi">
      {loading ? (
        <p className="text-sm text-muted-foreground">Yükleniyor...</p>
      ) : error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : (
        <>
          {/* Today's breakdown (resets at 00:00) */}
          <div className="mb-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
            <CountBox label="Gerçek" value={real} tone="green" hint="Kısa + uzun" />
            <CountBox label="Kısa" value={counts.short} tone="amber" />
            <CountBox label="Uzun" value={counts.long} tone="green" />
            <CountBox label="Cevapsız" value={counts.unanswered} tone="slate" />
            <CountBox label="Toplam" value={total} tone="blue" hint="Cevapsız dahil" />
          </div>

          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Numaraya göre ara"
              inputMode="tel"
              className="h-9 w-full rounded-xl border border-border/70 bg-muted/40 pl-9 pr-3 text-sm outline-none focus-visible:border-ring/60 focus-visible:bg-card"
            />
          </div>

          {filtered.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{calls.length === 0 ? "Bugün çağrı kaydı yok." : "Eşleşen çağrı yok."}</p>
          ) : (
          <ul className="max-h-[28rem] space-y-1 overflow-y-auto">
            {filtered.map((c) => {
              const counterpart = c.direction === "outbound" ? c.toNumber : c.fromNumber;
              const isOpen = open === c.uuid;
              const q = callQuality(c.disposition, c.durationSeconds);
              const Arrow = c.direction === "inbound" ? PhoneIncoming : PhoneOutgoing;
              return (
                <li key={c.uuid} className="overflow-hidden rounded-xl ring-1 ring-transparent transition hover:ring-border/60">
                  <button
                    onClick={() => setOpen(isOpen ? null : c.uuid)}
                    onContextMenu={(e) => rowMenu(e, counterpart)}
                    className={cn("flex w-full items-center gap-3 border-l-2 py-2.5 pl-2.5 pr-2 text-left transition hover:bg-accent", q.border)}
                  >
                    <Arrow className={cn("size-4 shrink-0", q.text)} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold tabular-nums">{displayNumber(counterpart) || "—"}</span>
                      <span className={cn("text-xs", q.text)}>{q.label}{c.durationSeconds > 0 && <span className="text-muted-foreground"> · {formatDuration(c.durationSeconds)}</span>}</span>
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">{formatStamp(c.startedAt)}</span>
                    <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", isOpen && "rotate-180")} />
                  </button>

                  {isOpen && (
                    <div className="space-y-2 bg-muted/20 px-3 pb-3 pt-2 text-sm">
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
        </>
      )}
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
    </Card>
  );
}

function CountBox({ label, value, tone, hint }: { label: string; value: number; tone: "green" | "amber" | "slate" | "blue"; hint?: string }) {
  const toneClass: Record<string, string> = {
    green: "text-success",
    amber: "text-warning",
    slate: "text-muted-foreground",
    blue: "text-primary",
  };
  return (
    <div className="rounded-xl bg-muted/40 px-3 py-2 text-center" title={hint}>
      <div className={cn("text-xl font-bold tabular-nums leading-none", toneClass[tone])}>{value}</div>
      <div className="mt-1 text-[0.7rem] text-muted-foreground">{label}</div>
    </div>
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
