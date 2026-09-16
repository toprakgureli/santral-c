import { useEffect, useRef } from "react";
import type { Phone } from "./useSoftphone";
import { normalizeDial } from "./dial";

function postPanel(type: string, extra?: Record<string, unknown>) {
  window.postMessage({ santralc: "panel", type, ...(extra ?? {}) }, "*");
}

// usePanelBridge connects the panel's live softphone to the SantralC extension
// (via the content-script bridge in this tab). The panel owns the SIP session
// and microphone; it publishes state to the per-tab widgets and runs the
// commands they send back, so the mini widget mirrors and controls this panel.
export function usePanelBridge(phone: Phone, active: boolean) {
  const phoneRef = useRef(phone);
  const activeRef = useRef(active);
  phoneRef.current = phone;
  activeRef.current = active;

  // Announce that this tab is the panel (so the extension hides its own widget
  // here and routes commands to this page), and heartbeat while the panel stays
  // open so the widget only appears on other tabs while SantralC is running.
  useEffect(() => {
    postPanel("hello");
    const t = window.setInterval(() => postPanel("alive"), 4000);
    return () => window.clearInterval(t);
  }, []);

  // Publish call state whenever it changes (only from the owning tab).
  useEffect(() => {
    if (!active) return;
    postPanel("state", {
      state: {
        status: phone.status,
        peer: phone.peer,
        muted: phone.muted,
        held: phone.held,
        extension: phone.extension,
        endReason: phone.endReason,
        callStartedAt: phone.callStartedAt,
        answeredAt: phone.answeredAt,
      },
    });
  }, [active, phone.status, phone.peer, phone.muted, phone.held, phone.extension, phone.endReason, phone.callStartedAt, phone.answeredAt]);

  // Run commands coming from the widgets on the live session.
  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      const d = e.data;
      if (e.source !== window || !d || d.santralc !== "ext" || d.type !== "cmd") return;
      if (!activeRef.current) return;
      const p = phoneRef.current;
      const arg = String(d.arg ?? "");
      switch (d.cmd) {
        case "call": {
          // Widgets send whatever was typed ("+90 0530...", "90530...");
          // coerce it to the form the PBX dials, like the panel's own dialer.
          const n = normalizeDial(arg);
          if (n) void p.call(n).catch(() => undefined);
          break;
        }
        case "answer":
          void p.answer().catch(() => undefined);
          break;
        case "hangup":
          void p.hangup().catch(() => undefined);
          break;
        case "mute":
          p.toggleMute();
          break;
        case "hold":
          void p.toggleHold().catch(() => undefined);
          break;
        case "transfer": {
          const n = normalizeDial(arg);
          if (n) void p.transfer(n).catch(() => undefined);
          break;
        }
        case "dtmf":
          p.sendDtmf(arg);
          break;
      }
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, []);
}
