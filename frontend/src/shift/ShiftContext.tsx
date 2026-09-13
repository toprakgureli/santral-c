// Shift state shared by the topbar button and the softphone: the dialer stays
// closed until the agent starts a shift, and the server closes a forgotten
// shift at 19:20, so the panel re-reads the state periodically.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api, ApiError } from "@/api/client";
import type { ShiftStatus } from "@/api/types";

interface ShiftState {
  status: ShiftStatus | null;
  loading: boolean;
  busy: boolean;
  error: string | null;
  active: boolean;
  start: () => Promise<void>;
  end: () => Promise<void>;
  refresh: () => Promise<void>;
}

const Ctx = createContext<ShiftState | undefined>(undefined);

const POLL_MS = 60000;

export function ShiftProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<ShiftStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setStatus(await api.shiftStatus());
    } catch {
      // keep the last known state; the next poll retries
    }
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
    const timer = window.setInterval(refresh, POLL_MS);
    const onFocus = () => { void refresh(); };
    window.addEventListener("focus", onFocus);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener("focus", onFocus);
    };
  }, [refresh]);

  const run = useCallback(async (call: () => Promise<ShiftStatus>) => {
    setBusy(true);
    setError(null);
    try {
      setStatus(await call());
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem tamamlanamadı.");
      await refresh();
    } finally {
      setBusy(false);
    }
  }, [refresh]);

  const start = useCallback(() => run(api.startShift), [run]);
  const end = useCallback(() => run(api.endShift), [run]);

  const value = useMemo<ShiftState>(
    () => ({ status, loading, busy, error, active: !!status?.shift, start, end, refresh }),
    [status, loading, busy, error, start, end, refresh],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useShift(): ShiftState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useShift must be used within ShiftProvider");
  return ctx;
}
