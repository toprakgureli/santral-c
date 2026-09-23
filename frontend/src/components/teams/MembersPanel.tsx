// MembersPanel: who is in the room, grouped like the app's own sidebar:
// people in the chat and online first, then offline, then pending
// invites. Owners promote and demote, admins toggle who may write and
// remove people, all from the right-click menu.

import { useMemo, useState } from "react";
import { Crown, MailPlus, MicOff, Shield, UserPlus } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { TeamsGroupDetail, TeamsMember } from "@/api/types";
import { ContextMenu, type MenuItem } from "@/components/ContextMenu";
import { OnlineDot, presenceTone, seenLabel } from "@/components/teams/Presence";
import ProfilePopover, { type PopoverAnchor } from "@/components/teams/ProfilePopover";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { useTeams } from "@/teams/TeamsContext";

const RANK: Record<string, number> = { owner: 0, admin: 1, member: 2 };

export default function MembersPanel({ group, selfId, onChanged, onAdd, onInvite }: { group: TeamsGroupDetail; selfId: number; onChanged: (g: TeamsGroupDetail) => void; onAdd: () => void; onInvite: () => void }) {
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [profile, setProfile] = useState<PopoverAnchor | null>(null);
  const { presenceOf } = useTeams();
  const isOwner = group.myRole === "owner";

  const { online, offline } = useMemo(() => {
    const by = (a: TeamsMember, b: TeamsMember) => (RANK[a.role] - RANK[b.role]) || a.name.localeCompare(b.name, "tr");
    const on = group.members.filter((m) => presenceOf(m, group.id).online).sort(by);
    const off = group.members.filter((m) => !presenceOf(m, group.id).online).sort(by);
    return { online: on, offline: off };
  }, [group.members, group.id, presenceOf]);

  async function run(p: Promise<TeamsGroupDetail | void>) {
    try {
      const g = await p;
      if (g) onChanged(g);
      else onChanged(await api.teamsGroup(group.id));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem tamamlanamadı.");
    }
  }

  function open(e: React.MouseEvent, m: TeamsMember) {
    if (!group.canManage || m.id === selfId) return;
    e.preventDefault();
    const items: MenuItem[] = [];
    if (group.postPolicy === "everyone" && m.role === "member") {
      items.push({ label: m.canPost ? "Yazmasını kapat" : "Yazmasına izin ver", onClick: () => void run(api.teamsUpdateMember(group.id, m.id, { canPost: !m.canPost })) });
    }
    if (isOwner && m.role !== "owner") {
      items.push({ label: m.role === "admin" ? "Yöneticilikten al" : "Yönetici yap", onClick: () => void run(api.teamsUpdateMember(group.id, m.id, { role: m.role === "admin" ? "member" : "admin" })) });
      items.push({ label: "Sahipliği devret", onClick: () => void run(api.teamsUpdateMember(group.id, m.id, { role: "owner" })) });
    }
    if (m.role !== "owner") items.push({ label: "Gruptan çıkar", danger: true, onClick: () => void run(api.teamsRemoveMember(group.id, m.id)) });
    if (items.length === 0) return;
    setMenu({ x: e.clientX, y: e.clientY, items });
  }

  const Row = ({ m }: { m: TeamsMember }) => {
    const p = presenceOf(m, group.id);
    const manageable = group.canManage && m.id !== selfId;
    return (
      <li>
        <div
          onContextMenu={(e) => open(e, m)}
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            setProfile({ userId: m.id, x: r.left - 308, y: r.top, room: group.id });
          }}
          className={cn(
            "group flex cursor-pointer items-center gap-2.5 rounded-xl px-2.5 py-1.5 transition-[background-color,opacity] duration-200",
            "hover:bg-sidebar-accent/60",
            !p.online && "opacity-60 hover:opacity-100",
            manageable && "cursor-context-menu",
          )}
          title={manageable ? "Sağ tık: rol ve yazma hakkı" : undefined}
        >
          <span className="relative inline-flex shrink-0">
            <UserAvatar userId={m.id} name={m.name} hasAvatar={m.hasAvatar} version={m.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
            <OnlineDot presence={p} className="-right-0.5 -bottom-0.5 size-2.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5">
              <span className="truncate text-sm font-medium">{m.name}</span>
              {m.id === selfId && <span className="text-[0.65rem] text-muted-foreground">sen</span>}
              {m.role === "owner" && <Crown className="size-3 shrink-0 text-warning" aria-label="Sahip" />}
              {m.role === "admin" && <Shield className="size-3 shrink-0 text-primary" aria-label="Yönetici" />}
              {!m.canPost && <MicOff className="size-3 shrink-0 text-destructive" aria-label="Yazamaz" />}
            </span>
            <span className={cn("block truncate text-[0.7rem]", presenceTone(p))}>{seenLabel(p)}</span>
          </span>
        </div>
      </li>
    );
  };

  const Section = ({ title, count, children }: { title: string; count: number; children: React.ReactNode }) =>
    count === 0 ? null : (
      <div className="space-y-0.5">
        <p className="px-3 pb-1 text-[0.6875rem] font-semibold tracking-wider text-muted-foreground/70 uppercase">
          {title} <span className="text-muted-foreground/50">— {count}</span>
        </p>
        <ul className="space-y-0.5">{children}</ul>
      </div>
    );

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-l border-sidebar-border bg-sidebar">
      <div className="flex h-12 shrink-0 items-center gap-2 border-b border-sidebar-border px-4">
        <span className="text-sm font-semibold tracking-tight">Üyeler</span>
        <span className="rounded-full bg-muted px-1.5 text-[0.65rem] font-semibold tabular-nums text-muted-foreground">{group.members.length}</span>
        <span className="ml-auto flex items-center gap-0.5">
          {group.canAdd && (
            <button type="button" onClick={onAdd} title="Üye ekle (doğrudan katılır)" className="flex size-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-sidebar-accent hover:text-foreground">
              <UserPlus className="size-4" />
            </button>
          )}
          {group.canInvite && (
            <button type="button" onClick={onInvite} title="Davet gönder (kabul edince katılır)" className="flex size-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-sidebar-accent hover:text-foreground">
              <MailPlus className="size-4" />
            </button>
          )}
        </span>
      </div>

      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-3 pt-3 pb-4">
        <Section title="Çevrimiçi" count={online.length}>
          {online.map((m) => <Row key={m.id} m={m} />)}
        </Section>
        <Section title="Çevrimdışı" count={offline.length}>
          {offline.map((m) => <Row key={m.id} m={m} />)}
        </Section>
        <Section title="Davet bekliyor" count={group.invited.length}>
          {group.invited.map((p) => (
            <li key={`inv-${p.id}`} className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 opacity-60">
              <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-8" fallbackClassName="bg-muted text-xs" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">{p.name}</span>
                <span className="block text-[0.7rem] text-muted-foreground">Davet gönderildi</span>
              </span>
            </li>
          ))}
        </Section>
      </div>

      <div className="shrink-0 border-t border-sidebar-border px-4 py-2.5 text-[0.65rem] text-muted-foreground">
        <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="inline-flex items-center gap-1"><Crown className="size-3 text-warning" /> Sahip</span>
          <span className="inline-flex items-center gap-1"><Shield className="size-3 text-primary" /> Yönetici</span>
          <span className="inline-flex items-center gap-1"><MicOff className="size-3 text-destructive" /> Yazamaz</span>
        </span>
        {group.canManage && <p className="mt-1">Sağ tık ile rol ve yazma hakkı.</p>}
      </div>

      {error && <p className="px-4 pb-3 text-xs text-destructive">{error}</p>}
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
      {profile && <ProfilePopover anchor={profile} selfId={selfId} onClose={() => setProfile(null)} />}
    </aside>
  );
}
