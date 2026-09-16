import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Eye, EyeOff, Loader2, Pencil } from "lucide-react";
import { api } from "@/api/client";
import type { LoginChallenge, User } from "@/api/types";
import { useAuth } from "@/auth/AuthContext";
import AuthError from "@/components/auth/AuthError";
import AuthLayout from "@/components/auth/AuthLayout";
import MfaChallenge from "@/components/auth/MfaChallenge";
import MfaEnroll from "@/components/auth/MfaEnroll";
import PasswordChange from "@/components/auth/PasswordChange";
import { SUCCESS_DELAY_MS, errorMessage } from "@/components/auth/messages";
import { Button, Field, Input } from "@/components/ui";
import SuccessCheck from "@/components/ui/SuccessCheck";

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

// Login is the sign-in state machine: email -> password, then whichever
// security step the backend asks for (forced MFA enrollment, MFA code, forced
// password change), each rendered by its own screen.
export function Login() {
  const { setUser } = useAuth();
  const navigate = useNavigate();

  const [step, setStep] = useState<"email" | "password">("email");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [emailError, setEmailError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  const [challenge, setChallenge] = useState<string | null>(null);
  const [enroll, setEnroll] = useState<string | null>(null);
  const [passwordToken, setPasswordToken] = useState<string | null>(null);

  const passwordRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (step === "password") passwordRef.current?.focus();
  }, [step]);

  const reset = () => {
    setChallenge(null);
    setEnroll(null);
    setPasswordToken(null);
    setPassword("");
    setStep("email");
  };

  const finish = (user: User) => {
    setUser(user);
    navigate("/");
  };

  // route sends the next security step to its screen; the backend issues one
  // challenge at a time.
  const route = (c: LoginChallenge) => {
    if (c.mfaSetupRequired && c.mfaToken) {
      setEnroll(c.mfaToken);
      return;
    }
    if (c.mfaRequired && c.mfaToken) {
      setChallenge(c.mfaToken);
      return;
    }
    if (c.passwordChangeRequired && c.passwordToken) {
      setPasswordToken(c.passwordToken);
      return;
    }
    setFormError("Beklenmeyen bir yanıt alındı.");
  };

  const continueWithEmail = (e: React.FormEvent) => {
    e.preventDefault();
    const value = email.trim();
    if (!value) {
      setEmailError("E-posta gerekli.");
      return;
    }
    if (!EMAIL_PATTERN.test(value)) {
      setEmailError("Geçerli bir e-posta girin.");
      return;
    }
    setEmailError(null);
    setFormError(null);
    setStep("password");
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password.length < 8) {
      setFormError("Şifre en az 8 karakter olmalı.");
      return;
    }
    setLoading(true);
    setFormError(null);
    try {
      const res = await api.login(email.trim(), password);
      setLoading(false);
      if (res.user) {
        const user = res.user;
        setSuccess(true);
        window.setTimeout(() => finish(user), SUCCESS_DELAY_MS);
        return;
      }
      if (res.challenge) {
        route(res.challenge);
        return;
      }
      setFormError("Beklenmeyen bir yanıt alındı.");
    } catch (err) {
      setLoading(false);
      setFormError(errorMessage(err));
      setPassword("");
      passwordRef.current?.focus();
    }
  };

  if (passwordToken) {
    return <PasswordChange token={passwordToken} onCancel={reset} onUser={finish} onChallenge={route} />;
  }
  if (enroll) {
    return <MfaEnroll token={enroll} onCancel={reset} onUser={finish} onChallenge={route} />;
  }
  if (challenge) {
    return <MfaChallenge token={challenge} onCancel={reset} onUser={finish} onChallenge={route} />;
  }

  return (
    <AuthLayout
      title="Giriş yap"
      description={step === "email" ? "Devam etmek için e-posta adresini gir." : "Şifreni girerek oturumunu aç."}
    >
      {step === "email" ? (
        <form onSubmit={continueWithEmail} noValidate className="animate-in fade-in slide-in-from-left-2 duration-300 ease-out">
          <div className="flex flex-col gap-6">
            {formError && <AuthError message={formError} />}

            <div className="flex flex-col gap-2">
              <Field label="E-posta">
                <Input
                  type="email"
                  autoComplete="username"
                  autoFocus
                  placeholder="ornek@mail.com"
                  value={email}
                  aria-invalid={Boolean(emailError)}
                  className={emailError ? "border-destructive/60" : undefined}
                  onChange={(e) => {
                    setEmail(e.target.value);
                    setEmailError(null);
                  }}
                />
              </Field>
              {emailError && <p className="text-xs text-destructive">{emailError}</p>}
            </div>

            <Button type="submit" className="w-full">Devam et</Button>
          </div>
        </form>
      ) : (
        <form onSubmit={submit} noValidate className="animate-in fade-in slide-in-from-right-2 duration-300 ease-out">
          <div className="flex flex-col gap-6">
            <button
              type="button"
              title="E-posta adresini değiştir"
              onClick={() => {
                setStep("email");
                setPassword("");
                setFormError(null);
              }}
              className="group mx-auto flex max-w-full items-center gap-2 rounded-full border border-border/50 bg-muted/30 py-1 pr-3 pl-1 text-xs text-muted-foreground outline-none transition-[color,background-color,border-color,box-shadow] duration-200 ease-out hover:border-border hover:bg-muted/60 hover:text-foreground focus-visible:ring-4 focus-visible:ring-ring/25"
            >
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-card text-[0.7rem] font-semibold text-foreground uppercase ring-1 ring-border/60">
                {email.trim().charAt(0)}
              </span>
              <span className="truncate font-medium">{email.trim()}</span>
              <Pencil className="size-3 shrink-0 opacity-50 transition-opacity duration-200 group-hover:opacity-100" />
            </button>

            {formError && <AuthError message={formError} />}

            <Field label="Şifre">
              <div className="relative">
                <Input
                  ref={passwordRef}
                  type={showPassword ? "text" : "password"}
                  autoComplete="current-password"
                  placeholder="••••••••"
                  className="pr-10"
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    setFormError(null);
                  }}
                />
                <button
                  type="button"
                  tabIndex={-1}
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? "Şifreyi gizle" : "Şifreyi göster"}
                  className="absolute top-1/2 right-2 flex size-8 -translate-y-1/2 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:text-foreground"
                >
                  {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </Field>

            <Button
              type="submit"
              disabled={loading || success}
              className={success ? "w-full bg-success text-white shadow-sm hover:bg-success disabled:opacity-100" : "w-full"}
            >
              {success ? (
                <>
                  <SuccessCheck />
                  Giriş başarılı
                </>
              ) : (
                <>
                  {loading && <Loader2 className="animate-spin" />}
                  {loading ? "Giriş yapılıyor..." : "Giriş Yap"}
                </>
              )}
            </Button>
          </div>
        </form>
      )}

      <p className="mt-5 text-center text-xs text-muted-foreground">Sorun olursa yöneticinle iletişime geç.</p>
    </AuthLayout>
  );
}
