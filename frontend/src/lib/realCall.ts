// How long an answered call must last to count as a real conversation.
// The server counts with this setting; the screens only read it to label
// their figures ("60 saniye ve üstü"). It is read once per page load and
// shared; the settings card pushes a new value to every screen at once.

import { useEffect, useState } from "react";
import { api } from "@/api/client";

export const DEFAULT_REAL_SECONDS = 30;

let current: number | null = null;
let loading: Promise<number> | null = null;
const listeners = new Set<(seconds: number) => void>();

export function loadRealCallSeconds(): Promise<number> {
  if (current !== null) return Promise.resolve(current);
  if (!loading) {
    loading = api
      .realCall()
      .then((r) => {
        rememberRealCallSeconds(r.seconds);
        return r.seconds;
      })
      .catch(() => {
        // An unreadable setting leaves the labels on the default; the next
        // page load asks again.
        loading = null;
        return DEFAULT_REAL_SECONDS;
      });
  }
  return loading;
}

// rememberRealCallSeconds stores a value read or just saved and tells every
// screen showing it.
export function rememberRealCallSeconds(seconds: number) {
  current = seconds;
  listeners.forEach((l) => l(seconds));
}

export function useRealCallSeconds(): number {
  const [seconds, setSeconds] = useState(current ?? DEFAULT_REAL_SECONDS);
  useEffect(() => {
    listeners.add(setSeconds);
    void loadRealCallSeconds().then(setSeconds);
    return () => {
      listeners.delete(setSeconds);
    };
  }, []);
  return seconds;
}
