import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { useSoftphone, type Phone } from "./useSoftphone";
import { usePanelBridge } from "./extensionBridge";

type SoftphoneValue = Phone & { secondary: boolean; takeOver: () => void };

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
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const { leader, takeOver } = useLeader();
  const hasExtension = !!user?.sipExtension;
  const enabled = leader && hasExtension;

  const phone = useSoftphone(enabled);
  usePanelBridge(phone, enabled);

  const value: SoftphoneValue = { ...phone, secondary: hasExtension && !leader, takeOver };

  return (
    <Ctx.Provider value={value}>
      {children}
      <audio ref={phone.audioRef} autoPlay className="hidden" />
    </Ctx.Provider>
  );
}

export function useSoftphoneContext(): SoftphoneValue {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useSoftphoneContext must be used within SoftphoneProvider");
  return ctx;
}
