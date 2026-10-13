import { test } from "node:test";
import assert from "node:assert/strict";
import { WebSocket } from "ws";
import { BridgeServer, isAllowedOrigin } from "../src/bridgeServer.js";

test("isAllowedOrigin accepts panel origin and local dev", () => {
  assert.equal(isAllowedOrigin("https://panel.example.com", ["https://panel.example.com"]), true);
  assert.equal(isAllowedOrigin("http://localhost:5173"), true);
  assert.equal(isAllowedOrigin("http://127.0.0.1:8090"), true);
  assert.equal(isAllowedOrigin("chrome-extension://abcdefghijklmnop"), true);
  assert.equal(isAllowedOrigin("https://evil.attacker.com", ["https://panel.example.com"]), false);
  assert.equal(isAllowedOrigin("http://insecure-http.com"), false);
  assert.equal(isAllowedOrigin(""), false);
  assert.equal(isAllowedOrigin(null), false);
});

test("server rejects unauthorized origins on websocket upgrade", async () => {
  const server = new BridgeServer({ port: 48991, allowedOrigins: ["https://panel.example.com"] });
  await server.start();

  try {
    await new Promise((resolve) => {
      const ws = new WebSocket("ws://127.0.0.1:48991", {
        headers: { Origin: "https://evil.attacker.com" },
      });
      ws.on("error", (err) => {
        assert.match(err.message, /403/);
        resolve();
      });
      ws.on("open", () => {
        assert.fail("Should not open connection for evil origin");
      });
    });
  } finally {
    await server.stop();
  }
});

test("server connects with valid origin, relays state and commands", async () => {
  const server = new BridgeServer({ port: 48992, allowedOrigins: ["https://panel.example.com"] });
  await server.start();

  try {
    const receivedCommands = [];
    const ws = new WebSocket("ws://127.0.0.1:48992", {
      headers: { Origin: "https://panel.example.com" },
    });

    await new Promise((resolve, reject) => {
      ws.on("open", resolve);
      ws.on("error", reject);
    });

    ws.on("message", (raw) => {
      receivedCommands.push(JSON.parse(raw.toString("utf8")));
    });

    // 1. Panel sends state
    ws.send(
      JSON.stringify({
        type: "state",
        state: {
          status: "in-call",
          peer: "05321112233",
          callStartedAt: 1728000000,
          answeredAt: 1728000010,
        },
      })
    );

    await new Promise((r) => setTimeout(r, 40));

    const s = server.getState();
    assert.equal(s.status, "in-call");
    assert.equal(s.peer, "05321112233");

    // 2. Desktop sends command to panel
    const sent = server.sendCommand("mute");
    assert.equal(sent, true);

    await new Promise((r) => setTimeout(r, 40));

    assert.equal(receivedCommands.length, 1);
    assert.equal(receivedCommands[0].type, "cmd");
    assert.equal(receivedCommands[0].cmd, "mute");

    // 3. Unauthorized command is rejected
    const bad = server.sendCommand("malicious_command");
    assert.equal(bad, false);

    ws.close();
  } finally {
    await server.stop();
  }
});
