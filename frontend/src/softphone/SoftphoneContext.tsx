import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { useShift } from "@/shift/ShiftContext";
import { useSoftphone, type Phone } from "./useSoftphone";
import { settleCallLogs } from "./callLogQueue";
import { LIVE_STATES, useTabPhone } from "./tabPhone";

export type { EndedCall } from "./useSoftphone";
import { usePanelBridge } from "./extensionBridge";

export type SoftphoneValue = Phone & {
  secondary: boolean;
  // takeOver moves the line to this tab. A call live in another tab is
  // hung up there first (and its end sent), so ask before calling it.
  takeOver: () => Promise<void>;
  // liveHere / liveElsewhere: a call is live in this tab / in another one.
  liveHere: boolean;
  liveElsewhere: boolean;
  // endCalls hangs up a live call in any tab and waits until its end has
  // reached the server, so signing out never loses it.
  endCalls: () => Promise<void>;
};

const Ctx = createContext<SoftphoneValue | null>(null);

const LOCK = "santral-softphone";

// useLeader elects a single tab to own the SIP registration via the Web Locks
// API, so two open tabs of the panel do not fight over the same extension.
//
// A tab left open elsewhere (another window, a session the browser restored
// in the background, yesterday's tab) keeps the lock, so a freshly opened tab
// would stay secondary for no visible reason. takeOver() steals the lock: the
// browser aborts the old holder's request, that tab drops to secondary and
// unregisters, and this tab becomes the softphone.
function useLeader(): { leader: boolean; takeOver: () => void } {
  const [leader, setLeader] = useState(false);
  const release = useRef<() => void>(() => {});
  const active = useRef(true);

  const hold = useCallback((steal: boolean) => {
    if (!("locks" in navigator)) {
      setLeader(true);
      return;
    }
    // Let go of any request this tab already holds before asking again.
    release.current();
    const held = new Promise<void>((resolve) => {
      release.current = resolve;
    });
    navigator.locks
      .request(LOCK, { mode: "exclusive", steal }, async () => {
        if (active.current) setLeader(true);
        await held;
      })
      .catch((e: unknown) => {
        if (!active.current) return;
        // AbortError means another tab took the lock over: step down. Any
        // other failure means the API is unusable, so run standalone.
        const stolen = e instanceof DOMException && e.name === "AbortError";
        setLeader(!stolen);
      });
  }, []);

  useEffect(() => {
    active.current = true;
    hold(false);
    return () => {
      active.current = false;
      release.current();
    };
  }, [hold]);

  const takeOver = useCallback(() => hold(true), [hold]);
  return { leader, takeOver };
}

// SoftphoneProvider runs the SIP user agent once for the whole session (in the
// leader tab, where the microphone works) and keeps it alive across navigation.
// The SantralC extension mirrors and controls this same session from other
// tabs, so the mini widget and the panel are always in sync.
//
// The extension only registers while a shift is open. Off shift nothing rings
// anywhere (panel or mini widget): the PBX sees the extension as unregistered,
// on top of the do-not-disturb the server sets when a shift ends.
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user, can } = useAuth();
  const shift = useShift();
  const { leader, takeOver: steal } = useLeader();
  const hasExtension = !!user?.sipExtension;
  const enabled = leader && hasExtension && shift.active;

  const phone = useSoftphone(enabled);
  // The tab holding the line speaks for the phone, also off shift (the
  // widget then says the shift has not started).
  usePanelBridge(phone, leader && hasExtension, can("call.originate") && shift.active);

  // Hanging up here waits for the call's end to reach the server.
  const hangup = phone.hangup;
  const statusRef = useRef(phone.status);
  statusRef.current = phone.status;
  const endLocal = useCallback(async () => {
    await hangup();
    // The end is logged when the line reports the call over: wait for that
    // (at most two seconds), then for the send itself.
    for (let waited = 0; LIVE_STATES.has(statusRef.current) && waited < 2000; waited += 50) {
      await new Promise((resolve) => window.setTimeout(resolve, 50));
    }
    await settleCallLogs();
  }, [hangup]);
  const { liveElsewhere, endElsewhere } = useTabPhone(phone.status, endLocal);
  const liveHere = LIVE_STATES.has(phone.status);

  const endCalls = useCallback(async () => {
    if (liveHere) await endLocal();
    if (liveElsewhere) await endElsewhere();
    await settleCallLogs();
  }, [liveHere, liveElsewhere, endLocal, endElsewhere]);

  const takeOver = useCallback(async () => {
    await endElsewhere();
    steal();
  }, [endElsewhere, steal]);

  const value: SoftphoneValue = { ...phone, secondary: hasExtension && !leader, takeOver, liveHere, liveElsewhere, endCalls };

  return (
    <Ctx.Provider value={value}>
      {children}
      <audio ref={phone.audioRef} autoPlay className="hidden" />
    </Ctx.Provider>
  );
}

// SoftphoneMockProvider supplies a fixed value, for the development preview.
export function SoftphoneMockProvider({ value, children }: { value: SoftphoneValue; children: ReactNode }) {
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSoftphoneContext(): SoftphoneValue {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useSoftphoneContext must be used within SoftphoneProvider");
  return ctx;
}
