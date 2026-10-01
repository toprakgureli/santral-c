// Presence (available / break / backoffice / dnd) shared by the dashboard
// status bar and the break overlay, so a pause is visible on every page and
// ending it from either place updates both at once.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { api } from "@/api/client";
import type { AgentPresence, AgentPresenceState } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import { useShift } from "@/shift/ShiftContext";

interface PresenceState {
  data: AgentPresence | null;
  // When `data` was read, so live timers can tick between polls.
  fetchedAt: number;
  hasExtension: boolean;
  // pbxPending: the phone system has not confirmed the latest change yet.
  pbxPending: boolean;
  refresh: () => void;
  change: (state: AgentPresenceState) => Promise<void>;
}

const Ctx = createContext<PresenceState | undefined>(undefined);

const POLL_MS = 20000;
// While the phone system has not confirmed a change, look again sooner.
const PENDING_POLL_MS = 3000;

export function PresenceProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const shift = useShift();
  const hasExtension = !!user?.sipExtension;
  const [data, setData] = useState<AgentPresence | null>(null);
  const [fetchedAt, setFetchedAt] = useState(() => Date.now());

  const refresh = useCallback(() => {
    if (!hasExtension) return;
    api
      .getAgentStatus()
      .then((s) => {
        setData(s);
        setFetchedAt(Date.now());
      })
      .catch(() => undefined);
  }, [hasExtension]);

  // Re-read on every shift change too, so "Mesai dışı" clears the moment the
  // agent starts a shift.
  useEffect(() => {
    if (!hasExtension) return;
    refresh();
    const timer = window.setInterval(refresh, POLL_MS);
    return () => window.clearInterval(timer);
  }, [hasExtension, refresh, shift.active]);

  const pbxPending = !!data?.pbxPending;
  useEffect(() => {
    if (!pbxPending) return;
    const timer = window.setInterval(refresh, PENDING_POLL_MS);
    return () => window.clearInterval(timer);
  }, [pbxPending, refresh]);

  // Optimistic: the UI flips at once, the server read that follows settles
  // the exact "since" and the totals.
  const change = useCallback(
    async (state: AgentPresenceState) => {
      const now = new Date().toISOString();
      setData((d) => (d ? { ...d, state, since: now } : { state, since: now, totals: {} }));
      setFetchedAt(Date.now());
      try {
        const res = await api.setAgentStatus(state);
        if (res?.pbxPending) setData((d) => (d ? { ...d, pbxPending: true } : d));
      } finally {
        refresh();
      }
    },
    [refresh],
  );

  const value = useMemo(() => ({ data, fetchedAt, hasExtension, pbxPending, refresh, change }), [data, fetchedAt, hasExtension, pbxPending, refresh, change]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function usePresence(): PresenceState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("usePresence must be used within PresenceProvider");
  return ctx;
}
