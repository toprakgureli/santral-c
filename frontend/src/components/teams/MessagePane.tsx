// MessagePane: the room's history and composer. Messages group by sender
// and minute, days are separated, a "Yeni mesajlar" line marks where the
// unread ones start, deleted lines read "Bu mesaj silindi.", reactions sit
// under the line and the quick ones are one hover away. A reply quotes
// the original and jumps to it on click. Lines can be edited by their
// author. Files dropped, pasted or picked go to Google Drive and show as a
// grid of previews with a lightbox.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { CornerUpLeft, ImagePlus, Pencil, SmilePlus, Trash2 } from "lucide-react";
import AttachmentGrid, { mediaOf } from "@/components/teams/AttachmentGrid";
import Lightbox from "@/components/teams/Lightbox";
import ProfilePopover, { type PopoverAnchor } from "@/components/teams/ProfilePopover";
import { useUploads } from "@/teams/useUploads";
import type { TeamsAttachment, TeamsPerson } from "@/api/types";
import Composer, { EVERYONE, type Outgoing } from "@/components/teams/Composer";
import { ContextMenu, type MenuItem } from "@/components/ContextMenu";
import { ConfirmDialog, Modal } from "@/components/ui";
import { api, ApiError } from "@/api/client";
import type { TeamsEvent, TeamsGroupDetail, TeamsMessage, TeamsReceipt } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { statusOf, Ticks } from "@/components/teams/Presence";
import { renderMarkup, stripMarkup } from "@/lib/markup";
import GameCard from "@/games/GameCard";
import { previewLabel } from "@/lib/attachments";
import { cn } from "@/lib/utils";
import { useTeams } from "@/teams/TeamsContext";

