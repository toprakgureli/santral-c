// useWhatsAppWrite decides where "WhatsApp'tan yaz" goes. With the
// whatsapp.write_business permission (and a business number set up), the
// message goes out from the company number with an approved template and
// the reply lands in the inbox. Without it, the agent's own WhatsApp opens
// with their saved text, as before.

import { useAuth } from "@/auth/AuthContext";
import { can } from "@/lib/permissions";
import { whatsappLink, whatsappNumber, whatsappTextFor, type WhatsAppKind } from "@/lib/whatsapp";
import { displayNumber } from "@/softphone/dial";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";

export function useWhatsAppWrite() {
  const wa = useWhatsApp();
  const { user } = useAuth();
  const business = wa.enabled && can(user, "whatsapp.write_business") && can(user, "whatsapp.template_send");
  const write = (raw: string, kind: WhatsAppKind = "unreached") => {
    const num = whatsappNumber(raw);
    if (!num) return;
    if (business) {
      wa.startChat({ number: raw });
      return;
    }
    window.open(whatsappLink(num, whatsappTextFor(user, kind, displayNumber(raw))), "_blank", "noopener");
  };
  return { business, write };
}
