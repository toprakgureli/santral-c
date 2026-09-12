import { useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/api/client";
import type { Role, User } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import AuthError from "@/components/auth/AuthError";
import { errorMessage } from "@/components/auth/messages";
import { Button, CharCount, Field, FieldGroup, FieldHint, Input, Modal, Separator } from "@/components/ui";
import PasswordField, { PasswordRules } from "@/components/ui/PasswordField";
import type { Handoff } from "@/components/user/CredentialsHandoff";
import { LIMITS } from "@/lib/limits";
import { passwordValid } from "@/lib/password";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";

type UserFormProps = {
  user: User | null;
  roles: Role[];
  onClose: () => void;
  onSaved: () => void;
  // onHandoff receives the temporary password once it is set, so the caller can
  // show it for copying; the form itself never displays it again.
  onHandoff?: (handoff: Handoff) => void;
};

// UserForm creates or edits an account. Editing also hosts the password reset,
// the SIP account and the activate/deactivate switch, so everything about one
// user lives in one place.
export default function UserForm({ user, roles, onClose, onSaved, onHandoff }: UserFormProps) {
  const { user: me } = useAuth();
  const editing = Boolean(user);
  const canAssign = can(me, "role.assign");
  const canDeactivate = can(me, "user.deactivate");

  const [form, setForm] = useState({
    name: user?.name ?? "",
    email: user?.email ?? "",
    password: "",
    roleIds: user?.roleIds ?? ([] as number[]),
    sipExtension: user?.sipExtension ?? "",
    sipPassword: "",
  });
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const update = (key: "name" | "email" | "password" | "sipExtension" | "sipPassword") => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm((prev) => ({ ...prev, [key]: e.target.value }));
    setError(null);
    setNotice(null);
  };

  const toggleRole = (id: number) => {
    setForm((prev) => ({
      ...prev,
      roleIds: prev.roleIds.includes(id) ? prev.roleIds.filter((r) => r !== id) : [...prev.roleIds, id],
    }));
    setError(null);
  };

  // run executes one action and reports its error inline; it returns whether it
  // succeeded so callers decide whether to close.
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
      return true;
    } catch (e) {
      setError(errorMessage(e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    if (form.name.trim().length < 2) {
      setError("İsim en az 2 karakter olmalı.");
      return;
    }
    if (!form.email.trim()) {
      setError("E-posta gerekli.");
      return;
    }
    if (!form.roleIds.length) {
      setError("En az bir rol seçmelisin.");
      return;
    }
    if (!editing && !passwordValid(form.password)) {
      setError("Şifre tüm kuralları karşılamalı.");
      return;
    }
    const payload = { name: form.name.trim(), email: form.email.trim(), roleIds: form.roleIds };

    if (user) {
      if (await run(() => api.updateUser(user.id, payload))) onSaved();
      return;
    }

    const ext = form.sipExtension.trim();
    const ok = await run(async () => {
      const created = await api.createUser({ ...payload, password: form.password, sipExtension: ext || undefined });
      if (ext && form.sipPassword) await api.setUserSip(created.id, ext, form.sipPassword);
    });
    if (ok) {
      onHandoff?.({ name: payload.name, email: payload.email, password: form.password });
      onSaved();
    }
  };

  const resetPassword = async () => {
    if (!user) return;
    if (!passwordValid(form.password)) {
      setError("Yeni şifre tüm kuralları karşılamalı.");
      return;
    }
    if (await run(() => api.resetUserPassword(user.id, form.password))) {
      // Close the form and hand the password over once; it is never shown here.
      onHandoff?.({ name: user.name, email: user.email, password: form.password });
      onSaved();
    }
  };

  const pullSip = async () => {
    if (!user) return;
    const ext = form.sipExtension.trim();
    if (!ext) {
      setError("Dahili numarası gerekli.");
      return;
    }
    if (await run(() => api.syncUserSip(user.id, ext))) {
      setNotice(`${ext} dahilisinin SIP şifresi Verimor'dan çekildi.`);
    }
  };

  const saveSip = async () => {
    if (!user) return;
    const ext = form.sipExtension.trim();
    if (!ext || !form.sipPassword) {
      setError("Dahili ve SIP şifresi gerekli.");
      return;
    }
    if (await run(() => api.setUserSip(user.id, ext, form.sipPassword))) {
      setForm((prev) => ({ ...prev, sipPassword: "" }));
      setNotice(`${ext} dahilisi kaydedildi.`);
    }
  };

  const toggleActive = async () => {
    if (!user) return;
    if (await run(() => api.setUserActive(user.id, !user.active))) onSaved();
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={editing ? "Kullanıcıyı Düzenle" : "Yeni Kullanıcı"}
      description={editing ? user?.email : "Hesap açılır, kullanıcı ilk girişte kendi şifresini belirler."}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
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
        {notice && <p className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-sm text-success">{notice}</p>}

        <div className="space-y-1">
          <Field label="Ad Soyad">
            <Input value={form.name} maxLength={LIMITS.userName} onChange={update("name")} autoFocus />
          </Field>
          <CharCount value={form.name} max={LIMITS.userName} className="text-right" />
        </div>

        <Field label="E-posta">
          <Input type="email" value={form.email} maxLength={LIMITS.email} onChange={update("email")} />
        </Field>

        {!editing && (
          <div className="space-y-2">
            <Field label="Geçici şifre">
              <PasswordField
                generator
                autoComplete="new-password"
                value={form.password}
                onChange={(v) => { setForm((prev) => ({ ...prev, password: v })); setError(null); }}
                placeholder="Yaz ya da asa ile oluştur"
              />
            </Field>
            <PasswordRules value={form.password} />
            <FieldHint>Kullanıcı ilk girişinde kendi şifresini belirlemek zorunda kalacak.</FieldHint>
          </div>
        )}

        <FieldGroup label="Roller">
          <div className="flex flex-wrap gap-2">
            {roles.map((r) => {
              const selected = form.roleIds.includes(r.id);
              return (
                <button
                  key={r.id}
                  type="button"
                  disabled={editing && !canAssign}
                  onClick={() => toggleRole(r.id)}
                  className={cn(
                    "rounded-full border px-3 py-1.5 text-xs font-medium transition-colors duration-200 disabled:cursor-not-allowed disabled:opacity-60",
                    selected
                      ? "border-primary bg-primary text-primary-foreground"
                      : "border-border/70 bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  {r.displayName}
                </button>
              );
            })}
            {!roles.length && <FieldHint>Rol listesi yüklenemedi.</FieldHint>}
          </div>
          {editing && !canAssign && <FieldHint>Rolleri değiştirmek için rol atama yetkisi gerekir.</FieldHint>}
        </FieldGroup>

        {!editing && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Dahili (SIP)">
              <Input value={form.sipExtension} maxLength={LIMITS.sipExtension} onChange={update("sipExtension")} placeholder="1005" inputMode="numeric" />
            </Field>
            <div className="space-y-1.5">
              <Field label="SIP Şifresi">
                <Input type="password" autoComplete="off" value={form.sipPassword} onChange={update("sipPassword")} placeholder="İsteğe bağlı" />
              </Field>
              <FieldHint>Boş bırakılırsa sonradan Verimor'dan çekilebilir.</FieldHint>
            </div>
          </div>
        )}

        {editing && user && (
          <>
            <Separator />

            <FieldGroup label="SIP Hesabı">
              <div className="grid gap-2 sm:grid-cols-[7rem_1fr]">
                <Input value={form.sipExtension} maxLength={LIMITS.sipExtension} onChange={update("sipExtension")} placeholder="Dahili" inputMode="numeric" />
                <PasswordField autoComplete="off" value={form.sipPassword} onChange={(v) => { setForm((prev) => ({ ...prev, sipPassword: v })); setError(null); }} placeholder="SIP şifresi (elle girilecekse)" />
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" onClick={pullSip} disabled={busy || !form.sipExtension.trim()}>
                  Verimor'dan çek
                </Button>
                <Button variant="secondary" onClick={saveSip} disabled={busy || !form.sipExtension.trim() || !form.sipPassword}>
                  Elle kaydet
                </Button>
              </div>
              <FieldHint>Verimor'dan çekmek için dahilinin OİM'de bir personele bağlı olması gerekir.</FieldHint>
            </FieldGroup>

            <FieldGroup label="Şifre Sıfırla">
              <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
                <PasswordField
                  generator
                  autoComplete="new-password"
                  value={form.password}
                  onChange={(v) => { setForm((prev) => ({ ...prev, password: v })); setError(null); setNotice(null); }}
                  placeholder="Yeni şifre"
                />
                <Button variant="secondary" onClick={resetPassword} disabled={busy || !passwordValid(form.password)}>
                  Güncelle
                </Button>
              </div>
              <PasswordRules value={form.password} />
              <FieldHint>Açık oturumları kapanır ve bir sonraki girişte şifresini yeniden belirler.</FieldHint>
            </FieldGroup>

            {canDeactivate && user.id !== me?.id && (
              <FieldGroup label="Hesap Durumu">
                <div>
                  <Button variant={user.active ? "danger" : "secondary"} disabled={busy} onClick={toggleActive}>
                    {user.active ? "Pasife Al" : "Aktifleştir"}
                  </Button>
                </div>
              </FieldGroup>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
