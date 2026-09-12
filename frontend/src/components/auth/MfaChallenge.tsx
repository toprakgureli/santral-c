import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "@/api/client";
import type { LoginChallenge, User } from "@/api/types";
import { Button } from "@/components/ui";
import OtpInput, { type OtpHandle } from "@/components/ui/OtpInput";
import SuccessCheck from "@/components/ui/SuccessCheck";
import AuthError from "./AuthError";
import AuthLayout from "./AuthLayout";
import { SUCCESS_DELAY_MS, errorMessage } from "./messages";

type Props = {
  token: string;
  onCancel: () => void;
  onUser: (user: User) => void;
  onChallenge: (challenge: LoginChallenge) => void;
};

// MfaChallenge asks for the TOTP code at sign-in. It verifies automatically as
// soon as six digits are in, plays the green success state, then hands the
// session over. A failed code clears the boxes and refocuses the first one.
export default function MfaChallenge({ token, onCancel, onUser, onChallenge }: Props) {
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [success, setSuccess] = useState(false);
  const inputRef = useRef<OtpHandle>(null);
  const sending = useRef(false);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const verify = useCallback(
    async (value: string) => {
      if (value.length !== 6 || sending.current) return;
      sending.current = true;
      setBusy(true);
      setError(null);
      try {
        const res = await api.mfaVerify(token, value);
        setBusy(false);
        if (res.user) {
          const user = res.user;
          setSuccess(true);
          window.setTimeout(() => onUser(user), SUCCESS_DELAY_MS);
          return;
        }
        if (res.challenge) {
          onChallenge(res.challenge);
          return;
        }
        throw new Error("unexpected");
      } catch (e) {
        sending.current = false;
        setBusy(false);
        setError(errorMessage(e));
        setCode("");
        inputRef.current?.focus();
      }
    },
    [token, onUser, onChallenge],
  );

  return (
    <AuthLayout title="Doğrulama Kodu" description="Kimlik doğrulama uygulamandaki 6 haneli kodu gir.">
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          void verify(code);
        }}
      >
        <div className="flex flex-col gap-6">
          {error && <AuthError message={error} />}

          <div className="flex flex-col gap-2">
            <span className="text-xs font-medium text-muted-foreground">Kod</span>
            <OtpInput
              id="mfa-code"
              ref={inputRef}
              value={code}
              disabled={busy || success}
              onChange={(next) => {
                setCode(next);
                setError(null);
                void verify(next);
              }}
            />
          </div>

          <Button
            type="submit"
            className={success ? "w-full bg-success text-white shadow-sm hover:bg-success disabled:opacity-100" : "w-full"}
            disabled={busy || success || code.length !== 6}
          >
            {success ? (
              <>
                <SuccessCheck />
                Doğrulandı
              </>
            ) : (
              <>
                {busy && <Loader2 className="animate-spin" />}
                {busy ? "Doğrulanıyor..." : "Doğrula"}
              </>
            )}
          </Button>

          {!success && (
            <Button type="button" variant="ghost" className="w-full" onClick={onCancel} disabled={busy}>
              Geri dön
            </Button>
          )}
        </div>
      </form>
    </AuthLayout>
  );
}
