// Teams: in-house chat. Left, the rooms (groups and direct messages) with
// unread counts and pending invites; middle, the open room; right, its
// members: a three-column chat.

import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { BellOff, BellRing, Gamepad2, Hash, Images, Loader2, Plus, Search, Settings2, Trophy, Users, VolumeX, X } from "lucide-react";
import StartGameDialog from "../games/StartGameDialog";
import Leaderboard from "../games/Leaderboard";
import GameModal from "../games/GameModal";
import { api, ApiError } from "../api/client";
import type { TeamsGroup, TeamsGroupDetail, TeamsInvite } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { ContextMenu, type MenuItem } from "../components/ContextMenu";
import GroupAvatar from "../components/teams/GroupAvatar";
import { AddPeopleDialog, GroupSettingsDialog, NewDMDialog, NewGroupDialog } from "../components/teams/GroupDialogs";
import RoomPanel, { type PanelMode } from "../components/teams/RoomPanel";
import ProfilePopover, { type PopoverAnchor } from "../components/teams/ProfilePopover";
import MessagePane from "../components/teams/MessagePane";
import { OnlineDot, presenceTone, seenLabel, Ticks, type Presence } from "../components/teams/Presence";
import { previewLabel } from "../lib/attachments";
import { Badge, Button, ConfirmDialog } from "../components/ui";
import { can } from "../lib/permissions";
import { cn } from "../lib/utils";
import { useTeams } from "../teams/TeamsContext";
import { clockTime, isToday, shortDate } from "../lib/time";

function when(iso: string) {
  return isToday(iso) ? clockTime(iso) : shortDate(iso);
}

