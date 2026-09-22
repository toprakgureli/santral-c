// The WhatsApp follow-up: an agent's own message template, rendered with
// their name, opened as a wa.me link for the last number they spoke with.

export const WHATSAPP_PLACEHOLDERS: { key: string; label: string }[] = [
  { key: "{isim}", label: "Adın soyadın" },
  { key: "{ad}", label: "Sadece adın" },
  { key: "{numara}", label: "Müşterinin numarası" },
];

export const DEFAULT_WHATSAPP_TEMPLATE =
  "Merhaba, ben VatanSoft teknik destek uzmanınız {isim}. Sizi aradık ancak ulaşamadık. " +
  "Uygun olduğunuzda buradan bilgi verirseniz sizinle tekrar iletişime geçeceğiz.";

// The text used while the customer is on the phone right now.
export const DEFAULT_WHATSAPP_LIVE_TEMPLATE =
  "Merhaba, ben VatanSoft teknik destek uzmanınız {isim}. Şu an telefonda görüştüğümüz konu için buradan da yazışabiliriz. " +
  "Ekran görüntüsü veya dosya paylaşmanız gerekirse bu numaraya gönderebilirsiniz.";

export type WhatsAppKind = "unreached" | "live";

export const WHATSAPP_KINDS: { kind: WhatsAppKind; label: string; hint: string }[] = [
  { kind: "unreached", label: "Ulaşamadığımda", hint: "Cevapsız veya meşgul çağrının ardından" },
  { kind: "live", label: "Görüşme sırasında", hint: "Müşteri hatta iken" },
];

// whatsappTextFor renders the agent's text of the given kind for a customer.
export function whatsappTextFor(
  user: { name?: string; whatsappTemplate?: string; whatsappTemplateLive?: string } | null | undefined,
  kind: WhatsAppKind,
  number: string,
): string {
  const template = kind === "live" ? user?.whatsappTemplateLive ?? "" : user?.whatsappTemplate ?? "";
  const fallback = kind === "live" ? DEFAULT_WHATSAPP_LIVE_TEMPLATE : DEFAULT_WHATSAPP_TEMPLATE;
  return renderWhatsAppTemplate(template.trim() ? template : fallback, { name: user?.name ?? "", number });
}

export const WHATSAPP_TEMPLATE_MAX = 1000;

// renderWhatsAppTemplate fills the placeholders. An empty template means the default.
export function renderWhatsAppTemplate(template: string, vars: { name: string; number: string }): string {
  const t = template.trim() || DEFAULT_WHATSAPP_TEMPLATE;
  const first = vars.name.trim().split(/\s+/)[0] ?? "";
  return t.replace(/\{isim\}/g, vars.name.trim()).replace(/\{ad\}/g, first).replace(/\{numara\}/g, vars.number);
}

// whatsappNumber turns any Turkish number form into the E.164 digits wa.me
// wants (905304230113); returns "" for extensions and unknown shapes.
export function whatsappNumber(raw: string): string {
  const d = (raw || "").replace(/[^\d]/g, "");
  if (d.length < 10) return "";
  if (d.length === 10) return "90" + d;
  if (d.length === 11 && d.startsWith("0")) return "90" + d.slice(1);
  if (d.length === 12 && d.startsWith("90")) return d;
  if (d.length === 13 && d.startsWith("090")) return "90" + d.slice(3);
  if (d.length === 14 && d.startsWith("0090")) return "90" + d.slice(4);
  return d.length >= 11 ? d : "";
}

// whatsappLink builds the wa.me URL with the message prefilled.
export function whatsappLink(number: string, text: string): string {
  return `https://wa.me/${number}?text=${encodeURIComponent(text)}`;
}
