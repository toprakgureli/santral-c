// A tiny in-page bus so every escalation form on screen learns about a save
// made elsewhere (the wrap-up card saving for the number the dashboard card
// still shows). Records are matched by their bare 10-digit number.

import type { EscalationRecord } from "@/api/types";

const EVENT = "santral:escalation-saved";

export function emitEscalationSaved(record: EscalationRecord) {
  window.dispatchEvent(new CustomEvent<EscalationRecord>(EVENT, { detail: record }));
}

export function onEscalationSaved(handler: (record: EscalationRecord) => void): () => void {
  const listener = (e: Event) => handler((e as CustomEvent<EscalationRecord>).detail);
  window.addEventListener(EVENT, listener);
  return () => window.removeEventListener(EVENT, listener);
}

// sameNumber compares two numbers by their last ten digits.
export function sameNumber(a: string, b: string): boolean {
  const key = (s: string) => s.replace(/[^\d]/g, "").slice(-10);
  const ka = key(a);
  return ka !== "" && ka === key(b);
}
