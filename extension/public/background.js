// Service worker: a relay only. The SIP session lives in the santral-c panel
// tab (where the microphone already works); this worker carries call state from
// the panel to the per-tab widgets, and widget commands back to the panel.
//
// Only a tab on the panel's address counts as the panel. The address is set
// in the extension's popup; any other page that claims to be the panel is
// ignored, and commands are sent to panel tabs only.

const DEFAULT_PANEL = "https://cm.toprakgureli.com";
const STATUSES = new Set(["idle", "connecting", "registered", "calling", "ringing", "incoming", "in-call", "held", "error", "disabled", "unconfigured"]);
const COMMANDS = new Set(["call", "answer", "hangup", "mute", "hold", "transfer", "dtmf"]);

let lastState = { status: "idle" };

async function panelOrigin() {
  try {
    const { panelOrigin: o } = await chrome.storage.local.get("panelOrigin");
    return typeof o === "string" && o ? o : DEFAULT_PANEL;
  } catch {
    return DEFAULT_PANEL;
  }
}

function originOf(url) {
  try {
    return new URL(url).origin;
  } catch {
    return "";
  }
}

async function fromPanel(sender) {
  return !!sender.tab && originOf(sender.tab.url) === (await panelOrigin());
}

// cleanState keeps only the fields the widget shows, in the shapes it expects.
function cleanState(s) {
  if (!s || typeof s !== "object" || !STATUSES.has(s.status)) return { status: "idle" };
  const str = (v, n) => (typeof v === "string" ? v.slice(0, n) : "");
  const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);
  return {
    status: s.status,
    peer: str(s.peer, 40),
    muted: !!s.muted,
    held: !!s.held,
    extension: str(s.extension, 16),
    endReason: str(s.endReason, 60),
    callStartedAt: num(s.callStartedAt),
    answeredAt: num(s.answeredAt),
    // The panel says whether this agent may place calls; the widget offers
    // the dial pad only then.
    canCall: s.canCall !== false,
  };
}

async function broadcast(msg) {
  const tabs = await chrome.tabs.query({});
  for (const tab of tabs) {
    if (tab.id != null) chrome.tabs.sendMessage(tab.id, msg).catch(() => undefined);
  }
}

async function toPanel(msg) {
  const origin = await panelOrigin();
  const tabs = await chrome.tabs.query({ url: origin + "/*" });
  for (const tab of tabs) {
    if (tab.id != null && originOf(tab.url) === origin) chrome.tabs.sendMessage(tab.id, msg).catch(() => undefined);
  }
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || msg.to !== "sw") return;

  if (msg.type === "getState") {
    sendResponse({ state: lastState });
    return true;
  }
  if (msg.type === "getPanelOrigin") {
    panelOrigin().then((origin) => sendResponse({ origin }));
    return true;
  }

  void (async () => {
    if (msg.type === "state" || msg.type === "panelAlive") {
      if (!(await fromPanel(sender))) return;
      if (msg.type === "state") {
        lastState = cleanState(msg.state);
        broadcast({ to: "content", type: "state", state: lastState });
      }
      // The panel tab is open; tell every tab so the widget stays visible.
      broadcast({ to: "content", type: "panelPing" });
      return;
    }
    if (msg.type === "cmd" && COMMANDS.has(msg.cmd)) {
      // From a widget: hand it to the panel tab to run on the live session.
      const arg = typeof msg.arg === "string" ? msg.arg.slice(0, 32) : undefined;
      toPanel({ to: "content", type: "panelcmd", cmd: msg.cmd, arg });
    }
  })();
});
