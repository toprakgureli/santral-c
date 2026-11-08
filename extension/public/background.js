// Service worker: a relay only. The SIP session lives in the santral-c panel
// tab (where the microphone already works); this worker carries call state from
// the panel to the per-tab widgets, and widget commands back to the panel.
//
// In addition, it bridges third-party iframe sessions inside the panel:
// - Removes X-Frame-Options & CSP frame-ancestors headers on sub_frame loads
//   via declarativeNetRequest so external tools can open inside Santral's iframe.
// - Upgrades Set-Cookie headers received inside iframes (frameId > 0) to
//   SameSite=None; Secure via chrome.cookies.set so session cookies (e.g.
//   framework session cookies, XSRF-TOKEN, auth tokens) persist
//   across cross-origin iframe requests.
// - Protects freshly issued POST login cookies against immediate 302 redirect
//   races where Chrome's native redirect follower sends a GET before the
//   async chrome.cookies.set call finishes.
//
// Only a tab on the panel's address counts as the panel. The address is set
// in the extension's popup; any other page that claims to be the panel is
// ignored, and commands are sent to panel tabs only.

const DEFAULT_PANEL = "https://cm.toprakgureli.com";
const STATUSES = new Set(["idle", "connecting", "registered", "calling", "ringing", "incoming", "in-call", "held", "error", "disabled", "unconfigured"]);
const COMMANDS = new Set(["call", "answer", "hangup", "mute", "hold", "transfer", "dtmf"]);
const FRAME_HEADER_RULE_ID = 9001;
const POST_LOGIN_LOCK_MS = 4000;
const PENDING_REDIRECT_TTL_MS = 8000;

let lastState = { status: "idle" };

// Sequential queue for chrome.cookies.set operations so frame redirect checks
// always wait until all pending Set-Cookie upgrades have been committed.
let cookieQueue = Promise.resolve();

// Key: `${tabId}:${frameId}:${origin}` -> { expiresAt: number, cookies: Map<string, object> }
const postLoginLocks = new Map();

// Key: `${tabId}:${frameId}` -> { url: string, expiresAt: number }
const pendingRedirects = new Map();

function setupFrameBridge() {
  if (typeof chrome === "undefined") return;

  if (chrome.declarativeNetRequest && typeof chrome.declarativeNetRequest.updateDynamicRules === "function") {
    try {
      Promise.resolve(
        chrome.declarativeNetRequest.updateDynamicRules({
          removeRuleIds: [FRAME_HEADER_RULE_ID],
          addRules: [
            {
              id: FRAME_HEADER_RULE_ID,
              priority: 1,
              action: {
                type: "modifyHeaders",
                responseHeaders: [
                  { header: "x-frame-options", operation: "remove" },
                  { header: "content-security-policy", operation: "remove" },
                  { header: "x-content-security-policy", operation: "remove" },
                ],
              },
              condition: {
                urlFilter: "*",
                resourceTypes: ["sub_frame"],
              },
            },
          ],
        }),
      ).catch(() => {});
    } catch {}
  }

  if (chrome.contentSettings && chrome.contentSettings.cookies && typeof chrome.contentSettings.cookies.set === "function") {
    try {
      chrome.contentSettings.cookies.set({
        primaryPattern: "<all_urls>",
        secondaryPattern: "<all_urls>",
        setting: "allow",
      });
    } catch {}
  }
}

setupFrameBridge();

if (typeof chrome !== "undefined" && chrome.runtime) {
  if (chrome.runtime.onInstalled && typeof chrome.runtime.onInstalled.addListener === "function") {
    chrome.runtime.onInstalled.addListener(setupFrameBridge);
  }
  if (chrome.runtime.onStartup && typeof chrome.runtime.onStartup.addListener === "function") {
    chrome.runtime.onStartup.addListener(setupFrameBridge);
  }
}

function isLoginUrl(rawUrl) {
  try {
    const u = new URL(rawUrl);
    return /\/(login|signin|giris|auth\/login)(\b|\/|$)/i.test(u.pathname);
  } catch {
    return /\/(login|signin|giris)(\b|\/|\?|$)/i.test(String(rawUrl || ""));
  }
}

