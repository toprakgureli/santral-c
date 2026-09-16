import { useCallback, useEffect, useRef, useState } from "react";
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
import type { AgentPresenceState, Call, EscalationCategory, PBXExtension, PBXQueue, PBXStats } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { useSoftphoneContext } from "../softphone/SoftphoneContext";
import { useShift } from "../shift/ShiftContext";
import { usePresence } from "../presence/PresenceContext";
import { EscalationForm } from "../components/escalation/EscalationForm";
import { markWrapUpDone } from "../components/layout/WrapUpCard";
import { displayNumber, normalizeDial } from "../softphone/dial";
import { tones } from "../softphone/tones";
import { Badge, Button, Card, Select } from "../components/ui";
import { cn } from "../lib/utils";
import { ContextMenu, type MenuItem } from "../components/ContextMenu";
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
  OFF_SHIFT: { label: "Mesai dışı", tone: "slate" },
};

export function Dashboard() {
  const { user } = useAuth();
  const canSeeCalls = canAny(user, ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"]);
  const canTransfer = can(user, "call.transfer");
  const canEscalate = can(user, "escalation.view");
  const canSearchEsc = can(user, "escalation.search");
  const phone = useSoftphoneContext();
  // A connected conversation widens the middle column so the escalation
  // area grows while the agent is talking to the customer.
  const connected = phone.status === "in-call" || phone.status === "held";
  const shift = useShift();
  // Outbound calls need both the permission and an open shift.
  const canCall = can(user, "call.originate") && shift.active;

  const [exts, setExts] = useState<PBXExtension[]>([]);
  const [extsLoaded, setExtsLoaded] = useState(false);
  const [queues, setQueues] = useState<PBXQueue[]>([]);
  const [stats, setStats] = useState<PBXStats | null>(null);
  const [categories, setCategories] = useState<EscalationCategory[]>([]);

  useEffect(() => {
    if (!canTransfer) return;
    let live = true;
    const loadExts = () =>
      api.pbxExtensions().then((d) => live && setExts(d)).catch(() => undefined).finally(() => live && setExtsLoaded(true));
    loadExts();
    api.pbxQueues().then((d) => live && setQueues(d)).catch(() => undefined);

    // Live agent statuses over SSE make presence changes appear instantly. But a
    // proxy (e.g. Cloudflare) can hold an SSE response open and buffer it, so no
    // frames arrive and no error fires either. So always run a slow poll as the
    // source of truth; SSE only accelerates it when frames do come through.
    const poll = window.setInterval(loadExts, 20000);
    let es: EventSource | null = null;
    try {
      es = new EventSource("/api/v1/pbx/stream", { withCredentials: true });
      es.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          if (msg?.type === "extensions" && Array.isArray(msg.items) && live) { setExts(msg.items); setExtsLoaded(true); }
        } catch {
          // ignore malformed frames
        }
      };
    } catch {
      // the poll above already covers this
    }
    return () => {
      live = false;
      es?.close();
      window.clearInterval(poll);
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
      <div className={cn("grid gap-4 transition-[grid-template-columns] duration-300", connected ? "xl:grid-cols-[0.8fr_1.9fr_0.8fr]" : "xl:grid-cols-[1fr_1.3fr_1fr]")}>
        {canSeeCalls ? <CallHistory canCall={canCall} /> : <div className="hidden xl:block" />}
        {/* Softphone with the escalation panel directly below it. */}
        <div className="space-y-4">
          <Softphone hasExtension={!!user?.sipExtension} canCall={canCall} />
          {canEscalate && <Escalation categories={categories} activePeer={phone.peer ?? undefined} connected={connected} callId={phone.callId} canSearch={canSearchEsc} />}
        </div>
        {canTransfer ? <AgentsQueues exts={exts} queues={queues} canCall={canCall} loading={!extsLoaded} /> : <div className="hidden xl:block" />}
      </div>
    </div>
  );
}

const agentStates: Record<AgentPresenceState, { label: string; tone: "green" | "amber" | "red" | "slate" }> = {
  available: { label: "Müsait", tone: "green" },
  break: { label: "Molada", tone: "amber" },
  backoffice: { label: "Backoffice", tone: "amber" },
  dnd: { label: "Rahatsız Etmeyin", tone: "red" },
  off: { label: "Mesai Dışı", tone: "slate" },
};

function StatusBar({ totals, showTotals, extension, hasExtension, stats }: { totals: { available: number; talking: number; offline: number }; showTotals: boolean; extension?: string; hasExtension: boolean; stats: PBXStats | null }) {
  const phone = useSoftphoneContext();
  const shift = useShift();
  const presence = usePresence();
  const [agentState, setAgentState] = useState<AgentPresenceState>("available");
  const [since, setSince] = useState<number>(() => Date.now());
  const [nowTick, setNowTick] = useState<number>(() => Date.now());
  const [presenceTotals, setPresenceTotals] = useState<Record<string, number>>({});
  const [talk, setTalk] = useState(0);
  const [online, setOnline] = useState(0);
  const [fetchedAt, setFetchedAt] = useState<number>(() => Date.now());
  const busy = phone.status === "in-call" || phone.status === "held" || phone.status === "ringing" || phone.status === "calling" || phone.status === "incoming";
  const onCall = phone.status === "in-call" || phone.status === "held";
  const state = agentStates[agentState] ?? agentStates.available;

  // Presence is stored server-side and polled by PresenceProvider (shared with
  // the break overlay); mirror each read into the local timers. fetchedAt lets
  // us tick the current state's total live.
  const refresh = presence.refresh;
  useEffect(() => {
    const s = presence.data;
    if (!s) return;
    setAgentState(s.state);
    setSince(s.since ? Date.parse(s.since) : presence.fetchedAt);
    setPresenceTotals(s.totals ?? {});
    setTalk(s.talk ?? 0);
    setOnline(s.online ?? 0);
    setFetchedAt(presence.fetchedAt);
  }, [presence.data, presence.fetchedAt]);

  // A live clock so the "how long in this state / on this call" timer ticks.
  useEffect(() => {
    const t = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  // When a call ends, refresh shortly after so the idle-streak timer restarts
  // from the just-recorded hangup instead of waiting for the next 20s poll.
  useEffect(() => {
    if (busy || !hasExtension) return;
    const t = window.setTimeout(refresh, 1500);
    return () => window.clearTimeout(t);
  }, [busy, hasExtension, refresh]);

  function changeState(v: AgentPresenceState) {
    setAgentState(v);
    setSince(Date.now());
    setFetchedAt(Date.now());
    presence.change(v).catch(() => undefined);
  }

  // Placing a call while paused ends the pause on its own: the agent is
  // clearly working again, and DND must drop so the PBX routes to them.
  const paused = shift.active && hasExtension && agentState !== "available" && agentState !== "off";
  useEffect(() => {
    if (paused && phone.status === "calling") changeState("available");
  }, [paused, phone.status]); // eslint-disable-line react-hooks/exhaustive-deps

  // While in a call the live call status wins; otherwise the presence badge
  // reflects the agent's chosen state (so "Molada" no longer shows green).
  const badgeTone = busy ? statusTone[phone.status] : state.tone;
  const badgeLabel = busy ? statusLabel[phone.status] : hasExtension ? state.label : statusLabel[phone.status];
  const dotColor = badgeTone === "green" ? "bg-success" : badgeTone === "red" ? "bg-destructive" : badgeTone === "blue" ? "bg-primary" : badgeTone === "amber" ? "bg-warning" : "bg-muted-foreground/50";
  // Call start/answer live in the global softphone, so these timers survive
  // navigating between menus instead of restarting.
  const callSince = phone.callStartedAt ?? nowTick;
  const timerSeconds = busy
    ? Math.max(0, Math.floor((nowTick - callSince) / 1000))
    : shift.active
      ? Math.max(0, Math.floor((nowTick - since) / 1000))
      : 0;

  // Tick the current state's total (and talk time during a call) live between
  // 20s refreshes, so the "Bugün toplam" strip keeps moving.
  // Off shift every clock is frozen at zero; the next "Mesai Başlat" restarts them.
  const liveDelta = shift.active ? Math.max(0, (nowTick - fetchedAt) / 1000) : 0;
  // While on a call, the presence state pauses and call time grows instead, so
  // a call is not counted as idle/available time.
  const totalFor = (key: AgentPresenceState) => Math.round((presenceTotals[key] ?? 0) + (agentState === key && !busy ? liveDelta : 0));
  const talkLive = Math.round((talk ?? 0) + (busy ? liveDelta : 0));
  // Online time grows every second the panel is up; the buckets above partition
  // it (talk + idle + break + backoffice + dnd == online).
  const onlineLive = Math.round(online + liveDelta);

  const pauseLabel: Record<string, string> = { break: "Moladasın", backoffice: "Backoffice'tesin", dnd: "Rahatsız etmeyin açık" };

  return (
    <>
    {paused && !busy && (
      /* A pause is easy to forget; make it impossible to miss and one click to end. */
      <div
        className={cn(
          "flex flex-wrap items-center justify-between gap-3 rounded-2xl px-5 py-3 ring-1",
          agentState === "dnd" ? "bg-destructive/10 ring-destructive/30" : "bg-warning/10 ring-warning/30",
        )}
      >
        <div className="flex items-center gap-3">
          <span className={cn("size-3 animate-pulse rounded-full", agentState === "dnd" ? "bg-destructive" : "bg-warning")} />
          <span className="text-lg font-semibold">{pauseLabel[agentState] ?? state.label}</span>
          <span className="font-mono text-sm tabular-nums text-muted-foreground">{formatClock(timerSeconds)}</span>
          <span className="hidden text-sm text-muted-foreground sm:inline">Bu sırada sana çağrı düşmez.</span>
        </div>
        <Button onClick={() => changeState("available")} className={agentState === "dnd" ? "bg-destructive hover:bg-destructive/90" : "bg-warning text-black hover:bg-warning/90"}>
          {agentState === "break" ? "Molayı bitir" : "Müsait ol"}
        </Button>
      </div>
    )}
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
          <Select
            value={agentState}
            onChange={(e) => changeState(e.target.value as AgentPresenceState)}
            disabled={!shift.active}
            title={shift.active ? undefined : "Durum değiştirmek için mesai başlatın"}
            className="h-9 w-40"
          >
            {Object.entries(agentStates)
              .filter(([v]) => v !== "off" || agentState === "off")
              .map(([v, s]) => (
                <option key={v} value={v} disabled={v === "off"}>
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
          <span className="font-semibold text-muted-foreground">Bu mesai · Çevrimiçi</span>
          <span className="font-mono font-semibold tabular-nums">{formatClock(onlineLive)}</span>
          <span className="text-muted-foreground/40">·</span>
          <Dur label="Görüşme" seconds={talkLive} dot="bg-primary" />
          <Dur label="Müsait" seconds={totalFor("available")} dot="bg-success" />
          <Dur label="Mola" seconds={totalFor("break")} dot="bg-warning" />
          <Dur label="Backoffice" seconds={totalFor("backoffice")} dot="bg-warning" />
          {totalFor("dnd") > 0 && <Dur label="Rahatsız Etmeyin" seconds={totalFor("dnd")} dot="bg-destructive" />}
        </div>
      )}
    </div>
    </>
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
  const shift = useShift();
  const [target, setTarget] = useState("");
  const [showKeypad, setShowKeypad] = useState(false);
  const [nowTick, setNowTick] = useState<number>(() => Date.now());

  const idle = phone.status === "registered" || phone.status === "error" || phone.status === "connecting";
  const outgoing = phone.status === "calling" || phone.status === "ringing";
  const active = phone.status === "in-call" || phone.status === "held";
  // Answer time lives in the global softphone, so the duration survives menu
  // switches instead of restarting from zero.
  const dur = phone.answeredAt ? Math.max(0, Math.floor((nowTick - phone.answeredAt) / 1000)) : 0;

  useEffect(() => {
    const t = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);

  function callNow() {
    if (!canCall) return;
    const n = normalizeDial(target);
    if (n) phone.call(n).catch(() => undefined);
  }

  // Clicking the number on the call face copies it bare (5304230113).
  const [peerCopied, setPeerCopied] = useState(false);
  function copyPeer() {
    const val = displayNumber(phone.peer || "");
    if (!val) return;
    navigator.clipboard?.writeText(val).catch(() => undefined);
    setPeerCopied(true);
    window.setTimeout(() => setPeerCopied(false), 1200);
  }

  return (
    <Card title="Softphone">
      {!hasExtension ? (
        <p className="text-sm text-muted-foreground">Hesabınıza bir dahili numara atanmamış. Yöneticinizle görüşün.</p>
      ) : phone.secondary ? (
        /* Another tab (maybe a forgotten one) holds the softphone; offer to pull it here. */
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Phone className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium">Softphone başka bir sekmede açık</p>
            <p className="mx-auto max-w-xs text-xs leading-relaxed text-muted-foreground">
              Çağrılar o sekmede yönetiliyor. Açık kalmış eski bir sekme olabilir, buradan devam edersen softphone bu sekmeye geçer, diğer sekme devre dışı kalır.
            </p>
          </div>
          <Button onClick={phone.takeOver} className="mt-1">Bu tarayıcıdan devam et</Button>
        </div>
      ) : !shift.active && (idle || phone.status === "disabled") ? (
        /* Off shift the softphone is not registered at all: nothing rings, nothing dials. */
        <div className="flex flex-col items-center gap-3 py-10 text-center">
          <span className="flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground">
            <Phone className="size-5" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium">Mesai başlatılmadı</p>
            <p className="mx-auto max-w-xs text-xs leading-relaxed text-muted-foreground">
              Mesai başlamadan çağrı gelmez ve arama yapılamaz. Mesai 18:30&apos;da biter, bitirilmezse 19:20&apos;de sistem kapatır.
            </p>
          </div>
          {shift.error && <p className="text-xs text-destructive">{shift.error}</p>}
          <Button onClick={() => void shift.start()} disabled={shift.busy || shift.loading} className="mt-1">
            Mesai Başlat
          </Button>
        </div>
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
                <button
                  type="button"
                  onClick={copyPeer}
                  title="Numarayı kopyala"
                  className="inline-flex items-center gap-2 rounded-lg px-2 py-0.5 text-2xl font-semibold tracking-wide transition-colors hover:bg-accent"
                >
                  {displayNumber(phone.peer || "") || phone.peer || "—"}
                  {peerCopied ? <span className="text-xs font-medium text-success">Kopyalandı</span> : <Copy className="size-4 text-muted-foreground" />}
                </button>
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
                    <div className="grid w-full max-w-[15rem] grid-cols-3 gap-2">
                      {keypadKeys.map((k) => (
                        <button key={k} onClick={() => { tones.dtmf(k); phone.sendDtmf(k); }} className="h-12 rounded-xl bg-muted text-lg font-semibold text-foreground transition active:scale-95 hover:bg-accent">
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

function Escalation({ categories, activePeer, connected, callId, canSearch }: { categories: EscalationCategory[]; activePeer?: string; connected: boolean; callId: string | null; canSearch: boolean }) {
  const [customer, setCustomer] = useState("");
  const [historyCount, setHistoryCount] = useState(0);
  const onActiveCall = !!activePeer;

  // Follow the live call's number; keep it after the call ends so the agent can
  // still wrap up. There is no manual number entry here anymore.
  useEffect(() => {
    if (activePeer) setCustomer(displayNumber(activePeer));
  }, [activePeer]);

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
    <EscalationFrame active={onActiveCall} connected={connected}>
      <div className="space-y-4">
        {/* Prominent customer header, highlighted during a live call */}
        <div className={cn("rounded-2xl px-4 py-3 transition", onActiveCall ? "bg-primary/10 ring-1 ring-primary/30" : "bg-muted/40")}>
          <div className="flex items-center justify-between gap-2">
            <div>
              <div className="text-xs text-muted-foreground">{onActiveCall ? "Görüşülen müşteri" : "Son müşteri"}</div>
              <div className={cn("font-bold tabular-nums tracking-wide", connected ? "text-3xl" : "text-2xl")}>{customer}</div>
            </div>
            {canSearch && (
              <Badge tone={historyCount ? "amber" : "slate"}>{historyCount} geçmiş kayıt</Badge>
            )}
          </div>
          {connected && (
            <p className="mt-2 text-xs text-primary">Görüşme bitince eskalasyon kartı açılır. Şimdiden girersen çağrı sonunda tekrar sorulmaz.</p>
          )}
        </div>

        <EscalationForm
          categories={categories}
          number={customer}
          canSearch={canSearch}
          callUuid={connected && callId ? callId : undefined}
          onSaved={() => {
            if (connected && callId) markWrapUpDone(callId);
          }}
          onHistory={(items) => setHistoryCount(items.length)}
        />
      </div>
    </EscalationFrame>
  );
}

// EscalationFrame is a deliberately prominent card: a coloured header with an
// icon and a strong ring so the escalation area stands out during a call.
function EscalationFrame({ active, connected, children }: { active?: boolean; connected?: boolean; children: React.ReactNode }) {
  return (
    <section className={cn("rounded-2xl bg-card shadow-md ring-2 transition", connected ? "ring-primary shadow-lg shadow-primary/15" : active ? "ring-primary/50" : "ring-primary/20")}>
      <header className="flex items-center gap-3 rounded-t-2xl border-b border-primary/15 bg-gradient-to-r from-primary/10 to-transparent px-5 py-3.5">
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

// joinNames reads "Toprak", "Toprak ve Ahmet", "Toprak, Ahmet ve Mehmet".
function joinNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return `${names.slice(0, -1).join(", ")} ve ${names[names.length - 1]}`;
}

function AgentsQueues({ exts, queues, canCall, loading }: { exts: PBXExtension[]; queues: PBXQueue[]; canCall: boolean; loading?: boolean }) {
  const phone = useSoftphoneContext();
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [confirmExt, setConfirmExt] = useState<string | null>(null);
  const inCall = phone.status === "in-call" || phone.status === "held";
  const canDial = phone.status === "registered" && canCall;

  const sorted = [...exts].sort((a, b) => (a.status === "UNREGISTERED" ? 1 : 0) - (b.status === "UNREGISTERED" ? 1 : 0));

  function agentMenu(e: React.MouseEvent, ext: string, status: string) {
    e.preventDefault();
    // Listening dials the PBX spy code (*5 + extension), so it needs the target
    // to be on a call and our own line to be free.
    const talking = status === "TALKING";
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: [
        { label: `${ext} dahilisini ara`, onClick: () => phone.call(ext).catch(() => undefined), disabled: !canDial || inCall },
        { label: `Çağrıyı ${ext} dahilisine aktar`, onClick: () => phone.transfer(ext).catch(() => undefined), disabled: !inCall },
        {
          label: talking ? `${ext} dahilisinin çağrısını dinle` : `Çağrıyı dinle (${ext} görüşmede değil)`,
          onClick: () => phone.call(`*5${ext}`).catch(() => undefined),
          disabled: !talking || !canDial || inCall,
        },
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
          {loading && exts.length === 0 &&
            Array.from({ length: 5 }).map((_, i) => (
              <li key={`sk-${i}`} className="flex items-center justify-between rounded-lg px-2 py-1.5">
                <span className="flex items-center gap-2">
                  <span className="size-2 rounded-full bg-muted animate-pulse" />
                  <span className="h-3.5 w-12 rounded bg-muted/70 animate-pulse" />
                </span>
                <span className="h-4 w-16 rounded bg-muted/70 animate-pulse" />
              </li>
            ))}
          {sorted.map((e) => {
            const s = agentStatus[e.status] ?? { label: e.status, tone: "slate" as const };
            return (
              <li
                key={e.extension}
                onContextMenu={(ev) => agentMenu(ev, e.extension, e.status)}
                onClick={() => canDial && !inCall && setConfirmExt(e.extension)}
                title="Sol tık: ara · Sağ tık: aktar veya dinle"
                className="flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 hover:bg-accent"
              >
                <span className="flex min-w-0 items-center gap-2 text-sm">
                  <span className={cn("size-2 shrink-0 rounded-full", s.tone === "green" ? "bg-success" : s.tone === "amber" ? "bg-warning" : s.tone === "red" ? "bg-destructive" : "bg-muted-foreground/50")} />
                  <span className="min-w-0">
                    <span className="flex items-center gap-2">
                      <span className="font-medium">{e.extension}</span>
                      {e.names && e.names.length > 0 && (
                        <span className="truncate text-muted-foreground" title={joinNames(e.names)}>{joinNames(e.names)}</span>
                      )}
                    </span>
                    {e.status === "TALKING" && e.peer && (
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground" title="Görüştüğü numara">
                        <PhoneOutgoing className="size-3 shrink-0 text-primary" />
                        <span className="font-mono tabular-nums">{displayNumber(e.peer) || e.peer}</span>
                        {e.peerName && <span className="truncate">{e.peerName}</span>}
                      </span>
                    )}
                  </span>
                </span>
                <Badge tone={s.tone}>{s.label}</Badge>
              </li>
            );
          })}
          {!loading && exts.length === 0 && <li className="py-4 text-center text-sm text-muted-foreground">Liste alınamadı.</li>}
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
  const [counts, setCounts] = useState({ short: 0, long: 0, unanswered: 0, inbound: 0, outbound: 0, inboundMissed: 0, outboundMissed: 0, inboundReal: 0, outboundReal: 0 });
  const [showMissed, setShowMissed] = useState(false);
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
          setCounts({ short: r.short, long: r.long, unanswered: r.unanswered, inbound: r.inbound ?? 0, outbound: r.outbound ?? 0, inboundMissed: r.inboundMissed ?? 0, outboundMissed: r.outboundMissed ?? 0, inboundReal: r.inboundReal ?? 0, outboundReal: r.outboundReal ?? 0 });
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
          {/* Reached: real (30s+) conversations, split by direction. */}
          <div className="mb-2 rounded-xl border border-success/30 bg-success/5 p-2">
            <div className="mb-1.5 px-1 text-xs font-semibold text-success">Ulaşılanlar</div>
            <div className="grid grid-cols-3 gap-2">
              <CountBox label="Gerçek çağrı" sub="30 saniye ve üstü" value={counts.long} tone="green" />
              <CountBox label="Gelen" sub="gerçek çağrı" value={counts.inboundReal} tone="green" />
              <CountBox label="Giden" sub="gerçek çağrı" value={counts.outboundReal} tone="green" />
            </div>
          </div>
          {/* Not reached: unanswered plus too-short calls, folded away by default. */}
          <div className="mb-3 rounded-xl border border-border/60 bg-muted/20 p-2">
            <button
              type="button"
              onClick={() => setShowMissed((v) => !v)}
              className="flex w-full items-center justify-between px-1 text-xs font-semibold text-muted-foreground hover:text-foreground"
            >
              <span>
                Ulaşılamayanlar · {counts.unanswered + counts.short}
                <span className="ml-1 font-normal text-muted-foreground/80">({counts.unanswered} cevapsız, {counts.short} geçersiz)</span>
              </span>
              <ChevronDown className={cn("size-4 transition-transform", showMissed && "rotate-180")} />
            </button>
            {showMissed && (
              <div className="mt-1.5 space-y-2">
                <div className="grid grid-cols-3 gap-2">
                  <CountBox label="Cevapsız" sub="hiç bağlanmadı" value={counts.unanswered} tone="slate" />
                  <CountBox label="Gelen" sub="arayan, cevaplanmadı" value={counts.inboundMissed} tone="slate" />
                  <CountBox label="Giden" sub="aradın, açılmadı" value={counts.outboundMissed} tone="slate" />
                </div>
                <div className="grid grid-cols-3 gap-2">
                  <CountBox label="Geçersiz çağrı" sub="bağlandı, 30 saniye dolmadı" value={counts.short} tone="amber" />
                </div>
              </div>
            )}
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

// minus is the unanswered share of the count, shown small and red beside it.
function CountBox({ label, sub, value, minus, tone }: { label: string; sub?: string; value: number; minus?: number; tone: "green" | "amber" | "slate" | "blue" }) {
  const toneClass: Record<string, string> = {
    green: "text-success",
    amber: "text-warning",
    slate: "text-muted-foreground",
    blue: "text-primary",
  };
  return (
    <div className="rounded-xl bg-muted/40 px-3 py-2 text-center">
      <div className={cn("text-xl font-bold tabular-nums leading-none", toneClass[tone])}>
        {value}
        {minus ? <span className="ml-1 align-middle text-xs font-semibold text-destructive" title="Bağlanmayan">-{minus}</span> : null}
      </div>
      <div className="mt-1 text-[0.7rem] font-medium text-foreground/80">{label}</div>
      {sub && <div className="text-[0.65rem] text-muted-foreground">{sub}</div>}
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
