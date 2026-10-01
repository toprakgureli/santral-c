// MessagePane: the room's history and composer. Messages group by sender
// and minute, days are separated, a "Yeni mesajlar" line marks where the
// unread ones start, deleted lines read "Bu mesaj silindi.", reactions sit
// under the line and the quick ones are one hover away. A reply quotes
// the original and jumps to it on click. Lines can be edited by their
// author. Files dropped, pasted or picked go to Google Drive and show as a
// grid of previews with a lightbox.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ImagePlus } from "lucide-react";
import Lightbox from "@/components/teams/Lightbox";
import ProfilePopover, { type PopoverAnchor } from "@/components/teams/ProfilePopover";
import { useUploads } from "@/teams/useUploads";
import type { TeamsAttachment, TeamsPerson } from "@/api/types";
import Composer, { type Outgoing } from "@/components/teams/Composer";
import { ContextMenu, type MenuItem } from "@/components/ContextMenu";
import { ConfirmDialog } from "@/components/ui";
import { api, ApiError } from "@/api/client";
import type { TeamsEvent, TeamsGroupDetail, TeamsMessage } from "@/api/types";
import { previewLabel } from "@/lib/attachments";
import { useTeams } from "@/teams/TeamsContext";
import { dayKey, dayName } from "@/lib/time";
import MessageRow, { type Seats } from "@/components/teams/MessageRow";
import { MessageInfo, ReactionPeople } from "@/components/teams/MessageInfo";

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

  // One page request at a time: the scroll handler fires again and again
  // while the edge is in view, and each request would add the same page.
  const paging = useRef(false);
  // The room a reply belongs to; a reply for a room left meanwhile is dropped.
  const room = useRef(group.id);
  room.current = group.id;

  async function loadOlder() {
    if (!items.length || paging.current) return;
    paging.current = true;
    const from = group.id;
    const el = list.current;
    const before = el ? el.scrollHeight - el.scrollTop : 0;
    try {
      const r = await api.teamsMessages(from, { before: items[0].id });
      if (room.current !== from) return;
      setItems((cur) => withoutKnown(r.items, cur).concat(cur));
      setMore(r.more);
      stickToBottom.current = false;
      requestAnimationFrame(() => {
        if (el) el.scrollTop = el.scrollHeight - before;
      });
    } catch {
      // keep what we have; scrolling up again retries
    } finally {
      paging.current = false;
    }
  }

  async function loadNewer() {
    if (!items.length || !moreNewer || paging.current) return;
    paging.current = true;
    const from = group.id;
    try {
      const r = await api.teamsMessages(from, { after: items[items.length - 1].id });
      if (room.current !== from) return;
      setItems((cur) => cur.concat(withoutKnown(r.items, cur)));
      setMoreNewer(r.moreNewer);
      stickToBottom.current = false;
      if (!r.moreNewer && r.items.length) void api.teamsMarkRead(from, r.items[r.items.length - 1].id).catch(() => undefined);
    } catch {
      // keep what we have; scrolling down again retries
    } finally {
      paging.current = false;
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
      const day = dayKey(m.createdAt);
      const newDay = day !== lastDay;
      lastDay = day;
      out.push({ m, head: true, day: newDay ? dayName(m.createdAt) : null });
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
        {rows.map(({ m, head, day }) => (
          <MessageRow
            key={m.id}
            m={m}
            head={head}
            day={day}
            unread={firstUnread === m.id}
            selfId={selfId}
            seats={seats}
            flash={flash === m.id}
            editing={editing?.id === m.id}
            picking={picker === m.id}
            canPost={group.canPost}
            onMenu={lineMenu}
            onProfile={openProfile}
            onJump={jump}
            onReact={(msg, e) => void react(msg, e)}
            onPicker={() => setPicker(picker === m.id ? null : m.id)}
            onReply={(msg) => {
              setEditing(null);
              setReply(msg);
            }}
            onEdit={(msg) => {
              setReply(null);
              setEditing(msg);
            }}
            onDelete={setConfirmDelete}
            onWho={setWho}
            onGallery={(items, index) => setGallery({ items, index })}
            onOpenGame={onOpenGame}
          />
        ))}
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
// withoutKnown drops the lines already in the list, so a page that overlaps
// what is shown never repeats a message.
function withoutKnown(page: TeamsMessage[], cur: TeamsMessage[]): TeamsMessage[] {
  const known = new Set(cur.map((m) => m.id));
  return page.filter((m) => !known.has(m.id));
}