function isLogoutUrl(rawUrl) {
  return /\/(logout|signout|cikis|log-out|sign-out)(\b|\/|\?|$)/i.test(String(rawUrl || ""));
}

function parseSetCookie(rawHeader, requestUrl) {
  if (typeof rawHeader !== "string" || !rawHeader.trim()) return null;
  let url;
  try {
    url = new URL(requestUrl);
  } catch {
    return null;
  }

  const parts = rawHeader.split(";").map((p) => p.trim()).filter(Boolean);
  if (parts.length === 0) return null;

  const firstEq = parts[0].indexOf("=");
  if (firstEq <= 0) return null;

  const name = parts[0].slice(0, firstEq).trim();
  const value = parts[0].slice(firstEq + 1);
  if (!name) return null;

  let domain;
  let path = "/";
  let httpOnly = false;
  let expirationDate;
  let isDeletion = false;

  for (let i = 1; i < parts.length; i++) {
    const seg = parts[i];
    const eq = seg.indexOf("=");
    const attrName = (eq === -1 ? seg : seg.slice(0, eq)).trim().toLowerCase();
    const attrVal = eq === -1 ? "" : seg.slice(eq + 1).trim();

    if (attrName === "domain" && attrVal) {
      domain = attrVal.replace(/^\.+/, "").toLowerCase();
    } else if (attrName === "path" && attrVal) {
      path = attrVal.startsWith("/") ? attrVal : "/" + attrVal;
    } else if (attrName === "httponly") {
      httpOnly = true;
    } else if (attrName === "max-age") {
      const maxAge = Number(attrVal);
      if (Number.isFinite(maxAge)) {
        if (maxAge <= 0) {
          isDeletion = true;
        } else {
          expirationDate = Math.floor(Date.now() / 1000) + maxAge;
        }
      }
    } else if (attrName === "expires" && attrVal && expirationDate === undefined && !isDeletion) {
      const parsedMs = Date.parse(attrVal);
      if (Number.isFinite(parsedMs)) {
        if (parsedMs <= Date.now()) {
          isDeletion = true;
        } else {
          expirationDate = Math.floor(parsedMs / 1000);
        }
      }
    }
  }

  if (value === "" || value.toLowerCase() === "deleted") {
    if (isDeletion || (expirationDate !== undefined && expirationDate <= Math.floor(Date.now() / 1000))) {
      isDeletion = true;
    }
  }

  // __Host- prefixed cookies must not specify a domain and must have path="/"
  if (name.startsWith("__Host-")) {
    domain = undefined;
    path = "/";
  }

  const host = domain || url.hostname;
  const cookieUrl = `https://${host}${path}`;

  const details = {
    url: cookieUrl,
    name,
    value,
    path,
    secure: true,
    httpOnly,
    sameSite: "no_restriction",
  };
  if (domain) {
    details.domain = domain;
  }
  if (expirationDate !== undefined) {
    details.expirationDate = expirationDate;
  }

  return {
    details,
    isDeletion,
    key: `${domain || url.hostname}|${path}|${name}`,
  };
}

function enqueueCookieSet(details) {
  if (typeof chrome === "undefined" || !chrome.cookies || typeof chrome.cookies.set !== "function") {
    return Promise.resolve();
  }
  cookieQueue = cookieQueue
    .then(() => chrome.cookies.set(details))
    .catch(() => {});
  return cookieQueue;
}

function triggerFrameRedirect(tabId, frameId, targetUrl) {
  if (typeof chrome === "undefined" || !chrome.tabs || typeof chrome.tabs.sendMessage !== "function") return;
  if (typeof tabId !== "number" || tabId < 0 || typeof frameId !== "number" || frameId <= 0) return;
  cookieQueue.then(() => {
    try {
      chrome.tabs
        .sendMessage(tabId, { to: "frame", type: "navigate", url: targetUrl }, { frameId })
        .catch(() => {});
    } catch {}
  });
}

