// MembersPanel: who is in the room, with roles and posting rights. Owners
// promote and demote, admins toggle who may write and remove people.

import { useState } from "react";
import { Crown, MicOff, Shield, UserMinus, UserPlus, MailPlus } from "lucide-react";
import { Link } from "react-router-dom";
import { api, ApiError } from "@/api/client";
import type { TeamsGroupDetail, TeamsMember } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { ContextMenu, type MenuItem } from "@/components/ContextMenu";
import { cn } from "@/lib/utils";

const ROLE_LABEL: Record<string, string> = { owner: "Sahip", admin: "Yönetici", member: "Üye" };

export default function MembersPanel({ group, selfId, onChanged, onAdd, onInvite }: { group: TeamsGroupDetail; selfId: number; onChanged: (g: TeamsGroupDetail) => void; onAdd: () => void; onInvite: () => void }) {
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const isOwner = group.myRole === "owner";

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

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-l border-border/60 bg-card/60">
      <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
        <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Üyeler · {group.members.length}</span>
        <span className="flex items-center gap-1">
          {group.canAdd && (
            <button type="button" onClick={onAdd} title="Üye ekle (doğrudan)" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground">
              <UserPlus className="size-4" />
            </button>
          )}
          {group.canInvite && (
            <button type="button" onClick={onInvite} title="Davet gönder" className="rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground">
              <MailPlus className="size-4" />
            </button>
          )}
        </span>
      </div>
      <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto p-2">
        {group.members.map((m) => (
          <li key={m.id}>
            <div
              onContextMenu={(e) => open(e, m)}
              className={cn("flex items-center gap-2.5 rounded-lg px-2 py-1.5", group.canManage && m.id !== selfId && "cursor-context-menu hover:bg-accent")}
              title={group.canManage && m.id !== selfId ? "Sağ tık: rol ve yazma hakkı" : undefined}
            >
              <UserAvatar userId={m.id} name={m.name} hasAvatar={m.hasAvatar} version={m.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" />
              <span className="min-w-0 flex-1">
                <Link to={`/profile/${m.id}`} className="block truncate text-sm font-medium hover:underline">{m.name}{m.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</Link>
                <span className="flex items-center gap-1 text-[0.7rem] text-muted-foreground">
                  {m.role === "owner" && <Crown className="size-3 text-warning" />}
                  {m.role === "admin" && <Shield className="size-3 text-primary" />}
                  {ROLE_LABEL[m.role]}
                  {!m.canPost && <span className="ml-1 inline-flex items-center gap-0.5 text-destructive"><MicOff className="size-3" /> yazamaz</span>}
                </span>
              </span>
            </div>
          </li>
        ))}
        {group.invited.length > 0 && (
          <>
            <li className="px-2 pt-3 pb-1 text-[0.7rem] font-semibold uppercase tracking-wide text-muted-foreground">Davetli · {group.invited.length}</li>
            {group.invited.map((p) => (
              <li key={`inv-${p.id}`} className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 opacity-70">
                <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-8" fallbackClassName="bg-muted text-xs" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm">{p.name}</span>
                  <span className="text-[0.7rem] text-muted-foreground">Davet bekliyor</span>
                </span>
              </li>
            ))}
          </>
        )}
      </ul>
      {error && <p className="px-4 pb-3 text-xs text-destructive">{error}</p>}
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
      <span className="sr-only"><UserMinus /></span>
    </aside>
  );
}
