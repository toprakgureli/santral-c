// Every softphone problem an agent sees is said in plain Turkish. The SIP
// library, the browser and the page loader all fail with English text of
// their own; those never reach the screen.

// PhoneError carries a message written for the agent, shown as it is.
export class PhoneError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PhoneError";
  }
}

const RELOAD = "Panelin yeni sürümü yüklendi. Sayfayı yenileyip tekrar dene.";
const LINE = "Santrale bağlanılamadı. İnternet bağlantını kontrol et; bağlantı gelince telefon kendiliğinden yeniden bağlanır.";

// phoneMessage turns any failure into a message the agent can act on.
// Microphone problems get their own advice; a message the panel itself
// wrote (PhoneError, or an answer from the server) is kept; anything else
// falls back to the given plain sentence.
export function phoneMessage(e: unknown, fallback: string): string {
  if (e instanceof PhoneError) return e.message;
  const name = e instanceof Error ? e.name : "";
  switch (name) {
    case "NotAllowedError":
    case "SecurityError":
      return "Mikrofon izni verilmedi. Adres çubuğundaki kilit simgesinden mikrofona izin verip sayfayı yenile.";
    case "NotFoundError":
    case "OverconstrainedError":
      return "Mikrofon bulunamadı. Bir mikrofon bağlayıp sayfayı yenile.";
    case "NotReadableError":
    case "AbortError":
      return "Mikrofon başka bir uygulama tarafından kullanılıyor.";
    case "ChunkLoadError":
      return RELOAD;
  }
  // An answer from the panel's own server is already in Turkish.
  if (e instanceof Error && (e as { status?: unknown }).status !== undefined && typeof (e as { code?: unknown }).code === "string") {
    return e.message || fallback;
  }
  const text = e instanceof Error ? e.message : typeof e === "string" ? e : "";
  if (/dynamically imported module|importing a module script|failed to fetch dynamically|loading chunk/i.test(text)) return RELOAD;
  if (/transport|websocket|not connected|connection (closed|lost|refused)|network/i.test(text)) return LINE;
  return fallback;
}
