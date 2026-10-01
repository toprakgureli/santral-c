// Call log delivery that survives a deploy and a closing tab. The "end"
// phase is the one that matters: if it never reaches the server the row is
// closed hours later by the stale sweeper at the 2 hour cap. So an end that
// fails (the API was restarting) is kept in localStorage and retried with
// backoff, and a tab that closes mid-call sends its end as a beacon.

import { ApiError, api } from "@/api/client";
import { userKey } from "@/lib/userStorage";

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

function readPending(): CallLogBody[] {
  try {
    const raw = window.localStorage.getItem(userKey(KEY));
    return raw ? (JSON.parse(raw) as CallLogBody[]) : [];
  } catch {
    return [];
  }
}

function writePending(items: CallLogBody[]) {
  try {
    if (items.length === 0) window.localStorage.removeItem(userKey(KEY));
    else window.localStorage.setItem(userKey(KEY), JSON.stringify(items.slice(-50)));
  } catch {
    // storage unavailable; the in-memory retry below still runs
  }
}

function remember(body: CallLogBody) {
  const items = readPending().filter((p) => !(p.callId === body.callId && p.phase === body.phase));
  items.push(body);
  writePending(items);
}

function forget(body: CallLogBody) {
  writePending(readPending().filter((p) => !(p.callId === body.callId && p.phase === body.phase)));
}

// sendCallLog posts the phase; an end phase that fails is remembered and
// retried until it lands or the retries run out (it stays stored for the
// next page load either way).
export function sendCallLog(body: CallLogBody, attempt = 0): void {
  if (body.phase === "end") remember(body);
  api
    .logCall(body)
    .then(() => forget(body))
    .catch((e) => {
      // The server refused it for good (not ours, malformed): stop trying.
      if (e instanceof ApiError && e.status >= 400 && e.status < 500 && ![401, 408, 429].includes(e.status)) {
        forget(body);
        return;
      }
      if (body.phase !== "end") return;
      if (attempt < DELAYS.length) window.setTimeout(() => sendCallLog(body, attempt + 1), DELAYS[attempt]);
    });
}

// flushPendingCallLogs resends whatever an earlier page left behind.
export function flushPendingCallLogs(): void {
  for (const body of readPending()) sendCallLog(body);
}

// beaconCallEnd reports a hangup from a tab that is going away; a beacon is
// the only request the browser still delivers at that point.
export function beaconCallEnd(body: CallLogBody): void {
  remember(body);
  try {
    const blob = new Blob([JSON.stringify(body)], { type: "application/json" });
    // Handed to the browser: it is delivered after the tab closes, so the
    // stored copy must not be sent again on the next load.
    if (navigator.sendBeacon("/api/v1/calls/log/", blob)) forget(body);
  } catch {
    // fall through: the stored copy is retried on the next load
  }
}
