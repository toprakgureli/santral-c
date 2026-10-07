// The working-day settings, read once per page load and shared: when the
// day ends and each role's daily target of real calls. The settings card
// pushes a new value to every screen at once.

import { useEffect, useState } from "react";
import { api } from "@/api/client";
import type { Workday } from "@/api/types";

let current: Workday | null = null;
let loading: Promise<Workday | null> | null = null;
const listeners = new Set<(w: Workday) => void>();

function load(): Promise<Workday | null> {
  if (current) return Promise.resolve(current);
  if (!loading) {
    loading = api
      .workday()
      .then((w) => {
        rememberWorkday(w);
        return w;
      })
      .catch(() => {
        // the next page load asks again
        loading = null;
        return null;
      });
  }
  return loading;
}

// rememberWorkday stores a value read or just saved and tells every screen.
export function rememberWorkday(w: Workday) {
  current = w;
  listeners.forEach((l) => l(w));
}

export function useWorkday(): Workday | null {
  const [w, setW] = useState<Workday | null>(current);
  useEffect(() => {
    listeners.add(setW);
    void load().then((v) => v && setW(v));
    return () => {
      listeners.delete(setW);
    };
  }, []);
  return w;
}

// targetFor is the daily target of someone holding the given roles: the
// highest of their roles' targets, 0 when none has one.
export function targetFor(w: Workday | null, roleIds: number[] | undefined): number {
  if (!w || !roleIds) return 0;
  return w.roles.filter((r) => roleIds.includes(r.id)).reduce((best, r) => Math.max(best, r.target), 0);
}
