// Role and permission management. Clicking a row edits it; "Kopyala" carries an
// existing role's permissions into a new one (the technical name is left blank
// on purpose, it cannot be changed later so it must be typed deliberately).

import { useCallback, useEffect, useState } from "react";
import { Copy, KeyRound, Plus, ShieldCheck } from "lucide-react";
import { ListRow } from "../components/ui/rows";
import { api } from "../api/client";
import type { PermissionGroup, Role } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, EmptyState, Skeleton } from "../components/ui";
import RoleForm from "../components/role/RoleForm";

export function Roles() {
  const { user } = useAuth();
  const canManage = can(user, "role.manage");
  const [roles, setRoles] = useState<Role[]>([]);
  const [groups, setGroups] = useState<PermissionGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<Role | null>(null);
  const [copying, setCopying] = useState<Role | null>(null);
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    const [list, perms] = await Promise.all([
      api.listRoles().catch(() => [] as Role[]),
      api.rolePermissions().catch(() => [] as PermissionGroup[]),
    ]);
    setRoles(list);
    setGroups(perms);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const close = () => {
    setEditing(null);
    setCopying(null);
    setCreating(false);
  };

  const saved = () => {
    close();
    void load();
  };

  const totalPermissions = groups.reduce((sum, g) => sum + g.items.length, 0);

  return (
    <div className="space-y-6">
      <Card
        title="Roller"
        icon={KeyRound}
        actions={
          <div className="flex items-center gap-3">
            <span className="text-xs text-muted-foreground">{roles.length} rol · {totalPermissions} yetki</span>
            {canManage && (
              <Button onClick={() => setCreating(true)}>
                <Plus />
                Yeni Rol
              </Button>
            )}
          </div>
        }
      >
        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2].map((row) => (
              <Skeleton key={row} className="h-12 w-full rounded-xl" />
            ))}
          </div>
        ) : !roles.length ? (
          <EmptyState icon={<ShieldCheck />} title="Rol bulunamadı" />
        ) : (
          <div className="space-y-1">
            {roles.map((r) => {
              const ratio = totalPermissions ? (r.permissionIds.length / totalPermissions) * 100 : 0;
              return (
                <ListRow
                  key={r.id}
                  icon={r.system ? ShieldCheck : KeyRound}
                  tone={r.system ? "primary" : "violet"}
                  onClick={canManage ? () => setEditing(r) : undefined}
                  title={<span className="flex items-center gap-2">{r.displayName}<Badge tone={r.system ? "blue" : "slate"}>{r.system ? "Sistem" : "Özel"}</Badge></span>}
                  sub={<span>{r.name}{r.description ? ` · ${r.description}` : ""} · {r.userCount} kullanıcı</span>}
                  trailing={
                    <>
                      <span className="flex items-center gap-2" title={`${r.permissionIds.length} / ${totalPermissions} yetki`}>
                        <span className="h-1.5 w-24 overflow-hidden rounded-full bg-muted">
                          <span className="block h-full rounded-full bg-primary/70 transition-[width] duration-500" style={{ width: `${ratio}%` }} />
                        </span>
                        <span className="w-14 text-right text-xs tabular-nums text-muted-foreground">{r.permissionIds.length} / {totalPermissions}</span>
                      </span>
                      {canManage && (
                        <span
                          role="button"
                          tabIndex={0}
                          title={`${r.displayName} yetkilerini yeni bir role kopyala`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setCopying(r);
                          }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              e.stopPropagation();
                              setCopying(r);
                            }
                          }}
                          className="flex size-8 items-center justify-center rounded-xl bg-muted/70 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                        >
                          <Copy className="size-3.5" />
                        </span>
                      )}
                    </>
                  }
                />
              );
            })}
          </div>
        )}
      </Card>

      {(creating || editing || copying) && (
        <RoleForm role={editing} copyFrom={copying} groups={groups} onClose={close} onSaved={saved} />
      )}
    </div>
  );
}
