import { useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/api/client";
import type { LoginChallenge, User } from "@/api/types";
import { Button, Field, Input } from "@/components/ui";
import PasswordField, { PasswordRules } from "@/components/ui/PasswordField";
import { PASSWORD_MAX, passwordValid } from "@/lib/password";
import AuthError from "./AuthError";
import AuthLayout from "./AuthLayout";
import { errorMessage } from "./messages";

type Props = {
  token: string;
  onCancel: () => void;
  onUser: (user: User) => void;
  onChallenge: (challenge: LoginChallenge) => void;
};

// PasswordChange is the forced first-login password screen with the policy
// checklist, a generator and a confirmation field.
export default function PasswordChange({ token, onCancel, onUser, onChallenge }: Props) {
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const valid = passwordValid(password);
  const mismatch = confirm.length > 0 && confirm !== password;
  const ready = valid && password === confirm;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) {
      setError("Şifre tüm kuralları karşılamalı.");
      return;
    }
    if (password !== confirm) {
      setError("Şifreler eşleşmiyor.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await api.passwordChange(token, password);
      setBusy(false);
      if (res.user) {
        onUser(res.user);
        return;
      }
      if (res.challenge) {
        onChallenge(res.challenge);
        return;
      }
      throw new Error("unexpected");
    } catch (err) {
      setBusy(false);
      setError(errorMessage(err));
      setPassword("");
      setConfirm("");
      inputRef.current?.focus();
    }
  };

  return (
    <AuthLayout
      title="Şifreni Belirle"
      description="Hesabına ilk kez giriyorsun. Devam etmek için kendine ait bir şifre belirle."
    >
      <form onSubmit={submit} noValidate>
        <div className="flex flex-col gap-6">
          {error && <AuthError message={error} />}

          <div className="flex flex-col gap-2.5">
            <Field label="Yeni şifre">
              <PasswordField
                ref={inputRef}
                generator
                autoComplete="new-password"
                placeholder="••••••••"
                value={password}
                onChange={(v) => {
                  setPassword(v);
                  setError(null);
                }}
              />
            </Field>
            <PasswordRules value={password} />
          </div>

          <div className="flex flex-col gap-2">
            <Field label="Yeni şifre (tekrar)">
              <Input
                type="password"
                autoComplete="new-password"
                maxLength={PASSWORD_MAX}
                placeholder="••••••••"
                value={confirm}
                aria-invalid={mismatch}
                className={mismatch ? "border-destructive/60" : undefined}
                onChange={(e) => {
                  setConfirm(e.target.value);
                  setError(null);
                }}
              />
            </Field>
            {mismatch && <p className="text-xs text-destructive">Şifreler eşleşmiyor.</p>}
          </div>

          <Button type="submit" className="w-full" disabled={busy || !ready}>
            {busy && <Loader2 className="animate-spin" />}
            {busy ? "Kaydediliyor..." : "Şifreyi Kaydet ve Devam Et"}
          </Button>

          <Button type="button" variant="ghost" className="w-full" onClick={onCancel} disabled={busy}>
            Geri dön
          </Button>
        </div>
      </form>
    </AuthLayout>
  );
}