// RoomRow is one room in the left column. It lives outside the page so a
// typing or presence event re-renders the rows whose props changed and
// never remounts them: keyboard focus stays where it was.
const RoomRow = memo(function RoomRow({
  g,
  active,
  typing,
  presence,
  busy,
  onOpen,
  onMenu,
}: {
  g: TeamsGroup;
  active: boolean;
  typing: string | null;
  presence: Presence | null;
  busy: boolean;
  onOpen: (id: number) => void;
  onMenu: (e: React.MouseEvent, g: TeamsGroup) => void;
}) {
  const last = g.lastMessage;
  return (
    <button
      type="button"
      onClick={() => onOpen(g.id)}
      onContextMenu={(e) => onMenu(e, g)}
      aria-busy={busy || undefined}
      className={cn("flex w-full items-center gap-3 rounded-2xl px-2.5 py-2 text-left transition-[background-color,box-shadow,opacity] duration-200", active ? "bg-card shadow-sm ring-1 ring-border/60" : "hover:bg-card/60", busy && "opacity-70")}
    >
      <span className="relative inline-flex shrink-0">
        <GroupAvatar group={g} className="size-11 text-sm" />
        {g.kind === "dm" && presence && <OnlineDot presence={presence} />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className={cn("min-w-0 flex-1 truncate text-sm", g.unread && !g.muted ? "font-semibold text-foreground" : "font-medium")}>{g.name}</span>
          {g.muted && <VolumeX className={cn("size-3 shrink-0", g.mute === "all" ? "text-destructive/70" : "text-muted-foreground")} aria-label={g.mute === "all" ? "Tamamen sessiz" : "Sessiz, etiketler bildirir"} />}
          {busy ? (
            <Loader2 className="size-3 shrink-0 animate-spin text-muted-foreground" aria-label="İşleniyor" />
          ) : (
            last && <span className="shrink-0 text-[0.65rem] tabular-nums text-muted-foreground">{when(last.createdAt)}</span>
          )}
        </span>
        <span className="flex items-center gap-1.5">
          {typing ? (
            <span className="min-w-0 flex-1 truncate text-xs italic text-primary">{typing}</span>
          ) : (
            <>
              {last?.mine && !last.deleted && <Ticks status={last.status} size="size-4" />}
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {last ? (last.deleted ? "Bu mesaj silindi." : last.kind === "system" ? last.body : `${g.kind === "dm" ? (last.mine ? "Sen" : "") : (last.sender?.name.split(" ")[0] ?? "")}${g.kind === "dm" && !last.mine ? "" : ": "}${previewLabel(last.body, last.attachments, last.kind)}`) : "Henüz mesaj yok"}
              </span>
            </>
          )}
          {g.unread > 0 && (
            <span className={cn("shrink-0 rounded-full px-1.5 py-0.5 text-[0.6rem] font-semibold tabular-nums leading-none", g.muted ? "bg-muted text-muted-foreground" : "bg-primary text-primary-foreground shadow-sm shadow-primary/30")}>{g.unread > 99 ? "99+" : g.unread}</span>
          )}
        </span>
      </span>
    </button>
  );
}, (a, b) =>
  a.g === b.g &&
  a.active === b.active &&
  a.typing === b.typing &&
  a.busy === b.busy &&
  a.onOpen === b.onOpen &&
  a.onMenu === b.onMenu &&
  a.presence?.online === b.presence?.online &&
  a.presence?.here === b.presence?.here &&
  a.presence?.lastSeen === b.presence?.lastSeen);

// InviteCard is one pending invite with its two answers. While an answer
// is on its way both buttons wait; a failure is shown on the card.
function InviteCard({ inv, onDone }: { inv: TeamsInvite; onDone: (accepted: boolean) => Promise<void> }) {
  const [busy, setBusy] = useState<"accept" | "decline" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const decide = async (decision: "accept" | "decline") => {
    setBusy(decision);
    setError(null);
    try {
      await api.teamsDecideInvite(inv.id, decision);
      await onDone(decision === "accept");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : decision === "accept" ? "Gruba katılamadın, tekrar dene." : "Davet reddedilemedi, tekrar dene.");
      setBusy(null);
    }
  };
  return (
    <div className="rounded-xl border border-primary/30 bg-primary/5 p-2.5" aria-busy={!!busy || undefined}>
      <div className="flex items-center gap-2">
        <GroupAvatar group={{ id: inv.groupId, kind: "group", name: inv.groupName, hasAvatar: inv.hasAvatar, avatarVersion: inv.avatarVersion }} className="size-8 text-xs" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{inv.groupName}</span>
          <span className="block truncate text-xs text-muted-foreground">{inv.invitedBy?.name ?? "Biri"} davet etti</span>
        </span>
      </div>
      <div className="mt-2 flex gap-1.5">
        <Button onClick={() => void decide("accept")} disabled={!!busy} className="h-7 flex-1 text-xs">{busy === "accept" ? "Katılıyorsun..." : "Katıl"}</Button>
        <Button variant="secondary" onClick={() => void decide("decline")} disabled={!!busy} className="h-7 flex-1 text-xs">{busy === "decline" ? "Reddediliyor..." : "Reddet"}</Button>
      </div>
      {error && <p role="alert" className="mt-1.5 text-xs text-destructive">{error}</p>}
    </div>
  );
}

