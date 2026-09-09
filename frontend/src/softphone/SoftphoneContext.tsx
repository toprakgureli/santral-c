import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { api } from "@/api/client";
import { useSoftphone, type Phone } from "./useSoftphone";
import { useExtensionPhone } from "./extensionBridge";

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

// SoftphoneProvider gives the whole session one softphone. If the SantralC
// browser extension is present, the panel shares the extension's single
// registration (so the mini widget and the panel are the same call, in sync,
// and the panel pushes its credentials so nothing is entered by hand).
// Otherwise the panel registers locally in the leader tab.
export function SoftphoneProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const leader = useLeader();
  const hasExtension = !!user?.sipExtension;

  const ext = useExtensionPhone();
  const usingExtension = ext.active === true;

  // Register locally only when the browser extension is not handling telephony.
  const localEnabled = ext.active === false && leader && hasExtension;
  const local = useSoftphone(localEnabled);

  // Feed the panel user's credentials to the extension so it registers as them.
  useEffect(() => {
    if (!usingExtension || !hasExtension) return;
    api
      .sipCredentials()
      .then((c) =>
        ext.pushConfig({ ext: c.extension, password: c.password, wss: c.webSocketUrl, domain: c.domain, stun: c.stunUrl }),
      )
      .catch(() => undefined);
  }, [usingExtension, hasExtension]); // eslint-disable-line react-hooks/exhaustive-deps

  const value: SoftphoneValue = usingExtension
    ? { ...ext.phone, secondary: false }
    : { ...local, secondary: hasExtension && !leader };

  return (
    <Ctx.Provider value={value}>
      {children}
      {!usingExtension && <audio ref={local.audioRef} autoPlay className="hidden" />}
    </Ctx.Provider>
  );
}

export function useSoftphoneContext(): SoftphoneValue {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useSoftphoneContext must be used within SoftphoneProvider");
  return ctx;
}