if (
  typeof chrome !== "undefined" &&
  chrome.webRequest &&
  chrome.webRequest.onHeadersReceived &&
  typeof chrome.webRequest.onHeadersReceived.addListener === "function"
) {
  const extraInfoSpec = ["responseHeaders", "extraHeaders"];
  try {
    chrome.webRequest.onHeadersReceived.addListener(
      (details) => {
        if (!details || !(details.frameId > 0) || !Array.isArray(details.responseHeaders)) return;

        const setCookieHeaders = [];
        let locationHeader = "";

        for (const h of details.responseHeaders) {
          if (!h || typeof h.name !== "string") continue;
          const lower = h.name.toLowerCase();
          if (lower === "set-cookie" && typeof h.value === "string") {
            setCookieHeaders.push(h.value);
          } else if (lower === "location" && typeof h.value === "string" && !locationHeader) {
            locationHeader = h.value;
          }
        }

        const origin = originOf(details.url);
        const lockKey = `${details.tabId}:${details.frameId}:${origin}`;
        const frameKey = `${details.tabId}:${details.frameId}`;
        const now = Date.now();

        let activeLock = postLoginLocks.get(lockKey);
        if (activeLock && activeLock.expiresAt <= now) {
          postLoginLocks.delete(lockKey);
          activeLock = undefined;
        }

        const parsedCookies = setCookieHeaders
          .map((raw) => parseSetCookie(raw, details.url))
          .filter((c) => c !== null);

        const nonDeleted = parsedCookies.filter((c) => !c.isDeletion);
        const method = String(details.method || "GET").toUpperCase();
        const isPost = method === "POST";
        const isRedirect =
          details.statusCode === 301 ||
          details.statusCode === 302 ||
          details.statusCode === 303 ||
          details.statusCode === 307 ||
          details.statusCode === 308;

        if (isPost && !isLogoutUrl(details.url)) {
          if (nonDeleted.length > 0) {
            const lockedMap = activeLock ? activeLock.cookies : new Map();
            for (const c of nonDeleted) {
              lockedMap.set(c.key, c.details);
              enqueueCookieSet(c.details);
            }
            postLoginLocks.set(lockKey, {
              expiresAt: now + POST_LOGIN_LOCK_MS,
              cookies: lockedMap,
            });
          }

          if (isRedirect && locationHeader) {
            try {
              const resolvedTarget = new URL(locationHeader, details.url).href;
              pendingRedirects.set(frameKey, {
                url: resolvedTarget,
                expiresAt: now + PENDING_REDIRECT_TTL_MS,
              });
            } catch {}
          }
          return;
        }

        // If a GET request inside the iframe is made right after a POST login
        // and the server bounces it back to /login (because Chrome's native 302
        // follower didn't attach the async SameSite=None cookie yet), preserve
        // the POST login cookies and schedule a recovery navigation.
        if (activeLock) {
          for (const c of nonDeleted) {
            if (activeLock.cookies.has(c.key)) {
              enqueueCookieSet(activeLock.cookies.get(c.key));
            } else {
              enqueueCookieSet(c.details);
            }
          }

          if (details.type === "sub_frame" && isRedirect && locationHeader && !isLoginUrl(details.url)) {
            try {
              const bouncedTo = new URL(locationHeader, details.url).href;
              if (isLoginUrl(bouncedTo) && !pendingRedirects.has(frameKey)) {
                pendingRedirects.set(frameKey, {
                  url: details.url,
                  expiresAt: now + PENDING_REDIRECT_TTL_MS,
                });
              }
            } catch {}
          }

          const pending = pendingRedirects.get(frameKey);
          if (pending && pending.expiresAt > now && details.type === "sub_frame" && isLoginUrl(details.url)) {
            triggerFrameRedirect(details.tabId, details.frameId, pending.url);
          }
          return;
        }

        for (const c of nonDeleted) {
          enqueueCookieSet(c.details);
        }
      },
      { urls: ["<all_urls>"] },
      extraInfoSpec,
    );
  } catch {}
}

// Local Windows Desktop Mini App Bridge (ws://127.0.0.1:48123)
const DESKTOP_BRIDGE_URL = "ws://127.0.0.1:48123";
let desktopWs = null;
let desktopRetryTimer = null;