export function Teams() {
  const { id } = useParams();
  const [search, setSearch] = useSearchParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const teams = useTeams();
  const selfId = user?.id ?? 0;
  const groupId = id ? Number(id) : null;

  const [detail, setDetail] = useState<TeamsGroupDetail | null>(null);
  const [detailError, setDetailError] = useState<{ id: number; text: string } | null>(null);
  const [panel, setPanel] = useState<PanelMode | null>("members");
  const [target, setTarget] = useState<{ id: number; nonce: number } | null>(null);
  const [plus, setPlus] = useState<{ x: number; y: number } | null>(null);
  const [q, setQ] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  // The group about to be left, waiting for a yes.
  const [leaving, setLeaving] = useState<TeamsGroup | null>(null);
  const [leaveBusy, setLeaveBusy] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);

  const leave = async () => {
    if (!leaving) return;
    setLeaveBusy(true);
    setLeaveError(null);
    try {
      await api.teamsRemoveMember(leaving.id, selfId);
      if (groupId === leaving.id) navigate("/teams");
      setLeaving(null);
      void teams.refresh();
    } catch (e) {
      setLeaveError(e instanceof ApiError ? e.message : "Gruptan ayrılamadın.");
    } finally {
      setLeaveBusy(false);
    }
  };
  const [newGroup, setNewGroup] = useState(false);
  const [newDM, setNewDM] = useState(false);
  const [settings, setSettings] = useState(false);
  const [people, setPeople] = useState<"add" | "invite" | null>(null);
  const [profile, setProfile] = useState<PopoverAnchor | null>(null);

  const canCreate = can(user, "teams.group_create");
  const canPlay = can(user, "games.play") && !!teams.games?.enabled;
  const [startGame, setStartGame] = useState(false);
  const [openGame, setOpenGame] = useState<number | null>(null);
  useEffect(() => {
    const q = Number(search.get("game"));
    if (q > 0) {
      setOpenGame(q);
      setSearch({}, { replace: true });
    }
  }, [search, setSearch]);
  const [board, setBoard] = useState(false);

  // Tell the context which room is open so its messages do not notify.
  const { setOpenGroupId, notifications, askNotifications } = teams;
  useEffect(() => {
    setOpenGroupId(groupId);
    return () => setOpenGroupId(null);
  }, [groupId, setOpenGroupId]);

  useEffect(() => {
    if (notifications === "default") void askNotifications();
  }, [notifications, askNotifications]);

  // The room shown follows the address; an answer for a room left in the
  // meantime is dropped, and a message to jump to belongs to the old room.
  // Until the room in the address has loaded, the old one is not shown at
  // all, so nothing typed can go to it.
  const shown = useRef(groupId);
  shown.current = groupId;
  useEffect(() => setTarget(null), [groupId]);
  const loadDetail = useCallback(() => {
    if (!groupId) {
      setDetail(null);
      return;
    }
    api
      .teamsGroup(groupId)
      .then((g) => {
        if (shown.current !== groupId) return;
        setDetail(g);
        setDetailError(null);
      })
      .catch((e) => {
        if (shown.current !== groupId) return;
        setDetail(null);
        setDetailError({ id: groupId, text: e instanceof ApiError ? e.message : "Grup açılamadı." });
      });
  }, [groupId]);
  const current = detail && detail.id === groupId ? detail : null;
  const currentError = detailError && detailError.id === groupId ? detailError.text : null;
  // A changed room from a panel or dialog replaces the open one only if it
  // is still the room in the address.
  const onChanged = useCallback((g: TeamsGroupDetail) => {
    if (g.id === shown.current) setDetail(g);
  }, []);

  useEffect(loadDetail, [loadDetail]);

  // A room change pushed by the server (members, name, photo) refreshes the open room.
  useEffect(() => teams.subscribe((e) => {
    if (e.type === "group" && e.groupId === groupId) loadDetail();
  }), [teams, groupId, loadDetail]);

  const groups = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase("tr");
    return teams.groups.filter((g) => !needle || g.name.toLocaleLowerCase("tr").includes(needle));
  }, [teams.groups, q]);
  const rooms = groups.filter((g) => g.kind === "group");
  const dms = groups.filter((g) => g.kind === "dm");

  // A room menu action runs one at a time per room: the row shows it is
  // working, the menu's items wait, and a failure is said above the list.
  const [roomBusy, setRoomBusy] = useState<number | null>(null);
  const [roomError, setRoomError] = useState<string | null>(null);
  const roomAct = useCallback(
    async (id: number, work: () => Promise<unknown>, fail: string) => {
      setRoomBusy(id);
      setRoomError(null);
      try {
        await work();
        await teams.refresh();
      } catch (e) {
        setRoomError(e instanceof ApiError ? e.message : fail);
      } finally {
        setRoomBusy((cur) => (cur === id ? null : cur));
      }
    },
    [teams],
  );

  const roomMenu = (e: React.MouseEvent, g: TeamsGroup) => {
    e.preventDefault();
    const busy = roomBusy === g.id;
    const items: MenuItem[] = [
      g.unread > 0
        ? { label: "Okundu olarak işaretle", disabled: busy, onClick: () => void roomAct(g.id, () => api.teamsMarkRead(g.id, 0), "Okundu olarak işaretlenemedi.") }
        : {
            label: "Okunmadı olarak işaretle",
            disabled: busy,
            onClick: () =>
              void roomAct(
                g.id,
                async () => {
                  await api.teamsMarkUnread(g.id);
                  if (shown.current === g.id) navigate("/teams");
                },
                "Okunmadı olarak işaretlenemedi.",
              ),
          },
      ...(g.mute !== "none" ? [{ label: "Sesi aç", disabled: busy, onClick: () => void roomAct(g.id, () => api.teamsMute(g.id, "none"), "Ses açılamadı.") }] : []),
      ...(g.mute !== "mentions" ? [{ label: "Sessize al (yalnızca etiketler bildirir)", disabled: busy, onClick: () => void roomAct(g.id, () => api.teamsMute(g.id, "mentions"), "Sessize alınamadı.") }] : []),
      ...(g.mute !== "all" ? [{ label: "Tamamen sessize al", disabled: busy, onClick: () => void roomAct(g.id, () => api.teamsMute(g.id, "all"), "Sessize alınamadı.") }] : []),
    ];
    if (g.kind === "group" && g.myRole !== "owner") items.push({ label: "Gruptan ayrıl", danger: true, disabled: busy, onClick: () => { setLeaveError(null); setLeaving(g); } });
    setMenu({ x: e.clientX, y: e.clientY, items });
  };
  // The rows get callbacks that never change, so a row re-renders only when
  // its own room does.
  const menuRef = useRef(roomMenu);
  menuRef.current = roomMenu;
  const onRoomMenu = useCallback((e: React.MouseEvent, g: TeamsGroup) => menuRef.current(e, g), []);
  const onOpenRoom = useCallback((id: number) => navigate(`/teams/${id}`), [navigate]);
  const row = (g: TeamsGroup) => (
    <RoomRow
      key={g.id}
      g={g}
      active={g.id === groupId}
      typing={teams.typingLabel(g.id)}
      presence={g.kind === "dm" ? teams.presenceOf(g.peer, g.id) : null}
      busy={roomBusy === g.id}
      onOpen={onOpenRoom}
      onMenu={onRoomMenu}
    />
  );
  const onInviteDone = useCallback(
    async (gid: number, accepted: boolean) => {
      await teams.refresh();
      if (accepted) navigate(`/teams/${gid}`);
    },
    [teams, navigate],
  );

  return (
    <div className="-mx-4 -my-6 flex h-[calc(100svh-4rem)] overflow-hidden md:-mx-6 lg:-mx-8">
      {/* Rooms */}
      <aside className="flex w-[19rem] shrink-0 flex-col border-r border-border/50 bg-gradient-to-b from-card/70 to-card/30">
        <div className="flex items-center justify-between gap-2 px-3 pt-3 pb-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Sohbet ara" className="h-9 w-full rounded-full border border-transparent bg-muted/60 pl-9 pr-3 text-sm outline-none transition-[background-color,box-shadow] placeholder:text-muted-foreground/60 focus:border-ring/40 focus:bg-card focus:ring-4 focus:ring-ring/15" />
          </div>
          <button
            type="button"
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              setPlus({ x: r.left - 150, y: r.bottom + 4 });
            }}
            aria-label="Yeni" data-tip="Yeni"
            className={cn("flex size-9 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-md shadow-primary/30 transition-transform hover:scale-105", plus && "scale-95")}
          >
            <Plus className="size-4" />
          </button>
        </div>

        {roomError && (
          <div role="alert" className="mx-3 mb-2 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-xs text-destructive">
            <span className="min-w-0 flex-1">{roomError}</span>
            <button type="button" onClick={() => setRoomError(null)} aria-label="Uyarıyı kapat" className="shrink-0 rounded p-0.5 hover:bg-destructive/10"><X className="size-3.5" /></button>
          </div>
        )}

        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {teams.invites.length > 0 && (
            <div className="mb-3 space-y-1.5">
              <p className="px-2 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Davetler</p>
              {teams.invites.map((inv) => (
                <InviteCard key={inv.id} inv={inv} onDone={(accepted) => onInviteDone(inv.groupId, accepted)} />
              ))}
            </div>
          )}

          <p className="px-2.5 pb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/60">Gruplar</p>
          <div className="space-y-0.5">
            {rooms.map(row)}
            {rooms.length === 0 && <p className="px-2 py-3 text-xs text-muted-foreground">{canCreate ? "Henüz grup yok. Artı ile bir grup aç." : "Henüz bir gruba eklenmedin."}</p>}
          </div>

          <p className="px-2.5 pt-5 pb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/60">Özel mesajlar</p>
          <div className="space-y-0.5">
            {dms.map(row)}
            {dms.length === 0 && <p className="px-2 py-3 text-xs text-muted-foreground">Kimseyle yazışmadın. Yeni mesaj ile başla.</p>}
          </div>
        </div>

        <div className="flex items-center justify-between border-t border-border/50 px-3 py-2 text-[0.65rem] text-muted-foreground">
          {teams.games && (
            <button type="button" onClick={() => setBoard(true)} className="inline-flex items-center gap-1 hover:text-foreground" data-tip="Oyun sıralaması"><Trophy className="size-3 text-warning" /> Sıralama</button>
          )}
          {teams.notifications === "granted" ? (
            <span className="inline-flex items-center gap-1"><BellRing className="size-3 text-success" /> Bildirimler açık</span>
          ) : teams.notifications === "denied" ? (
            <span className="inline-flex items-center gap-1"><BellOff className="size-3" /> Tarayıcı bildirimleri kapalı, sadece ses</span>
          ) : (
            <button type="button" onClick={() => void teams.askNotifications()} className="inline-flex items-center gap-1 hover:text-foreground"><BellRing className="size-3" /> Bildirimlere izin ver</button>
          )}
        </div>
      </aside>

      {/* Room */}
      <section className="flex min-w-0 flex-1 flex-col bg-background">
        {!groupId ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <span className="flex size-20 items-center justify-center rounded-[1.75rem] bg-gradient-to-br from-primary/20 to-violet-500/20 text-primary shadow-inner"><Users className="size-8" /></span>
            <p className="text-sm font-medium">Bir grup ya da kişi seç</p>
            <p className="max-w-xs text-xs text-muted-foreground">Soldan bir sohbet aç. Yeni mesaj ile bir kişiye yaz{canCreate ? ", artı ile bir grup oluştur" : ""}.</p>
          </div>
        ) : currentError ? (
          <div className="flex h-full items-center justify-center text-sm text-destructive">{currentError}</div>
        ) : !current ? (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground" aria-busy="true">
            <Loader2 className="size-4 animate-spin" /> Yükleniyor...
          </div>
        ) : (
          <>
            <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border/50 bg-card/60 px-4 backdrop-blur-md">
              <button
                type="button"
                disabled={current.kind !== "dm" || !current.peer}
                onClick={(e) => {
                  if (!current.peer) return;
                  const r = e.currentTarget.getBoundingClientRect();
                  setProfile({ userId: current.peer.id, x: r.left, y: r.bottom, room: current.id });
                }}
                className="relative inline-flex shrink-0 rounded-full disabled:cursor-default"
                aria-label={current.kind === "dm" ? "Profili aç" : undefined} data-tip={current.kind === "dm" ? "Profili aç" : undefined}
              >
                <GroupAvatar group={current} className="size-10 text-sm" />
                {current.kind === "dm" && <OnlineDot presence={teams.presenceOf(current.peer, current.id)} />}
              </button>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  {current.kind === "group" && <Hash className="size-4 shrink-0 text-muted-foreground" />}
                  <h2 className="truncate text-[0.9375rem] font-semibold tracking-tight">{current.name}</h2>
                  {current.postPolicy === "admins" && <Badge tone="amber">Duyuru</Badge>}
                  {current.muted && <VolumeX className="size-3.5 text-muted-foreground" />}
                </div>
                {(() => {
                  const typing = teams.typingLabel(current.id);
                  if (typing) return <p className="truncate text-xs italic text-primary">{typing}</p>;
                  if (current.kind === "dm") {
                    const p = teams.presenceOf(current.peer, current.id);
                    return <p className={cn("truncate text-xs", presenceTone(p))}>{p.here ? "Sohbette · aynı sohbettesin" : seenLabel(p)}</p>;
                  }
                  const on = current.members.filter((m) => teams.presenceOf(m, current.id).online).length;
                  const here = current.members.filter((m) => teams.presenceOf(m, current.id).here).length;
                  return <p className="truncate text-xs text-muted-foreground">{current.description ? `${current.description} · ` : ""}{current.memberCount} üye · {on} çevrimiçi{here ? ` · ${here} sohbette` : ""}</p>;
                })()}
              </div>
              {canPlay && current.canPost && (
                <button type="button" onClick={() => setStartGame(true)} aria-label="Oyun başlat" data-tip="Oyun başlat" className="flex size-9 items-center justify-center rounded-full text-violet-500 transition-colors hover:bg-violet-500/10"><Gamepad2 className="size-4" /></button>
              )}
              <button type="button" onClick={() => setPanel((p) => (p === "search" ? null : "search"))} aria-label="Mesajlarda ara" data-tip="Mesajlarda ara" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "search" && "bg-primary/10 text-primary")}><Search className="size-4" /></button>
              <button type="button" onClick={() => setPanel((p) => (p === "media" ? null : "media"))} aria-label="Görseller, videolar ve dosyalar" data-tip="Görseller, videolar ve dosyalar" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "media" && "bg-primary/10 text-primary")}><Images className="size-4" /></button>
              {current.kind === "group" && (
                <>
                  <button type="button" onClick={() => setPanel((p) => (p === "members" ? null : "members"))} aria-label="Üyeler" data-tip="Üyeler" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "members" && "bg-primary/10 text-primary")}><Users className="size-4" /></button>
                  {current.canManage && (
                    <button type="button" onClick={() => setSettings(true)} aria-label="Grup ayarları" data-tip="Grup ayarları" className="flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"><Settings2 className="size-4" /></button>
                  )}
                </>
              )}
            </header>
            <div className="flex min-h-0 flex-1">
              <MessagePane key={current.id} group={current} selfId={selfId} target={target} onOpenGame={setOpenGame} />
              {panel && (panel !== "members" || current.kind === "group") && (
                <RoomPanel
                  key={current.id}
                  mode={panel}
                  group={current}
                  selfId={selfId}
                  onChanged={onChanged}
                  onAdd={() => setPeople("add")}
                  onInvite={() => setPeople("invite")}
                  onJump={(id) => setTarget({ id, nonce: Date.now() })}
                  onClose={() => setPanel(null)}
                />
              )}
            </div>
          </>
        )}
      </section>

      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
      <ConfirmDialog
        open={!!leaving}
        title="Gruptan ayrılınsın mı?"
        description={`${leaving?.name ?? ""} grubundaki mesajları artık göremezsin. Geri dönmek için birinin seni yeniden eklemesi gerekir.`}
        confirmLabel="Ayrıl"
        busy={leaveBusy}
        error={leaveError}
        onCancel={() => setLeaving(null)}
        onConfirm={() => void leave()}
      />
      {plus && (
        <ContextMenu
          x={plus.x}
          y={plus.y}
          items={[
            ...(canCreate ? [{ label: "＋  Grup oluştur", onClick: () => setNewGroup(true) }] : []),
            { label: "✉  Kişiye mesaj yaz", onClick: () => setNewDM(true) },
            ...(canPlay && current ? [{ label: "🎮  Oyun başlat", onClick: () => setStartGame(true) }] : []),
            ...(teams.games ? [{ label: "🏆  Oyun sıralaması", onClick: () => setBoard(true) }] : []),
          ]}
          onClose={() => setPlus(null)}
        />
      )}
      {profile && <ProfilePopover anchor={profile} selfId={selfId} onClose={() => setProfile(null)} />}
      <NewGroupDialog open={newGroup} onClose={() => setNewGroup(false)} onCreated={(g) => { void teams.refresh(); navigate(`/teams/${g.id}`); }} />
      <NewDMDialog open={newDM} onClose={() => setNewDM(false)} selfId={selfId} onOpened={(g) => { void teams.refresh(); navigate(`/teams/${g.id}`); }} />
      {current && <GroupSettingsDialog group={current} open={settings} onClose={() => setSettings(false)} onSaved={(g) => { onChanged(g); void teams.refresh(); }} />}
      {current && teams.games && <StartGameDialog groupId={current.id} config={teams.games} open={startGame} onClose={() => setStartGame(false)} onCreated={(g) => { teams.reloadGames(); setOpenGame(g.id); }} onOpen={setOpenGame} />}
      {openGame && <GameModal gameId={openGame} selfId={selfId} metas={teams.games?.kinds ?? []} pauseOnCall={teams.games?.pauseOnCall ?? true} members={current?.members.map((m) => m.id) ?? []} onClose={() => setOpenGame(null)} />}
      {teams.games && <Leaderboard open={board} onClose={() => setBoard(false)} kinds={teams.games.kinds} selfId={selfId} />}
      {current && people && <AddPeopleDialog group={current} mode={people} open onClose={() => setPeople(null)} onDone={(g) => { onChanged(g); void teams.refresh(); }} />}
    </div>
  );
}
