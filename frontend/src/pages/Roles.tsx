// Role and permission management. Clicking a row edits it; "Kopyala" carries an
// existing role's permissions into a new one (the technical name is left blank
// on purpose, it cannot be changed later so it must be typed deliberately).

import { useCallback, useEffect, useState } from "react";
import { Copy, Plus, ShieldCheck } from "lucide-react";
import { api } from "../api/client";
import type { PermissionGroup, Role } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, EmptyState, Skeleton } from "../components/ui";
import RoleForm from "../components/role/RoleForm";
import { cn } from "../lib/utils";

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
          <div className="overflow-x-auto">
            <table className="w-full min-w-[42rem] text-sm">
              <thead>
                <tr className="text-left text-xs text-muted-foreground">
                  <th className="pb-2">Rol</th>
                  <th className="pb-2">Tür</th>
                  <th className="pb-2">Kullanıcı</th>
                  <th className="pb-2">Yetki</th>
                  <th className="pb-2"></th>
                </tr>
              </thead>
              <tbody>
                {roles.map((r) => {
                  const ratio = totalPermissions ? (r.permissionIds.length / totalPermissions) * 100 : 0;
                  return (
                    <tr
                      key={r.id}
                      onClick={() => canManage && setEditing(r)}
                      className={cn("border-t border-border/60", canManage && "cursor-pointer transition-colors hover:bg-accent/50")}
                    >
                      <td className="py-2.5">
                        <span className="block font-medium">{r.displayName}</span>
                        <span className="block text-xs text-muted-foreground">
                          {r.name}
                          {r.description ? ` · ${r.description}` : ""}
                        </span>
                      </td>
                      <td className="py-2.5">
                        <Badge tone={r.system ? "blue" : "slate"}>{r.system ? "Sistem" : "Özel"}</Badge>
                      </td>
                      <td className="py-2.5 tabular-nums">{r.userCount}</td>
                      <td className="py-2.5">
                        <div className="flex items-center gap-2">
                          <span className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
                            <span className="block h-full rounded-full bg-primary/70 transition-[width] duration-500" style={{ width: `${ratio}%` }} />
                          </span>
                          <span className="text-xs tabular-nums text-muted-foreground">
                            {r.permissionIds.length} / {totalPermissions}
                          </span>
                        </div>
                      </td>
                      <td className="py-2.5 text-right">
                        {canManage && (
                          <Button
                            variant="ghost"
                            className="h-8 px-2.5 text-xs"
                            title={`${r.displayName} yetkilerini yeni bir role kopyala`}
                            onClick={(e) => {
                              e.stopPropagation();
                              setCopying(r);
                            }}
                          >
                            <Copy />
                            Kopyala
                          </Button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {(creating || editing || copying) && (
        <RoleForm role={editing} copyFrom={copying} groups={groups} onClose={close} onSaved={saved} />
      )}
    </div>
  );
}
