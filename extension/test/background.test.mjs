// Runs the service worker with a stand-in for the chrome API and checks
// that only the panel's tab is believed and that commands reach only it.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const PANEL = "https://cm.toprakgureli.com";

function load(storedOrigin, mockFetch) {
  const sent = [];
  const cookiesSet = [];
  const dnrRules = [];
  const fetchCalls = [];
  let listener;
  let headersListener;
  const fetchImpl = mockFetch || (async (url, init) => {
    fetchCalls.push({ url, init });
    return {
      status: 200,
      statusText: "OK",
      url,
      headers: new Map([["content-type", "application/json"]]),
      text: async () => '{"ok":true}',
    };
  });
  const tabs = [
    { id: 1, url: PANEL + "/dashboard" },
    { id: 2, url: "https://evil.example/page" },
    { id: 3, url: "https://news.example/" },
  ];
  const chrome = {
    storage: { local: { get: async () => (storedOrigin ? { panelOrigin: storedOrigin } : {}) } },
    tabs: {
      query: async (q) => (q && q.url ? tabs.filter((t) => t.url.startsWith(q.url.replace("/*", ""))) : tabs),
      sendMessage: (id, msg, opts) => {
        sent.push({ id, msg, opts });
        return Promise.resolve();
      },
    },
    cookies: {
      set: async (details) => {
        cookiesSet.push(details);
        return details;
      },
    },
    declarativeNetRequest: {
      updateDynamicRules: async (spec) => {
        dnrRules.push(spec);
      },
    },
    contentSettings: {
      cookies: {
        set: () => {},
      },
    },
    webRequest: {
      onHeadersReceived: {
        addListener: (fn) => {
          headersListener = fn;
        },
      },
    },
    runtime: {
      onMessage: { addListener: (fn) => (listener = fn) },
      onInstalled: { addListener: () => {} },
      onStartup: { addListener: () => {} },
    },
  };
  vm.runInNewContext(readFileSync(new URL("../public/background.js", import.meta.url), "utf8"), { chrome, URL, console, fetch: fetchImpl, AbortController, setTimeout, clearTimeout });
  const send = (msg, tab, frameId = 0, url) =>
    new Promise((resolve) => {
      const kept = listener(msg, { tab, frameId, url }, resolve);
      if (!kept) setTimeout(resolve, 20);
    });
  return { send, sent, tabs, cookiesSet, dnrRules, fetchCalls, onHeadersReceived: (d) => headersListener(d) };
}

test("a page that is not the panel cannot set the call state", async () => {
  const w = load();
  await w.send({ to: "sw", type: "state", state: { status: "incoming", peer: "05550000000" } }, w.tabs[1]);
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.sent.length, 0);
  const res = await w.send({ to: "sw", type: "getState" }, w.tabs[2]);
  assert.equal(res.state.status, "idle");
});

test("the panel's state reaches every tab, trimmed to known fields", async () => {
  const w = load();
  await w.send({ to: "sw", type: "state", state: { status: "in-call", peer: "05551234567", evil: "<script>" } }, w.tabs[0]);
  await new Promise((r) => setTimeout(r, 20));
  const states = w.sent.filter((s) => s.msg.type === "state");
  assert.equal(states.length, 3);
  assert.equal(states[0].msg.state.peer, "05551234567");
  assert.equal("evil" in states[0].msg.state, false);
});

test("an unknown status is replaced by idle", async () => {
  const w = load();
  await w.send({ to: "sw", type: "state", state: { status: "pwned" } }, w.tabs[0]);
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.sent.find((s) => s.msg.type === "state").msg.state.status, "idle");
});

test("widget commands go to the panel tab only", async () => {
  const w = load();
  await w.send({ to: "sw", type: "cmd", cmd: "hangup" }, w.tabs[2]);
  await new Promise((r) => setTimeout(r, 20));
  assert.deepEqual(w.sent.map((s) => s.id), [1]);
});

test("unknown commands are dropped", async () => {
  const w = load();
  await w.send({ to: "sw", type: "cmd", cmd: "rm -rf" }, w.tabs[2]);
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.sent.length, 0);
});

