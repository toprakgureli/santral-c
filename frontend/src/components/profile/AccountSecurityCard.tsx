// AccountSecurityCard lets a person change their own password and sign out
// on every device, for example after using a shared computer.

import { useState } from "react";
import { KeyRound, Loader2, LogOut } from "lucide-react";
import { api, ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { Button, Card, ConfirmDialog, Field, FieldHint } from "@/components/ui";
import PasswordField, { PasswordRules } from "@/components/ui/PasswordField";
import { passwordValid } from "@/lib/password";

export default function AccountSecurityCard() {
  const { setUser } = useAuth();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [asking, setAsking] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);

  const change = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!current || !passwordValid(next) || busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const res = await api.changeOwnPassword(current, next);
      if (res.user) setUser(res.user);
      setCurrent("");
      setNext("");
      setNotice("Şifren değişti. Diğer cihazlardaki oturumların kapandı, bu cihazda devam ediyorsun.");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Şifre değiştirilemedi.");
    } finally {
      setBusy(false);
    }
  };

  const leaveEverywhere = async () => {
    setLeaving(true);
    setLeaveError(null);
    try {
      await api.logoutEverywhere();
      setUser(null);
    } catch (err) {
      setLeaveError(err instanceof ApiError ? err.message : "Oturumlar kapatılamadı.");
      setLeaving(false);
    }
  };

  return (
    <Card title="Hesap güvenliği" icon={KeyRound}>
      <form onSubmit={change} className="space-y-3">
        {error && <p className="rounded-xl bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        {notice && <p className="rounded-xl bg-success/10 px-3 py-2 text-sm text-success">{notice}</p>}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Mevcut şifre">
            <PasswordField autoComplete="current-password" value={current} onChange={(v) => { setCurrent(v); setError(null); }} />
          </Field>
          <Field label="Yeni şifre">
            <PasswordField generator autoComplete="new-password" value={next} onChange={(v) => { setNext(v); setError(null); }} />
          </Field>
        </div>
        <PasswordRules value={next} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <FieldHint>Şifreni değiştirince diğer cihazlardaki oturumların kapanır.</FieldHint>
          <Button type="submit" disabled={busy || !current || !passwordValid(next)}>
            {busy && <Loader2 className="animate-spin" />}
            {busy ? "Değiştiriliyor..." : "Şifremi değiştir"}
          </Button>
        </div>
      </form>

      <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border/60 pt-4">
        <div className="min-w-0">
          <p className="text-sm font-medium">Bütün cihazlardan çık</p>
          <p className="text-xs text-muted-foreground">Başka bilgisayarlarda açık unuttuğun oturumlar dahil, her yerden çıkış yapılır.</p>
        </div>
        <Button variant="secondary" onClick={() => { setLeaveError(null); setAsking(true); }}>
          <LogOut className="size-4" /> Her yerden çık
        </Button>
      </div>

      <ConfirmDialog
        open={asking}
        tone="warning"
        title="Bütün cihazlardan çıkılsın mı?"
        description="Bu cihaz dahil her yerde oturumun kapanır. Devam etmek için yeniden giriş yapman gerekir."
        confirmLabel="Her yerden çık"
        busy={leaving}
        error={leaveError}
        onCancel={() => setAsking(false)}
        onConfirm={() => void leaveEverywhere()}
      />
    </Card>
  );
}
