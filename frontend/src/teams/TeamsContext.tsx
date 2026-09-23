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
import type { Presence } from "@/components/teams/Presence";
import { gamesApi } from "@/games/api";
import type { GamesConfig } from "@/games/types";
import { previewLabel } from "@/lib/attachments";

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
  // Live presence by user id; falls back to what the server sent with the card.
  presence: Record<number, PresenceInfo>;
  // room: the room the card is shown in, so "here" means "looking at it".
  presenceOf: (p: { id: number; online?: boolean; lastSeen?: string; inRoom?: boolean } | undefined | null, room?: number) => Presence;
  // Tags waiting to be noticed; they stay until dismissed.
  mentions: MentionToast[];
  dismissMention: (id: number) => void;
  // The latest ordinary message, shown briefly; only one at a time.
  toast: MentionToast | null;
  dismissToast: () => void;
  // "Toprak yazıyor..." for a room, or null.
  typingLabel: (groupId: number) => string | null;
  // Mini games: switches and catalogue, null until loaded.
  games: GamesConfig | null;
  reloadGames: () => void;
}

export interface MentionToast {
  id: number;
  groupId: number;
  groupName: string;
  sender: { id: number; name: string; hasAvatar: boolean; avatarVersion?: number };
  body: string;
  at: string;
}

const MENTIONS_KEY = "teams.mentions";

function loadMentions(): MentionToast[] {
  try {
    const raw = localStorage.getItem(MENTIONS_KEY);
    const list = raw ? (JSON.parse(raw) as MentionToast[]) : [];
    return Array.isArray(list) ? list.slice(-20) : [];
  } catch {
    return [];
  }
}

export interface PresenceInfo {
  online: boolean;
  lastSeen?: string;
  // The room the person is looking at, 0 for none, undefined for unknown.
  room?: number;
}

type Typing = Record<number, Record<number, { name: string; until: number }>>;
const TYPING_TTL_MS = 4500;

export function typingText(names: string[]): string | null {
  if (names.length === 0) return null;
  if (names.length === 1) return `${names[0]} yazıyor...`;
  if (names.length <= 3) return `${names.slice(0, -1).join(", ")} ve ${names[names.length - 1]} yazıyor...`;
  return `${names[0]} ve ${names.length - 1} kişi yazıyor...`;
}

const Ctx = createContext<TeamsState | undefined>(undefined);
const POLL_MS = 30000;

