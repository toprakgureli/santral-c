import { useState } from "react";
import { Copy, Info, Loader2, Trash2 } from "lucide-react";
import { api } from "@/api/client";
import type { PermissionGroup, PermissionItem, Role } from "@/api/types";
import AuthError from "@/components/auth/AuthError";
import { Button, CharCount, Field, FieldGroup, FieldHint, Input, Modal, Notice } from "@/components/ui";
import { errorMessage } from "@/components/auth/messages";
import { LIMITS } from "@/lib/limits";
import { cn } from "@/lib/utils";

type RoleFormProps = {
  role: Role | null;
  copyFrom?: Role | null;
  groups: PermissionGroup[];
  onClose: () => void;
  onSaved: () => void;
};

export default function RoleForm({ role, copyFrom, groups, onClose, onSaved }: RoleFormProps) {
  const editing = Boolean(role);

  // When copying, the technical name is left blank on purpose: it cannot be
  // changed afterwards, so the user has to type it consciously.
  const [form, setForm] = useState({
    name: role?.name ?? "",
    displayName: role?.displayName ?? (copyFrom ? `${copyFrom.displayName} (kopya)` : ""),
    description: role?.description ?? copyFrom?.description ?? "",
  });
  const [selected, setSelected] = useState<Set<number>>(() => new Set(role?.permissionIds ?? copyFrom?.permissionIds ?? []));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const update = (key: "name" | "displayName" | "description") => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm((prev) => ({ ...prev, [key]: e.target.value }));
    setError(null);
  };

  const toggle = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
    setError(null);
  };

  const toggleGroup = (items: PermissionItem[]) => {
    const ids = items.map((i) => i.id);
    const allOn = ids.every((id) => selected.has(id));
    setSelected((prev) => {
      const next = new Set(prev);
      ids.forEach((id) => (allOn ? next.delete(id) : next.add(id)));
      return next;
    });
  };

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
      onSaved();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const save = () => {
    if (form.displayName.trim().length < 2) {
      setError("Görünen ad en az 2 karakter olmalı.");
      return;
    }
    if (!editing && form.name.trim().length < 3) {
      setError("Rol adı en az 3 karakter olmalı.");
      return;
    }
    if (selected.size === 0) {
      setError("En az bir yetki seçmelisin.");
      return;
    }
    const payload = {
      displayName: form.displayName.trim(),
      description: form.description.trim(),
      permissionIds: [...selected],
    };
    if (role) {
      void run(() => api.updateRole(role.id, payload));
      return;
    }
    void run(() => api.createRole({ ...payload, name: form.name.trim() }));
  };

  const remove = () => {
    if (!role) return;
    void run(() => api.deleteRole(role.id));
  };

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={role ? role.displayName : copyFrom ? "Rolü Kopyala" : "Yeni Rol"}
      description={`${selected.size} yetki seçili`}
      footer={
        <>
          {role && !role.system && (
            <Button variant="ghost" className="mr-auto text-destructive hover:text-destructive" onClick={remove} disabled={busy}>
              <Trash2 />
              Sil
            </Button>
          )}
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Vazgeç
          </Button>
          <Button onClick={save} disabled={busy}>
            {busy && <Loader2 className="animate-spin" />}
            {busy ? "Kaydediliyor..." : "Kaydet"}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {error && <AuthError message={error} />}

        {role?.system && (
          <Notice icon={<Info />}>Bu bir sistem rolü. Yetkileri değiştirilebilir ama rol silinemez.</Notice>
        )}

        {!editing && copyFrom && (
          <Notice icon={<Copy />}>
            <strong className="font-medium text-foreground">{copyFrom.displayName}</strong> rolünün {copyFrom.permissionIds.length} yetkisi
            seçili geldi. İstediğini değiştirip yeni bir rol olarak kaydedebilirsin.
          </Notice>
        )}

        {!editing && (
          <div className="space-y-1.5">
            <Field label="Rol Adı">
              <Input value={form.name} maxLength={LIMITS.roleName} onChange={update("name")} placeholder="Örn: destek_ekibi" autoFocus />
            </Field>
            <FieldHint>Teknik ad. Boşluklar alt çizgiye çevrilir, sonradan değiştirilemez.</FieldHint>
          </div>
        )}

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1">
            <Field label="Görünen Ad">
              <Input value={form.displayName} maxLength={LIMITS.roleDisplayName} onChange={update("displayName")} placeholder="Örn: Destek Ekibi" />
            </Field>
            <CharCount value={form.displayName} max={LIMITS.roleDisplayName} className="text-right" />
          </div>
          <div className="space-y-1">
            <Field label="Açıklama">
              <Input value={form.description} maxLength={LIMITS.roleDescription} onChange={update("description")} />
            </Field>
            <CharCount value={form.description} max={LIMITS.roleDescription} className="text-right" />
          </div>
        </div>

        <FieldGroup label="Yetkiler">
          <div className="grid gap-3 sm:grid-cols-2">
            {groups.map((g) => {
              const allOn = g.items.every((i) => selected.has(i.id));
              return (
                <section key={g.module} className="space-y-1 rounded-xl border border-border/60 bg-muted/25 p-3">
                  <header className="flex items-center justify-between gap-2 pb-1">
                    <strong className="text-xs font-semibold">{g.label}</strong>
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={() => toggleGroup(g.items)}>
                      {allOn ? "Kaldır" : "Tümü"}
                    </Button>
                  </header>
                  {g.items.map((item) => {
                    const on = selected.has(item.id);
                    return (
                      <label
                        key={item.id}
                        className={cn(
                          "flex cursor-pointer items-start gap-2.5 rounded-lg px-2 py-1.5 transition-colors duration-150",
                          on ? "bg-accent/70" : "hover:bg-accent/40",
                        )}
                      >
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
        </FieldGroup>
      </div>
    </Modal>
  );
}
