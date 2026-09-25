// Teams: in-house chat. Left, the rooms (groups and direct messages) with
// unread counts and pending invites; middle, the open room; right, its
// members: a three-column chat.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { BellOff, BellRing, Gamepad2, Hash, Images, Plus, Search, Settings2, Trophy, Users, VolumeX } from "lucide-react";
import StartGameDialog from "../games/StartGameDialog";
import Leaderboard from "../games/Leaderboard";
import GameModal from "../games/GameModal";
import { api, ApiError } from "../api/client";
import type { TeamsGroup, TeamsGroupDetail } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { ContextMenu, type MenuItem } from "../components/ContextMenu";
import GroupAvatar from "../components/teams/GroupAvatar";
import { AddPeopleDialog, GroupSettingsDialog, NewDMDialog, NewGroupDialog } from "../components/teams/GroupDialogs";
import RoomPanel, { type PanelMode } from "../components/teams/RoomPanel";
import ProfilePopover, { type PopoverAnchor } from "../components/teams/ProfilePopover";
import MessagePane from "../components/teams/MessagePane";
import { OnlineDot, presenceTone, seenLabel, Ticks } from "../components/teams/Presence";
import { previewLabel } from "../lib/attachments";
import { Badge, Button } from "../components/ui";
import { can } from "../lib/permissions";
import { cn } from "../lib/utils";
import { useTeams } from "../teams/TeamsContext";

