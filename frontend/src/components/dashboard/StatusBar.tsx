import { useEffect, useState } from "react";
import { Headset } from "lucide-react";
import type { AgentPresenceState, PBXStats } from "../../api/types";
import { STATUS_COLOR, chipClass, dotClass, type StatusTone } from "../../lib/status";
import { useSoftphoneContext } from "../../softphone/SoftphoneContext";
import { useShift } from "../../shift/ShiftContext";
import { usePresence } from "../../presence/PresenceContext";
import { Badge, Button, Select } from "../ui";
import { cn } from "../../lib/utils";
import { formatClock } from "../../pages/callFormat";

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
  "in-call": "red",
  held: "amber",
  calling: "amber",
  ringing: "amber",
  incoming: "amber",
  connecting: "amber",
  error: "red",
  disabled: "slate",
};

const agentStates: Record<AgentPresenceState, { label: string; tone: StatusTone }> = {
  available: { label: "Müsait", tone: STATUS_COLOR.available.tone },
  break: { label: "Molada", tone: STATUS_COLOR.break.tone },
  backoffice: { label: "Backoffice", tone: STATUS_COLOR.backoffice.tone },
  dnd: { label: "Rahatsız Etmeyin", tone: STATUS_COLOR.dnd.tone },
  off: { label: "Mesai Dışı", tone: STATUS_COLOR.off.tone },
};

export function StatusBar({ totals, showTotals, extension, hasExtension, stats }: { totals: { available: number; talking: number; offline: number }; showTotals: boolean; extension?: string; hasExtension: boolean; stats: PBXStats | null }) {
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
  const dotColor = dotClass(badgeTone);
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
    <div className="rounded-2xl bg-card px-4 py-3 shadow-sm ring-1 ring-border/60">
      <div className="flex flex-wrap items-center justify-between gap-4">
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-3">
          <span className={cn("relative flex size-10 items-center justify-center rounded-2xl", chipClass(badgeTone))}>
            <Headset className="size-5" />
            <span className={cn("absolute -right-0.5 -bottom-0.5 size-3 rounded-full ring-2 ring-card", dotColor, !busy && state.tone === "green" && "animate-pulse")} />
          </span>
          <div>
            <div className="text-xs text-muted-foreground">Dahili</div>
            <div className="text-lg font-semibold leading-tight">{phone.extension ?? extension ?? "—"}</div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge tone={badgeTone}>{badgeLabel}</Badge>
          {hasExtension && <span className="font-mono text-sm tabular-nums text-muted-foreground" data-tip={onCall ? "Görüşme süresi" : "Bu durumdaki süre"}>{formatClock(timerSeconds)}</span>}
        </div>
        {hasExtension && (
          <Select
            value={agentState}
            onChange={(e) => changeState(e.target.value as AgentPresenceState)}
            disabled={!shift.active}
            data-tip={shift.active ? undefined : "Durum değiştirmek için mesaini başlat"}
            className="h-9 w-40 rounded-xl"
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
            <Stat dot="bg-destructive" label="Görüşmede" value={totals.talking} />
            <Stat dot="bg-muted-foreground/60" label="Çevrimdışı" value={totals.offline} />
          </>
        )}
      </div>
      </div>

      {hasExtension && (
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl bg-muted/40 px-3 py-2 text-xs">
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
