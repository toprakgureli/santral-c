// NoticeBell: the person's own notices, such as a colleague reaching a
// customer they could not. It asks for new ones every 20 seconds and when
// the tab comes back to the front, shows each new one once as a toast on
// whichever page is open, and keeps the last thirty days under the bell
// until read.

import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Bell, CheckCheck, X } from "lucide-react";
import { clockTime, isToday, shortMonthDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { followApi, type Notice } from "@/followups/api";

const POLL_MS = 20000;
const TOAST_MS = 12000;

function when(iso: string): string {
  return isToday(iso) ? clockTime(iso) : `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

export default function NoticeBell() {
  const navigate = useNavigate();
  const [items, setItems] = useState<Notice[]>([]);
  const [unread, setUnread] = useState(0);
  const [open, setOpen] = useState(false);
  const [toasts, setToasts] = useState<Notice[]>([]);
  const last = useRef(0);
  // Notices already known: those of the first read fill the bell quietly,
  // and only ones never seen before become toasts.
  const known = useRef<Set<number> | null>(null);
  const box = useRef<HTMLDivElement>(null);

  const poll = useCallback(async () => {
    try {
      const r = await followApi.notices(last.current);
      if (r.items.length > 0) {
        last.current = Math.max(last.current, ...r.items.map((n) => n.id));
        setItems((cur) => [...r.items, ...cur.filter((c) => !r.items.some((n) => n.id === c.id))].slice(0, 50));
      }
      if (known.current === null) {
        known.current = new Set(r.items.map((n) => n.id));
      } else {
        const fresh = r.items.filter((n) => !n.read && !known.current!.has(n.id));
        fresh.forEach((n) => known.current!.add(n.id));
        if (fresh.length > 0) setToasts((cur) => [...fresh, ...cur].slice(0, 3));
      }
      setUnread(r.unread);
    } catch {
      // the next poll tries again
    }
  }, []);

  useEffect(() => {
    void poll();
    const t = window.setInterval(() => void poll(), POLL_MS);
    const onFocus = () => document.visibilityState === "visible" && void poll();
    document.addEventListener("visibilitychange", onFocus);
    return () => {
      window.clearInterval(t);
      document.removeEventListener("visibilitychange", onFocus);
    };
  }, [poll]);

  useEffect(() => {
    if (toasts.length === 0) return;
    const t = window.setTimeout(() => setToasts((cur) => cur.slice(0, -1)), TOAST_MS);
    return () => window.clearTimeout(t);
  }, [toasts]);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const markRead = async (body: { ids?: number[]; all?: boolean }) => {
    try {
      await followApi.readNotices(body);
      setItems((cur) => cur.map((n) => (body.all || body.ids?.includes(n.id) ? { ...n, read: true } : n)));
      setUnread((u) => (body.all ? 0 : Math.max(0, u - (body.ids?.length ?? 0))));
    } catch {
      // stays unread; nothing else depends on it
    }
  };

  const openNotice = (n: Notice) => {
    setOpen(false);
    setToasts((cur) => cur.filter((t) => t.id !== n.id));
    if (!n.read) void markRead({ ids: [n.id] });
    if (n.link) navigate(n.link);
  };

  return (
    <div className="relative" ref={box}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={unread ? `Bildirimler, ${unread} okunmamış` : "Bildirimler"}
        data-tip="Bildirimler"
        className="relative flex size-10 items-center justify-center rounded-xl text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <Bell className="size-[1.15rem]" />
        {unread > 0 && (
          <span className="absolute top-1.5 right-1.5 flex min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[0.6rem] leading-4 font-bold text-white tabular-nums">
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>

      {open && (
        <div className="animate-in fade-in slide-in-from-top-1 absolute right-0 mt-2 w-[22rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-lg duration-150">
          <div className="flex items-center justify-between border-b border-border/60 px-3 py-2">
            <span className="text-sm font-semibold">Bildirimler</span>
            {unread > 0 && (
              <button type="button" onClick={() => void markRead({ all: true })} className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium text-primary hover:bg-primary/10">
                <CheckCheck className="size-3.5" /> Tümünü okundu say
              </button>
            )}
          </div>
          <div className="max-h-96 overflow-y-auto p-1">
            {items.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">Bildirim yok.</p>
            ) : (
              items.map((n) => (
                <button key={n.id} type="button" onClick={() => openNotice(n)} className={cn("flex w-full gap-2.5 rounded-lg px-3 py-2.5 text-left hover:bg-accent", !n.read && "bg-primary/5")}>
                  <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-primary")} />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm leading-snug">{n.text}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">{when(n.createdAt)}</span>
                  </span>
                </button>
              ))
            )}
          </div>
        </div>
      )}

      {toasts.length > 0 && (
        <div className="fixed top-20 right-4 z-[72] flex w-[22rem] max-w-[calc(100vw-2rem)] flex-col gap-2">
          {toasts.map((n) => (
            <div key={n.id} role="status" className="animate-in fade-in slide-in-from-right-4 flex items-start gap-2.5 rounded-xl border border-border bg-popover p-3 text-popover-foreground shadow-xl duration-200">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Bell className="size-4" /></span>
              <button type="button" onClick={() => openNotice(n)} className="min-w-0 flex-1 text-left text-sm leading-snug">{n.text}</button>
              <button type="button" onClick={() => setToasts((cur) => cur.filter((t) => t.id !== n.id))} aria-label="Kapat" className="flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent">
                <X className="size-4" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
