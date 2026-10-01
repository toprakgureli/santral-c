// The API client's session handling: an expired access token is renewed
// once and the request retried, many requests and several tabs share one
// renewal, a network problem never signs anyone out, and only a session the
// server says is over sends the panel back to sign-in.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type Client = typeof import("./client");
type Answer = { status: number; body?: unknown } | "network-error";
type Handler = (path: string, method: string) => Answer | Promise<Answer>;

const BASE = "/api/v1";

// A stand-in server: every fetch is routed to handler, and each call is kept.
function server(handler: Handler) {
  const calls: { path: string; method: string }[] = [];
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    const path = url.startsWith(BASE) ? url.slice(BASE.length) : url;
    const method = init?.method ?? "GET";
    calls.push({ path, method });
    const a = await handler(path, method);
    if (a === "network-error") throw new TypeError("Failed to fetch");
    const text = a.body === undefined ? null : JSON.stringify(a.body);
    return new Response(a.status === 204 ? null : text, { status: a.status });
  });
  vi.stubGlobal("fetch", fetch);
  return {
    calls,
    refreshes: () => calls.filter((c) => c.path === "/auth/refresh").length,
  };
}

// A browser lock that lets one holder in at a time, shared by every tab of
// the test the way the real one is shared by every tab of a browser.
function installLocks() {
  let chain: Promise<unknown> = Promise.resolve();
  const request = vi.fn((_name: string, cb: () => Promise<unknown>) => {
    const turn = chain.then(() => cb());
    chain = turn.catch(() => undefined);
    return turn;
  });
  Object.defineProperty(navigator, "locks", { value: { request }, configurable: true });
  return request;
}

function removeLocks() {
  Object.defineProperty(navigator, "locks", { value: undefined, configurable: true });
}

// openTab loads a fresh copy of the client, as a new browser tab would.
async function openTab(): Promise<Client> {
  vi.resetModules();
  return import("./client");
}

function watchSessionEnd(c: Client) {
  const ended = vi.fn();
  window.addEventListener(c.SESSION_ENDED, ended);
  return ended;
}

const unauthorized = { status: 401, body: { code: "UNAUTHORIZED", message: "Oturumun kapandı." } };