function when(iso: string) {
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  return d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit" });
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
  const [detailError, setDetailError] = useState<string | null>(null);
  const [panel, setPanel] = useState<PanelMode | null>("members");
  const [target, setTarget] = useState<{ id: number; nonce: number } | null>(null);
  const [plus, setPlus] = useState<{ x: number; y: number } | null>(null);
  const [q, setQ] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
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
  useEffect(() => {
    teams.setOpenGroupId(groupId);
    return () => teams.setOpenGroupId(null);
  }, [groupId]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (teams.notifications === "default") void teams.askNotifications();
  }, [teams.notifications]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadDetail = useCallback(() => {
    if (!groupId) {
      setDetail(null);
      return;
    }
    api
      .teamsGroup(groupId)
      .then((g) => {
        setDetail(g);
        setDetailError(null);
      })
      .catch((e) => {
        setDetail(null);
        setDetailError(e instanceof ApiError ? e.message : "Grup açılamadı.");
      });
  }, [groupId]);

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

  function roomMenu(e: React.MouseEvent, g: TeamsGroup) {
    e.preventDefault();
    const items: MenuItem[] = [
      g.unread > 0
        ? { label: "Okundu olarak işaretle", onClick: () => void api.teamsMarkRead(g.id, 0).then(() => teams.refresh()) }
        : {
            label: "Okunmadı olarak işaretle",
            onClick: () =>
              void api.teamsMarkUnread(g.id).then(() => {
                if (groupId === g.id) navigate("/teams");
                return teams.refresh();
              }),
          },
      ...(g.mute !== "none" ? [{ label: "Sesi aç", onClick: () => void api.teamsMute(g.id, "none").then(() => teams.refresh()) }] : []),
      ...(g.mute !== "mentions" ? [{ label: "Sessize al (yalnızca etiketler bildirir)", onClick: () => void api.teamsMute(g.id, "mentions").then(() => teams.refresh()) }] : []),
      ...(g.mute !== "all" ? [{ label: "Tamamen sessize al", onClick: () => void api.teamsMute(g.id, "all").then(() => teams.refresh()) }] : []),
    ];
    if (g.kind === "group" && g.myRole !== "owner") items.push({ label: "Gruptan ayrıl", danger: true, onClick: () => void api.teamsRemoveMember(g.id, selfId).then(() => { teams.refresh(); if (groupId === g.id) navigate("/teams"); }) });
    setMenu({ x: e.clientX, y: e.clientY, items });
  }

  const Room = ({ g }: { g: TeamsGroup }) => {
    const active = g.id === groupId;
    const last = g.lastMessage;
    const typing = teams.typingLabel(g.id);
    return (
      <button
        type="button"
        onClick={() => navigate(`/teams/${g.id}`)}
        onContextMenu={(e) => roomMenu(e, g)}
        className={cn("flex w-full items-center gap-3 rounded-2xl px-2.5 py-2 text-left transition-[background-color,box-shadow] duration-200", active ? "bg-card shadow-sm ring-1 ring-border/60" : "hover:bg-card/60")}
      >
        <span className="relative inline-flex shrink-0">
          <GroupAvatar group={g} className="size-11 text-sm" />
          {g.kind === "dm" && <OnlineDot presence={teams.presenceOf(g.peer, g.id)} />}
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1.5">
            <span className={cn("min-w-0 flex-1 truncate text-sm", g.unread && !g.muted ? "font-semibold text-foreground" : "font-medium")}>{g.name}</span>
            {g.muted && <VolumeX className={cn("size-3 shrink-0", g.mute === "all" ? "text-destructive/70" : "text-muted-foreground")} aria-label={g.mute === "all" ? "Tamamen sessiz" : "Sessiz, etiketler bildirir"} />}
            {last && <span className="shrink-0 text-[0.65rem] tabular-nums text-muted-foreground">{when(last.createdAt)}</span>}
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
  };

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
            data-tip="Yeni"
            className={cn("flex size-9 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-md shadow-primary/30 transition-transform hover:scale-105", plus && "scale-95")}
          >
            <Plus className="size-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {teams.invites.length > 0 && (
            <div className="mb-3 space-y-1.5">
              <p className="px-2 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Davetler</p>
              {teams.invites.map((inv) => (
                <div key={inv.id} className="rounded-xl border border-primary/30 bg-primary/5 p-2.5">
                  <div className="flex items-center gap-2">
                    <GroupAvatar group={{ id: inv.groupId, kind: "group", name: inv.groupName, hasAvatar: inv.hasAvatar, avatarVersion: inv.avatarVersion }} className="size-8 text-xs" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{inv.groupName}</span>
                      <span className="block truncate text-xs text-muted-foreground">{inv.invitedBy?.name ?? "Biri"} davet etti</span>
                    </span>
                  </div>
                  <div className="mt-2 flex gap-1.5">
                    <Button onClick={() => void api.teamsDecideInvite(inv.id, "accept").then(() => teams.refresh()).then(() => navigate(`/teams/${inv.groupId}`))} className="h-7 flex-1 text-xs">Katıl</Button>
                    <Button variant="secondary" onClick={() => void api.teamsDecideInvite(inv.id, "decline").then(() => teams.refresh())} className="h-7 flex-1 text-xs">Reddet</Button>
                  </div>
                </div>
              ))}
            </div>
          )}

          <p className="px-2.5 pb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/60">Gruplar</p>
          <div className="space-y-0.5">
            {rooms.map((g) => <Room key={g.id} g={g} />)}
            {rooms.length === 0 && <p className="px-2 py-3 text-xs text-muted-foreground">{canCreate ? "Henüz grup yok. Artı ile bir grup aç." : "Henüz bir gruba eklenmedin."}</p>}
          </div>

          <p className="px-2.5 pt-5 pb-1.5 text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground/60">Özel mesajlar</p>
          <div className="space-y-0.5">
            {dms.map((g) => <Room key={g.id} g={g} />)}
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
        ) : detailError ? (
          <div className="flex h-full items-center justify-center text-sm text-destructive">{detailError}</div>
        ) : !detail ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Yükleniyor...</div>
        ) : (
          <>
            <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border/50 bg-card/60 px-4 backdrop-blur-md">
              <button
                type="button"
                disabled={detail.kind !== "dm" || !detail.peer}
                onClick={(e) => {
                  if (!detail.peer) return;
                  const r = e.currentTarget.getBoundingClientRect();
                  setProfile({ userId: detail.peer.id, x: r.left, y: r.bottom, room: detail.id });
                }}
                className="relative inline-flex shrink-0 rounded-full disabled:cursor-default"
                data-tip={detail.kind === "dm" ? "Profili aç" : undefined}
              >
                <GroupAvatar group={detail} className="size-10 text-sm" />
                {detail.kind === "dm" && <OnlineDot presence={teams.presenceOf(detail.peer, detail.id)} />}
              </button>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  {detail.kind === "group" && <Hash className="size-4 shrink-0 text-muted-foreground" />}
                  <h2 className="truncate text-[0.9375rem] font-semibold tracking-tight">{detail.name}</h2>
                  {detail.postPolicy === "admins" && <Badge tone="amber">Duyuru</Badge>}
                  {detail.muted && <VolumeX className="size-3.5 text-muted-foreground" />}
                </div>
                {(() => {
                  const typing = teams.typingLabel(detail.id);
                  if (typing) return <p className="truncate text-xs italic text-primary">{typing}</p>;
                  if (detail.kind === "dm") {
                    const p = teams.presenceOf(detail.peer, detail.id);
                    return <p className={cn("truncate text-xs", presenceTone(p))}>{p.here ? "Sohbette · aynı sohbettesiniz" : seenLabel(p)}</p>;
                  }
                  const on = detail.members.filter((m) => teams.presenceOf(m, detail.id).online).length;
                  const here = detail.members.filter((m) => teams.presenceOf(m, detail.id).here).length;
                  return <p className="truncate text-xs text-muted-foreground">{detail.description ? `${detail.description} · ` : ""}{detail.memberCount} üye · {on} çevrimiçi{here ? ` · ${here} sohbette` : ""}</p>;
                })()}
              </div>
              {canPlay && detail.canPost && (
                <button type="button" onClick={() => setStartGame(true)} data-tip="Oyun başlat" className="flex size-9 items-center justify-center rounded-full text-violet-500 transition-colors hover:bg-violet-500/10"><Gamepad2 className="size-4" /></button>
              )}
              <button type="button" onClick={() => setPanel((p) => (p === "search" ? null : "search"))} data-tip="Mesajlarda ara" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "search" && "bg-primary/10 text-primary")}><Search className="size-4" /></button>
              <button type="button" onClick={() => setPanel((p) => (p === "media" ? null : "media"))} data-tip="Görseller, videolar ve dosyalar" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "media" && "bg-primary/10 text-primary")}><Images className="size-4" /></button>
              {detail.kind === "group" && (
                <>
                  <button type="button" onClick={() => setPanel((p) => (p === "members" ? null : "members"))} data-tip="Üyeler" className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", panel === "members" && "bg-primary/10 text-primary")}><Users className="size-4" /></button>
                  {detail.canManage && (
                    <button type="button" onClick={() => setSettings(true)} data-tip="Grup ayarları" className="flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"><Settings2 className="size-4" /></button>
                  )}
                </>
              )}
            </header>
            <div className="flex min-h-0 flex-1">
              <MessagePane key={detail.id} group={detail} selfId={selfId} target={target} onOpenGame={setOpenGame} />
              {panel && (panel !== "members" || detail.kind === "group") && (
                <RoomPanel
                  mode={panel}
                  group={detail}
                  selfId={selfId}
                  onChanged={setDetail}
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
      {plus && (
        <ContextMenu
          x={plus.x}
          y={plus.y}
          items={[
            ...(canCreate ? [{ label: "＋  Grup oluştur", onClick: () => setNewGroup(true) }] : []),
            { label: "✉  Kişiye mesaj yaz", onClick: () => setNewDM(true) },
            ...(canPlay && detail ? [{ label: "🎮  Oyun başlat", onClick: () => setStartGame(true) }] : []),
            ...(teams.games ? [{ label: "🏆  Oyun sıralaması", onClick: () => setBoard(true) }] : []),
          ]}
          onClose={() => setPlus(null)}
        />
      )}
      {profile && <ProfilePopover anchor={profile} selfId={selfId} onClose={() => setProfile(null)} />}
      <NewGroupDialog open={newGroup} onClose={() => setNewGroup(false)} onCreated={(g) => { void teams.refresh(); navigate(`/teams/${g.id}`); }} />
      <NewDMDialog open={newDM} onClose={() => setNewDM(false)} selfId={selfId} onOpened={(g) => { void teams.refresh(); navigate(`/teams/${g.id}`); }} />
      {detail && <GroupSettingsDialog group={detail} open={settings} onClose={() => setSettings(false)} onSaved={(g) => { setDetail(g); void teams.refresh(); }} />}
      {detail && teams.games && <StartGameDialog groupId={detail.id} config={teams.games} open={startGame} onClose={() => setStartGame(false)} onCreated={(g) => { teams.reloadGames(); setOpenGame(g.id); }} onOpen={setOpenGame} />}
      {openGame && <GameModal gameId={openGame} selfId={selfId} metas={teams.games?.kinds ?? []} pauseOnCall={teams.games?.pauseOnCall ?? true} members={detail?.members.map((m) => m.id) ?? []} onClose={() => setOpenGame(null)} />}
      {teams.games && <Leaderboard open={board} onClose={() => setBoard(false)} kinds={teams.games.kinds} selfId={selfId} />}
      {detail && people && <AddPeopleDialog group={detail} mode={people} open onClose={() => setPeople(null)} onDone={(g) => { setDetail(g); void teams.refresh(); }} />}
    </div>
  );
}
