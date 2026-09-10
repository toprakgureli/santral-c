// normalizeDial makes a dialed string acceptable to the PBX. Internal
// extensions (short) pass through; Turkish mobile/landline numbers are coerced
// to the 0XXXXXXXXXX form the switch expects, so "5304230113", "+905304230113"
// and "905304230113" all become "05304230113".
export function normalizeDial(raw: string): string {
  const plus = raw.trim().startsWith("+");
  let d = raw.replace(/[^\d]/g, "");
  if (!d) return "";
  if (d.length <= 5) return d; // internal extension or short code
  if (plus && d.startsWith("90")) d = "0" + d.slice(2);
  else if (d.startsWith("0090")) d = "0" + d.slice(4);
  else if (d.startsWith("90") && d.length === 12) d = "0" + d.slice(2);
  else if (d.length === 10) d = "0" + d;
  return d;
}
