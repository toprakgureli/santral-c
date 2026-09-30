import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { ApiError, api, SESSION_ENDED } from "../api/client";
import type { User } from "../api/types";

interface AuthState {
  user: User | null;
  loading: boolean;
  setUser: (u: User | null) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  can: (permission: string) => boolean;
}

const Ctx = createContext<AuthState | undefined>(undefined);

// How often an open panel re-reads the signed-in user, so a role change shows
// up and an ended session is noticed even while nothing else is requested.
const RECHECK_MS = 60_000;

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
    } catch {
      setUser(null);
    }
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } finally {
      setUser(null);
    }
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
  }, [refresh]);

  // The API client reports a session the server rejected and could not renew;
  // dropping the user unmounts the panel, which unregisters the softphone and
  // closes the live streams.
  useEffect(() => {
    const ended = () => setUser(null);
    window.addEventListener(SESSION_ENDED, ended);
    return () => window.removeEventListener(SESSION_ENDED, ended);
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
        if (!cancelled && e instanceof ApiError && e.status === 401) setUser(null);
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
    () => ({ user, loading, setUser, refresh, logout, can }),
    [user, loading, refresh, logout, can],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

// AuthMockProvider signs in a fixed user, for the development preview.
export function AuthMockProvider({ user, children }: { user: User; children: ReactNode }) {
  const value = useMemo<AuthState>(
    () => ({ user, loading: false, setUser: () => undefined, refresh: async () => undefined, logout: async () => undefined, can: (p) => user.permissions.includes("*") || user.permissions.includes(p) }),
    [user],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
