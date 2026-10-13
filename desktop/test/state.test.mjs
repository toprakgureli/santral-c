import { test } from "node:test";
import assert from "node:assert/strict";
import { cleanState, normalizeDial } from "../src/state.js";

test("cleanState sanitizes malicious fields and preserves valid state", () => {
  const input = {
    status: "calling",
    peer: "05321112233" + "x".repeat(100),
    muted: true,
    held: false,
    extension: "101",
    callStartedAt: 1728000000,
    answeredAt: 1728000010,
    canCall: true,
    hacked: "<script>alert(1)</script>",
  };

  const output = cleanState(input);
  assert.equal(output.status, "calling");
  assert.equal(output.peer.length, 40); // truncated
  assert.equal(output.muted, true);
  assert.equal(output.extension, "101");
  assert.equal("hacked" in output, false);
});

test("cleanState falls back to idle for unknown status", () => {
  assert.equal(cleanState({ status: "invalid_status" }).status, "idle");
  assert.equal(cleanState(null).status, "idle");
  assert.equal(cleanState(undefined).status, "idle");
});

test("normalizeDial formats numbers properly", () => {
  assert.equal(normalizeDial("05321112233"), "05321112233");
  assert.equal(normalizeDial("+90 532 111 22 33"), "05321112233");
  assert.equal(normalizeDial("905321112233"), "05321112233");
  assert.equal(normalizeDial("5321112233"), "05321112233");
  assert.equal(normalizeDial("101"), "101"); // internal extension
  assert.equal(normalizeDial("*98"), "*98"); // feature code
  assert.equal(normalizeDial(""), "");
});