describe("session renewal", () => {
  let lock: ReturnType<typeof installLocks>;

  beforeEach(() => {
    localStorage.clear();
    lock = installLocks();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    removeLocks();
  });

  it("renews an expired token once and retries the request", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    let renewed = false;
    const s = server((path, method) => {
      if (path === "/auth/refresh" && method === "POST") {
        renewed = true;
        return { status: 200, body: {} };
      }
      if (path === "/contacts/7") return renewed ? { status: 200, body: { id: 7 } } : unauthorized;
      return { status: 404 };
    });

    await expect(c.request("/contacts/7")).resolves.toEqual({ id: 7 });
    expect(s.calls.map((x) => `${x.method} ${x.path}`)).toEqual(["GET /contacts/7", "POST /auth/refresh", "GET /contacts/7"]);
    expect(lock).toHaveBeenCalledTimes(1);
    expect(lock.mock.calls[0][0]).toBe("santral-refresh");
    expect(ended).not.toHaveBeenCalled();
  });

  it("sends one renewal for a burst of expired requests in one tab", async () => {
    const c = await openTab();
    let renewed = false;
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const s = server(async (path) => {
      if (path === "/auth/refresh") {
        // Hold the renewal until every request has asked for it.
        await gate;
        renewed = true;
        return { status: 200, body: {} };
      }
      return renewed ? { status: 200, body: { path } } : unauthorized;
    });

    // A busy panel: sixty requests in flight when the token runs out.
    const all = Promise.all(Array.from({ length: 60 }, (_, i) => c.request<{ path: string }>(`/calls/log/lookup?n=${i}`)));
    await vi.waitFor(() => expect(s.calls.filter((x) => x.path !== "/auth/refresh").length).toBe(60));
    release();
    const out = await all;

    expect(out).toHaveLength(60);
    expect(out[59]).toEqual({ path: "/calls/log/lookup?n=59" });
    expect(s.refreshes()).toBe(1);
    expect(lock).toHaveBeenCalledTimes(1);
  });

  it("lets a second tab reuse the renewal the first tab just made", async () => {
    const a = await openTab();
    const b = await openTab();
    expect(a).not.toBe(b);
    let renewed = false;
    const s = server((path) => {
      if (path === "/auth/refresh") {
        renewed = true;
        return { status: 200, body: {} };
      }
      return renewed ? { status: 200, body: { ok: true } } : unauthorized;
    });

    const [ra, rb] = await Promise.all([a.request("/teams/overview"), b.request("/teams/overview")]);

    expect(ra).toEqual({ ok: true });
    expect(rb).toEqual({ ok: true });
    // Both tabs took their turn at the lock, but only the first one asked
    // the server; the second saw the fresh renewal and only retried.
    expect(lock).toHaveBeenCalledTimes(2);
    expect(s.refreshes()).toBe(1);
  });

  it("renews again once the last renewal is no longer fresh", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-01T09:00:00Z"));
    const c = await openTab();
    let fresh = false;
    const s = server((path) => {
      if (path === "/auth/refresh") {
        fresh = true;
        return { status: 200, body: {} };
      }
      return fresh ? { status: 200, body: {} } : unauthorized;
    });

    await c.request("/shift/");
    expect(s.refreshes()).toBe(1);

    // The token runs out again long after: the finished renewal is not
    // handed out a second time.
    fresh = false;
    vi.setSystemTime(new Date("2026-10-01T09:00:20Z"));
    await c.request("/shift/");
    expect(s.refreshes()).toBe(2);
  });

  it("still renews in a browser without the lock", async () => {
    removeLocks();
    const c = await openTab();
    let renewed = false;
    const s = server((path) => {
      if (path === "/auth/refresh") {
        renewed = true;
        return { status: 200, body: {} };
      }
      return renewed ? { status: 200, body: { id: 1 } } : unauthorized;
    });

    await expect(c.request("/users/1")).resolves.toEqual({ id: 1 });
    expect(s.refreshes()).toBe(1);
  });

  it("keeps the user signed in when the renewal cannot reach the server", async () => {
    vi.useFakeTimers();
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server((path) => (path === "/auth/refresh" ? "network-error" : unauthorized));

    const result = c.request("/pbx/stats");
    const check = expect(result).rejects.toMatchObject({ status: 503, code: "UNAVAILABLE" });
    // The renewal is tried four times over about eleven seconds.
    await vi.advanceTimersByTimeAsync(12_000);
    await check;

    expect(s.refreshes()).toBe(4);
    expect(ended).not.toHaveBeenCalled();
  });

  it("keeps the user signed in while the server is overloaded", async () => {
    vi.useFakeTimers();
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server((path) => (path === "/auth/refresh" ? { status: 503 } : unauthorized));

    const result = c.request("/pbx/stats");
    const check = expect(result).rejects.toMatchObject({ status: 503, code: "UNAVAILABLE" });
    await vi.advanceTimersByTimeAsync(12_000);
    await check;

    expect(s.refreshes()).toBe(4);
    expect(ended).not.toHaveBeenCalled();
  });

  it("keeps the user signed in when the request itself hits a network error", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server(() => "network-error");

    await expect(c.request("/calls/log/")).rejects.toBeInstanceOf(TypeError);
    expect(s.refreshes()).toBe(0);
    expect(ended).not.toHaveBeenCalled();
  });

  it("signs out when the server refuses the renewal", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server(() => unauthorized);

    await expect(c.request("/calls/log/")).rejects.toMatchObject({ status: 401, code: "UNAUTHORIZED" });
    // A refused renewal is final: no second try.
    expect(s.refreshes()).toBe(1);
    expect(ended).toHaveBeenCalledTimes(1);
  });

  it("signs out when the request is still refused after a renewal", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server((path) => (path === "/auth/refresh" ? { status: 200, body: {} } : unauthorized));

    await expect(c.request("/users/")).rejects.toMatchObject({ status: 401 });
    expect(s.calls.filter((x) => x.path === "/users/")).toHaveLength(2);
    expect(s.refreshes()).toBe(1);
    expect(ended).toHaveBeenCalledTimes(1);
  });

  it("does not renew or sign out on a wrong password", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server(() => ({ status: 401, body: { code: "INVALID_CREDENTIALS", message: "E-posta ya da şifre yanlış." } }));

    await expect(c.api.login("a@b.c", "yanlis")).rejects.toMatchObject({ status: 401, code: "INVALID_CREDENTIALS" });
    expect(s.refreshes()).toBe(0);
    expect(ended).not.toHaveBeenCalled();
  });
});

describe("ensureSession", () => {
  beforeEach(() => {
    localStorage.clear();
    installLocks();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    removeLocks();
  });

  it("says renewed when the session is still good", async () => {
    const c = await openTab();
    const s = server(() => ({ status: 200, body: { id: 1 } }));
    await expect(c.ensureSession()).resolves.toBe("renewed");
    expect(s.refreshes()).toBe(0);
  });

  it("renews an expired session", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server((path) => (path === "/auth/refresh" ? { status: 200, body: {} } : unauthorized));
    await expect(c.ensureSession()).resolves.toBe("renewed");
    expect(s.refreshes()).toBe(1);
    expect(ended).not.toHaveBeenCalled();
  });

  it("says ended and signs out when the session is over", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    server(() => unauthorized);
    await expect(c.ensureSession()).resolves.toBe("ended");
    expect(ended).toHaveBeenCalledTimes(1);
  });

  it("says unknown, without renewing, when the server cannot be reached", async () => {
    const c = await openTab();
    const ended = watchSessionEnd(c);
    const s = server(() => "network-error");
    await expect(c.ensureSession()).resolves.toBe("unknown");
    expect(s.refreshes()).toBe(0);
    expect(ended).not.toHaveBeenCalled();
  });

  it("says unknown, without renewing, when the server answers with an error", async () => {
    const c = await openTab();
    const s = server(() => ({ status: 502 }));
    await expect(c.ensureSession()).resolves.toBe("unknown");
    expect(s.refreshes()).toBe(0);
  });

  it("says unknown when the renewal itself cannot reach the server", async () => {
    vi.useFakeTimers();
    const c = await openTab();
    const ended = watchSessionEnd(c);
    server((path) => (path === "/auth/refresh" ? "network-error" : unauthorized));
    const result = c.ensureSession();
    await vi.advanceTimersByTimeAsync(12_000);
    await expect(result).resolves.toBe("unknown");
    expect(ended).not.toHaveBeenCalled();
  });
});
