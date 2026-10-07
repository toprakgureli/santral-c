// Account ("Hesap ve güvenlik"): the person's own password and sign-ins,
// opened from the profile menu at the top right. WhatsApp's own settings
// stay under WhatsApp.

import { useNavigate } from "react-router-dom";
import { ArrowLeft, KeyRound } from "lucide-react";
import AccountSecurityCard from "@/components/profile/AccountSecurityCard";

export function Account() {
  const navigate = useNavigate();

  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <div className="flex items-center gap-3">
        <button type="button" onClick={() => (window.history.length > 1 ? navigate(-1) : navigate("/"))} aria-label="Geri" data-tip="Geri" className="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border/70 bg-card text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground">
          <ArrowLeft className="size-4" />
        </button>
        <span className="flex size-10 items-center justify-center rounded-2xl bg-primary/10 text-primary"><KeyRound className="size-5" /></span>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Hesap ve güvenlik</h1>
          <p className="text-xs text-muted-foreground">Şifren ve açık oturumların. Buradaki işlemler sadece senin hesabını etkiler.</p>
        </div>
      </div>

      <AccountSecurityCard />
    </div>
  );
}