export function TeamsProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const enabled = can(user, "teams.view");
  const [groups, setGroups] = useState<TeamsGroup[]>([]);
  const [invites, setInvites] = useState<TeamsInvite[]>([]);
  const [openGroupId, setOpenGroupId] = useState<number | null>(null);
  const selfId = user?.id ?? 0;
  const openRef = useRef<number | null>(null);
  openRef.current = openGroupId;
  const groupsRef = useRef<TeamsGroup[]>([]);
  groupsRef.current = groups;
  const listeners = useRef(new Set<(e: TeamsEvent) => void>());
  const [presence, setPresence] = useState<Record<number, PresenceInfo>>({});
  const [mentions, setMentions] = useState<MentionToast[]>(loadMentions);
  const [toast, setToast] = useState<MentionToast | null>(null);
  const toastTimer = useRef<number | null>(null);
  const dismissToast = useCallback(() => {
    if (toastTimer.current) window.clearTimeout(toastTimer.current);
    toastTimer.current = null;
    setToast(null);
  }, []);
  const showToast = useCallback((t: MentionToast) => {
    if (toastTimer.current) window.clearTimeout(toastTimer.current);
    setToast(t);
    toastTimer.current = window.setTimeout(() => setToast(null), 8000);
  }, []);

  // Reactions to your lines: one chime, then a quiet window in which further
  // reactions only bump the card's count, so a burst never becomes spam.
  const reactionBurst = useRef<{ at: number; count: number; names: string[]; emojis: string[]; groupId: number } | null>(null);
  const REACTION_QUIET_MS = 10000;
  const notifyReaction = useCallback(
    (e: TeamsEvent) => {
      if (!e.added || !e.userId || e.userId === selfId || e.senderId !== selfId || !e.groupId) return;
      const g = groupsRef.current.find((x) => x.id === e.groupId);
      const mute = g?.mute ?? (g?.muted ? "mentions" : "none");
      if (mute !== "none") return;
      const roomOpen = openRef.current === e.groupId && document.visibilityState === "visible" && window.location.pathname.startsWith("/teams");
      if (roomOpen) return;
      const now = Date.now();
      const b = reactionBurst.current;
      const name = e.name ?? "Biri";
      const emoji = e.emoji ?? "";
      if (b && now - b.at < REACTION_QUIET_MS) {
        b.count += 1;
        if (!b.names.includes(name)) b.names.push(name);
        if (emoji && !b.emojis.includes(emoji)) b.emojis.push(emoji);
        const others = b.names.length - 1;
        const who = others > 0 ? `${b.names[0]} ve ${others} kişi` : b.names[0];
        showToast({ id: -e.groupId, groupId: e.groupId, groupName: g?.name ?? "Teams", sender: { id: e.userId, name, hasAvatar: false }, body: `${b.emojis.join(" ")} ${who} mesajlarına tepki verdi (${b.count})`, at: new Date().toISOString() });
        return;
      }
      reactionBurst.current = { at: now, count: 1, names: [name], emojis: emoji ? [emoji] : [], groupId: e.groupId };
      tones.notify();
      showToast({ id: -e.groupId, groupId: e.groupId, groupName: g?.name ?? "Teams", sender: { id: e.userId, name, hasAvatar: false }, body: `${emoji} ${name} mesajına tepki verdi`, at: new Date().toISOString() });
      if (typeof Notification !== "undefined" && Notification.permission === "granted" && document.visibilityState !== "visible") {
        try {
          const n = new Notification(`${name} tepki verdi${g ? ` · ${g.name}` : ""}`, { body: `${emoji} mesajına`, tag: `teams-react-${e.groupId}`, silent: true });
          n.onclick = () => {
            window.focus();
            window.location.assign(`/teams/${e.groupId}`);
            n.close();
          };
        } catch {
          // notifications unavailable
        }
      }
    },
    [selfId, showToast],
  );
  useEffect(() => {
    try {
      localStorage.setItem(MENTIONS_KEY, JSON.stringify(mentions));
    } catch {
      // storage unavailable
    }
  }, [mentions]);
  const dismissMention = useCallback((id: number) => setMentions((cur) => cur.filter((t) => t.id !== id)), []);
  const [typing, setTyping] = useState<Typing>({});
  const [games, setGames] = useState<GamesConfig | null>(null);
  const reloadGames = useCallback(() => {
    if (!enabled) return;
    gamesApi.config().then(setGames).catch(() => setGames(null));
  }, [enabled]);
  useEffect(reloadGames, [reloadGames]);

  // Expire "yazıyor" entries a few seconds after the last keystroke.
  useEffect(() => {
    if (Object.keys(typing).length === 0) return;
    const t = window.setInterval(() => {
      const now = Date.now();
      setTyping((cur) => {
        let changed = false;
        const next: Typing = {};
        for (const [gid, users] of Object.entries(cur)) {
          const live: Record<number, { name: string; until: number }> = {};
          for (const [uid, v] of Object.entries(users)) {
            if (v.until > now) live[Number(uid)] = v;
            else changed = true;
          }
          if (Object.keys(live).length) next[Number(gid)] = live;
        }
        return changed ? next : cur;
      });
    }, 1000);
    return () => window.clearInterval(t);
  }, [typing]);

  const typingLabel = useCallback(
    (groupId: number) => {
      const users = typing[groupId];
      if (!users) return null;
      const names = Object.values(users).map((v) => v.name.split(" ")[0]);
      return typingText(names);
    },
    [typing],
  );

  // Tell the server whether a room is open in front of us ("Sohbette").
  const lastState = useRef<number | null>(null);
  const sendState = useCallback((force = false) => {
    if (!enabled) return;
    const room = openRef.current && document.visibilityState === "visible" && window.location.pathname.startsWith("/teams") ? openRef.current : 0;
    if (!force && lastState.current === room) return;
    lastState.current = room;
    void api.teamsPresence(room).catch(() => undefined);
  }, [enabled]);
  useEffect(() => {
    sendState();
    const onVis = () => sendState();
    document.addEventListener("visibilitychange", onVis);
    window.addEventListener("focus", onVis);
    window.addEventListener("blur", onVis);
    return () => {
      document.removeEventListener("visibilitychange", onVis);
      window.removeEventListener("focus", onVis);
      window.removeEventListener("blur", onVis);
    };
  }, [openGroupId, sendState]);
  const refreshTimer = useRef<number | null>(null);
  const [notifications, setNotifications] = useState<NotificationPermission | "unsupported">(() =>
    typeof Notification === "undefined" ? "unsupported" : Notification.permission,
  );

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      const o = await api.teamsOverview();
      setGroups(o.groups);
      setInvites(o.invites);
      setPresence((cur) => {
        const next = { ...cur };
        for (const g of o.groups) if (g.peer) next[g.peer.id] = { online: !!g.peer.online, lastSeen: g.peer.lastSeen, room: g.peer.inRoom ? g.id : next[g.peer.id]?.room };
        return next;
      });
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
      if (m.mine || m.kind === "system" || m.sender?.id === selfId) return;
      // "all" silences everything; "mentions" lets only @ tags through.
      const mute = g?.mute ?? (g?.muted ? "mentions" : "none");
      if (mute === "all") return;
      const taggedMe = m.mentions?.includes(selfId) ?? false;
      const tagged = taggedMe || m.mentionsAll;
      if (mute === "mentions" && !tagged) return;
      const roomOpen = openRef.current === m.groupId && document.visibilityState === "visible" && window.location.pathname.startsWith("/teams");
      if (roomOpen) return;
      if (tagged) {
        tones.mention();
        if (m.sender) {
          const sender = m.sender;
          setMentions((cur) => [...cur.filter((t) => t.id !== m.id), { id: m.id, groupId: m.groupId, groupName: g?.name ?? "Teams", sender, body: previewLabel(m.body, m.attachments, m.kind), at: m.createdAt }].slice(-20));
        }
      } else {
        tones.notify();
        if (m.sender) showToast({ id: m.id, groupId: m.groupId, groupName: g?.name ?? "Teams", sender: m.sender, body: previewLabel(m.body, m.attachments, m.kind), at: m.createdAt });
      }
      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        const title = tagged
          ? `${m.sender?.name ?? "Biri"} seni etiketledi${g ? ` · ${g.name}` : ""}`
          : g ? (g.kind === "dm" ? g.name : `${g.name} · ${m.sender?.name ?? ""}`) : m.sender?.name ?? "Teams";
        try {
          const n = new Notification(title, { body: previewLabel(m.body, m.attachments, m.kind).slice(0, 140), tag: tagged ? `teams-mention-${m.id}` : `teams-${m.groupId}`, silent: true, requireInteraction: tagged });
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
    [selfId, showToast],
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
        // The server forgets activity across restarts; say it again.
        sendState(true);
      };
      es.onmessage = (ev) => {
        let e: TeamsEvent;
        try {
          e = JSON.parse(ev.data) as TeamsEvent;
        } catch {
          return;
        }
        if (e.type === "typing" && e.groupId && e.userId && e.userId !== selfId) {
          const gid = e.groupId;
          const uid = e.userId;
          const name = e.name ?? "Biri";
          setTyping((cur) => ({ ...cur, [gid]: { ...(cur[gid] ?? {}), [uid]: { name, until: Date.now() + TYPING_TTL_MS } } }));
        } else if (e.type === "message.edited" && e.message) {
          const m = e.message;
          setGroups((list) => list.map((g) => (g.id === m.groupId && g.lastMessage?.id === m.id ? { ...g, lastMessage: { ...g.lastMessage, body: m.body, editedAt: m.editedAt } } : g)));
        } else if (e.type === "message" && e.message) {
          const m = e.message;
          if (m.sender) {
            const gid = m.groupId;
            const uid = m.sender.id;
            setTyping((cur) => {
              if (!cur[gid]?.[uid]) return cur;
              const users = { ...cur[gid] };
              delete users[uid];
              const next = { ...cur };
              if (Object.keys(users).length) next[gid] = users;
              else delete next[gid];
              return next;
            });
          }
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
        } else if (e.type === "reaction") {
          notifyReaction(e);
        } else if (e.type === "message.deleted" && e.groupId) {
          // The preview falls back to the previous line; the server knows it.
          setGroups((list) => list.map((g) => (g.id === e.groupId && g.lastMessage && g.lastMessage.id === e.id ? { ...g, lastMessage: { ...g.lastMessage, deleted: true, body: "", attachments: [] } } : g)));
          if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
          refreshTimer.current = window.setTimeout(() => void refresh(), 800);
        } else if (e.type === "group" || e.type === "invite") {
          void refresh();
        } else if (e.type === "presence" && e.userId) {
          const uid = e.userId;
          setPresence((cur) => ({ ...cur, [uid]: { online: !!e.online, lastSeen: e.lastSeen ?? cur[uid]?.lastSeen, room: !e.online ? 0 : e.room !== undefined ? e.room : cur[uid]?.room } }));
        } else if (e.type === "receipt" && e.groupId) {
          // A direct message has one reader, so its preview tick is exact;
          // a group's needs every seat, so its list entry is refreshed lazily.
          setGroups((list) =>
            list.map((g) => {
              if (g.id !== e.groupId || !g.lastMessage?.mine || g.lastMessage.status === "read") return g;
              if (g.kind !== "dm") {
                if (refreshTimer.current) window.clearTimeout(refreshTimer.current);
                refreshTimer.current = window.setTimeout(() => void refresh(), 1500);
                return g;
              }
              const id = g.lastMessage.id;
              const status = (e.readId ?? 0) >= id ? "read" : (e.deliveredId ?? 0) >= id ? "delivered" : g.lastMessage.status;
              return status === g.lastMessage.status ? g : { ...g, lastMessage: { ...g.lastMessage, status } };
            }),
          );
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
  }, [enabled, refresh, notify, notifyReaction]);

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

  const presenceOf = useCallback(
    (p: { id: number; online?: boolean; lastSeen?: string; inRoom?: boolean } | undefined | null, room?: number): Presence => {
      if (!p) return { online: false };
      const live = presence[p.id];
      const online = live ? live.online : !!p.online;
      const lastSeen = live?.lastSeen ?? p.lastSeen;
      // A live room wins; until one arrives, trust what the card said.
      const here = !!room && online && (live?.room !== undefined ? live.room === room : !!p.inRoom);
      return { online, lastSeen, here };
    },
    [presence],
  );

  const value = useMemo<TeamsState>(
    () => ({ enabled, groups, invites, unread, refresh, openGroupId, setOpenGroupId, subscribe, bumpGroup, clearUnread, notifications, askNotifications, presence, presenceOf, mentions, dismissMention, toast, dismissToast, typingLabel, games, reloadGames }),
    [enabled, groups, invites, unread, refresh, openGroupId, subscribe, bumpGroup, clearUnread, notifications, askNotifications, presence, presenceOf, mentions, dismissMention, toast, dismissToast, typingLabel, games, reloadGames],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useTeams(): TeamsState {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useTeams must be used within TeamsProvider");
  return ctx;
}
