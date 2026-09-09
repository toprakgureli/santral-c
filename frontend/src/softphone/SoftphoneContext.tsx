import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { useSoftphone, type Phone } from "./useSoftphone";
import { usePanelBridge } from "./extensionBridge";

type SoftphoneValue = Phone & { secondary: boolean };

const Ctx = createContext<SoftphoneValue | null>(null);

// useLeader elects a single tab to own the SIP registration via the Web Locks
// API, so two open tabs of the panel do not fight over the same extension.
function useLeader(): boolean {
  const [leader, setLeader] = useState(false);
  useEffect(() => {
    if (!("locks" in navigator)) {
      setLeader(true);
      return;
    }
    let active = true;
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    navigator.locks
      .request("santral-softphone", { mode: "exclusive" }, async () => {
        if (active) setLeader(true);
        await held;
      })
      .catch(() => {
        if (active) setLeader(true);
      });
    return () => {
      active = false;
      release();
    };
  }, []);
  return leader;
}

// SoftphoneProvider runs the SIP user agent once for the whole session (in the
// leader tab, where the microphone works) and keeps it alive across navigation.
// The SantralC extension mirrors and controls this same session from other
// tabs, so the mini widget and the panel are always in sync.
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const leader = useLeader();
  const hasExtension = !!user?.sipExtension;
  const enabled = leader && hasExtension;

  const phone = useSoftphone(enabled);
  usePanelBridge(phone, enabled);

  const value: SoftphoneValue = { ...phone, secondary: hasExtension && !leader };

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
