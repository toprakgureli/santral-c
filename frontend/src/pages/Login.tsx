import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Eye, EyeOff, Loader2, Pencil } from "lucide-react";
import { api, ApiError } from "../api/client";
import type { LoginResult } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import AuthLayout from "../components/auth/AuthLayout";
import { Button, ErrorText, Field, Input } from "../components/ui";
import { APP_FULL_NAME } from "@/lib/brand";

type Step = "email" | "password" | "passwordChange" | "mfa" | "enroll";

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function Login() {
  const { setUser } = useAuth();
  const navigate = useNavigate();

  const [step, setStep] = useState<Step>("email");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [code, setCode] = useState("");
  const [token, setToken] = useState("");
  const [enrollQr, setEnrollQr] = useState("");
  const [enrollSecret, setEnrollSecret] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const passwordRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (step === "password") passwordRef.current?.focus();
  }, [step]);

  function handleResult(result: LoginResult) {
    if (result.user) {
      setUser(result.user);
      navigate("/");
      return;
    }
    const c = result.challenge ?? {};
    if (c.passwordChangeRequired && c.passwordToken) {
      setToken(c.passwordToken);
      setStep("passwordChange");
    } else if (c.mfaSetupRequired && c.mfaToken) {
      setToken(c.mfaToken);
      void startEnroll(c.mfaToken);
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
      setEnrollQr(res.qr);
      setEnrollSecret(res.secret);
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

  const description =
    step === "email" ? "Devam etmek için e-posta adresini gir." : "Şifreni girerek oturumunu aç.";

  return (
    <AuthLayout title={APP_FULL_NAME} description={step === "email" || step === "password" ? description : "Güvenlik adımı"}>
      {step === "email" && (
        <form
          className="animate-in fade-in slide-in-from-left-2 space-y-6 duration-300 ease-out"
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            if (!EMAIL_PATTERN.test(email.trim())) {
              setError("Geçerli bir e-posta girin.");
              return;
            }
            setError(null);
            setStep("password");
          }}
        >
          <Field label="E-posta">
            <Input type="email" autoComplete="username" autoFocus placeholder="ornek@mail.com" value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <ErrorText>{error}</ErrorText>
          <Button type="submit" className="w-full">Devam et</Button>
        </form>
      )}

      {step === "password" && (
        <form
          className="animate-in fade-in slide-in-from-right-2 space-y-6 duration-300 ease-out"
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            run(() => api.login(email.trim(), password));
          }}
        >
          <button
            type="button"
            onClick={() => { setStep("email"); setPassword(""); setError(null); }}
            className="mx-auto flex max-w-full items-center gap-2 rounded-full border border-border/50 bg-muted/30 py-1 pr-3 pl-1 text-xs text-muted-foreground hover:bg-muted/60 hover:text-foreground"
          >
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-card text-[0.7rem] font-semibold text-foreground uppercase ring-1 ring-border/60">
              {email.trim().charAt(0)}
            </span>
            <span className="truncate font-medium">{email.trim()}</span>
            <Pencil className="size-3 shrink-0 opacity-50" />
          </button>

          <Field label="Şifre">
            <div className="relative">
              <Input
                ref={passwordRef}
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                placeholder="••••••••"
                className="pr-10"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <button
                type="button"
                tabIndex={-1}
                onClick={() => setShowPassword((v) => !v)}
                aria-label={showPassword ? "Şifreyi gizle" : "Şifreyi göster"}
                className="absolute top-1/2 right-2 flex size-8 -translate-y-1/2 items-center justify-center rounded-lg text-muted-foreground hover:text-foreground"
              >
                {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
          </Field>
          <ErrorText>{error}</ErrorText>
          <Button type="submit" className="w-full" disabled={busy}>
            {busy && <Loader2 className="size-4 animate-spin" />}
            {busy ? "Giriş yapılıyor..." : "Giriş Yap"}
          </Button>
        </form>
      )}

      {step === "passwordChange" && (
        <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); run(() => api.passwordChange(token, newPassword)); }}>
          <p className="text-sm text-muted-foreground">İlk giriş: lütfen yeni bir şifre belirleyin.</p>
          <Field label="Yeni şifre">
            <Input type="password" autoFocus minLength={8} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
          </Field>
          <ErrorText>{error}</ErrorText>
          <Button type="submit" className="w-full" disabled={busy}>Şifreyi belirle</Button>
        </form>
      )}

      {step === "mfa" && (
        <form className="space-y-6" onSubmit={(e) => { e.preventDefault(); run(() => api.mfaVerify(token, code)); }}>
          <p className="text-sm text-muted-foreground">Doğrulama uygulamandaki 6 haneli kodu gir.</p>
          <Field label="Kod">
            <Input inputMode="numeric" maxLength={6} autoFocus value={code} onChange={(e) => setCode(e.target.value)} />
          </Field>
          <ErrorText>{error}</ErrorText>
          <Button type="submit" className="w-full" disabled={busy}>Doğrula</Button>
        </form>
      )}

      {step === "enroll" && (
        <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); run(() => api.mfaEnrollVerify(token, code)); }}>
          <p className="text-sm text-muted-foreground">İki adımlı doğrulamayı kur: QR'ı doğrulama uygulamanla (Google Authenticator, Authy...) tara, sonra üretilen 6 haneli kodu gir.</p>
          {enrollQr && (
            <img src={enrollQr} alt="TOTP QR" width={192} height={192} className="mx-auto rounded-xl bg-white p-2 shadow-sm" />
          )}
          <div className="space-y-1">
            <p className="text-center text-xs text-muted-foreground">QR okutamıyorsan bu anahtarı elle ekle:</p>
            <div className="rounded-lg bg-muted/50 p-2 text-center font-mono text-xs tracking-wider break-all text-muted-foreground">{enrollSecret}</div>
          </div>
          <Field label="Kod">
            <Input inputMode="numeric" maxLength={6} autoFocus value={code} onChange={(e) => setCode(e.target.value)} />
          </Field>
          <ErrorText>{error}</ErrorText>
          <Button type="submit" className="w-full" disabled={busy}>Kur ve gir</Button>
        </form>
      )}

      <p className="mt-5 text-center text-xs text-muted-foreground">Sorun olursa yöneticinle iletişime geç.</p>
    </AuthLayout>
  );
}

function message(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return "Bir hata oluştu.";
}
