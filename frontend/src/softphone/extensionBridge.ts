import { useEffect, useRef } from "react";
import type { Phone } from "./useSoftphone";
import { normalizeDial } from "./dial";

const DESKTOP_BRIDGE_URL = "ws://127.0.0.1:48123";

function postPanel(type: string, extra?: Record<string, unknown>) {
  window.postMessage({ santralc: "panel", type, ...(extra ?? {}) }, window.location.origin);
}

function executeBridgeCommand(cmd: string, arg: string, phone: Phone, canCall: boolean) {
  switch (cmd) {
    case "call": {
      // The same rule as the panel's own dialer: the right to call, a
      // ready line and no call already on it.
      if (!canCall || phone.status !== "registered") break;
      // Widgets send whatever was typed ("+90 0530...", "90530...");
      // coerce it to the form the PBX dials, like the panel's own dialer.
      const n = normalizeDial(arg);
      if (n) void phone.call(n).catch(() => undefined);
      break;
    }
    case "answer":
      void phone.answer().catch(() => undefined);
      break;
    case "hangup":
      void phone.hangup().catch(() => undefined);
      break;
    case "mute":
      phone.toggleMute();
      break;
    case "hold":
      void phone.toggleHold().catch(() => undefined);
      break;
    case "transfer": {
      const n = normalizeDial(arg);
      if (n) void phone.transfer(n).catch(() => undefined);
      break;
    }
    case "dtmf":
      phone.sendDtmf(arg);
      break;
  }
}

// usePanelBridge connects the panel's live softphone to both:
// 1) The SantralC Chrome extension (via postMessage to content scripts)
// 2) The SantralC Windows Desktop Mini App (via local loopback WebSocket ws://127.0.0.1:48123)
// The panel owns the SIP session and microphone; it publishes state to the
// widgets and runs the commands they send back, so the mini widget mirrors
// and controls this panel seamlessly.
export function usePanelBridge(phone: Phone, active: boolean, canCall: boolean) {
  const phoneRef = useRef(phone);
  const activeRef = useRef(active);
  const canCallRef = useRef(canCall);
  const desktopWsRef = useRef<WebSocket | null>(null);

  phoneRef.current = phone;
  activeRef.current = active;
  canCallRef.current = canCall;

  // Announce that this tab is the panel (so the extension hides its own widget
  // here and routes commands to this page), and heartbeat while the panel stays
  // open so the widget only appears on other tabs while SantralC is running.
  useEffect(() => {
    postPanel("hello");
    const t = window.setInterval(() => {
      postPanel("alive");
      if (desktopWsRef.current && desktopWsRef.current.readyState === WebSocket.OPEN) {
        try {
          desktopWsRef.current.send(JSON.stringify({ type: "alive" }));
        } catch {
          // ignore
        }
      }
    }, 4000);
    return () => window.clearInterval(t);
  }, []);

  // Publish call state whenever it changes (only from the owning tab).
  useEffect(() => {
    if (!active) return;
    const state = {
      status: phone.status,
      peer: phone.peer,
      muted: phone.muted,
      held: phone.held,
      extension: phone.extension,
      endReason: phone.endReason,
      callStartedAt: phone.callStartedAt,
      answeredAt: phone.answeredAt,
      canCall,
    };
    postPanel("state", { state });
    if (desktopWsRef.current && desktopWsRef.current.readyState === WebSocket.OPEN) {
      try {
        desktopWsRef.current.send(JSON.stringify({ type: "state", state }));
      } catch {
        // ignore
      }
    }
  }, [
    active,
    canCall,
    phone.status,
    phone.peer,
    phone.muted,
    phone.held,
    phone.extension,
    phone.endReason,
    phone.callStartedAt,
    phone.answeredAt,
  ]);

  // Connect to the local Windows Desktop Mini App via loopback WebSocket
  // when this tab is the active softphone session.
  useEffect(() => {
    if (!active) {
      if (desktopWsRef.current) {
        try {
          desktopWsRef.current.close(1000, "Panel tab not active");
        } catch {
          // ignore
        }
        desktopWsRef.current = null;
      }
      return;
    }

    let ws: WebSocket | null = null;
    let retryTimer: number | null = null;
    let backoffMs = 3000;
    let stopped = false;

    function connect() {
      if (stopped) return;
      try {
        ws = new WebSocket(DESKTOP_BRIDGE_URL);
        desktopWsRef.current = ws;

        ws.onopen = () => {
          if (stopped) {
            try {
              ws?.close();
            } catch {
              // ignore
            }
            return;
          }
          backoffMs = 3000;
          try {
            ws?.send(JSON.stringify({ type: "hello" }));
            const p = phoneRef.current;
            ws?.send(
              JSON.stringify({
                type: "state",
                state: {
                  status: p.status,
                  peer: p.peer,
                  muted: p.muted,
                  held: p.held,
                  extension: p.extension,
                  endReason: p.endReason,
                  callStartedAt: p.callStartedAt,
                  answeredAt: p.answeredAt,
                  canCall: canCallRef.current,
                },
              })
            );
          } catch {
            // ignore send error
          }
        };

        ws.onmessage = (e) => {
          try {
            const data = JSON.parse(e.data);
            if (data && data.type === "cmd" && typeof data.cmd === "string") {
              executeBridgeCommand(
                data.cmd,
                String(data.arg ?? ""),
                phoneRef.current,
                canCallRef.current
              );
            }
          } catch {
            // ignore malformed message
          }
        };

        ws.onclose = () => {
          if (desktopWsRef.current === ws) {
            desktopWsRef.current = null;
          }
          if (!stopped) {
            scheduleRetry();
          }
        };

        ws.onerror = () => {
          // Handled silently by onclose
        };
      } catch {
        if (!stopped) {
          scheduleRetry();
        }
      }
    }

    function scheduleRetry() {
      if (retryTimer) window.clearTimeout(retryTimer);
      retryTimer = window.setTimeout(() => {
        connect();
      }, backoffMs);
      backoffMs = Math.min(Math.round(backoffMs * 1.5), 10000);
    }

    connect();

    return () => {
      stopped = true;
      if (retryTimer) {
        window.clearTimeout(retryTimer);
        retryTimer = null;
      }
      if (ws) {
        ws.onclose = null;
        ws.onerror = null;
        ws.onmessage = null;
        ws.onopen = null;
        try {
          ws.close(1000, "tab unmounted or inactive");
        } catch {
          // ignore
        }
        if (desktopWsRef.current === ws) {
          desktopWsRef.current = null;
        }
      }
    };
  }, [active]);

  // Run commands coming from the widgets on the live session.
  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      const d = e.data;
      if (
        e.source !== window ||
        e.origin !== window.location.origin ||
        !d ||
        d.santralc !== "ext" ||
        d.type !== "cmd"
      )
        return;
      if (!activeRef.current) return;
      executeBridgeCommand(
        String(d.cmd ?? ""),
        String(d.arg ?? ""),
        phoneRef.current,
        canCallRef.current
      );
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, []);
}
