// Runs the service worker with a stand-in for the chrome API and checks
// that only the panel's tab is believed and that commands reach only it.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const PANEL = "https://cm.toprakgureli.com";

function load(storedOrigin) {
  const sent = [];
  let listener;
  const tabs = [
    { id: 1, url: PANEL + "/dashboard" },
    { id: 2, url: "https://evil.example/page" },
    { id: 3, url: "https://news.example/" },
  ];
  const chrome = {
    storage: { local: { get: async () => (storedOrigin ? { panelOrigin: storedOrigin } : {}) } },
    tabs: {
      query: async (q) => (q && q.url ? tabs.filter((t) => t.url.startsWith(q.url.replace("/*", ""))) : tabs),
      sendMessage: (id, msg) => {
        sent.push({ id, msg });
        return Promise.resolve();
      },
    },
    runtime: { onMessage: { addListener: (fn) => (listener = fn) } },
  };
  vm.runInNewContext(readFileSync(new URL("../public/background.js", import.meta.url), "utf8"), { chrome, URL, console });
  const send = (msg, tab) =>
    new Promise((resolve) => {
      const kept = listener(msg, { tab }, resolve);
      if (!kept) setTimeout(resolve, 20);
    });
  return { send, sent, tabs };
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
