// WhatsAppContext keeps the inbox in step for the whole panel: the list of
// conversations the person sees, their badges, who is typing where, and
// the short alerts ("size yeni bir sohbet atandı"). Events come on the
// live stream (the chat's, or the module's own for people without the
// chat). After a lost connection only what changed since the last known
// version is fetched, so nothing is missed and nothing is loaded twice.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useAuth } from "@/auth/AuthContext";
import { can } from "@/lib/permissions";
import { tones } from "@/softphone/tones";
import NewChatDialog from "@/components/whatsapp/NewChatDialog";
import { useTeams } from "@/teams/TeamsContext";
import { waApi } from "@/whatsapp/api";
import type { WAConversation, WAEvent, WAMessage, WAMute, WAPrefs } from "@/whatsapp/types";
import { isMine, isPool, isWaiting, isResolved } from "@/whatsapp/util";

export interface WAAlert {
  id: number;
  text: string;
  level: "info" | "warning";
  conversationId?: number;
}

interface WAState {
  enabled: boolean;
  loaded: boolean;
  me: number;
  conversations: WAConversation[];
  byId: (id: number) => WAConversation | undefined;
  upsert: (c: WAConversation) => void;
  openId: number | null;
  setOpenId: (id: number | null) => void;
  typing: (id: number) => string | null;
  onMessage: (fn: (m: WAMessage) => void) => () => void;
  counts: { unread: number; mineUnread: number; waiting: number; pool: number; badge: number };
  alerts: WAAlert[];
  dismissAlert: (id: number) => void;
  reload: () => Promise<void>;
  // the reply assistant is set up and this person may use it
  ai: boolean;
  // opens "WhatsApp'tan yaz" from anywhere, optionally for a number
  startChat: (opts?: { number?: string; name?: string }) => void;
  // the person's own preferences: sounds, a mute for everything, and
  // mutes and pins on single conversations
  prefs: WAPrefs;
  muted: (id: number) => boolean;
  pinned: (id: number) => boolean;
  mutedAll: boolean;
  setPrefs: (body: { sound?: boolean; desktop?: boolean; mute?: WAMute }) => Promise<void>;
  setConvPref: (id: number, body: { mute?: WAMute; pin?: boolean }) => Promise<void>;
  markUnread: (id: number) => Promise<void>;
  markRead: (id: number) => Promise<void>;
}

const NO_PREFS: WAPrefs = { sound: true, desktop: true, conversations: [] };
const live = (iso?: string) => !!iso && Date.parse(iso) > Date.now();

const Ctx = createContext<WAState | undefined>(undefined);

