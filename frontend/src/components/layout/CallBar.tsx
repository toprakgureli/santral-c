import { useCallback, useEffect, useRef, useState } from "react";
import { GripVertical, Mic, MicOff, Pause, Phone, PhoneOff, Play } from "lucide-react";
import VoiceMixer from "@/components/layout/VoiceMixer";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { useAuth } from "@/auth/AuthContext";
import WhatsAppIcon from "@/components/icons/WhatsAppIcon";
import { displayNumber } from "@/softphone/dial";
import { whatsappLink, whatsappNumber, whatsappTextFor } from "@/lib/whatsapp";

const statusLabel: Record<string, string> = {
  calling: "Aranıyor",
  ringing: "Çalıyor",
  incoming: "Gelen çağrı",
  "in-call": "Görüşme",
  held: "Beklemede",
};

const POS_KEY = "callbar-pos";
const WIDTH = 288; // w-72
const MARGIN = 12;

type Pos = { x: number; y: number };

function clampPos(p: Pos, height = 220): Pos {
  const maxX = Math.max(MARGIN, window.innerWidth - WIDTH - MARGIN);
  const maxY = Math.max(MARGIN, window.innerHeight - height - MARGIN);
  return { x: Math.min(Math.max(MARGIN, p.x), maxX), y: Math.min(Math.max(MARGIN, p.y), maxY) };
}

// A floating call bar shown on every page while a call is active, so incoming
// calls can be answered and ongoing calls controlled regardless of the route.
// It can be dragged by its header and remembers where it was left.
export default function CallBar() {
  const phone = useSoftphoneContext();
  const { user } = useAuth();
  const waNumber = whatsappNumber(phone.peer ?? "");
  function openWhatsApp() {
    if (!waNumber) return;
    window.open(whatsappLink(waNumber, whatsappTextFor(user, "live", displayNumber(phone.peer ?? ""))), "_blank", "noopener");
  }
  const [pos, setPos] = useState<Pos | null>(() => {
    try {
      const raw = localStorage.getItem(POS_KEY);
      return raw ? (JSON.parse(raw) as Pos) : null;
    } catch {
      return null;
    }
  });
  const ref = useRef<HTMLDivElement>(null);
  const drag = useRef<{ dx: number; dy: number; x: number; y: number } | null>(null);

  // During a drag we write the position straight to the DOM and only commit to
  // React state (and storage) on release, so there is no per-frame re-render and
  // the widget follows the cursor smoothly.
  const onPointerMove = useCallback((e: PointerEvent) => {
    const d = drag.current;
    const el = ref.current;
    if (!d || !el) return;
    const p = clampPos({ x: e.clientX - d.dx, y: e.clientY - d.dy }, el.offsetHeight);
    d.x = p.x;
    d.y = p.y;
    el.style.left = `${p.x}px`;
    el.style.top = `${p.y}px`;
    el.style.right = "auto";
    el.style.bottom = "auto";
  }, []);

  const endDrag = useCallback(() => {
    window.removeEventListener("pointermove", onPointerMove);
    const d = drag.current;
    drag.current = null;
    if (d) {
      const p = { x: d.x, y: d.y };
      setPos(p);
      try { localStorage.setItem(POS_KEY, JSON.stringify(p)); } catch { /* ignore */ }
    }
  }, [onPointerMove]);

  useEffect(() => {
    const onResize = () => setPos((p) => (p ? clampPos(p, ref.current?.offsetHeight) : p));
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  useEffect(() => () => window.removeEventListener("pointermove", onPointerMove), [onPointerMove]);

  function startDrag(e: React.PointerEvent) {
    const el = ref.current;
    const rect = el?.getBoundingClientRect();
    if (!el || !rect) return;
    el.style.transition = "none"; // no easing lag while following the cursor
    drag.current = { dx: e.clientX - rect.left, dy: e.clientY - rect.top, x: rect.left, y: rect.top };
    el.setPointerCapture?.(e.pointerId);
    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", endDrag, { once: true });
    e.preventDefault();
  }

  const active = ["calling", "ringing", "incoming", "in-call", "held"].includes(phone.status);
  if (!active) return null;

  const inCall = phone.status === "in-call" || phone.status === "held";
  const style = pos ? { left: pos.x, top: pos.y, right: "auto" as const, bottom: "auto" as const } : undefined;

  return (
    <div
      ref={ref}
      style={style}
      className="animate-in fade-in slide-in-from-bottom-2 fixed right-5 bottom-5 z-[70] w-72 rounded-2xl border border-border bg-popover p-3 text-popover-foreground shadow-xl duration-200"
    >
      <div className="mb-3 flex items-center gap-3">
        <span
          onPointerDown={startDrag}
          title="Taşı"
          className="flex size-9 shrink-0 cursor-move touch-none items-center justify-center rounded-full bg-primary/15 text-primary"
        >
          <GripVertical className="size-4" />
        </span>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold">{phone.peer ?? "—"}</div>
          <div className="text-xs text-muted-foreground">{statusLabel[phone.status]}</div>
        </div>
      </div>

      {phone.error && <p className="mb-2 text-xs leading-snug text-destructive">{phone.error}</p>}

      {inCall && <VoiceMixer className="mb-3" />}

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
              {waNumber && (
                <button
                  onClick={openWhatsApp}
                  title="Müşteriye WhatsApp'tan yaz"
                  className="flex size-10 items-center justify-center rounded-xl border border-[#25D366]/40 bg-[#25D366]/10 text-[#1da851] hover:bg-[#25D366]/20 dark:text-[#4fe08a]"
                >
                  <WhatsAppIcon className="size-4" />
                </button>
              )}
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
