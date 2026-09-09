import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { LoginResult } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { Button, ErrorText, Field, Input } from "../components/ui";

type Step = "credentials" | "password" | "mfa" | "enroll";

export function Login() {
  const { setUser } = useAuth();
  const navigate = useNavigate();

  const [step, setStep] = useState<Step>("credentials");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [code, setCode] = useState("");
  const [token, setToken] = useState("");
  const [enrollUri, setEnrollUri] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function handleResult(result: LoginResult) {
    if (result.user) {
      setUser(result.user);
      navigate("/");
      return;
    }
    const c = result.challenge ?? {};
    if (c.passwordChangeRequired && c.passwordToken) {
      setToken(c.passwordToken);
      setStep("password");
    } else if (c.mfaSetupRequired && c.mfaToken) {
      setToken(c.mfaToken);
      startEnroll(c.mfaToken);
    } else if (c.mfaRequired && c.mfaToken) {
      setToken(c.mfaToken);
      setStep("mfa");
    } else {
      setError("Beklenmeyen bir yanıt alındı.");
    }
  }

  async function startEnroll(t: string) {
    try {
      const res = await api.mfaEnroll(t);
      setEnrollUri(res.otpauthUrl ?? res.uri ?? res.secret);
      setStep("enroll");
    } catch (e) {
      setError(message(e));
    }
  }

  async function run(fn: () => Promise<LoginResult>) {
    setBusy(true);
    setError(null);
    try {
      handleResult(await fn());
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-full items-center justify-center bg-slate-100 px-4">
      <div className="w-full max-w-sm rounded-2xl border border-slate-200 bg-white p-8 shadow-sm">
        <h1 className="mb-1 text-2xl font-bold text-brand-600">santral-c</h1>
        <p className="mb-6 text-sm text-slate-500">Çağrı yönetim paneli</p>

        {step === "credentials" && (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              run(() => api.login(email, password));
            }}
          >
            <Field label="E-posta">
              <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoFocus required />
            </Field>
            <Field label="Parola">
              <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </Field>
            <ErrorText>{error}</ErrorText>
            <Button type="submit" className="w-full" disabled={busy}>
              Giriş yap
            </Button>
          </form>
        )}

        {step === "password" && (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              run(() => api.passwordChange(token, newPassword));
            }}
          >
            <p className="text-sm text-slate-600">İlk giriş: lütfen yeni bir parola belirleyin.</p>
            <Field label="Yeni parola">
              <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoFocus required minLength={8} />
            </Field>
            <ErrorText>{error}</ErrorText>
            <Button type="submit" className="w-full" disabled={busy}>
              Parolayı belirle
            </Button>
          </form>
        )}

        {step === "mfa" && (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              run(() => api.mfaVerify(token, code));
            }}
          >
            <p className="text-sm text-slate-600">Doğrulama uygulamanızdaki 6 haneli kodu girin.</p>
            <Field label="Kod">
              <Input value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" maxLength={6} autoFocus required />
            </Field>
            <ErrorText>{error}</ErrorText>
            <Button type="submit" className="w-full" disabled={busy}>
              Doğrula
            </Button>
          </form>
        )}

        {step === "enroll" && (
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              run(() => api.mfaEnrollVerify(token, code));
            }}
          >
            <p className="text-sm text-slate-600">
              İki adımlı doğrulamayı kurun. Uygulamanıza aşağıdaki anahtarı ekleyin, ardından üretilen kodu girin.
            </p>
            <div className="break-all rounded-lg bg-slate-50 p-2 text-xs text-slate-500">{enrollUri}</div>
            <Field label="Kod">
              <Input value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" maxLength={6} autoFocus required />
            </Field>
            <ErrorText>{error}</ErrorText>
            <Button type="submit" className="w-full" disabled={busy}>
              Kur ve gir
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}

function message(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return "Bir hata oluştu.";
}
