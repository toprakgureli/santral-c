// Call log delivery that survives a deploy and a closing tab. The "end"
// phase is the one that matters: if it never reaches the server the row is
// closed later by the stale sweeper with an unknown length. So an end that
// fails (the API was restarting) is kept in localStorage and retried with
// backoff, and a tab that closes mid-call sends its end as a beacon.
//
// Every record belongs to the person whose call it was. It is stored under
// that person's key and sent only while that person is signed in, so a
// sign-out (or someone else signing in on the same computer) never sends it
// with the wrong session or files it where nobody looks.

import { ApiError, api } from "@/api/client";
import { currentStorageUser, keyFor } from "@/lib/userStorage";

export interface CallLogBody {
  callId: string;
  phase: "start" | "answer" | "end";
  direction?: string;
  peer?: string;
  disposition?: string;
  durationSeconds?: number;
}

const KEY = "santral.calllog.pending";
const DELAYS = [3000, 10000, 30000, 60000, 120000];

function readPending(owner: number): CallLogBody[] {
  try {
    const raw = window.localStorage.getItem(keyFor(KEY, owner));
    return raw ? (JSON.parse(raw) as CallLogBody[]) : [];
  } catch {
    return [];
  }
}

function writePending(owner: number, items: CallLogBody[]) {
  try {
    if (items.length === 0) window.localStorage.removeItem(keyFor(KEY, owner));
    else window.localStorage.setItem(keyFor(KEY, owner), JSON.stringify(items.slice(-50)));
  } catch {
    // storage unavailable; the in-memory retry below still runs
  }
}

const same = (a: CallLogBody, b: CallLogBody) => a.callId === b.callId && a.phase === b.phase;

function remember(owner: number, body: CallLogBody) {
  if (!owner) return;
  const items = readPending(owner).filter((p) => !same(p, body));
  items.push(body);
  writePending(owner, items);
}

function forget(owner: number, body: CallLogBody) {
  if (!owner) return;
  writePending(owner, readPending(owner).filter((p) => !same(p, body)));
}

// inFlight holds the sends that have not been answered yet, so a sign-out
// can wait for a call's end to land before the session goes.
const inFlight = new Set<Promise<void>>();

// sendCallLog posts the phase for owner (the signed-in user when the call
// was placed; the current one by default). An end phase that fails is
// remembered and retried until it lands or the retries run out (it stays
// stored for the owner's next page load either way). The returned promise
// settles when this attempt has been answered.
export function sendCallLog(body: CallLogBody, owner = currentStorageUser(), attempt = 0): Promise<void> {
  if (body.phase === "end") remember(owner, body);
  // Someone else's session (or none) would refuse it: the stored copy
  // waits for the owner to sign in again.
  if (!owner || owner !== currentStorageUser()) return Promise.resolve();
  const sent: Promise<void> = api
    .logCall(body)
    .then(() => forget(owner, body))
    .catch((e) => {
      // The server refused it for good (not ours, malformed): stop trying.
      if (e instanceof ApiError && e.status >= 400 && e.status < 500 && ![401, 408, 429].includes(e.status)) {
        forget(owner, body);
        return;
      }
      if (body.phase !== "end") return;
      if (attempt < DELAYS.length) window.setTimeout(() => void sendCallLog(body, owner, attempt + 1), DELAYS[attempt]);
    })
    .finally(() => {
      inFlight.delete(sent);
    });
  inFlight.add(sent);
  return sent;
}

// settleCallLogs waits until every send in flight has been answered, or
// until the time is up.
export async function settleCallLogs(withinMs = 4000): Promise<void> {
  const pending = [...inFlight];
  if (pending.length === 0) return;
  await Promise.race([Promise.allSettled(pending), new Promise((resolve) => window.setTimeout(resolve, withinMs))]);
}

// flushPendingCallLogs resends whatever an earlier page left behind for
// the signed-in user.
export function flushPendingCallLogs(): void {
  const owner = currentStorageUser();
  for (const body of readPending(owner)) void sendCallLog(body, owner);
}

// beaconCallEnd reports a hangup from a tab that is going away; a beacon is
// the only request the browser still delivers at that point.
export function beaconCallEnd(body: CallLogBody, owner = currentStorageUser()): void {
  remember(owner, body);
  if (!owner || owner !== currentStorageUser()) return;
  try {
    const blob = new Blob([JSON.stringify(body)], { type: "application/json" });
    // Handed to the browser: it is delivered after the tab closes, so the
    // stored copy must not be sent again on the next load.
    if (navigator.sendBeacon("/api/v1/calls/log/", blob)) forget(owner, body);
  } catch {
    // fall through: the stored copy is retried on the next load
  }
}
