import { Mic, MicOff, Pause, Phone, PhoneOff, Play } from "lucide-react";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

const statusLabel: Record<string, string> = {
  calling: "Aranıyor",
  ringing: "Çalıyor",
  incoming: "Gelen çağrı",
  "in-call": "Görüşme",
  held: "Beklemede",
};

// A floating call bar shown on every page while a call is active, so incoming
// calls can be answered and ongoing calls controlled regardless of the route.
export default function CallBar() {
  const phone = useSoftphoneContext();
  const active = ["calling", "ringing", "incoming", "in-call", "held"].includes(phone.status);
  if (!active) return null;

  const inCall = phone.status === "in-call" || phone.status === "held";

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 fixed right-5 bottom-5 z-50 w-72 rounded-2xl border border-border bg-popover p-3 text-popover-foreground shadow-xl duration-200">
      <div className="mb-3 flex items-center gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
          <Phone className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{phone.peer ?? "—"}</div>
          <div className="text-xs text-muted-foreground">{statusLabel[phone.status]}</div>
        </div>
      </div>

      {phone.status === "incoming" ? (
        <div className="grid grid-cols-2 gap-2">
          <button
            onClick={() => phone.answer().catch(() => undefined)}
            className="flex items-center justify-center gap-2 rounded-xl bg-success px-3 py-2 text-sm font-medium text-white hover:opacity-90"
          >
            <Phone className="size-4" /> Cevapla
          </button>
          <button
            onClick={() => phone.hangup().catch(() => undefined)}
            className="flex items-center justify-center gap-2 rounded-xl bg-destructive px-3 py-2 text-sm font-medium text-destructive-foreground hover:opacity-90"
          >
            <PhoneOff className="size-4" /> Reddet
          </button>
        </div>
      ) : (
        <div className="flex items-center gap-2">
          {inCall && (
            <>
              <button
                onClick={phone.toggleMute}
                title={phone.muted ? "Susturmayı aç" : "Sustur"}
                className="flex size-10 items-center justify-center rounded-xl border border-border/70 hover:bg-accent"
              >
                {phone.muted ? <MicOff className="size-4" /> : <Mic className="size-4" />}
              </button>
              <button
                onClick={() => phone.toggleHold().catch(() => undefined)}
                title={phone.held ? "Devam et" : "Beklet"}
                className="flex size-10 items-center justify-center rounded-xl border border-border/70 hover:bg-accent"
              >
                {phone.held ? <Play className="size-4" /> : <Pause className="size-4" />}
              </button>
            </>
          )}
          <button
            onClick={() => phone.hangup().catch(() => undefined)}
            className="ml-auto flex items-center justify-center gap-2 rounded-xl bg-destructive px-4 py-2 text-sm font-medium text-destructive-foreground hover:opacity-90"
          >
            <PhoneOff className="size-4" /> Kapat
          </button>
        </div>
      )}
    </div>
  );
}
