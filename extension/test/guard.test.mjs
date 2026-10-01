// Runs the widget's press guard against a stand-in page and checks that a
// press acts only while the widget is plainly visible under the pointer.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

function page() {
  let clock = 1_000;
  const html = { parentElement: null, style: {}, css: {} };
  const host = {
    isConnected: true,
    parentElement: html,
    style: { display: "" },
    css: {},
    rect: { left: 900, top: 600, width: 260, height: 120 },
    getBoundingClientRect() {
      return this.rect;
    },
  };
  const style = (el) => ({ display: "block", visibility: "visible", opacity: "1", filter: "none", clipPath: "none", transform: "none", ...el.css });
  // What sits on top at a point: the host inside its box, unless an overlay was laid over it.
  let overlay = null;
  const document = {
    elementFromPoint(x, y) {
      const r = host.rect;
      const inside = x >= r.left && x <= r.left + r.width && y >= r.top && y <= r.top + r.height;
      if (!inside) return html;
      return overlay ?? host;
    },
  };
  const window = { getComputedStyle: style, setInterval: () => 0 };
  const sandbox = { globalThis: {} };
  sandbox.globalThis = sandbox;
  vm.runInNewContext(readFileSync(new URL("../public/guard.js", import.meta.url), "utf8"), sandbox);
  const guard = sandbox.santralcGuard.createGuard(host, { window, document, now: () => clock });
  return {
    guard,
    host,
    html,
    later: (ms) => (clock += ms),
    cover: (el) => (overlay = el),
    click: (x = 1000, y = 650) => ({ clientX: x, clientY: y, detail: 1 }),
    settle() {
      guard.tick();
      clock += sandbox.santralcGuard.SETTLE_MS + 1;
    },
  };
}

test("a press on a widget that has been plainly shown acts", () => {
  const p = page();
  p.settle();
  assert.equal(p.guard.allows(p.click()), true);
});

test("a widget that only just appeared does not act yet", () => {
  const p = page();
  p.guard.tick();
  p.later(100);
  assert.equal(p.guard.allows(p.click()), false);
});

test("a widget the page made transparent does not act", () => {
  const p = page();
  p.settle();
  p.host.css.opacity = "0";
  assert.equal(p.guard.allows(p.click()), false);
  p.host.css.opacity = "0.98";
  assert.equal(p.guard.allows(p.click()), false);
});

test("fading the whole page or filtering it counts too", () => {
  const p = page();
  p.settle();
  p.html.css.opacity = "0.1";
  assert.equal(p.guard.allows(p.click()), false);
  p.html.css = { filter: "opacity(0)" };
  assert.equal(p.guard.allows(p.click()), false);
});

test("hidden, clipped, shrunk or transformed widgets do not act", () => {
  for (const css of [{ visibility: "hidden" }, { clipPath: "inset(50%)" }, { transform: "scale(0.01)" }, { display: "none" }]) {
    const p = page();
    p.settle();
    p.host.css = css;
    assert.equal(p.guard.allows(p.click()), false, JSON.stringify(css));
  }
  const p = page();
  p.settle();
  p.host.rect = { left: 900, top: 600, width: 2, height: 2 };
  assert.equal(p.guard.allows(p.click(901, 601)), false);
});

test("a widget shown again must settle again", () => {
  const p = page();
  p.settle();
  p.host.css.opacity = "0";
  p.guard.tick();
  p.host.css.opacity = "1";
  // The page shows it for the instant of the click.
  assert.equal(p.guard.allows(p.click()), false);
  p.later(600);
  assert.equal(p.guard.allows(p.click()), true);
});

test("a press under something laid over the widget does not act", () => {
  const p = page();
  p.settle();
  p.cover({ id: "decoy" });
  assert.equal(p.guard.allows(p.click()), false);
});

test("a press outside the widget's box does not act", () => {
  const p = page();
  p.settle();
  assert.equal(p.guard.allows(p.click(10, 10)), false);
});

test("a key press is checked at the middle of the button", () => {
  const p = page();
  p.settle();
  const button = { getBoundingClientRect: () => ({ left: 1000, top: 640, width: 40, height: 40 }) };
  assert.equal(p.guard.allows({ key: "Enter", currentTarget: button }), true);
  p.cover({ id: "decoy" });
  assert.equal(p.guard.allows({ key: "Enter", currentTarget: button }), false);
});

test("a widget the extension hid itself never acts", () => {
  const p = page();
  p.settle();
  p.host.style.display = "none";
  assert.equal(p.guard.allows(p.click()), false);
});
