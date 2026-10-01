// Start-up while the server is briefly away (a deploy): the panel waits and
// keeps trying instead of showing the sign-in page; only a refused session
// signs out.
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({ me: vi.fn(), logout: vi.fn() }));
vi.mock("../api/client", () => {
  class ApiError extends Error {
    status: number;
    code: string;
    constructor(status: number, code: string, message: string) {
      super(message);
      this.status = status;
      this.code = code;
    }
  }
  return { api: apiMock, ApiError, SESSION_ENDED: "santral:session-ended" };
});

import { ApiError } from "../api/client";
import { AuthProvider, LogoutFailed, useAuth } from "./AuthContext";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type Auth = ReturnType<typeof useAuth>;

function mount() {
  const ref: { current: Auth } = { current: undefined as unknown as Auth };
  const seen: string[] = [];
  function Probe() {
    ref.current = useAuth();
    const a = ref.current;
    seen.push(a.loading ? (a.waiting ? "waiting" : "loading") : a.user ? "signed-in" : "signed-out");
    return null;
  }
  const root = createRoot(document.createElement("div"));
  act(() =>
    root.render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    ),
  );
  return { auth: ref, seen, unmount: () => act(() => root.unmount()) };
}

const user = { id: 3, name: "Ayşe", email: "a@example.com", permissions: [] };

let mounted: ReturnType<typeof mount> | null = null;
beforeEach(() => {
  vi.useFakeTimers();
  apiMock.me.mockReset();
});
afterEach(() => {
  mounted?.unmount();
  mounted = null;
  vi.useRealTimers();
});

describe("start-up", () => {
  it("waits through a deploy and signs in once the server answers", async () => {
    apiMock.me
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(new ApiError(502, "BAD_GATEWAY", "Bad gateway"))
      .mockRejectedValueOnce(new ApiError(503, "UNAVAILABLE", "Sunucuya şu an ulaşılamıyor."))
      .mockResolvedValue(user);
    mounted = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.loading).toBe(true);
    expect(mounted.auth.current.waiting).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000 + 2000 + 4000 + 100);
    });
    expect(apiMock.me).toHaveBeenCalledTimes(4);
    expect(mounted.auth.current.user?.id).toBe(3);
    expect(mounted.auth.current.waiting).toBe(false);
    // The sign-in page never showed in between.
    expect(mounted.seen).not.toContain("signed-out");
  });

  it("tries again at once when asked", async () => {
    apiMock.me.mockRejectedValueOnce(new TypeError("Failed to fetch")).mockResolvedValue(user);
    mounted = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.waiting).toBe(true);
    await act(async () => {
      mounted!.auth.current.retryNow();
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.user?.id).toBe(3);
  });

  it("shows the sign-in page when the session is really over", async () => {
    apiMock.me.mockRejectedValue(new ApiError(401, "UNAUTHORIZED", "Oturum bulunamadı."));
    mounted = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.loading).toBe(false);
    expect(mounted.auth.current.user).toBeNull();
    expect(apiMock.me).toHaveBeenCalledTimes(1);
    expect(mounted.seen).not.toContain("waiting");
  });

  it("a later check that fails for the network keeps the user", async () => {
    apiMock.me.mockResolvedValueOnce(user).mockRejectedValueOnce(new TypeError("Failed to fetch"));
    mounted = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.user?.id).toBe(3);
    await act(async () => {
      await mounted!.auth.current.refresh();
    });
    expect(mounted.auth.current.user?.id).toBe(3);
  });
});

describe("sign-out", () => {
  async function signedIn() {
    apiMock.me.mockResolvedValue(user);
    mounted = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mounted.auth.current.user?.id).toBe(3);
    return mounted;
  }

  beforeEach(() => {
    apiMock.logout.mockReset();
  });

  it("rides out a short outage and then signs out", async () => {
    const m = await signedIn();
    apiMock.logout
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockRejectedValueOnce(new ApiError(502, "BAD_GATEWAY", "Bad gateway"))
      .mockResolvedValue(undefined);
    let done = false;
    await act(async () => {
      const p = m.auth.current.logout().then(() => (done = true));
      await vi.advanceTimersByTimeAsync(5000);
      await p;
    });
    expect(done).toBe(true);
    expect(apiMock.logout).toHaveBeenCalledTimes(3);
    expect(m.auth.current.user).toBeNull();
  });

  it("stays signed in when the server never hears the sign-out", async () => {
    const m = await signedIn();
    apiMock.logout.mockRejectedValue(new TypeError("Failed to fetch"));
    let failure: unknown = null;
    await act(async () => {
      const p = m.auth.current.logout().catch((e) => (failure = e));
      await vi.advanceTimersByTimeAsync(10000);
      await p;
    });
    expect(failure).toBeInstanceOf(LogoutFailed);
    expect(apiMock.logout).toHaveBeenCalledTimes(3);
    // The session is still open on the server, so the screen says so too.
    expect(m.auth.current.user?.id).toBe(3);
  });

  it("an already ended session counts as signed out", async () => {
    const m = await signedIn();
    apiMock.logout.mockRejectedValue(new ApiError(401, "UNAUTHORIZED", "Oturum bulunamadı."));
    await act(async () => {
      await m.auth.current.logout();
    });
    expect(apiMock.logout).toHaveBeenCalledTimes(1);
    expect(m.auth.current.user).toBeNull();
  });
});