function sendToDesktop(payload) {
  if (desktopWs && desktopWs.readyState === 1 /* OPEN */) {
    try {
      desktopWs.send(JSON.stringify(payload));
    } catch {}
  }
}

function connectDesktopBridge() {
  if (typeof WebSocket === "undefined") return;
  if (desktopWs && (desktopWs.readyState === 0 || desktopWs.readyState === 1)) {
    return;
  }
  try {
    desktopWs = new WebSocket(DESKTOP_BRIDGE_URL);
    desktopWs.onopen = () => {
      sendToDesktop({ type: "hello" });
      if (lastState && lastState.status && lastState.status !== "idle") {
        sendToDesktop({ type: "state", state: lastState });
      }
    };
    desktopWs.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data && data.type === "cmd" && COMMANDS.has(data.cmd)) {
          const arg = typeof data.arg === "string" ? data.arg.slice(0, 32) : undefined;
          toPanel({ to: "content", type: "panelcmd", cmd: data.cmd, arg });
        }
      } catch {}
    };
    desktopWs.onclose = () => {
      desktopWs = null;
      scheduleDesktopRetry();
    };
    desktopWs.onerror = () => {
      // Handled by onclose
    };
  } catch {
    desktopWs = null;
    scheduleDesktopRetry();
  }
}

function scheduleDesktopRetry() {
  if (typeof setTimeout === "undefined") return;
  if (desktopRetryTimer) clearTimeout(desktopRetryTimer);
  desktopRetryTimer = setTimeout(connectDesktopBridge, 3000);
}

connectDesktopBridge();

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
  if (msg.type === "checkFrameRedirect" || msg.type === "completeFormPost") {
    const tabId = sender && sender.tab ? sender.tab.id : undefined;
    const frameId = sender ? sender.frameId : undefined;
    if (typeof tabId !== "number" || typeof frameId !== "number" || frameId <= 0) {
      sendResponse({ redirectUrl: null });
      return true;
    }
    const frameKey = `${tabId}:${frameId}`;
    cookieQueue.then(() => {
      const pending = pendingRedirects.get(frameKey);
      if (!pending || pending.expiresAt <= Date.now()) {
        pendingRedirects.delete(frameKey);
        sendResponse({ redirectUrl: null });
        return;
      }
      pendingRedirects.delete(frameKey);
      if (msg.type === "completeFormPost") {
        sendResponse({ redirectUrl: pending.url });
        return;
      }
      const currentHref = typeof msg.href === "string" && msg.href ? msg.href : sender.url || "";
      if (currentHref && !isLoginUrl(currentHref) && currentHref === pending.url) {
        sendResponse({ redirectUrl: null });
        return;
      }
      sendResponse({ redirectUrl: pending.url });
    });
    return true;
  }

  void (async () => {
    if (msg.type === "state" || msg.type === "panelAlive") {
      if (!(await fromPanel(sender))) return;
      if (msg.type === "state") {
        lastState = cleanState(msg.state);
        broadcast({ to: "content", type: "state", state: lastState });
        sendToDesktop({ type: "state", state: lastState });
      }
      // The panel tab is open; tell every tab so the widget stays visible.
      broadcast({ to: "content", type: "panelPing" });
      sendToDesktop({ type: "alive" });
      return;
    }
    if (msg.type === "cmd" && COMMANDS.has(msg.cmd)) {
      // From a widget: hand it to the panel tab to run on the live session.
      const arg = typeof msg.arg === "string" ? msg.arg.slice(0, 32) : undefined;
      toPanel({ to: "content", type: "panelcmd", cmd: msg.cmd, arg });
    }
  })();
});

if (typeof chrome !== "undefined" && chrome.tabs && chrome.tabs.onRemoved) {
  chrome.tabs.onRemoved.addListener(async () => {
    try {
      const origin = await panelOrigin();
      const tabs = await chrome.tabs.query({ url: origin + "/*" });
      if (tabs.length === 0) {
        lastState = { status: "disconnected" };
        sendToDesktop({ type: "state", state: lastState });
      }
    } catch {}
  });
}
