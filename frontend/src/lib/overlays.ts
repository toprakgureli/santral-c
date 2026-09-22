// A tiny shared flag so full-screen cards do not stack: while the WhatsApp
// prompt is up, the wrap-up card waits behind it and appears once it closes.

import { useSyncExternalStore } from "react";

let waPromptOpen = false;
const listeners = new Set<() => void>();

export function setWhatsAppPromptOpen(open: boolean) {
  if (waPromptOpen === open) return;
  waPromptOpen = open;
  listeners.forEach((l) => l());
}

export function useWhatsAppPromptOpen(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => waPromptOpen,
    () => false,
  );
}
