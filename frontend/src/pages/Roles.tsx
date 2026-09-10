import { useEffect, useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { PermissionGroup, Role } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, ErrorText, Field, Input, Modal } from "../components/ui";
import { cn } from "../lib/utils";

export function Roles() {
  const { user } = useAuth();
  const canManage = can(user, "role.manage");
  const [roles, setRoles] = useState<Role[]>([]);
  const [groups, setGroups] = useState<PermissionGroup[]>([]);
  const [editing, setEditing] = useState<Role | null>(null);
  const [creating, setCreating] = useState(false);

  const totalPermissions = useMemo(() => groups.reduce((n, g) => n + g.items.length, 0), [groups]);

  function load() {
    api.listRoles().then(setRoles).catch(() => setRoles([]));
  }
  useEffect(() => {
    load();
    api.rolePermissions().then(setGroups).catch(() => setGroups([]));
  }, []);

  return (
    <div className="space-y-6">
      <Card
        title="Roller"
        actions={canManage ? <Button onClick={() => setCreating(true)}><Plus className="size-4" /> Yeni rol</Button> : undefined}
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
              <th className="pb-2">Rol</th>
              <th className="pb-2">Tür</th>
              <th className="pb-2">Kullanıcı</th>
              <th className="pb-2">Yetki</th>
            </tr>
          </thead>
          <tbody>
            {roles.map((r) => (
              <tr
                key={r.id}
                onClick={() => canManage && setEditing(r)}
                className={cn("border-t border-border/60", canManage && "cursor-pointer hover:bg-accent/50")}
              >
                <td className="py-2.5">
                  <span className="block font-medium">{r.displayName}</span>
                  <span className="block text-xs text-muted-foreground">{r.name}{r.description ? ` · ${r.description}` : ""}</span>
                </td>
                <td className="py-2.5"><Badge tone={r.system ? "blue" : "slate"}>{r.system ? "Sistem" : "Özel"}</Badge></td>
                <td className="py-2.5 tabular-nums">{r.userCount}</td>
                <td className="py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="h-1.5 w-24 overflow-hidden rounded-full bg-muted">
                      <span className="block h-full rounded-full bg-primary/70 transition-[width] duration-500" style={{ width: `${totalPermissions ? (r.permissionIds.length / totalPermissions) * 100 : 0}%` }} />
                    </span>
                    <span className="text-xs tabular-nums text-muted-foreground">{r.permissionIds.length} / {totalPermissions}</span>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      {(editing || creating) && (
        <RoleForm
          role={editing}
          groups={groups}
          onClose={() => { setEditing(null); setCreating(false); }}
          onSaved={() => { setEditing(null); setCreating(false); load(); }}
        />
      )}
    </div>
  );
}

function RoleForm({ role, groups, onClose, onSaved }: { role: Role | null; groups: PermissionGroup[]; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(role?.name ?? "");
  const [displayName, setDisplayName] = useState(role?.displayName ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [selected, setSelected] = useState<Set<number>>(() => new Set(role?.permissionIds ?? []));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const creating = !role;

  function toggle(id: number) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }
  function toggleGroup(g: PermissionGroup) {
    const allOn = g.items.every((i) => selected.has(i.id));
    setSelected((prev) => {
      const next = new Set(prev);
      g.items.forEach((i) => (allOn ? next.delete(i.id) : next.add(i.id)));
      return next;
    });
  }

  async function save() {
    setError(null);
    if (displayName.trim().length < 2) { setError("Görünen ad en az 2 karakter olmalı."); return; }
    if (creating && name.trim().length < 3) { setError("Rol adı en az 3 karakter olmalı."); return; }
    if (selected.size === 0) { setError("En az bir yetki seçin."); return; }
    setSaving(true);
    try {
      const permissionIds = [...selected];
      if (creating) {
        await api.createRole({ name: name.trim(), displayName: displayName.trim(), description: description.trim(), permissionIds });
      } else {
        await api.updateRole(role.id, { displayName: displayName.trim(), description: description.trim(), permissionIds });
      }
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={creating ? "Yeni Rol" : role.displayName}
      description={`${selected.size} yetki seçili`}
      footer={
        <>
          {!creating && !role.system && (
            <Button
              variant="danger"
              className="mr-auto"
              onClick={async () => { await api.deleteRole(role.id).catch(() => undefined); onSaved(); }}
            >
              Sil
            </Button>
          )}
          <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
          <Button onClick={save} disabled={saving}>{saving ? "Kaydediliyor..." : "Kaydet"}</Button>
        </>
      }
    >
      <div className="space-y-4">
        {role?.system && (
          <p className="rounded-lg bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
            Sistem rolü: adı değiştirilemez, yetkileri düzenlenebilir.
          </p>
        )}
        <div className="grid gap-3 sm:grid-cols-2">
          {creating && <Field label="Rol Adı (teknik)"><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="ornek_rol" /></Field>}
          <Field label="Görünen Ad"><Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Örnek Rol" /></Field>
          <Field label="Açıklama"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
        </div>

        <div>
          <span className="mb-2 block text-xs font-medium text-muted-foreground">Yetkiler</span>
          <div className="grid gap-3 sm:grid-cols-2">
            {groups.map((g) => {
              const allOn = g.items.every((i) => selected.has(i.id));
              return (
                <section key={g.module} className="space-y-1 rounded-xl border border-border/60 bg-muted/25 p-3">
                  <header className="flex items-center justify-between gap-2 pb-1">
                    <strong className="text-xs font-semibold">{g.label}</strong>
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={() => toggleGroup(g)}>{allOn ? "Kaldır" : "Tümü"}</Button>
                  </header>
                  {g.items.map((item) => {
                    const on = selected.has(item.id);
                    return (
                      <label key={item.id} className={cn("flex cursor-pointer items-start gap-2.5 rounded-lg px-2 py-1.5 transition-colors", on ? "bg-accent/70" : "hover:bg-accent/40")}>
                        <input type="checkbox" checked={on} onChange={() => toggle(item.id)} className="mt-0.5 size-3.5 shrink-0 accent-primary" />
                        <span className="min-w-0">
                          <span className="block text-xs leading-snug">{item.description}</span>
                          <code className="block text-[0.625rem] text-muted-foreground/70">{item.key}</code>
                        </span>
                      </label>
                    );
                  })}
                </section>
              );
            })}
          </div>
        </div>
        <ErrorText>{error}</ErrorText>
      </div>
    </Modal>
  );
}
