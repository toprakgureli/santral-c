// MessagePane: the room's history and composer. Messages group by sender
// and minute, days are separated, deleted lines read "Bu mesaj silindi.",
// reactions sit under the line, hovering a line shows react, reply and
// delete. Attachments are reserved: the paperclip is there, disabled.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { CornerUpLeft, Paperclip, SendHorizontal, SmilePlus, Trash2, X } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { TeamsEvent, TeamsGroupDetail, TeamsMessage } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { statusOf, Ticks } from "@/components/teams/Presence";
import { cn } from "@/lib/utils";
import { useTeams } from "@/teams/TeamsContext";

const EMOJIS = ["👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "✅"];

function hhmm(iso: string) {
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
}

function dayLabel(iso: string) {
  const d = new Date(iso);
  const today = new Date();
  const y = new Date(today);
  y.setDate(today.getDate() - 1);
  const same = (a: Date, b: Date) => a.toDateString() === b.toDateString();
  if (same(d, today)) return "Bugün";
  if (same(d, y)) return "Dün";
  return d.toLocaleDateString("tr-TR", { day: "numeric", month: "long", year: d.getFullYear() === today.getFullYear() ? undefined : "numeric" });
}

export default function MessagePane({ group, selfId }: { group: TeamsGroupDetail; selfId: number }) {
  const teams = useTeams();
  const [items, setItems] = useState<TeamsMessage[]>([]);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reply, setReply] = useState<TeamsMessage | null>(null);
  const [picker, setPicker] = useState<number | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  // Other seats' pointers, so ticks update live from receipts.
  const [seats, setSeats] = useState<Record<number, { deliveredId: number; readId: number; name: string }>>({});
  useEffect(() => {
    const next: Record<number, { deliveredId: number; readId: number; name: string }> = {};
    for (const m of group.members) next[m.id] = { deliveredId: m.deliveredId, readId: m.readId, name: m.name };
    setSeats(next);
  }, [group.members]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.teamsMessages(group.id);
      setItems(r.items);
      setMore(r.more);
      setError(null);
      stickToBottom.current = true;
      if (r.items.length) void api.teamsMarkRead(group.id, r.items[r.items.length - 1].id).catch(() => undefined);
      teams.clearUnread(group.id);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Mesajlar alınamadı.");
    } finally {
      setLoading(false);
    }
  }, [group.id]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    setItems([]);
    setReply(null);
    setPicker(null);
    void load();
  }, [load]);

  async function loadOlder() {
    if (!items.length) return;
    const el = list.current;
    const before = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const r = await api.teamsMessages(group.id, items[0].id);
      setItems((cur) => [...r.items, ...cur]);
      setMore(r.more);
      stickToBottom.current = false;
      requestAnimationFrame(() => {
        if (el) el.scrollTop = el.scrollHeight - before;
      });
    } catch {
      // keep what we have
    }
  }

  // Live events for this room.
  useEffect(() => {
    return teams.subscribe((e: TeamsEvent) => {
      if (e.groupId !== group.id) return;
      if (e.type === "message" && e.message) {
        const m = e.message;
        setItems((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, { ...m, mine: m.sender?.id === selfId, canDelete: m.sender?.id === selfId || group.canManage }]));
        if (document.visibilityState === "visible") {
          void api.teamsMarkRead(group.id, m.id).catch(() => undefined);
          teams.clearUnread(group.id);
        }
      } else if (e.type === "receipt" && e.userId) {
        const uid = e.userId;
        setSeats((cur) => {
          const s = cur[uid];
          if (!s) return cur;
          const deliveredId = Math.max(s.deliveredId, e.deliveredId ?? 0, e.readId ?? 0);
          const readId = Math.max(s.readId, e.readId ?? 0);
          if (deliveredId === s.deliveredId && readId === s.readId) return cur;
          return { ...cur, [uid]: { ...s, deliveredId, readId } };
        });
      } else if (e.type === "message.deleted" && e.id) {
        setItems((cur) => cur.map((x) => (x.id === e.id ? { ...x, deleted: true, body: "", canDelete: false, reactions: [] } : x)));
      } else if (e.type === "reaction" && e.id) {
        // Re-read the page around the message: simplest correct refresh.
        api.teamsMessages(group.id).then((r) => setItems((cur) => {
          const fresh = new Map(r.items.map((m) => [m.id, m]));
          return cur.map((x) => fresh.get(x.id) ?? x);
        })).catch(() => undefined);
      }
    });
  }, [group.id, group.canManage, selfId, teams]);

  // Keep the view pinned to the newest line unless the reader scrolled up.
  useLayoutEffect(() => {
    const el = list.current;
    if (!el) return;
    if (stickToBottom.current) el.scrollTop = el.scrollHeight;
  }, [items]);

  function onScroll() {
    const el = list.current;
    if (!el) return;
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    if (el.scrollTop < 60 && more && !loading) void loadOlder();
  }

  async function send(body: string) {
    const m = await api.teamsSend(group.id, { body, replyToId: reply?.id });
    setItems((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, m]));
    teams.bumpGroup(group.id, m);
    setReply(null);
    stickToBottom.current = true;
  }

  async function remove(m: TeamsMessage) {
    try {
      await api.teamsDeleteMessage(group.id, m.id);
      setItems((cur) => cur.map((x) => (x.id === m.id ? { ...x, deleted: true, body: "", canDelete: false, reactions: [] } : x)));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Silinemedi.");
    }
  }

  async function react(m: TeamsMessage, emoji: string) {
    setPicker(null);
    try {
      await api.teamsReact(group.id, m.id, emoji);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Tepki eklenemedi.");
    }
  }

  // Group consecutive lines by the same sender within five minutes.
  const rows = useMemo(() => {
    const out: { m: TeamsMessage; head: boolean; day: string | null }[] = [];
    let lastDay = "";
    let prev: TeamsMessage | null = null;
    for (const m of items) {
      const day = new Date(m.createdAt).toDateString();
      const newDay = day !== lastDay;
      lastDay = day;
      const head = newDay || !prev || m.kind === "system" || prev.kind === "system" || prev.sender?.id !== m.sender?.id || new Date(m.createdAt).getTime() - new Date(prev.createdAt).getTime() > 5 * 60 * 1000;
      out.push({ m, head, day: newDay ? dayLabel(m.createdAt) : null });
      prev = m;
    }
    return out;
  }, [items]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div ref={list} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        {more && (
          <div className="mb-3 text-center">
            <button type="button" onClick={() => void loadOlder()} className="rounded-full border border-border/70 px-3 py-1 text-xs text-muted-foreground hover:bg-accent">Daha eski mesajlar</button>
          </div>
        )}
        {loading && items.length === 0 && <p className="py-10 text-center text-sm text-muted-foreground">Yükleniyor...</p>}
        {!loading && items.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
            <p className="text-sm font-medium">Henüz mesaj yok</p>
            <p className="text-xs text-muted-foreground">{group.canPost ? "İlk mesajı sen yaz." : "Bu grupta yazma yetkin yok."}</p>
          </div>
        )}
        {rows.map(({ m, head, day }) => (
          <div key={m.id}>
            {day && (
              <div className="my-4 flex items-center gap-3">
                <span className="h-px flex-1 bg-border/60" />
                <span className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">{day}</span>
                <span className="h-px flex-1 bg-border/60" />
              </div>
            )}
            {m.kind === "system" ? (
              <p className="my-2 text-center text-xs text-muted-foreground">{m.body}</p>
            ) : (
              <div className={cn("group relative flex gap-3 rounded-xl px-2 py-0.5 hover:bg-accent/40", head ? "mt-3" : "mt-0")}>
                <div className="w-9 shrink-0">
                  {head && m.sender && <UserAvatar userId={m.sender.id} name={m.sender.name} hasAvatar={m.sender.hasAvatar} version={m.sender.avatarVersion} className="size-9" fallbackClassName="bg-primary/10 text-xs text-primary" />}
                  {!head && <span className="hidden text-[0.65rem] tabular-nums text-muted-foreground group-hover:block">{hhmm(m.createdAt)}</span>}
                </div>
                <div className="min-w-0 flex-1">
                  {head && m.sender && (
                    <div className="flex items-baseline gap-2">
                      <span className={cn("text-sm font-semibold", m.mine && "text-primary")}>{m.sender.name}</span>
                      <span className="text-[0.7rem] text-muted-foreground">{hhmm(m.createdAt)}</span>
                    </div>
                  )}
                  {m.replyTo && (
                    <div className="mt-0.5 mb-1 border-l-2 border-primary/50 pl-2 text-xs text-muted-foreground">
                      <span className="font-medium text-foreground/80">{m.replyTo.sender}</span>
                      {": "}
                      <span className="italic">{m.replyTo.deleted ? "Bu mesaj silindi." : m.replyTo.body.slice(0, 120)}</span>
                    </div>
                  )}
                  {m.deleted ? (
                    <p className="text-sm italic text-muted-foreground">Bu mesaj silindi.</p>
                  ) : (
                    <p className="whitespace-pre-wrap break-words text-sm leading-relaxed">
                      {m.body}
                      {m.mine && (() => {
                        const st = statusOf(m.id, selfId, seats);
                        return <Ticks status={st.status} readBy={st.readBy} className="ml-1.5 align-text-bottom" />;
                      })()}
                    </p>
                  )}
                  {m.reactions.length > 0 && (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {m.reactions.map((r) => (
                        <button
                          key={r.emoji}
                          type="button"
                          onClick={() => void react(m, r.emoji)}
                          title={r.names.join(", ")}
                          className={cn("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors", r.mine ? "border-primary/50 bg-primary/10" : "border-border/70 bg-muted/40 hover:bg-accent")}
                        >
                          <span>{r.emoji}</span>
                          <span className="tabular-nums">{r.count}</span>
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                {!m.deleted && (
                  <div className="absolute -top-3 right-3 hidden items-center gap-0.5 rounded-lg border border-border bg-card p-0.5 shadow-sm group-hover:flex">
                    <button type="button" onClick={() => setPicker(picker === m.id ? null : m.id)} title="Tepki ver" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><SmilePlus className="size-4" /></button>
                    {group.canPost && <button type="button" onClick={() => setReply(m)} title="Yanıtla" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><CornerUpLeft className="size-4" /></button>}
                    {m.canDelete && <button type="button" onClick={() => void remove(m)} title="Sil" className="rounded-md p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>}
                  </div>
                )}
                {picker === m.id && (
                  <div className="absolute top-5 right-3 z-10 flex gap-0.5 rounded-xl border border-border bg-popover p-1 shadow-lg">
                    {EMOJIS.map((e) => (
                      <button key={e} type="button" onClick={() => void react(m, e)} className="rounded-lg px-1.5 py-1 text-lg hover:bg-accent">{e}</button>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        ))}
      </div>

      {error && <p className="px-5 pb-1 text-xs text-destructive">{error}</p>}

      <Composer group={group} reply={reply} onCancelReply={() => setReply(null)} onSend={send} />
    </div>
  );
}

function Composer({ group, reply, onCancelReply, onSend }: { group: TeamsGroupDetail; reply: TeamsMessage | null; onCancelReply: () => void; onSend: (body: string) => Promise<void> }) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    setText("");
    setError(null);
  }, [group.id]);

  useEffect(() => {
    if (reply) area.current?.focus();
  }, [reply]);

  async function submit() {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSend(body);
      setText("");
      requestAnimationFrame(() => area.current?.focus());
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
    } finally {
      setBusy(false);
    }
  }

  if (!group.canPost) {
    return (
      <div className="border-t border-border/60 px-5 py-3 text-center text-xs text-muted-foreground">
        {group.postPolicy === "admins" ? "Bu bir duyuru grubu, yalnızca yöneticiler yazabilir." : "Bu grupta yazma yetkin kapatılmış."}
      </div>
    );
  }

  return (
    <div className="border-t border-border/60 px-4 py-3">
      {reply && (
        <div className="mb-2 flex items-center gap-2 rounded-lg bg-muted/40 px-3 py-1.5 text-xs">
          <CornerUpLeft className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium">{reply.sender?.name}</span>
            <span className="text-muted-foreground">: {reply.body.slice(0, 100)}</span>
          </span>
          <button type="button" onClick={onCancelReply} aria-label="Yanıtı iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      )}
      <div className="flex items-end gap-2 rounded-2xl border border-border/70 bg-muted/30 px-2 py-1.5 focus-within:border-ring/60 focus-within:ring-4 focus-within:ring-ring/15">
        <button type="button" disabled title="Dosya ekleme yakında" className="mb-1 rounded-lg p-1.5 text-muted-foreground/50"><Paperclip className="size-4" /></button>
        <textarea
          ref={area}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void submit();
            }
          }}
          rows={1}
          maxLength={4000}
          placeholder={group.kind === "dm" ? `${group.name} kişisine yaz...` : `#${group.name} grubuna yaz...`}
          className="max-h-40 min-h-9 flex-1 resize-none bg-transparent px-1 py-1.5 text-sm outline-none placeholder:text-muted-foreground/60"
          style={{ height: "auto" }}
          onInput={(e) => {
            const el = e.currentTarget;
            el.style.height = "auto";
            el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
          }}
        />
        <button type="button" onClick={() => void submit()} disabled={busy || !text.trim()} aria-label="Gönder" className="mb-0.5 flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground transition-opacity disabled:opacity-40">
          <SendHorizontal className="size-4" />
        </button>
      </div>
      <div className="mt-1 flex items-center justify-between px-1 text-[0.65rem] text-muted-foreground/70">
        <span>Enter gönderir, Shift+Enter yeni satır.</span>
        {error ? <span className="text-destructive">{error}</span> : <span>{text.length} / 4000</span>}
      </div>
    </div>
  );
}
