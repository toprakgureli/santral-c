// Teams state shared by the sidebar badge, the page and the notifier: the
// room list with unread counts, pending invites, and the live event stream.
// A new message in a room that is not open (or in a hidden tab) rings a
// chime and, when allowed, a browser notification; muted rooms stay quiet
// and stay out of the badge.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api } from "@/api/client";
import type { TeamsEvent, TeamsGroup, TeamsInvite, TeamsMessage } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import { can } from "@/lib/permissions";
import { tones } from "@/softphone/tones";

interface TeamsState {
  enabled: boolean;
  groups: TeamsGroup[];
  invites: TeamsInvite[];
  unread: number;
  refresh: () => Promise<void>;
  // The room the page currently shows; its messages do not notify.
  openGroupId: number | null;
  setOpenGroupId: (id: number | null) => void;
  // Live events for the page: each subscriber gets every event.
  subscribe: (fn: (e: TeamsEvent) => void) => () => void;
  // Optimistic local touch-ups so the list reacts before the next refresh.
  bumpGroup: (groupId: number, message: TeamsMessage) => void;
  clearUnread: (groupId: number) => void;
  notifications: NotificationPermission | "unsupported";
  askNotifications: () => Promise<void>;
}

const Ctx = createContext<TeamsState | undefined>(undefined);
const POLL_MS = 30000;

export function TeamsProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const enabled = can(user, "teams.view");
  const [groups, setGroups] = useState<TeamsGroup[]>([]);
  const [invites, setInvites] = useState<TeamsInvite[]>([]);
  const [openGroupId, setOpenGroupId] = useState<number | null>(null);
  const openRef = useRef<number | null>(null);
  openRef.current = openGroupId;
  const groupsRef = useRef<TeamsGroup[]>([]);
  groupsRef.current = groups;
  const listeners = useRef(new Set<(e: TeamsEvent) => void>());
  const [notifications, setNotifications] = useState<NotificationPermission | "unsupported">(() =>
    typeof Notification === "undefined" ? "unsupported" : Notification.permission,
  );

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      const o = await api.teamsOverview();
      setGroups(o.groups);
      setInvites(o.invites);
    } catch {
      // keep the last known state; the next poll retries
    }
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
    const t = window.setInterval(() => void refresh(), POLL_MS);
    return () => window.clearInterval(t);
  }, [enabled, refresh]);

  const notify = useCallback(
    (m: TeamsMessage) => {
      const g = groupsRef.current.find((x) => x.id === m.groupId);
      if (g?.muted || m.mine || m.kind === "system") return;
      const roomOpen = openRef.current === m.groupId && document.visibilityState === "visible" && window.location.pathname.startsWith("/teams");
      if (roomOpen) return;
      tones.notify();
      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        const title = g ? (g.kind === "dm" ? g.name : `${g.name} · ${m.sender?.name ?? ""}`) : m.sender?.name ?? "Teams";
        try {
          const n = new Notification(title, { body: m.body.slice(0, 140), tag: `teams-${m.groupId}`, silent: true });
          n.onclick = () => {
            window.focus();
            window.location.assign(`/teams/${m.groupId}`);
            n.close();
          };
        } catch {
          // notifications unavailable in this context
        }
      }
    },
    [],
  );

  // Live stream with a polling fallback (a proxy may buffer SSE).
  useEffect(() => {
    if (!enabled) return;
    let es: EventSource | null = null;
    let closed = false;
    let retry = 0;
    const connect = () => {
      if (closed) return;
      try {
        es = new EventSource("/api/v1/teams/stream", { withCredentials: true });
      } catch {
        return;
      }
      es.onopen = () => {
        retry = 0;
        void refresh();
      };
      es.onmessage = (ev) => {
        let e: TeamsEvent;
        try {
          e = JSON.parse(ev.data) as TeamsEvent;
        } catch {
          return;
        }
        if (e.type === "message" && e.message) {
          const m = e.message;
          setGroups((list) => {
            const idx = list.findIndex((g) => g.id === m.groupId);
            if (idx === -1) {
              void refresh();
              return list;
            }
            const g = list[idx];
            const isOpen = openRef.current === m.groupId && document.visibilityState === "visible";
            const next: TeamsGroup = { ...g, lastMessage: m, updatedAt: m.createdAt, unread: g.unread + (m.mine || isOpen || m.kind === "system" ? 0 : 1) };
            return [next, ...list.slice(0, idx), ...list.slice(idx + 1)];
          });
          notify(m);
        } else if (e.type === "group" || e.type === "invite") {
          void refresh();
        }
        listeners.current.forEach((fn) => fn(e));
      };
      es.onerror = () => {
        es?.close();
        es = null;
        if (closed) return;
        retry = Math.min(retry + 1, 6);
        window.setTimeout(connect, 1000 * 2 ** retry);
      };
    };
    connect();
    return () => {
      closed = true;
      es?.close();
    };
  }, [enabled, refresh, notify]);

  const subscribe = useCallback((fn: (e: TeamsEvent) => void) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);

  const bumpGroup = useCallback((groupId: number, message: TeamsMessage) => {
    setGroups((list) => {
      const idx = list.findIndex((g) => g.id === groupId);
      if (idx === -1) return list;
      const next = { ...list[idx], lastMessage: message, updatedAt: message.createdAt };
      return [next, ...list.slice(0, idx), ...list.slice(idx + 1)];
    });
  }, []);

  const clearUnread = useCallback((groupId: number) => {
    setGroups((list) => list.map((g) => (g.id === groupId && g.unread ? { ...g, unread: 0 } : g)));
  }, []);

  const askNotifications = useCallback(async () => {
    if (typeof Notification === "undefined") return;
    try {
      const p = await Notification.requestPermission();
      setNotifications(p);
    } catch {
      // ignore
    }
  }, []);

  const unread = useMemo(() => groups.reduce((n, g) => n + (g.muted ? 0 : g.unread), 0), [groups]);

  const value = useMemo<TeamsState>(
    () => ({ enabled, groups, invites, unread, refresh, openGroupId, setOpenGroupId, subscribe, bumpGroup, clearUnread, notifications, askNotifications }),
    [enabled, groups, invites, unread, refresh, openGroupId, subscribe, bumpGroup, clearUnread, notifications, askNotifications],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useTeams(): TeamsState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useTeams must be used within TeamsProvider");
  return ctx;
}
