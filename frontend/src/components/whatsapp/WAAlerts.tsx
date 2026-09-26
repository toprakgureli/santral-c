// WAAlerts shows the module's short notices in the corner of every page:
// a chat assigned to you, a customer waiting too long, a callback request,
// a template approved, a device problem.

import { useNavigate } from "react-router-dom";
import { AlertTriangle, MessageCircle, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";

export default function WAAlerts() {
  const wa = useWhatsApp();
  const navigate = useNavigate();
  if (!wa.enabled || wa.alerts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-5 left-1/2 z-[75] flex w-[min(26rem,calc(100vw-2rem))] -translate-x-1/2 flex-col gap-2">
      {wa.alerts.map((a) => (
        <div key={a.id} className={cn("animate-in fade-in slide-in-from-bottom-2 pointer-events-auto flex items-start gap-3 rounded-2xl border bg-popover px-3.5 py-3 text-sm shadow-xl duration-200", a.level === "warning" ? "border-warning/40" : "border-border")}>
          <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-xl", a.level === "warning" ? "bg-warning/12 text-warning" : "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400")}>
            {a.level === "warning" ? <AlertTriangle className="size-4" /> : <MessageCircle className="size-4" />}
          </span>
          <button type="button" className="min-w-0 flex-1 pt-1 text-left leading-snug" onClick={() => { if (a.conversationId) navigate(`/whatsapp/${a.conversationId}`); wa.dismissAlert(a.id); }}>
            {a.text}
            {a.conversationId && <span className="mt-0.5 block text-xs font-medium text-primary">Sohbeti aç</span>}
          </button>
          <button type="button" onClick={() => wa.dismissAlert(a.id)} aria-label="Kapat" className="rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-4" /></button>
        </div>
      ))}
    </div>
  );
}
