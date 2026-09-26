// Ticks show where an outgoing message is: a clock while it waits to go,
// one tick when WhatsApp took it, two when it reached the phone, two blue
// when read, and a red mark when it could not be sent.

import { AlertCircle, Check, CheckCheck, Clock3 } from "lucide-react";
import { cn } from "@/lib/utils";

export default function Ticks({ status, className }: { status: string; className?: string }) {
  const size = cn("size-3.5 shrink-0", className);
  switch (status) {
    case "queued":
      return <Clock3 className={cn(size, "text-wa-meta/80")} aria-label="Gönderiliyor" />;
    case "sent":
      return <Check className={cn(size, "text-wa-meta")} aria-label="Gönderildi" />;
    case "delivered":
      return <CheckCheck className={cn(size, "text-wa-meta")} aria-label="İletildi" />;
    case "read":
      return <CheckCheck className={cn(size, "text-[#53bdeb]")} aria-label="Okundu" />;
    case "failed":
      return <AlertCircle className={cn(size, "text-destructive")} aria-label="Gönderilemedi" />;
  }
  return null;
}
