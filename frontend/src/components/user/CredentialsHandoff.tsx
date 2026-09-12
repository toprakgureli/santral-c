import { useEffect, useState } from "react";
import { Check, Copy, Eye, EyeOff } from "lucide-react";
import { Button, FieldGroup, FieldHint, Modal } from "@/components/ui";
import { cn } from "@/lib/utils";

export type Handoff = {
  name: string;
  email: string;
  password: string;
};

// handoffMessage is the note an admin pastes to the user after creating the
// account or resetting the password.
export function handoffMessage(h: Handoff): string {
  const host = window.location.host;
  return [
    `Sayın ${h.name},`,
    "",
    `${host} üzerinden ${h.email} e-posta adresi ve ${h.password} geçici şifresiyle sisteme giriş yapabilirsiniz. İlk girişte sizden kendinize ait yeni bir şifre belirlemeniz istenecek.`,
    "",
    "İki adımlı doğrulama (TOTP) için telefonunuza Google Authenticator veya Microsoft Authenticator uygulamasını indirip giriş sırasında çıkan QR kodu okutmanız yeterli.",
    "",
    "Sorun yaşarsanız yöneticinizle iletişime geçin.",
  ].join("\n");
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Older browsers or a non-secure context: fall back to a hidden textarea.
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  }
}

// CopyButton copies text and briefly confirms it.
function CopyButton({ text, label, variant = "secondary" }: { text: string; label: string; variant?: "primary" | "secondary" }) {
  const [done, setDone] = useState(false);
  useEffect(() => {
    if (!done) return;
    const t = window.setTimeout(() => setDone(false), 2000);
    return () => window.clearTimeout(t);
  }, [done]);
  return (
    <Button
      variant={variant}
      className={cn(done && "bg-success text-white hover:bg-success")}
      onClick={async () => {
        if (await copyText(text)) setDone(true);
      }}
    >
      {done ? <Check /> : <Copy />}
      {done ? "Kopyalandı" : label}
    </Button>
  );
}

// CredentialsHandoff shows the temporary password once, masked, with one-click
// copies for the password alone and for a ready-to-send welcome message. It is
// the only place the plain password is ever displayed.
export default function CredentialsHandoff({ handoff, onClose }: { handoff: Handoff; onClose: () => void }) {
  const [show, setShow] = useState(false);
  const message = handoffMessage(handoff);

  return (
    <Modal open onClose={onClose} title="Giriş Bilgileri Hazır" description={`${handoff.name} · ${handoff.email}`} footer={<Button onClick={onClose}>Kapat</Button>}>
      <div className="space-y-5">
        <FieldGroup label="Geçici şifre">
          <div className="flex items-center gap-2">
            <code
              className={cn(
                "flex h-10 flex-1 items-center rounded-xl border border-border/70 bg-muted/40 px-3.5 font-mono text-sm tracking-wider select-all",
                !show && "tracking-[0.3em]",
              )}
            >
              {show ? handoff.password : "•".repeat(handoff.password.length)}
            </code>
            <Button variant="secondary" className="px-3" onClick={() => setShow((v) => !v)} aria-label={show ? "Şifreyi gizle" : "Şifreyi göster"}>
              {show ? <EyeOff /> : <Eye />}
            </Button>
            <CopyButton text={handoff.password} label="Şifreyi kopyala" />
          </div>
          <FieldHint>Bu şifre bir daha gösterilmez. Kullanıcı ilk girişte kendi şifresini belirleyecek.</FieldHint>
        </FieldGroup>

        <FieldGroup label="Kullanıcıya gönderilecek mesaj">
          <textarea
            readOnly
            value={message}
            rows={9}
            onFocus={(e) => e.target.select()}
            className="w-full resize-none rounded-xl border border-border/70 bg-muted/40 px-3.5 py-2.5 text-sm leading-relaxed text-foreground outline-none focus-visible:border-ring/60 focus-visible:ring-4 focus-visible:ring-ring/20"
          />
          <div className="flex justify-end">
            <CopyButton text={message} label="Mesajı kopyala" variant="primary" />
          </div>
        </FieldGroup>
      </div>
    </Modal>
  );
}
