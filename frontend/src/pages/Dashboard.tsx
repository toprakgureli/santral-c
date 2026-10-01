import { useEffect, useState } from "react";
import { openLiveStream } from "../lib/liveStream";
import { api } from "../api/client";
import type { EscalationCategory, PBXExtension, PBXQueue, PBXStats } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can, canAny } from "../lib/permissions";
import { useSoftphoneContext } from "../softphone/SoftphoneContext";
import { useShift } from "../shift/ShiftContext";
import { cn } from "../lib/utils";
import { StatusBar } from "../components/dashboard/StatusBar";
import { Softphone } from "../components/dashboard/Softphone";
import { Escalation } from "../components/dashboard/Escalation";
import { AgentsQueues } from "../components/dashboard/AgentsQueues";
import { CallHistory } from "../components/dashboard/CallHistory";

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
    const close = openLiveStream("/api/v1/pbx/stream", {
      onMessage: (data) => {
        try {
          const msg = JSON.parse(data);
          if (msg?.type === "extensions" && Array.isArray(msg.items) && live) { setExts(msg.items); setExtsLoaded(true); }
        } catch {
          // ignore malformed frames
        }
      },
    });
    return () => {
      live = false;
      close();
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
          {/* A ringing incoming call does not replace the customer being written up; the switch happens when it is answered. */}
          {canEscalate && <Escalation categories={categories} activePeer={phone.status === "incoming" ? undefined : phone.peer ?? undefined} connected={connected} callId={phone.callId} canSearch={canSearchEsc} />}
        </div>
        {canTransfer ? <AgentsQueues exts={exts} queues={queues} canCall={canCall} loading={!extsLoaded} /> : <div className="hidden xl:block" />}
      </div>
    </div>
  );
}
