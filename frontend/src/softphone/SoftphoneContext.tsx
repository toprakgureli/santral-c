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
  // Defer to the extension only when it is actually registered/handling a call,
  // not merely installed. Otherwise the panel registers and dials itself, so a
  // present-but-unregistered extension never blocks calling.
  const extReady =
    ext.active === true &&
    ["registered", "calling", "ringing", "incoming", "in-call", "held"].includes(ext.phone.status);
  const usingExtension = extReady;

  const localEnabled = !extReady && leader && hasExtension;
  const local = useSoftphone(localEnabled);

  // Feed the panel user's credentials to the extension so it registers as them.
  // Pushed twice to cover the race where the offscreen booted before the
  // credentials were stored.
  useEffect(() => {
    if (ext.active !== true || !hasExtension) return;
    let cancelled = false;
    const push = () =>
      api
        .sipCredentials()
        .then((c) => {
          if (!cancelled) {
            ext.pushConfig({ ext: c.extension, password: c.password, wss: c.webSocketUrl, domain: c.domain, stun: c.stunUrl });
          }
        })
        .catch(() => undefined);
    push();
    const timer = window.setTimeout(push, 1500);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [ext.active, hasExtension]); // eslint-disable-line react-hooks/exhaustive-deps

  const value: SoftphoneValue = usingExtension
    ? { ...ext.phone, extension: ext.phone.extension ?? user?.sipExtension ?? null, secondary: false }
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
