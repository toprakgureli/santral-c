// UnreachedPrompt appears the moment an outbound call ends without reaching
// the customer (no answer, busy, cancelled while ringing, or "answered" by
// the PBX only for a short announcement) and offers the WhatsApp follow-up
// with the agent's own message. Only Şimdi değil or writing closes it: a
// call that arrives meanwhile rings in the floating call bar above the card
// and never closes it. It shows once per unreached call.

import { useEffect, useState } from "react";
import { PhoneMissed, Settings2, X } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import WhatsAppIcon from "@/components/icons/WhatsAppIcon";
import WhatsAppTemplateDialog from "@/components/WhatsAppTemplateDialog";
import { Button } from "@/components/ui";
import { setWhatsAppPromptOpen } from "@/lib/overlays";
import { whatsappLink, whatsappNumber, whatsappTextFor } from "@/lib/whatsapp";
import { displayNumber } from "@/softphone/dial";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const REASONS: Record<string, string> = {
  no_answer: "Cevap vermedi",
  busy: "Meşgul",
  canceled: "Çalarken kapattın",
  short: "Santral anonsu, görüşme olmadı",
};

export default function UnreachedPrompt() {
  const phone = useSoftphoneContext();
  const { user } = useAuth();
  const [seenId, setSeenId] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);

  const call = phone.lastUnreached;
  const busy = ["calling", "ringing", "incoming", "in-call", "held"].includes(phone.status);
  const number = call ? whatsappNumber(call.peer) : "";
  const show = !!call && call.id !== seenId && number !== "";

  useEffect(() => {
    setWhatsAppPromptOpen(show);
    return () => setWhatsAppPromptOpen(false);
  }, [show]);

  if (!show || !call) return null;

  const shown = displayNumber(call.peer) || call.peer;

  function dismiss() {
    setSeenId(call!.id);
  }

  function write() {
    window.open(whatsappLink(number, whatsappTextFor(user, "unreached", shown)), "_blank", "noopener");
    dismiss();
  }

  return (
    <div className="fixed inset-0 z-[58] flex items-center justify-center bg-background/60 p-4 backdrop-blur-sm">
      <WhatsAppTemplateDialog open={editing} onClose={() => setEditing(false)} previewNumber={shown} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="unreached-title"
        className="animate-in fade-in zoom-in-95 relative w-full max-w-md overflow-hidden rounded-3xl border-2 border-[#25D366]/40 bg-card shadow-2xl shadow-[#25D366]/10 duration-300"
      >
        <button type="button" onClick={dismiss} aria-label="Kapat" className="absolute right-4 top-4 z-10 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" data-tip="Kapat">
          <X className="size-4" />
        </button>

        <div className="relative px-7 pt-7 pb-5">
          <div className="pointer-events-none absolute -right-10 -top-10 size-40 rounded-full bg-[#25D366]/15 blur-3xl" />
          <div className="flex items-center gap-3">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-warning/15 text-warning">
              <PhoneMissed className="size-5" />
            </span>
            <div className="min-w-0">
              <h2 id="unreached-title" className="text-lg font-semibold leading-tight">Ulaşamadın</h2>
              <p className="text-xs text-muted-foreground">{REASONS[call.reason] ?? "Görüşme olmadı"}. WhatsApp'tan yazmak ister misin?</p>
              {busy && <p className="mt-0.5 text-xs font-medium text-success">Çağrı sürüyor, kart açık kalır.</p>}
            </div>
          </div>

          <div className="mt-5 flex items-center justify-between gap-3 rounded-2xl bg-muted/40 px-4 py-3 ring-1 ring-border/60">
            <div>
              <div className="text-[0.7rem] font-medium uppercase tracking-wide text-muted-foreground">Müşteri</div>
              <div className="mt-0.5 font-mono text-2xl font-bold tabular-nums tracking-wide">{shown}</div>
            </div>
            <button
              type="button"
              onClick={() => setEditing(true)}
              data-tip="Mesajı düzenle"
              className="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              <Settings2 className="size-3.5" /> Mesajı düzenle
            </button>
          </div>

          <div className="mt-3 rounded-2xl rounded-tl-sm bg-[#25D366]/10 px-4 py-3 text-sm leading-relaxed ring-1 ring-[#25D366]/25">
            {whatsappTextFor(user, "unreached", shown)}
          </div>
        </div>

        <div className="flex gap-2 px-7 pb-7">
          <Button variant="secondary" onClick={dismiss} className="h-12 flex-1 text-base">Şimdi değil</Button>
          <Button onClick={write} className="h-12 flex-[1.4] gap-2 bg-[#25D366] text-base text-black shadow-md hover:bg-[#25D366]/90">
            <WhatsAppIcon className="size-5" /> WhatsApp'tan yaz
          </Button>
        </div>
      </div>
    </div>
  );
}
