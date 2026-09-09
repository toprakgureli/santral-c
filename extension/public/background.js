// Service worker: a relay only. The SIP session lives in the santral-c panel
// tab (where the microphone already works); this worker carries call state from
// the panel to the per-tab widgets, and widget commands back to the panel.

let lastState = { status: "idle" };

async function broadcast(msg) {
  const tabs = await chrome.tabs.query({});
  for (const tab of tabs) {
    if (tab.id != null) chrome.tabs.sendMessage(tab.id, msg).catch(() => undefined);
  }
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (!msg || msg.to !== "sw") return;

  if (msg.type === "state") {
    lastState = msg.state || { status: "idle" };
    broadcast({ to: "content", type: "state", state: lastState });
    return;
  }
  if (msg.type === "cmd") {
    // From a widget: hand it to the panel tab(s) to run on the live session.
    broadcast({ to: "content", type: "panelcmd", cmd: msg.cmd, arg: msg.arg });
    return;
  }
  if (msg.type === "getState") {
    sendResponse({ state: lastState });
    return true;
  }
});