const QUICK = ["👍", "❤️", "😂", "😮", "🔥", "✅"];
const ALL = ["👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "✅", "👏", "🎉", "👀", "💯"];

type Seats = Record<number, { deliveredId: number; readId: number; name: string }>;

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

export default function MessagePane({ group, selfId, target, onOpenGame }: { group: TeamsGroupDetail; selfId: number; target?: { id: number; nonce: number } | null; onOpenGame: (id: number) => void }) {
  const teams = useTeams();
  const [items, setItems] = useState<TeamsMessage[]>([]);
  const [more, setMore] = useState(false);
  // Inside the history (after a jump) newer lines exist below the window.
  const [moreNewer, setMoreNewer] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reply, setReply] = useState<TeamsMessage | null>(null);
  const [editing, setEditing] = useState<TeamsMessage | null>(null);
  const [picker, setPicker] = useState<number | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [info, setInfo] = useState<TeamsMessage | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<TeamsMessage | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [flash, setFlash] = useState<number | null>(null);
  // Where the unread lines started when the room was opened.
  const [firstUnread, setFirstUnread] = useState<number | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);
  const [seats, setSeats] = useState<Seats>({});
  const uploads = useUploads(group.id);
  const [over, setOver] = useState(false);
  const [gallery, setGallery] = useState<{ items: TeamsAttachment[]; index: number } | null>(null);
  const [profile, setProfile] = useState<PopoverAnchor | null>(null);
  const [who, setWho] = useState<{ x: number; y: number; emoji: string; people: TeamsPerson[] } | null>(null);
  const openProfile = (e: React.MouseEvent, userId: number) => {
    e.stopPropagation();
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setProfile({ userId, x: r.right, y: r.top, room: group.id });
  };
  useEffect(() => {
    const next: Seats = {};
    for (const m of group.members) next[m.id] = { deliveredId: m.deliveredId, readId: m.readId, name: m.name };
    setSeats(next);
  }, [group.members]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await api.teamsMessages(group.id);
      setItems(r.items);
      setMore(r.more);
      setMoreNewer(false);
      setError(null);
      stickToBottom.current = true;
      const unread = group.unread;
      const fresh = r.items.filter((m) => !m.mine && m.kind !== "system");
      setFirstUnread(unread > 0 && fresh.length ? fresh[Math.max(0, fresh.length - unread)].id : null);
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
    setEditing(null);
    setPicker(null);
    void load();
  }, [load]);

  async function loadOlder() {
    if (!items.length) return;
    const el = list.current;
    const before = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const r = await api.teamsMessages(group.id, { before: items[0].id });
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

  async function loadNewer() {
    if (!items.length || !moreNewer) return;
    try {
      const r = await api.teamsMessages(group.id, { after: items[items.length - 1].id });
      setItems((cur) => [...cur, ...r.items]);
      setMoreNewer(r.moreNewer);
      stickToBottom.current = false;
      if (!r.moreNewer && r.items.length) void api.teamsMarkRead(group.id, r.items[r.items.length - 1].id).catch(() => undefined);
    } catch {
      // keep what we have
    }
  }

  // A search result or a shared file: load the window around it and flash.
  useEffect(() => {
    if (!target) return;
    const id = target.id;
    if (list.current?.querySelector(`[data-mid="${id}"]`)) {
      jump(id);
      return;
    }
    setLoading(true);
    api
      .teamsMessages(group.id, { around: id })
      .then((r) => {
        setItems(r.items);
        setMore(r.more);
        setMoreNewer(r.moreNewer);
        stickToBottom.current = false;
        setFirstUnread(null);
        window.setTimeout(() => jump(id), 60);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Mesaja gidilemedi."))
      .finally(() => setLoading(false));
  }, [target]); // eslint-disable-line react-hooks/exhaustive-deps

  // Live events for this room.
  useEffect(() => {
    return teams.subscribe((e: TeamsEvent) => {
      if (e.groupId !== group.id) return;
      if (e.type === "message" && e.message) {
        const m = e.message;
        // Inside the history the new line belongs below the window, not to it.
        if (moreNewer) return;
        setItems((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, { ...m, mine: m.sender?.id === selfId, canDelete: m.sender?.id === selfId || group.canManage }]));
        if (document.visibilityState === "visible") {
          void api.teamsMarkRead(group.id, m.id).catch(() => undefined);
          teams.clearUnread(group.id);
        }
      } else if (e.type === "message.edited" && e.message) {
        const m = e.message;
        setItems((cur) => cur.map((x) => (x.id === m.id ? { ...x, body: m.body, mentions: m.mentions, mentionsAll: m.mentionsAll, editedAt: m.editedAt, replyTo: m.replyTo } : x)));
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
        setEditing((cur) => (cur?.id === e.id ? null : cur));
      } else if (e.type === "reaction" && e.id) {
        api.teamsMessages(group.id).then((r) => setItems((cur) => {
          const fresh = new Map(r.items.map((m) => [m.id, m]));
          return cur.map((x) => fresh.get(x.id) ?? x);
        })).catch(() => undefined);
      }
    });
  }, [group.id, group.canManage, selfId, teams, moreNewer]);

  // Keep the view pinned to the newest line unless the reader scrolled up.
  useLayoutEffect(() => {
    const el = list.current;
    if (!el) return;
    if (stickToBottom.current) el.scrollTop = el.scrollHeight;
  }, [items]);

  function onScroll() {
    const el = list.current;
    if (!el) return;
    stickToBottom.current = !moreNewer && el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    if (el.scrollTop < 60 && more && !loading) void loadOlder();
    if (moreNewer && el.scrollHeight - el.scrollTop - el.clientHeight < 60 && !loading) void loadNewer();
  }

  async function send(out: Outgoing) {
    const m = await api.teamsSend(group.id, { body: out.body, replyToId: reply?.id, mentionIds: out.mentionIds, mentionsAll: out.mentionsAll, attachmentIds: out.attachmentIds });
    setItems((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, m]));
    teams.bumpGroup(group.id, m);
    setReply(null);
    uploads.clear();
    stickToBottom.current = true;
  }

  async function edit(id: number, out: Outgoing) {
    const m = await api.teamsEditMessage(group.id, id, { body: out.body, mentionIds: out.mentionIds, mentionsAll: out.mentionsAll });
    setItems((cur) => cur.map((x) => (x.id === id ? { ...x, body: m.body, mentions: m.mentions, mentionsAll: m.mentionsAll, editedAt: m.editedAt } : x)));
    setEditing(null);
  }

  function editLast() {
    const last = [...items].reverse().find((m) => m.mine && !m.deleted && m.kind === "text");
    if (last) {
      setReply(null);
      setEditing(last);
    }
  }

  async function remove(m: TeamsMessage) {
    setDeleting(true);
    try {
      await api.teamsDeleteMessage(group.id, m.id);
      setItems((cur) => cur.map((x) => (x.id === m.id ? { ...x, deleted: true, body: "", canDelete: false, reactions: [] } : x)));
      if (editing?.id === m.id) setEditing(null);
      setConfirmDelete(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Silinemedi.");
    } finally {
      setDeleting(false);
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

  // Scroll to a quoted line and flash it.
  function jump(id: number) {
    const el = list.current?.querySelector<HTMLElement>(`[data-mid="${id}"]`);
    if (!el) {
      setError("Mesaj bu sayfada değil, daha eski mesajları yükle.");
      return;
    }
    el.scrollIntoView({ block: "center", behavior: "smooth" });
    setFlash(id);
    window.setTimeout(() => setFlash((cur) => (cur === id ? null : cur)), 1600);
  }

  function lineMenu(e: React.MouseEvent, m: TeamsMessage) {
    if (m.deleted || m.kind === "system") return;
    e.preventDefault();
    const items: MenuItem[] = [];
    if (group.canPost) items.push({ label: "Yanıtla", onClick: () => { setEditing(null); setReply(m); } });
    if (m.mine && group.canPost && m.kind === "text") items.push({ label: "Düzenle", onClick: () => { setReply(null); setEditing(m); } });
    items.push({ label: "Metni kopyala", onClick: () => void navigator.clipboard?.writeText(m.body).catch(() => undefined) });
    items.push({ label: "Bilgi", onClick: () => setInfo(m) });
    if (m.canDelete) items.push({ label: "Sil", danger: true, onClick: () => setConfirmDelete(m) });
    setMenu({ x: e.clientX, y: e.clientY, items });
  }

  // Every line carries its own avatar, name and time; days are separated.
  const rows = useMemo(() => {
    const out: { m: TeamsMessage; head: boolean; day: string | null }[] = [];
    let lastDay = "";
    for (const m of items) {
      const day = new Date(m.createdAt).toDateString();
      const newDay = day !== lastDay;
      lastDay = day;
      out.push({ m, head: true, day: newDay ? dayLabel(m.createdAt) : null });
    }
    return out;
  }, [items]);

  const typing = teams.typingLabel(group.id);

  return (
    <div
      className="relative flex min-h-0 flex-1 flex-col"
      onDragOver={(e) => {
        if (group.canPost && e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          setOver(true);
        }
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver(false);
      }}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        if (!group.canPost) return;
        const files = [...e.dataTransfer.files];
        if (files.length) uploads.addFiles(files);
      }}
    >
      {over && (
        <div className="pointer-events-none absolute inset-2 z-20 flex flex-col items-center justify-center gap-2 rounded-2xl border-2 border-dashed border-primary bg-background/85 text-primary backdrop-blur-sm">
          <ImagePlus className="size-8" />
          <p className="text-sm font-medium">Bırak, ekleyelim</p>
          <p className="text-xs text-muted-foreground">Görsel 200 MB, video 1 GB, dosya 3 GB'a kadar</p>
        </div>
      )}
      {profile && <ProfilePopover anchor={profile} selfId={selfId} onClose={() => setProfile(null)} />}
      {who && (
        <ReactionPeople
          {...who}
          selfId={selfId}
          onPerson={(p, x, y) => {
            setWho(null);
            setProfile({ userId: p.id, x, y, room: group.id });
          }}
          onClose={() => setWho(null)}
        />
      )}
      {gallery && <Lightbox items={gallery.items} index={gallery.index} onIndex={(i) => setGallery({ ...gallery, index: i })} onClose={() => setGallery(null)} />}
      <div ref={list} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,color-mix(in_oklab,var(--foreground)_5%,transparent)_1px,transparent_0)] bg-[size:20px_20px] px-5 py-4">
        {more && (
          <div className="mb-3 text-center">
            <button type="button" onClick={() => void loadOlder()} className="rounded-full border border-border/70 px-3 py-1 text-xs text-muted-foreground hover:bg-accent">Daha eski mesajlar</button>
          </div>
        )}
        {loading && items.length === 0 && <p className="py-10 text-center text-sm text-muted-foreground">Yükleniyor...</p>}
        {moreNewer && (
          <div className="sticky top-0 z-10 mb-2 flex justify-center">
            <button type="button" onClick={() => void load()} className="rounded-full border border-primary/40 bg-card px-3 py-1 text-xs font-medium text-primary shadow-sm hover:bg-primary/10">Geçmişteysin · en yeni mesajlara dön</button>
          </div>
        )}
        {!loading && items.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
            <p className="text-sm font-medium">Henüz mesaj yok</p>
            <p className="text-xs text-muted-foreground">{group.canPost ? "İlk mesajı sen yaz." : "Bu grupta yazma yetkin yok."}</p>
          </div>
        )}
        {rows.map(({ m, head, day }) => {
          const mentionsMe = (m.mentions?.includes(selfId) || m.mentionsAll) && !m.mine;
          const labels = [...(m.mentions ?? []).map((id) => seats[id]?.name).filter((n): n is string => !!n).map((n) => `@${n}`), ...(m.mentionsAll ? [`@${EVERYONE}`] : [])];
          return (
            <div key={m.id} data-mid={m.id}>
              {day && (
                <div className="my-4 flex items-center justify-center">
                  <span className="rounded-full border border-border/60 bg-card/90 px-3 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground shadow-sm backdrop-blur">{day}</span>
                </div>
              )}
              {firstUnread === m.id && (
                <div className="my-3 flex items-center gap-3">
                  <span className="h-px flex-1 bg-destructive/50" />
                  <span className="rounded-full border border-destructive/40 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-destructive">Yeni mesajlar</span>
                  <span className="h-px flex-1 bg-destructive/50" />
                </div>
              )}
              {m.kind === "system" ? (
                <p className="my-2 text-center"><span className="rounded-full bg-muted/70 px-3 py-1 text-xs text-muted-foreground">{m.body}</span></p>
              ) : (
                <div
                  onContextMenu={(e) => lineMenu(e, m)}
                  className={cn(
                    "group relative flex gap-3 rounded-2xl px-2.5 py-1 transition-colors duration-700",
                    head ? "mt-2.5" : "mt-0",
                    flash === m.id ? "bg-primary/15" : "hover:bg-card/80",
                    editing?.id === m.id && "bg-warning/10",
                    mentionsMe && "bg-violet-500/[0.07] before:absolute before:top-1 before:bottom-1 before:left-0 before:w-[3px] before:rounded-full before:bg-violet-500 hover:bg-violet-500/10",
                  )}
                >
                  <div className={cn("w-9 shrink-0", m.replyTo && "mt-6")}>
                    {head && m.sender && (
                      <button type="button" onClick={(e) => openProfile(e, m.sender!.id)} className="rounded-full transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none" title="Profili aç">
                        <UserAvatar userId={m.sender.id} name={m.sender.name} hasAvatar={m.sender.hasAvatar} version={m.sender.avatarVersion} className="size-9 shadow-sm ring-2 ring-card" fallbackClassName="bg-primary/10 text-xs text-primary" />
                      </button>
                    )}
                    {!head && <span className="hidden pt-1 text-[0.65rem] tabular-nums text-muted-foreground group-hover:block">{hhmm(m.createdAt)}</span>}
                  </div>
                  <div className="min-w-0 flex-1">
                    {m.replyTo && (
                      <button
                        type="button"
                        onClick={() => jump(m.replyTo!.id)}
                        title="Yanıtlanan mesaja git"
                        className="group/reply relative mb-1 flex h-5 w-fit max-w-[75%] items-center gap-1.5 text-left text-xs text-muted-foreground transition-colors hover:text-foreground"
                      >
                        <span className="pointer-events-none absolute top-1/2 -left-[30px] h-[14px] w-[24px] rounded-tl-lg border-t-2 border-l-2 border-border" />
                        <UserAvatar name={m.replyTo.sender} hasAvatar={false} className="size-4" fallbackClassName="bg-primary/10 text-[0.5rem] text-primary" />
                        <span className="shrink-0 font-medium text-foreground/80">{m.replyTo.sender}</span>
                        <span className={cn("truncate", m.replyTo.deleted && "italic")}>{m.replyTo.deleted ? "Bu mesaj silindi." : stripMarkup(m.replyTo.body) || "Ek"}</span>
                      </button>
                    )}
                    {head && m.sender && (
                      <div className="flex items-center gap-2">
                        <button type="button" onClick={(e) => openProfile(e, m.sender!.id)} className={cn("text-sm font-semibold leading-tight hover:underline", m.mine && "text-primary")}>{m.sender.name}</button>
                        <span className="text-[0.7rem] text-muted-foreground">{hhmm(m.createdAt)}</span>
                        {m.mine && !m.deleted && (() => {
                          const st = statusOf(m.id, selfId, seats);
                          return <Ticks status={st.status} readBy={st.readBy} size="size-3.5" className="-ml-0.5" />;
                        })()}
                      </div>
                    )}
                    {m.deleted ? (
                      <p className="text-sm italic text-muted-foreground">Bu mesaj silindi.</p>
                    ) : m.kind === "game" && m.gameId ? (
                      <GameCard gameId={m.gameId} onOpen={() => onOpenGame(m.gameId!)} />
                    ) : (
                      <div className="whitespace-pre-wrap break-words text-sm leading-relaxed">
                        {m.attachments?.length > 0 && (
                          <AttachmentGrid attachments={m.attachments} className={m.body ? "mt-1 mb-1.5" : "mt-1"} onOpen={(i) => setGallery({ items: mediaOf(m.attachments), index: i })} />
                        )}
                        {m.body && renderMarkup(m.body, { mentions: labels })}
                        {m.editedAt && <span className="ml-1.5 text-[0.65rem] text-muted-foreground" title={`Düzenlendi: ${new Date(m.editedAt).toLocaleString("tr-TR")}`}>(düzenlendi)</span>}
                      </div>
                    )}
                    {m.reactions.length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {m.reactions.map((r) => (
                          <button
                            key={r.emoji}
                            type="button"
                            onClick={() => void react(m, r.emoji)}
                            onContextMenu={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              setWho({ x: e.clientX, y: e.clientY, emoji: r.emoji, people: r.people ?? [] });
                            }}
                            title={`${r.names.join(", ")}
Sağ tık: kimler verdi`}
                            className={cn("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors", r.mine ? "border-primary/50 bg-primary/10" : "border-border/70 bg-muted/40 hover:bg-accent")}
                          >
                            <span>{r.emoji}</span>
                            <span className="tabular-nums">{r.count}</span>
                          </button>
                        ))}
                        <button type="button" onClick={() => setPicker(picker === m.id ? null : m.id)} title="Başka tepki" className="inline-flex items-center rounded-full border border-dashed border-border/70 px-1.5 text-muted-foreground hover:bg-accent"><SmilePlus className="size-3.5" /></button>
                      </div>
                    )}
                  </div>
                  {!m.deleted && (
                    <div className={cn("absolute -top-4 right-3 items-center gap-0.5 rounded-full border border-border/70 bg-card px-1 py-0.5 shadow-md group-hover:flex", picker === m.id ? "flex" : "hidden")}>
                      {QUICK.map((e) => (
                        <button key={e} type="button" onClick={() => void react(m, e)} title="Tepki ver" className={cn("rounded-md px-1 py-0.5 text-base leading-none hover:bg-accent", m.reactions.some((r) => r.emoji === e && r.mine) && "bg-primary/15")}>{e}</button>
                      ))}
                      <button type="button" onClick={() => setPicker(picker === m.id ? null : m.id)} title="Daha fazla tepki" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><SmilePlus className="size-4" /></button>
                      <span className="mx-0.5 h-4 w-px bg-border" />
                      {group.canPost && <button type="button" onClick={() => { setEditing(null); setReply(m); }} title="Yanıtla" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><CornerUpLeft className="size-4" /></button>}
                      {m.mine && group.canPost && <button type="button" onClick={() => { setReply(null); setEditing(m); }} title="Düzenle" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-4" /></button>}
                      {m.canDelete && <button type="button" onClick={() => setConfirmDelete(m)} title="Sil" className="rounded-md p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>}
                    </div>
                  )}
                  {picker === m.id && (
                    <div className="absolute top-5 right-3 z-10 grid grid-cols-6 gap-0.5 rounded-xl border border-border bg-popover p-1 shadow-lg">
                      {ALL.map((e) => (
                        <button key={e} type="button" onClick={() => void react(m, e)} className="rounded-lg px-1.5 py-1 text-lg hover:bg-accent">{e}</button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {moreNewer && (
        <div className="px-5 pb-1 text-center">
          <button type="button" onClick={() => void loadNewer()} className="rounded-full border border-border/70 px-3 py-1 text-xs text-muted-foreground hover:bg-accent">Daha yeni mesajlar</button>
        </div>
      )}
      <div className="flex h-5 items-center px-5 text-xs">
        {typing ? (
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <span className="flex gap-0.5">
              <span className="size-1 animate-bounce rounded-full bg-primary [animation-delay:-0.3s]" />
              <span className="size-1 animate-bounce rounded-full bg-primary [animation-delay:-0.15s]" />
              <span className="size-1 animate-bounce rounded-full bg-primary" />
            </span>
            <span className="italic">{typing}</span>
          </span>
        ) : error ? (
          <span className="text-destructive">{error}</span>
        ) : null}
      </div>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
      {info && <MessageInfo message={info} group={group} onClose={() => setInfo(null)} />}
      <ConfirmDialog
        open={!!confirmDelete}
        title="Mesajı sil"
        description={
          <>
            {confirmDelete?.mine ? "Mesajın" : `${confirmDelete?.sender?.name ?? "Bu kişinin"} mesajı`} herkes için silinecek; yerinde "Bu mesaj silindi." kalır
            {confirmDelete?.attachments?.length ? ", ekleri de Drive'dan kaldırılır" : ""}.
            {confirmDelete?.body && <span className="mt-2 block truncate rounded-lg bg-muted/40 px-2.5 py-1.5 text-xs text-foreground/80">{previewLabel(confirmDelete.body, confirmDelete.attachments)}</span>}
          </>
        }
        confirmLabel="Evet, sil"
        busy={deleting}
        onConfirm={() => confirmDelete && void remove(confirmDelete)}
        onCancel={() => setConfirmDelete(null)}
      />

      <Composer
        group={group}
        selfId={selfId}
        reply={reply}
        editing={editing}
        onCancelReply={() => setReply(null)}
        onCancelEdit={() => setEditing(null)}
        onSend={send}
        onEdit={edit}
        onEditLast={editLast}
        uploads={uploads}
      />
    </div>
  );
}

// MessageInfo: when the line was sent, received and read. A direct message
// reads as a short timeline; a group lists every seat with both times.
function MessageInfo({ message, group, onClose }: { message: TeamsMessage; group: TeamsGroupDetail; onClose: () => void }) {
  const [rows, setRows] = useState<TeamsReceipt[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    api
      .teamsReceipts(group.id, message.id)
      .then(setRows)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Bilgi alınamadı."));
  }, [group.id, message.id]);

  const stamp = (iso?: string) => {
    if (!iso) return null;
    const d = new Date(iso);
    const today = d.toDateString() === new Date().toDateString();
    const time = d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
    return today ? time : `${d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" })} ${time}`;
  };
  const sent = new Date(message.createdAt).toLocaleString("tr-TR", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
  const peer = group.kind === "dm" ? rows?.[0] : undefined;
  const sorted = (rows ?? []).slice().sort((a, b) => (b.readAt ? 2 : b.deliveredAt ? 1 : 0) - (a.readAt ? 2 : a.deliveredAt ? 1 : 0) || a.name.localeCompare(b.name, "tr"));

  const Step = ({ status, title, when, hint }: { status: "sent" | "delivered" | "read"; title: string; when?: string | null; hint: string }) => (
    <li className="flex items-center gap-3">
      <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-full", when ? "bg-success/15" : "bg-muted")}>
        <Ticks status={status} size="size-4" className={cn(!when && "opacity-50")} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{title}</span>
        <span className={cn("block text-xs", when ? "text-muted-foreground" : "text-muted-foreground/60 italic")}>{when ?? hint}</span>
      </span>
    </li>
  );

  return (
    <Modal open onClose={onClose} title="Mesaj bilgisi" description={`${message.sender?.name ?? ""} · ${sent}${message.editedAt ? " · düzenlendi" : ""}`} size="md">
      <div className="space-y-4">
        <div className="rounded-xl bg-muted/40 px-3 py-2 text-sm whitespace-pre-wrap break-words">{message.body ? renderMarkup(message.body) : <span className="text-muted-foreground">{previewLabel("", message.attachments)}</span>}</div>
        {error && <p className="text-xs text-destructive">{error}</p>}
        {rows === null && !error && <p className="text-xs text-muted-foreground">Yükleniyor...</p>}
        {rows && group.kind === "dm" && (
          <ul className="space-y-3">
            <Step status="sent" title="Gönderildi" when={stamp(message.createdAt)} hint="" />
            <Step status="delivered" title="Teslim edildi" when={stamp(peer?.deliveredAt)} hint="Henüz teslim edilmedi" />
            <Step status="read" title="Okundu" when={stamp(peer?.readAt)} hint="Henüz okunmadı" />
          </ul>
        )}
        {rows && group.kind !== "dm" && (
          <div className="overflow-hidden rounded-xl border border-border/60">
            <div className="grid grid-cols-[1fr_auto_auto] gap-x-4 border-b border-border/60 bg-muted/30 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
              <span>Kişi</span>
              <span className="w-16 text-right">Teslim</span>
              <span className="w-16 text-right">Okundu</span>
            </div>
            {sorted.length === 0 && <p className="px-3 py-3 text-sm text-muted-foreground">Odada başka kimse yok.</p>}
            {sorted.map((r) => (
              <div key={r.id} className="grid grid-cols-[1fr_auto_auto] items-center gap-x-4 border-b border-border/40 px-3 py-1.5 text-sm last:border-b-0">
                <span className="flex min-w-0 items-center gap-2">
                  <UserAvatar userId={r.id} name={r.name} hasAvatar={r.hasAvatar} version={r.avatarVersion} className="size-6" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                  <span className="truncate">{r.name}</span>
                  <Ticks status={r.readAt ? "read" : r.deliveredAt ? "delivered" : "sent"} size="size-3.5" />
                </span>
                <span className={cn("w-16 text-right text-xs tabular-nums", r.deliveredAt ? "text-muted-foreground" : "text-muted-foreground/40")}>{stamp(r.deliveredAt) ?? "—"}</span>
                <span className={cn("w-16 text-right text-xs tabular-nums", r.readAt ? "text-success" : "text-muted-foreground/40")}>{stamp(r.readAt) ?? "—"}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}

// ReactionPeople: who gave one emoji, opened by right-clicking the chip.
function ReactionPeople({ x, y, emoji, people, selfId, onPerson, onClose }: { x: number; y: number; emoji: string; people: TeamsPerson[]; selfId: number; onPerson: (p: TeamsPerson, x: number, y: number) => void; onClose: () => void }) {
  useEffect(() => {
    const close = () => onClose();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const t = window.setTimeout(() => {
      window.addEventListener("mousedown", close);
      window.addEventListener("keydown", onKey);
    }, 0);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);
  const left = Math.min(x, window.innerWidth - 240);
  const top = Math.min(y, window.innerHeight - 40 - people.length * 40);
  return (
    <div className="animate-in fade-in zoom-in-95 fixed z-[60] w-56 overflow-hidden rounded-xl border border-border bg-popover p-1 shadow-lg duration-100" style={{ left, top }} onMouseDown={(e) => e.stopPropagation()}>
      <p className="flex items-center gap-1.5 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
        <span className="text-base leading-none">{emoji}</span> {people.length} kişi
      </p>
      {people.map((p) => (
        <button
          key={p.id}
          type="button"
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            onPerson(p, r.right, r.top);
          }}
          className="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-accent"
        >
          <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
          <span className="min-w-0 flex-1 truncate">{p.name}{p.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</span>
        </button>
      ))}
      {people.length === 0 && <p className="px-2 py-2 text-xs text-muted-foreground">Kimse yok</p>}
    </div>
  );
}
