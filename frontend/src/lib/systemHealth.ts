// The system warnings people holding system.health see: which ones are
// still open for them and which they closed. A closed warning stays closed
// until the server words it differently (its fingerprint changes: the disk
// fills further, a new notice fails), then it shows again.

import type { SystemWarning } from "@/api/client";
import type { User } from "@/api/types";
import { canAny } from "@/lib/permissions";
import { userKey } from "@/lib/userStorage";

export const SYSTEM_HEALTH_PERMISSION = "system.health";

// How often the panel asks the server; the server checks once a minute.
export const SYSTEM_HEALTH_POLL_MS = 60_000;

const DISMISSED_KEY = "santral.systemHealth.dismissed";

// LINK_PERMISSIONS says who can use the page a warning links to; the link
// shows only to them, the warning itself to everyone holding system.health.
const LINK_PERMISSIONS: Record<string, string[]> = {
  "/settings": ["system.backup"],
  "/whatsapp/settings?tab=events": ["whatsapp.channel_manage"],
  "/whatsapp/settings?tab=devices": ["whatsapp.channel_manage", "whatsapp.team_manage"],
};

// linkFor is the warning's page when the user may open it.
export function linkFor(user: User | null, w: SystemWarning): string | undefined {
  if (!w.link) return undefined;
  const need = LINK_PERMISSIONS[w.link];
  return need && !canAny(user, need) ? undefined : w.link;
}

// openWarnings is the warnings not closed, the most urgent first.
export function openWarnings(warnings: SystemWarning[], dismissed: Set<string>): SystemWarning[] {
  return warnings
    .filter((w) => !dismissed.has(w.fingerprint))
    .sort((a, b) => rank(a) - rank(b));
}

function rank(w: SystemWarning): number {
  return w.level === "critical" ? 0 : 1;
}

// keepCurrent drops closed fingerprints the server no longer reports, so
// the list does not grow and a problem that went away and came back with
// the same wording shows again.
export function keepCurrent(dismissed: Set<string>, warnings: SystemWarning[]): Set<string> {
  const live = new Set(warnings.map((w) => w.fingerprint));
  return new Set([...dismissed].filter((f) => live.has(f)));
}

export function readDismissed(): Set<string> {
  try {
    const raw = window.localStorage.getItem(userKey(DISMISSED_KEY));
    const list: unknown = raw ? JSON.parse(raw) : [];
    return new Set(Array.isArray(list) ? list.filter((v): v is string => typeof v === "string") : []);
  } catch {
    return new Set();
  }
}

export function writeDismissed(dismissed: Set<string>) {
  try {
    window.localStorage.setItem(userKey(DISMISSED_KEY), JSON.stringify([...dismissed].slice(-50)));
  } catch {
    // no storage: closed warnings come back after a reload
  }
}
