// normalizeDial makes a dialed string acceptable to the PBX. Internal
// extensions (short) pass through; Turkish mobile/landline numbers are coerced
// to the 0XXXXXXXXXX form the switch expects, so "5304230113", "+905304230113"
// and "905304230113" all become "05304230113".
export function normalizeDial(raw: string): string {
  const trimmed = raw.trim();
  // Feature/service codes (e.g. *60 echo test, *43, #-codes) pass through with
  // their * and # intact.
  if (/[*#]/.test(trimmed)) return trimmed.replace(/[^\d*#]/g, "");
  const plus = trimmed.startsWith("+");
  let d = raw.replace(/[^\d]/g, "");
  if (!d) return "";
  if (d.length <= 5) return d; // internal extension or short code
  if (plus && d.startsWith("90")) d = "0" + d.slice(2);
  else if (d.startsWith("0090")) d = "0" + d.slice(4);
  else if (d.startsWith("90") && d.length === 12) d = "0" + d.slice(2);
  else if (d.length === 10) d = "0" + d;
  return d;
}

// displayNumber strips a number down to its bare significant digits for copying
// and display: "05304230113", "905304230113" and "+905304230113" all become
// "5304230113". Short internal numbers are returned unchanged.
export function displayNumber(raw: string): string {
  const d = (raw || "").replace(/[^\d]/g, "");
  if (!d) return raw || "";
  if (d.length >= 10) return d.slice(-10);
  return d;
}
