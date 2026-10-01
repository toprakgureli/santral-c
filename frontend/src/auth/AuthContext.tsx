import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ApiError, api, SESSION_ENDED } from "../api/client";
import type { User } from "../api/types";
import { clearUserStorage, setStorageUser } from "../lib/userStorage";
import { forgetDrafts } from "../teams/drafts";
import { flushPendingCallLogs, settleCallLogs } from "../softphone/callLogQueue";

interface AuthState {
  user: User | null;
  loading: boolean;
  // waiting is true while the panel starts but cannot reach the server (a
  // deploy, a network blip); it keeps trying and the screen says so.
  waiting: boolean;
  // retryNow skips the pause before the next try while waiting.
  retryNow: () => void;
  setUser: (u: User | null) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  can: (permission: string) => boolean;
}

const Ctx = createContext<AuthState | undefined>(undefined);

// openTabs opens the channel the panel's tabs share, where available.
function openTabs(): BroadcastChannel | null {
  try {
    return typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel("santral-auth");
  } catch {
    return null;
  }
}

// tellTabs tells the other tabs that this browser signed out.
function tellTabs() {
  const channel = openTabs();
  if (!channel) return;
  channel.postMessage("signed-out");
  channel.close();
}

// How often an open panel re-reads the signed-in user, so a role change shows
// up and an ended session is noticed even while nothing else is requested.
const RECHECK_MS = 60_000;

// sessionOver reports whether a failure really means nobody is signed in:
// the server refused the session (401) or would not renew it. A network
// error or a 5xx (a deploy, an overloaded proxy) says nothing about the
// session.
export function sessionOver(e: unknown): boolean {
  return e instanceof ApiError && (e.status === 401 || e.status === 403);
}

// Pauses between tries while the server cannot be reached at start-up.
export const START_RETRY_MS = [1000, 2000, 4000, 8000, 15000];

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [waiting, setWaiting] = useState(false);
  const wakeRef = useRef<() => void>(() => undefined);
  // Storage keys follow whoever is signed in; set before children read them.
  setStorageUser(user?.id ?? 0);

  // refresh re-reads the signed-in user once. Only an answer that means the
  // session is over signs out; anything else keeps the current user.
  const refresh = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
    } catch (e) {
      if (sessionOver(e)) setUser(null);
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      // Call ends that could not be sent earlier get one more try while the
      // session still works; sign-out wipes what is left.
      flushPendingCallLogs();
      await settleCallLogs(3000);
    } catch {
      // nothing more to do; the server closes a lost call by itself
    }
    try {
      await api.logout();
    } finally {
      clearUserStorage();
      forgetDrafts();
      setUser(null);
      tellTabs();
    }
  }, []);

  // Start-up: find out who is signed in. A server that cannot answer yet
  // (a deploy, a network blip) is asked again with growing pauses while the
  // screen says the panel is waiting; nobody is sent to the sign-in page
  // for it. Only a refused session shows the sign-in page.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      for (let attempt = 0; !cancelled; attempt++) {
        try {
          const me = await api.me();
          if (cancelled) return;
          setUser(me);
          break;
        } catch (e) {
          if (cancelled) return;
          if (sessionOver(e)) {
            setUser(null);
            break;
          }
          setWaiting(true);
          await new Promise<void>((resolve) => {
            const timer = window.setTimeout(resolve, START_RETRY_MS[Math.min(attempt, START_RETRY_MS.length - 1)]);
            wakeRef.current = () => {
              window.clearTimeout(timer);
              resolve();
            };
          });
        }
      }
      if (!cancelled) {
        setWaiting(false);
        setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
      wakeRef.current();
    };
  }, []);

  const retryNow = useCallback(() => wakeRef.current(), []);

  // The API client reports a session the server rejected and could not renew;
  // dropping the user unmounts the panel, which unregisters the softphone and
  // closes the live streams.
  useEffect(() => {
    const ended = () => {
      clearUserStorage();
      setUser(null);
    };
    window.addEventListener(SESSION_ENDED, ended);
    // Signing out in one tab signs out the others at once, so a hidden tab
    // does not keep the phone ringing.
    const channel = openTabs();
    if (channel) channel.onmessage = (e) => e.data === "signed-out" && ended();
    return () => {
      window.removeEventListener(SESSION_ENDED, ended);
      channel?.close();
    };
  }, []);

  // While signed in, re-read the user now and then and whenever the tab comes
  // back into view. Only a rejected session signs out; a network blip keeps
  // the current user. The user object is replaced only when it changed, so
  // the panel does not re-render every minute.
  const signedIn = user !== null;
  useEffect(() => {
    if (!signedIn) return;
    let cancelled = false;
    const recheck = async () => {
      if (document.visibilityState !== "visible") return;
      try {
        const me = await api.me();
        if (cancelled) return;
        setUser((prev) => (prev && JSON.stringify(prev) === JSON.stringify(me) ? prev : me));
      } catch (e) {
        if (!cancelled && sessionOver(e)) setUser(null);
      }
    };
    const timer = window.setInterval(() => void recheck(), RECHECK_MS);
    const onVisible = () => void recheck();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [signedIn]);

  const can = useCallback((permission: string) => !!user && user.permissions.includes(permission), [user]);

  const value = useMemo<AuthState>(
    () => ({ user, loading, waiting, retryNow, setUser, refresh, logout, can }),
    [user, loading, waiting, retryNow, refresh, logout, can],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

// AuthMockProvider signs in a fixed user, for the development preview.
export function AuthMockProvider({ user, children }: { user: User; children: ReactNode }) {
  const value = useMemo<AuthState>(
    () => ({ user, loading: false, waiting: false, retryNow: () => undefined, setUser: () => undefined, refresh: async () => undefined, logout: async () => undefined, can: (p) => user.permissions.includes("*") || user.permissions.includes(p) }),
    [user],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