test("the panel address comes from the popup setting", async () => {
  const w = load("https://news.example");
  await w.send({ to: "sw", type: "cmd", cmd: "mute" }, w.tabs[1]);
  await new Promise((r) => setTimeout(r, 20));
  assert.deepEqual(w.sent.map((s) => s.id), [3]);
});

test("the panel's right to call reaches the widgets", async () => {
  const w = load();
  await w.send({ to: "sw", type: "state", state: { status: "registered", canCall: false } }, w.tabs[0]);
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.sent.find((s) => s.msg.type === "state").msg.state.canCall, false);
  const res = await w.send({ to: "sw", type: "getState" }, w.tabs[2]);
  assert.equal(res.state.canCall, false);
});

test("installs declarativeNetRequest rule to strip X-Frame-Options and CSP on sub_frames", () => {
  const w = load();
  assert.equal(w.dnrRules.length, 1);
  const rule = w.dnrRules[0].addRules[0];
  assert.deepEqual([...rule.condition.resourceTypes], ["sub_frame"]);
  const removedHeaders = rule.action.responseHeaders.map((h) => h.header);
  assert.equal(removedHeaders.includes("x-frame-options"), true);
  assert.equal(removedHeaders.includes("content-security-policy"), true);
});

test("upgrades Set-Cookie inside iframes (frameId > 0) to SameSite=None; Secure and ignores top frame", async () => {
  const w = load();

  // Top frame (frameId = 0) should be untouched
  w.onHeadersReceived({
    tabId: 1,
    frameId: 0,
    type: "main_frame",
    method: "GET",
    statusCode: 200,
    url: "https://portal.example.com/login",
    responseHeaders: [{ name: "Set-Cookie", value: "portal_session=top123; path=/; httponly" }],
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.cookiesSet.length, 0);

  // Sub-frame (frameId = 42) should be upgraded to SameSite=None; Secure
  w.onHeadersReceived({
    tabId: 1,
    frameId: 42,
    type: "xmlhttprequest",
    method: "POST",
    statusCode: 200,
    url: "https://chat.example.com/login",
    responseHeaders: [
      { name: "Set-Cookie", value: "ChatSession=abc999; Domain=.chat.example.com; Path=/; HttpOnly; Max-Age=7200" },
    ],
  });
  await new Promise((r) => setTimeout(r, 20));
  assert.equal(w.cookiesSet.length, 1);
  assert.equal(w.cookiesSet[0].name, "ChatSession");
  assert.equal(w.cookiesSet[0].value, "abc999");
  assert.equal(w.cookiesSet[0].domain, "chat.example.com");
  assert.equal(w.cookiesSet[0].sameSite, "no_restriction");
  assert.equal(w.cookiesSet[0].secure, true);
  assert.equal(w.cookiesSet[0].httpOnly, true);
});

test("protects POST login cookies against 302 redirect race and recovers frame navigation", async () => {
  const w = load();

  // 1. User submits login form inside iframe (frameId = 7): POST /login -> 302 to https://portal.example.com/
  w.onHeadersReceived({
    tabId: 1,
    frameId: 7,
    type: "sub_frame",
    method: "POST",
    statusCode: 302,
    url: "https://portal.example.com/login",
    responseHeaders: [
      { name: "Set-Cookie", value: "XSRF-TOKEN=auth_xsrf; Max-Age=7200; path=/" },
      { name: "Set-Cookie", value: "portal_session=S2_AUTH; Max-Age=7200; path=/; httponly" },
      { name: "Location", value: "https://portal.example.com/" },
    ],
  });

  // 2. Chrome's native redirect follower sends GET https://portal.example.com/ before async cookie set
  //    is seen by the request, so server responds with 302 -> /login and a guest cookie S3_GUEST
  w.onHeadersReceived({
    tabId: 1,
    frameId: 7,
    type: "sub_frame",
    method: "GET",
    statusCode: 302,
    url: "https://portal.example.com/",
    responseHeaders: [
      { name: "Set-Cookie", value: "portal_session=S3_GUEST; Max-Age=7200; path=/; httponly" },
      { name: "Location", value: "https://portal.example.com/login" },
    ],
  });

  // 3. The bounced GET /login page finishes loading and tries to set S4_GUEST
  w.onHeadersReceived({
    tabId: 1,
    frameId: 7,
    type: "sub_frame",
    method: "GET",
    statusCode: 200,
    url: "https://portal.example.com/login",
    responseHeaders: [
      { name: "Set-Cookie", value: "portal_session=S4_GUEST; Max-Age=7200; path=/; httponly" },
    ],
  });

  await new Promise((r) => setTimeout(r, 30));

  // Verify S3_GUEST and S4_GUEST never overwrote S2_AUTH!
  const sessionWrites = w.cookiesSet.filter((c) => c.name === "portal_session");
  assert.equal(sessionWrites.length > 0, true);
  for (const write of sessionWrites) {
    assert.equal(write.value, "S2_AUTH");
    assert.equal(write.sameSite, "no_restriction");
    assert.equal(write.secure, true);
  }

  // Verify content.js inside the iframe asking checkFrameRedirect receives the post-login URL
  const redir = await w.send(
    { to: "sw", type: "checkFrameRedirect", href: "https://portal.example.com/login" },
    w.tabs[0],
    7,
    "https://portal.example.com/login",
  );
  assert.equal(redir.redirectUrl, "https://portal.example.com/");

  // Subsequent checkFrameRedirect after consuming returns null (no infinite loop)
  const secondRedir = await w.send(
    { to: "sw", type: "checkFrameRedirect", href: "https://portal.example.com/" },
    w.tabs[0],
    7,
    "https://portal.example.com/",
  );
  assert.equal(secondRedir.redirectUrl, null);
});

test("supports multi-step login to OTP screen even when POST redirects back to /login with OTP session cookie", async () => {
  const w = load();

  // Step 1: POST /login validates credentials, sends SMS OTP, sets OTP session cookie, and redirects to /login (or /otp)
  w.onHeadersReceived({
    tabId: 1,
    frameId: 9,
    type: "xmlhttprequest",
    method: "POST",
    statusCode: 302,
    url: "https://portal.example.com/login",
    responseHeaders: [
      { name: "Set-Cookie", value: "portal_session=S2_OTP_STEP; Max-Age=7200; path=/; httponly" },
      { name: "Location", value: "https://portal.example.com/login" },
    ],
  });

  const reply = await w.send(
    { to: "sw", type: "completeFormPost", href: "https://portal.example.com/login", actionUrl: "https://portal.example.com/login" },
    w.tabs[0],
    9,
    "https://portal.example.com/login",
  );

  // Verify the OTP session cookie was committed as SameSite=None; Secure before completeFormPost resolved
  const otpCookie = w.cookiesSet.find((c) => c.value === "S2_OTP_STEP");
  assert.equal(Boolean(otpCookie), true);
  assert.equal(otpCookie.sameSite, "no_restriction");
  assert.equal(otpCookie.secure, true);

  // Verify completeFormPost returns the redirect target (/login) so the iframe loads the OTP entry screen
  assert.equal(reply.redirectUrl, "https://portal.example.com/login");
});

test("only the panel tab may execute apiRequest relay calls", async () => {
  const w = load();

  const denied = await w.send(
    { to: "sw", type: "apiRequest", req: { method: "GET", url: "https://customer.example/ping" } },
    w.tabs[1],
  );
  assert.equal(denied.ok, false);
  assert.equal(denied.code, "unauthorized_origin");
  assert.equal(w.fetchCalls.length, 0);

  const badProto = await w.send(
    { to: "sw", type: "apiRequest", req: { method: "GET", url: "file:///etc/passwd" } },
    w.tabs[0],
  );
  assert.equal(badProto.ok, false);

  const ok = await w.send(
    {
      to: "sw",
      type: "apiRequest",
      req: {
        method: "POST",
        url: "https://customer.example/orders",
        headers: [{ key: "Authorization", value: "Bearer tok" }],
        body: '{"id":42}',
      },
    },
    w.tabs[0],
  );
  assert.equal(ok.ok, true);
  assert.equal(ok.statusCode, 200);
  assert.equal(ok.body, '{"ok":true}');
  assert.equal(w.fetchCalls.length, 1);
  assert.equal(w.fetchCalls[0].init.method, "POST");
  assert.equal(w.fetchCalls[0].init.headers.Authorization, "Bearer tok");
});