export function WhatsAppProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const teams = useTeams();
  const enabled = can(user, "whatsapp.view");
  const me = user?.id ?? 0;
  const [map, setMap] = useState<Record<number, WAConversation>>({});
  const [loaded, setLoaded] = useState(false);
  const version = useRef(0);
  const [openId, setOpenIdState] = useState<number | null>(null);
  const openRef = useRef<number | null>(null);
  const [typingMap, setTyping] = useState<Record<number, { name: string; until: number }>>({});
  const [alerts, setAlerts] = useState<WAAlert[]>([]);
  const listeners = useRef(new Set<(m: WAMessage) => void>());
  const [prefs, setPrefsState] = useState<WAPrefs>(NO_PREFS);
  const prefsRef = useRef(prefs);
  prefsRef.current = prefs;
  // sounds and desktop notices, per conversation, as the person chose
  const quiet = (id?: number) => {
    const p = prefsRef.current;
    if (live(p.mutedUntil)) return { sound: false, desktop: false };
    const conv = id ? p.conversations.find((c) => c.id === id) : undefined;
    if (conv && live(conv.mutedUntil)) return { sound: false, desktop: false };
    return { sound: p.sound, desktop: p.desktop };
  };
  const mapRef = useRef(map);
  mapRef.current = map;

  const setOpenId = useCallback((id: number | null) => {
    openRef.current = id;
    setOpenIdState(id);
  }, []);

  const alert = useCallback((text: string, level: "info" | "warning", conversationId?: number) => {
    const id = Date.now() + Math.random();
    setAlerts((cur) => [...cur.slice(-3), { id, text, level, conversationId }]);
    window.setTimeout(() => setAlerts((cur) => cur.filter((a) => a.id !== id)), level === "warning" ? 15000 : 8000);
  }, []);

  const apply = useCallback((items: WAConversation[], hidden: number[]) => {
    setMap((cur) => {
      const next = { ...cur };
      for (const c of items) {
        const old = next[c.id];
        if (!old || old.version <= c.version) next[c.id] = c;
      }
      for (const id of hidden) delete next[id];
      return next;
    });
  }, []);

  const reload = useCallback(async () => {
    if (!enabled) return;
    try {
      const r = await waApi.conversations(0);
      const next: Record<number, WAConversation> = {};
      for (const c of r.items) next[c.id] = c;
      setMap(next);
      version.current = r.version;
      setLoaded(true);
    } catch {
      // the next event or poll tries again
    }
  }, [enabled]);

  const sync = useCallback(async () => {
    if (!enabled) return;
    if (!version.current) {
      await reload();
      return;
    }
    try {
      const r = await waApi.conversations(version.current);
      apply(r.items, r.hidden);
      version.current = Math.max(version.current, r.version);
    } catch {
      // try again later
    }
  }, [enabled, apply, reload]);

  useEffect(() => {
    if (enabled) void reload();
  }, [enabled, reload]);

  // Stay in step even when a proxy holds the stream back.
  useEffect(() => {
    if (!enabled) return;
    const t = window.setInterval(() => void sync(), 60000);
    const onVisible = () => document.visibilityState === "visible" && void sync();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.clearInterval(t);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [enabled, sync]);

  const notifyBrowser = useCallback((title: string, body: string, conversationId: number) => {
    if (typeof Notification === "undefined" || Notification.permission !== "granted" || document.visibilityState === "visible") return;
    try {
      const n = new Notification(title, { body, tag: `wa-${conversationId}` });
      n.onclick = () => {
        window.focus();
        window.location.assign(`/whatsapp/${conversationId}`);
        n.close();
      };
    } catch {
      // notifications unavailable here
    }
  }, []);

  const handle = useCallback(
    (e: WAEvent) => {
      if (e.type === "hello") {
        void sync();
        return;
      }
      if (!e.type.startsWith("wa.")) return;
      switch (e.type) {
        case "wa.conv": {
          const c = e.conversation;
          if (!c) return;
          const before = mapRef.current[c.id];
          version.current = Math.max(version.current, c.version);
          apply([c], []);
          const m = e.message;
          if (m) {
            listeners.current.forEach((fn) => fn(m));
            if (m.sender.userId || m.direction !== "in") {
              setTyping((cur) => {
                if (!cur[c.id]) return cur;
                const next = { ...cur };
                delete next[c.id];
                return next;
              });
            }
            const fresh = m.direction === "in" && m.kind !== "reaction" && (!before || (before.last?.id ?? 0) < m.id);
            const looking = openRef.current === c.id && document.visibilityState === "visible";
            if (fresh && !looking && (isMine(c, me) || isPool(c) || isWaiting(c))) {
              const q = quiet(c.id);
              if (q.sound) tones.notify();
              if (q.desktop) notifyBrowser(c.contact.display, m.body || "Yeni mesaj", c.id);
            }
          }
          if (before && !isWaiting(before) && isWaiting(c) && !isMine(c, me)) {
            // handled by wa.waiting with its own sound
          }
          break;
        }
        case "wa.gone":
          if (e.conversationId) {
            const id = e.conversationId;
            setMap((cur) => {
              if (!cur[id]) return cur;
              const next = { ...cur };
              delete next[id];
              return next;
            });
          }
          break;
        case "wa.typing":
          if (e.conversationId && e.userId !== me) {
            const id = e.conversationId;
            setTyping((cur) => ({ ...cur, [id]: { name: e.name ?? "Biri", until: Date.now() + 5000 } }));
          }
          break;
        case "wa.waiting":
          if (quiet(e.conversationId).sound) tones.mention();
          alert(e.text ?? "Cevap bekleyen bir müşteri var.", "warning", e.conversationId);
          break;
        case "wa.assigned":
          if (quiet(e.conversationId).sound) tones.mention();
          alert(e.text ?? "Size bir sohbet atandı.", "info", e.conversationId);
          if (e.conversationId && quiet(e.conversationId).desktop) notifyBrowser("WhatsApp", e.text ?? "Size bir sohbet atandı.", e.conversationId);
          break;
        case "wa.callback":
          if (quiet().sound) tones.notify();
          alert(e.text ?? "Yeni geri arama talebi.", "info", e.conversationId);
          break;
        case "wa.alert":
          alert(e.text ?? "", e.level === "warning" ? "warning" : "info", e.conversationId);
          break;
      }
    },
    [apply, alert, me, notifyBrowser, sync], // eslint-disable-line react-hooks/exhaustive-deps
  );

  // Events: on the chat's stream when the person uses the chat, otherwise
  // on the module's own.
  useEffect(() => {
    if (!enabled) return;
    if (teams.enabled) {
      return teams.subscribe((e) => handle(e as unknown as WAEvent));
    }
    let es: EventSource | null = null;
    let closed = false;
    let retry = 0;
    const connect = () => {
      if (closed) return;
      es = new EventSource("/api/v1/wa/stream", { withCredentials: true });
      es.onopen = () => {
        retry = 0;
      };
      es.onmessage = (ev) => {
        try {
          handle(JSON.parse(ev.data) as WAEvent);
        } catch {
          // ignore a broken frame
        }
      };
      es.onerror = () => {
        es?.close();
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
  }, [enabled, teams.enabled, teams.subscribe, handle]); // eslint-disable-line react-hooks/exhaustive-deps

  // Typing labels fade out on their own.
  useEffect(() => {
    const t = window.setInterval(() => {
      setTyping((cur) => {
        const now = Date.now();
        let changed = false;
        const next = { ...cur };
        for (const [k, v] of Object.entries(cur)) {
          if (v.until < now) {
            delete next[Number(k)];
            changed = true;
          }
        }
        return changed ? next : cur;
      });
    }, 1000);
    return () => window.clearInterval(t);
  }, []);

  const conversations = useMemo(() => Object.values(map), [map]);
  const counts = useMemo(() => {
    let unread = 0;
    let mineUnread = 0;
    let waiting = 0;
    let pool = 0;
    for (const c of conversations) {
      if (isResolved(c)) continue;
      if (live(prefs.conversations.find((x) => x.id === c.id)?.mutedUntil)) continue;
      if (isWaiting(c)) waiting++;
      if (isPool(c)) pool++;
      if (c.unread > 0) {
        unread += c.unread;
        if (isMine(c, me)) mineUnread += c.unread;
      }
    }
    return { unread, mineUnread, waiting, pool, badge: mineUnread + waiting + pool };
  }, [conversations, me, prefs]);

  const byId = useCallback((id: number) => map[id], [map]);
  const upsert = useCallback((c: WAConversation) => apply([c], []), [apply]);
  const typing = useCallback((id: number) => {
    const t = typingMap[id];
    return t ? `${t.name} yazıyor…` : null;
  }, [typingMap]);
  const onMessage = useCallback((fn: (m: WAMessage) => void) => {
    listeners.current.add(fn);
    return () => {
      listeners.current.delete(fn);
    };
  }, []);
  const dismissAlert = useCallback((id: number) => setAlerts((cur) => cur.filter((a) => a.id !== id)), []);

  useEffect(() => {
    if (!enabled) return;
    waApi.myPrefs().then(setPrefsState).catch(() => setPrefsState(NO_PREFS));
  }, [enabled]);
  const setPrefs = useCallback(async (body: { sound?: boolean; desktop?: boolean; mute?: WAMute }) => {
    setPrefsState(await waApi.savePrefs(body));
  }, []);
  const setConvPref = useCallback(async (id: number, body: { mute?: WAMute; pin?: boolean }) => {
    setPrefsState(await waApi.convPref(id, body));
  }, []);
  const muted = useCallback((id: number) => live(prefs.conversations.find((c) => c.id === id)?.mutedUntil), [prefs]);
  const pinned = useCallback((id: number) => !!prefs.conversations.find((c) => c.id === id)?.pinnedAt, [prefs]);
  const mutedAll = live(prefs.mutedUntil);
  const markUnread = useCallback(async (id: number) => {
    await waApi.unread(id);
  }, []);
  const markRead = useCallback(async (id: number) => {
    const c = mapRef.current[id];
    if (c?.last && c.last.direction === "in") await waApi.read(id, c.last.id);
  }, []);

  const [ai, setAI] = useState(false);
  useEffect(() => {
    if (!enabled) return;
    waApi.aiStatus().then((r) => setAI(r.available)).catch(() => setAI(false));
  }, [enabled]);

  const [chat, setChat] = useState<{ number?: string; name?: string } | null>(null);
  const startChat = useCallback((opts?: { number?: string; name?: string }) => setChat(opts ?? {}), []);

  const value = useMemo<WAState>(
    () => ({ enabled, loaded, me, conversations, byId, upsert, openId, setOpenId, typing, onMessage, counts, alerts, dismissAlert, reload, ai, startChat, prefs, muted, pinned, mutedAll, setPrefs, setConvPref, markUnread, markRead }),
    [enabled, loaded, me, conversations, byId, upsert, openId, setOpenId, typing, onMessage, counts, alerts, dismissAlert, reload, ai, startChat, prefs, muted, pinned, mutedAll, setPrefs, setConvPref, markUnread, markRead],
  );
  return (
    <Ctx.Provider value={value}>
      {children}
      {enabled && <NewChatDialog open={!!chat} number={chat?.number} name={chat?.name} onClose={() => setChat(null)} onStarted={upsert} />}
    </Ctx.Provider>
  );
}

export function useWhatsApp(): WAState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useWhatsApp must be used inside WhatsAppProvider");
  return v;
}
