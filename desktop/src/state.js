// State sanitizer and status definitions for Santral-C Desktop Widget.
export const STATUSES = new Set([
  "idle",
  "connecting",
  "registered",
  "calling",
  "ringing",
  "incoming",
  "in-call",
  "held",
  "error",
  "disabled",
  "unconfigured",
  "disconnected",
]);

export const COMMANDS = new Set([
  "call",
  "answer",
  "hangup",
  "mute",
  "hold",
  "transfer",
  "dtmf",
]);

// cleanState keeps only valid fields and shapes to prevent injection.
export function cleanState(s) {
  if (!s || typeof s !== "object" || !STATUSES.has(s.status)) {
    return { status: "idle" };
  }
  const str = (v, n) => (typeof v === "string" ? v.slice(0, n) : "");
  const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);
  return {
    status: s.status,
    peer: str(s.peer, 40),
    muted: !!s.muted,
    held: !!s.held,
    extension: str(s.extension, 16),
    endReason: str(s.endReason, 60),
    callStartedAt: num(s.callStartedAt),
    answeredAt: num(s.answeredAt),
    canCall: s.canCall !== false,
  };
}

// normalizeDial normalizes phone input: extensions & *# pass through,
// Turkish phone numbers are formatted to 0XXXXXXXXXX.
export function normalizeDial(raw) {
  const t = String(raw ?? "").trim();
  if (/[*#]/.test(t)) return t.replace(/[^\d*#]/g, "");
  const d = t.replace(/\D/g, "");
  if (!d) return "";
  if (d.length <= 5) return d; // internal extension
  if (d.startsWith("90") && d.length === 12) return "0" + d.slice(2);
  if (d.startsWith("0") && d.length === 11) return d;
  if (d.length === 10) return "0" + d;
  return d;
}
