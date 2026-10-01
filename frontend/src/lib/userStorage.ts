// Browser storage that belongs to one signed-in person. Some panel state is
// kept across reloads (mention cards, unsent call endings, wrap-up cards);
// it carries names, message text and customer numbers, so it is stored
// under the user's id and wiped when they sign out, and the next person on
// the same computer never sees it.

let current = 0;

// SENSITIVE lists the storage keys that hold someone's data.
const SENSITIVE = ["teams.mentions", "santral.calllog.pending", "santral.wrapup.pending", "santral.wrapup.done"];

// setStorageUser tells the helpers who is signed in (0 when nobody is).
export function setStorageUser(id: number) {
  current = id;
}

// currentStorageUser is who the helpers think is signed in (0 for nobody).
export function currentStorageUser(): number {
  return current;
}

// userKey scopes a storage key to the signed-in user.
export function userKey(base: string): string {
  return keyFor(base, current);
}

// keyFor scopes a storage key to a given user, for data that belongs to
// whoever it was recorded for rather than to whoever is signed in now.
export function keyFor(base: string, id: number): string {
  return `${base}:u${id}`;
}

// clearUserStorage removes every person-bound key, for everyone, from both
// storages; older keys without a user id go too.
export function clearUserStorage() {
  for (const store of [safeStore("local"), safeStore("session")]) {
    if (!store) continue;
    const drop: string[] = [];
    for (let i = 0; i < store.length; i++) {
      const k = store.key(i);
      if (k && SENSITIVE.some((base) => k === base || k.startsWith(base + ":"))) drop.push(k);
    }
    for (const k of drop) {
      try {
        store.removeItem(k);
      } catch {
        // ignore
      }
    }
  }
}

function safeStore(kind: "local" | "session"): Storage | null {
  try {
    return kind === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    return null;
  }
}
