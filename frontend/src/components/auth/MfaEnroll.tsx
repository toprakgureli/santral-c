import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/api/client";
import type { LoginChallenge, User } from "@/api/types";
import { Button } from "@/components/ui";
import OtpInput from "@/components/ui/OtpInput";
import AuthError from "./AuthError";
import AuthLayout from "./AuthLayout";
import { errorMessage } from "./messages";

type Props = {
  token: string;
  onCancel: () => void;
  onUser: (user: User) => void;
  onChallenge: (challenge: LoginChallenge) => void;
};

type Setup = { qr: string; secret: string };

const STEPS = [
  "Google Authenticator veya Microsoft Authenticator uygulamasını aç.",
  "Aşağıdaki QR kodunu okut.",
  "Uygulamadaki 6 haneli kodu gir.",
];

// MfaEnroll is the forced first-login TOTP setup: it mints the secret, shows the
// QR and the manual key, and completes enrollment with the first valid code.
export default function MfaEnroll({ token, onCancel, onUser, onChallenge }: Props) {
  const [setup, setSetup] = useState<Setup | null>(null);
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    api
      .mfaEnroll(token)
      .then((res) => {
        if (active) setSetup({ qr: res.qr, secret: res.secret });
      })
      .catch((e) => {
        if (active) setError(errorMessage(e));
      });
    return () => {
      active = false;
    };
  }, [token]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (code.length !== 6) {
      setError("6 haneli kodu gir.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const res = await api.mfaEnrollVerify(token, code);
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
      setCode("");
    }
  };

  return (
    <AuthLayout
      title="Güvenlik Kurulumu"
      description="Devam etmek için iki adımlı doğrulamayı kurman gerekiyor. Bu adım zorunludur."
    >
      <form onSubmit={submit} noValidate>
        <div className="flex flex-col gap-6">
          {error && <AuthError message={error} />}

          {!setup ? (
            <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Kurulum hazırlanıyor...
            </div>
          ) : (
            <>
              <ol className="list-inside list-decimal space-y-1 text-sm text-muted-foreground">
                {STEPS.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>

              <div className="flex justify-center rounded-lg border border-border/60 bg-card p-4">
                <img src={setup.qr} alt="QR kodu" width={180} height={180} />
              </div>

              <div className="flex flex-col gap-2">
                <span className="text-xs font-medium text-muted-foreground">QR okutamıyorsan bu anahtarı elle gir</span>
                <code className="block rounded-md bg-muted px-3 py-2 font-mono text-xs break-all text-muted-foreground">
                  {setup.secret}
                </code>
              </div>

              <div className="flex flex-col gap-2">
                <span className="text-xs font-medium text-muted-foreground">Uygulamadaki kod</span>
                <OtpInput
                  id="enroll-code"
                  value={code}
                  onChange={(next) => {
                    setCode(next);
                    setError(null);
                  }}
                />
                <p className="text-xs text-muted-foreground">Kurulum tamamlanana kadar oturumun açılmaz.</p>
              </div>

              <Button type="submit" className="w-full" disabled={busy || code.length !== 6}>
                {busy && <Loader2 className="animate-spin" />}
                {busy ? "Doğrulanıyor..." : "Kurulumu Tamamla"}
              </Button>
            </>
          )}

          <Button type="button" variant="ghost" className="w-full" onClick={onCancel} disabled={busy}>
            Geri dön
          </Button>
        </div>
      </form>
    </AuthLayout>
  );
}
