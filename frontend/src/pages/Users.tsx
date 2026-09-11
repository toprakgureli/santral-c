import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { Role, User } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { can } from "../lib/permissions";
import { Badge, Button, Card, ErrorText, Field, Input, Modal, Select } from "../components/ui";
import { cn } from "../lib/utils";

export function Users() {
  const { user } = useAuth();
  const canCreate = can(user, "user.create");
  const canDeactivate = can(user, "user.deactivate");
  const canReset = can(user, "user.update");
  const canAssignRoles = can(user, "role.assign");

  const [users, setUsers] = useState<User[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [creating, setCreating] = useState(false);
  const [editingRoles, setEditingRoles] = useState<User | null>(null);

  function load() {
    api.listUsers({ perPage: 100 }).then((r) => setUsers(r.items)).catch(() => setUsers([]));
  }
  useEffect(() => {
    load();
    if (canCreate || canAssignRoles) api.listRoles().then(setRoles).catch(() => setRoles([]));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="space-y-6">
      <Card
        title="Kullanıcılar"
        actions={
          <div className="flex items-center gap-2">
            {canReset && <SyncSipButton onDone={load} />}
            {canCreate && <Button onClick={() => setCreating((v) => !v)}>{creating ? "Kapat" : "Yeni kullanıcı"}</Button>}
          </div>
        }
      >
        {creating && <CreateUser roles={roles} onCreated={() => { setCreating(false); load(); }} />}
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
              <th className="pb-2">Ad</th>
              <th className="pb-2">E-posta</th>
              <th className="pb-2">Roller</th>
              <th className="pb-2">Dahili</th>
              <th className="pb-2">Durum</th>
              <th className="pb-2"></th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} className="border-t border-border/60">
                <td className="py-2 font-medium">{u.name}</td>
                <td className="py-2 text-muted-foreground">{u.email}</td>
                <td className="py-2">{u.roles.join(", ")}</td>
                <td className="py-2">{u.sipExtension ?? "—"}</td>
                <td className="py-2">{u.active ? <Badge tone="green">Aktif</Badge> : <Badge tone="red">Pasif</Badge>}</td>
                <td className="py-2 text-right">
                  <div className="flex justify-end gap-2">
                    {canAssignRoles && <Button variant="ghost" onClick={() => setEditingRoles(u)}>Roller</Button>}
                    {canReset && <SetSip id={u.id} ext={u.sipExtension} onDone={load} />}
                    {canReset && <ResetPassword id={u.id} />}
                    {canDeactivate && u.id !== user?.id && (
                      <Button
                        variant="secondary"
                        onClick={async () => { await api.setUserActive(u.id, !u.active).catch(() => undefined); load(); }}
                      >
                        {u.active ? "Pasifleştir" : "Aktifleştir"}
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      {editingRoles && (
        <EditRoles
          user={editingRoles}
          roles={roles}
          onClose={() => setEditingRoles(null)}
          onSaved={() => { setEditingRoles(null); load(); }}
        />
      )}
    </div>
  );
}

function EditRoles({ user, roles, onClose, onSaved }: { user: User; roles: Role[]; onClose: () => void; onSaved: () => void }) {
  const [selected, setSelected] = useState<number[]>(user.roleIds ?? []);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  function toggle(id: number) {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  async function save() {
    setError(null);
    if (selected.length === 0) { setError("En az bir rol seçin."); return; }
    setSaving(true);
    try {
      await api.setUserRoles(user.id, selected);
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
      title={`${user.name} · Roller`}
      description="Kullanıcının rollerini düzenleyin"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>Vazgeç</Button>
          <Button onClick={save} disabled={saving}>{saving ? "Kaydediliyor..." : "Kaydet"}</Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex flex-wrap gap-2">
          {roles.map((r) => {
            const on = selected.includes(r.id);
            return (
              <button
                key={r.id}
                type="button"
                onClick={() => toggle(r.id)}
                className={cn(
                  "rounded-full border px-3 py-1.5 text-xs font-medium transition-colors",
                  on ? "border-primary bg-primary text-primary-foreground" : "border-border/70 bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                {r.displayName}
              </button>
            );
          })}
        </div>
        <ErrorText>{error}</ErrorText>
      </div>
    </Modal>
  );
}

function CreateUser({ roles, onCreated }: { roles: Role[]; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [roleId, setRoleId] = useState<number | "">("");
  const [ext, setExt] = useState("");
  const [sipPassword, setSipPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (roleId === "") { setError("Rol seçin."); return; }
    try {
      const created = await api.createUser({ name, email, password, roleIds: [roleId], sipExtension: ext || undefined });
      if (ext && sipPassword) await api.setUserSip(created.id, ext, sipPassword);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Oluşturulamadı.");
    }
  }

  return (
    <form className="mb-4 grid gap-3 rounded-lg bg-muted/40 p-4 md:grid-cols-6" onSubmit={submit}>
      <Field label="Ad"><Input value={name} onChange={(e) => setName(e.target.value)} required /></Field>
      <Field label="E-posta"><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required /></Field>
      <Field label="Geçici parola"><Input type="text" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} /></Field>
      <Field label="Rol">
        <Select value={roleId} onChange={(e) => setRoleId(e.target.value ? Number(e.target.value) : "")}>
          <option value="">Seçin</option>
          {roles.map((r) => <option key={r.id} value={r.id}>{r.displayName}</option>)}
        </Select>
      </Field>
      <Field label="Dahili"><Input value={ext} onChange={(e) => setExt(e.target.value)} placeholder="1005" /></Field>
      <Field label="SIP parola"><Input value={sipPassword} onChange={(e) => setSipPassword(e.target.value)} placeholder="Verimor SIP" /></Field>
      <div className="md:col-span-6 flex items-center gap-3">
        <Button type="submit">Oluştur</Button>
        <ErrorText>{error}</ErrorText>
      </div>
    </form>
  );
}

function SetSip({ id, ext, onDone }: { id: number; ext?: string; onDone: () => void }) {
  const [open, setOpen] = useState(false);
  const [extension, setExtension] = useState(ext ?? "");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  if (!open) return <Button variant="ghost" onClick={() => setOpen(true)}>SIP</Button>;

  async function pull() {
    if (!extension) return;
    setBusy(true);
    setMsg(null);
    try {
      await api.syncUserSip(id, extension);
      setOpen(false);
      onDone();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Çekilemedi.");
    } finally {
      setBusy(false);
    }
  }
  async function manual() {
    setBusy(true);
    setMsg(null);
    try {
      await api.setUserSip(id, extension, password);
      setOpen(false);
      onDone();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Kaydedilemedi.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <span className="flex items-center gap-1">
      <Input value={extension} onChange={(e) => setExtension(e.target.value)} className="w-20" placeholder="Dahili" />
      <Button onClick={pull} disabled={!extension || busy}>{busy ? "..." : "Verimor'dan çek"}</Button>
      <Input value={password} onChange={(e) => setPassword(e.target.value)} className="w-28" placeholder="veya elle parola" />
      <Button variant="secondary" onClick={manual} disabled={!extension || !password || busy}>Kaydet</Button>
      {msg && <span className="text-xs text-destructive">{msg}</span>}
    </span>
  );
}

function SyncSipButton({ onDone }: { onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  async function run() {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api.syncAllSip();
      const fails = r.failedExtensions?.length ? ` (dahili ${r.failedExtensions.join(", ")} — Verimor'da personel yok)` : "";
      setMsg(`${r.synced} çekildi${r.failed ? ` · ${r.failed} başarısız${fails}` : ""}`);
      onDone();
    } catch (e) {
      setMsg(e instanceof ApiError ? e.message : "Hata");
    } finally {
      setBusy(false);
    }
  }
  return (
    <span className="flex items-center gap-2">
      {msg && <span className="text-xs text-muted-foreground">{msg}</span>}
      <Button variant="secondary" onClick={run} disabled={busy}>{busy ? "Senkronize..." : "SIP Senkronize (Verimor)"}</Button>
    </span>
  );
}

function ResetPassword({ id }: { id: number }) {
  const [open, setOpen] = useState(false);
  const [pw, setPw] = useState("");
  const [done, setDone] = useState(false);
  if (!open) return <Button variant="ghost" onClick={() => setOpen(true)}>Parola</Button>;
  return (
    <span className="flex items-center gap-1">
      <Input value={pw} onChange={(e) => setPw(e.target.value)} className="w-32" placeholder="Yeni parola" />
      <Button
        onClick={async () => { await api.resetUserPassword(id, pw).catch(() => undefined); setDone(true); setOpen(false); setPw(""); }}
        disabled={pw.length < 8}
      >
        {done ? "✓" : "Ayarla"}
      </Button>
    </span>
  );
}
