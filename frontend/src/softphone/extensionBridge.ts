import { useCallback, useEffect, useRef, useState } from "react";
import type { Phone, PhoneStatus } from "./useSoftphone";

// Config pushed to the extension so it registers with the panel user's own
// credentials (no manual entry).
export type ExtConfig = { ext: string; password: string; wss: string; domain: string; stun?: string };

type ExtState = {
  status: string;
  peer?: string;
  muted?: boolean;
  held?: boolean;
  extension?: string;
  endReason?: string;
  error?: string;
};

function mapStatus(s: string): PhoneStatus {
  switch (s) {
    case "idle":
    case "registered":
      return "registered";
    case "unconfigured":
      return "connecting";
    case "calling":
    case "ringing":
    case "incoming":
    case "in-call":
    case "held":
    case "error":
      return s;
    default:
      return "connecting";
  }
}

function postPanel(type: string, extra?: Record<string, unknown>) {
  window.postMessage({ santralc: "panel", type, ...(extra ?? {}) }, "*");
}

// useExtensionPhone detects the SantralC extension (via the content-script
// bridge in this tab) and, when present, exposes a Phone that controls the
// extension's single shared session. active is null while detecting.
export function useExtensionPhone(): { active: boolean | null; phone: Phone; pushConfig: (c: ExtConfig) => void } {
  const [active, setActive] = useState<boolean | null>(null);
  const [ext, setExt] = useState<ExtState>({ status: "registered" });
  const audioRef = useRef<HTMLAudioElement>(null);

  useEffect(() => {
    let resolved = false;
    const onMessage = (e: MessageEvent) => {
      const d = e.data;
      if (e.source !== window || !d || d.santralc !== "ext") return;
      if (d.type === "present") {
        resolved = true;
        setActive(true);
        if (d.state) setExt(d.state as ExtState);
      } else if (d.type === "state") {
        setActive(true);
        setExt((d.state as ExtState) ?? { status: "registered" });
      }
    };
    window.addEventListener("message", onMessage);
    postPanel("hello");
    const timer = window.setTimeout(() => {
      if (!resolved) setActive(false);
    }, 1200);
    return () => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(timer);
    };
  }, []);

  const cmd = useCallback((c: string, arg?: string) => postPanel("cmd", { cmd: c, arg }), []);
  const pushConfig = useCallback((c: ExtConfig) => postPanel("config", { config: c }), []);

  const phone: Phone = {
    status: mapStatus(ext.status),
    extension: ext.extension ?? null,
    error: ext.error ?? null,
    muted: !!ext.muted,
    held: !!ext.held,
    peer: ext.peer ?? null,
    endReason: ext.endReason ?? null,
    audioRef,
    call: async (t) => cmd("call", t),
    answer: async () => cmd("answer"),
    hangup: async () => cmd("hangup"),
    toggleMute: () => cmd("mute"),
    toggleHold: async () => cmd("hold"),
    transfer: async (t) => cmd("transfer", t),
    sendDtmf: (k) => cmd("dtmf", k),
  };

  return { active, phone, pushConfig };
}
