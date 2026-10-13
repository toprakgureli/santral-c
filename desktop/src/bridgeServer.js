import http from "node:http";
import { WebSocketServer, WebSocket } from "ws";
import { cleanState, COMMANDS } from "./state.js";

export const DEFAULT_PORT = 48123;

export function getEnvOrigins() {
  const val = process.env.SANTRAL_PANEL_ORIGINS || process.env.SANTRAL_PANEL_URL || "";
  return val
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => {
      try {
        return new URL(s).origin;
      } catch {
        return s;
      }
    });
}

export function isAllowedOrigin(origin, extraOrigins = []) {
  if (!origin) return false;
  try {
    const u = new URL(origin);
    if (u.hostname === "localhost" || u.hostname === "127.0.0.1") return true;
    if (u.protocol === "chrome-extension:") return true;

    const extras = Array.isArray(extraOrigins) ? extraOrigins : [extraOrigins];
    const allowed = new Set(
      [...getEnvOrigins(), ...extras.map((s) => {
        try {
          return new URL(s).origin;
        } catch {
          return s;
        }
      })].filter(Boolean)
    );

    if (allowed.size > 0) {
      if (allowed.has("*")) return true;
      return allowed.has(u.origin);
    }

    // Default: allow secure HTTPS origins, reject insecure non-local HTTP
    return u.protocol === "https:";
  } catch {
    return false;
  }
}

export class BridgeServer {
  constructor(options = {}) {
    this.port = options.port || DEFAULT_PORT;
    this.host = "127.0.0.1"; // loopback only
    this.allowedOrigins = options.allowedOrigins || [];
    this.httpServer = null;
    this.wss = null;
    this.panelSocket = null;
    this.lastPingAt = 0;
    this.currentState = { status: "disconnected" };
    this.stateListeners = new Set();
    this.connListeners = new Set();
    this.heartbeatTimer = null;
  }

  start() {
    return new Promise((resolve, reject) => {
      this.httpServer = http.createServer((req, res) => {
        // Status endpoint for health checks
        if (req.url === "/health" || req.url === "/status") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ ok: true, connected: this.isConnected() }));
          return;
        }
        res.writeHead(404);
        res.end();
      });

      this.wss = new WebSocketServer({
        noServer: true,
      });

      this.httpServer.on("upgrade", (req, socket, head) => {
        const origin = req.headers.origin;
        if (!isAllowedOrigin(origin, this.allowedOrigins)) {
          socket.write("HTTP/1.1 403 Forbidden\r\n\r\n");
          socket.destroy();
          return;
        }

        this.wss.handleUpgrade(req, socket, head, (ws) => {
          this.wss.emit("connection", ws, req);
        });
      });

      this.wss.on("connection", (ws, req) => {
        // A new panel connection arrived
        if (this.panelSocket && this.panelSocket.readyState === WebSocket.OPEN) {
          try {
            this.panelSocket.close(1000, "New panel tab connected");
          } catch {
            // ignore
          }
        }
        this.panelSocket = ws;
        this.lastPingAt = Date.now();
        this._emitConnection(true);

        ws.on("message", (raw) => {
          this.lastPingAt = Date.now();
          try {
            const data = JSON.parse(raw.toString("utf8"));
            this._handleMessage(data);
          } catch {
            // ignore malformed JSON
          }
        });

        ws.on("close", () => {
          if (this.panelSocket === ws) {
            this.panelSocket = null;
            this.currentState = { status: "disconnected" };
            this._emitState(this.currentState);
            this._emitConnection(false);
          }
        });

        ws.on("error", () => {
          // handled by close
        });
      });

      this.httpServer.on("error", (err) => {
        reject(err);
      });

      this.httpServer.listen(this.port, this.host, () => {
        this._startHeartbeatCheck();
        resolve(this.httpServer.address());
      });
    });
  }

  _startHeartbeatCheck() {
    if (this.heartbeatTimer) clearInterval(this.heartbeatTimer);
    this.heartbeatTimer = setInterval(() => {
      if (this.panelSocket && Date.now() - this.lastPingAt > 10000) {
        // Panel tab closed or frozen
        try {
          this.panelSocket.terminate();
        } catch {
          // ignore
        }
        this.panelSocket = null;
        this.currentState = { status: "disconnected" };
        this._emitState(this.currentState);
        this._emitConnection(false);
      }
    }, 3000);
  }

  _handleMessage(data) {
    if (!data || typeof data !== "object") return;
    if (data.type === "hello" || data.type === "alive") {
      // Panel alive ping
      return;
    }
    if (data.type === "state" && data.state) {
      this.currentState = cleanState(data.state);
      this._emitState(this.currentState);
    }
  }

  sendCommand(cmd, arg) {
    if (!COMMANDS.has(cmd)) return false;
    if (!this.panelSocket || this.panelSocket.readyState !== WebSocket.OPEN) {
      return false;
    }
    const cleanArg = typeof arg === "string" ? arg.slice(0, 32) : undefined;
    const payload = JSON.stringify({
      type: "cmd",
      cmd,
      arg: cleanArg,
    });
    this.panelSocket.send(payload);
    return true;
  }

  isConnected() {
    return !!(this.panelSocket && this.panelSocket.readyState === WebSocket.OPEN);
  }

  getState() {
    return this.currentState;
  }

  onState(listener) {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  }

  onConnectionChange(listener) {
    this.connListeners.add(listener);
    return () => this.connListeners.delete(listener);
  }

  _emitState(state) {
    for (const fn of this.stateListeners) {
      try {
        fn(state);
      } catch {
        // ignore
      }
    }
  }

  _emitConnection(connected) {
    for (const fn of this.connListeners) {
      try {
        fn(connected);
      } catch {
        // ignore
      }
    }
  }

  stop() {
    return new Promise((resolve) => {
      if (this.heartbeatTimer) {
        clearInterval(this.heartbeatTimer);
        this.heartbeatTimer = null;
      }
      if (this.panelSocket) {
        try {
          this.panelSocket.terminate();
        } catch {}
        this.panelSocket = null;
      }
      if (this.wss) {
        this.wss.close(() => {
          if (this.httpServer) {
            this.httpServer.close(() => resolve());
          } else {
            resolve();
          }
        });
      } else if (this.httpServer) {
        this.httpServer.close(() => resolve());
      } else {
        resolve();
      }
    });
  }
}
