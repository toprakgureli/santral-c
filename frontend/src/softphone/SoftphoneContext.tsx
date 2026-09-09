import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { useSoftphone, type Phone } from "./useSoftphone";

type SoftphoneValue = Phone & { secondary: boolean };

const Ctx = createContext<SoftphoneValue | null>(null);

// useLeader elects a single tab to own the SIP registration via the Web Locks
// API, so two open tabs of the panel do not fight over the same extension. The
// tab holding the exclusive lock is the leader; when it closes, the lock frees
// and another tab takes over.
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

// SoftphoneProvider runs the SIP user agent once for the whole session (only in
// the leader tab), so the registration and any active call survive navigation.
// The audio sink lives here too, not inside a page, for the same reason.
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const leader = useLeader();
  const hasExtension = !!user?.sipExtension;
  const phone = useSoftphone(leader && hasExtension);
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
