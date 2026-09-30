// Dates and times as the panel shows them: always in Istanbul time, the
// company's clock, whatever the viewer's computer is set to.

export const ZONE = "Europe/Istanbul";

// clockTime is "18:30" for a moment, or "" when there is none.
export function clockTime(iso?: string | null): string {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit", timeZone: ZONE });
}
