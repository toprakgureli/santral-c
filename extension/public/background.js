// Service worker: keeps a single offscreen document alive (the one SIP
// registration and WebRTC media) and relays messages between the per-tab
// content-script widgets and that offscreen document.

let lastState = { status: "idle" };

async function ensureOffscreen() {
  const has = await chrome.offscreen.hasDocument();
  if (has) return;
  await chrome.offscreen.createDocument({
    url: "offscreen.html",
    reasons: ["USER_MEDIA"],
    justification: "SIP softphone registration and call audio.",
  });
}

chrome.runtime.onStartup.addListener(() => ensureOffscreen());
chrome.runtime.onInstalled.addListener(() => ensureOffscreen());

async function broadcastToTabs(msg) {
  const tabs = await chrome.tabs.query({});
  for (const tab of tabs) {
    if (tab.id != null) {
      chrome.tabs.sendMessage(tab.id, msg).catch(() => undefined);
    }
  }
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || typeof msg !== "object") return;

  if (msg.to === "offscreen" && !msg._fwd) {
    // A command from a widget or the popup: make sure the phone exists, forward
    // it once (the _fwd guard stops the worker re-handling its own broadcast).
    ensureOffscreen().then(() => chrome.runtime.sendMessage({ ...msg, _fwd: true }).catch(() => undefined));
    return;
  }

  if (msg.to === "sw") {
    if (msg.type === "state") {
      lastState = msg.state;
      broadcastToTabs({ to: "content", type: "state", state: lastState });
      return;
    }
    if (msg.type === "getState") {
      ensureOffscreen();
      sendResponse({ state: lastState });
      return true;
    }
  }
});
