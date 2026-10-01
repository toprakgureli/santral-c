// The panel's tabs tell each other about the phone. Only one tab holds the
// line, but any tab can sign out or take the line over, and both would cut
// a call that is live in the tab holding it. So every tab announces whether
// its phone has a live call, and a tab about to cut it asks the holder to
// hang up properly (and send the call's end to the server) first.

import { useCallback, useEffect, useRef, useState } from "react";
import type { PhoneStatus } from "./useSoftphone";

// Phone states in which the line carries a call (ringing included).
export const LIVE_STATES: ReadonlySet<PhoneStatus> = new Set<PhoneStatus>(["calling", "ringing", "incoming", "in-call", "held"]);

export type TabMessage =
  | { kind: "state"; from: string; live: boolean }
  | { kind: "ask"; from: string }
  | { kind: "end"; from: string; id: string }
  | { kind: "ended"; from: string; id: string }
  | { kind: "gone"; from: string };

// A channel the tabs share; BroadcastChannel in the browser, a stand-in in
// tests.
export interface TabChannel {
  postMessage(m: TabMessage): void;
  onmessage: ((e: { data: TabMessage }) => void) | null;
  close(): void;
}

export const CHANNEL = "santral-phone";

// A tab that has not said anything for this long is gone (closed, frozen).
const STALE_MS = 10_000;
const HEARTBEAT_MS = 3_000;

function openChannel(): TabChannel | null {
  try {
    return typeof BroadcastChannel === "undefined" ? null : (new BroadcastChannel(CHANNEL) as unknown as TabChannel);
  } catch {
    return null;
  }
}

function newId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `t-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  }
}

// TabPhone is one tab's side of the conversation.
export class TabPhone {
  readonly id = newId();
  private live = false;
  private others = new Map<string, { live: boolean; at: number }>();
  private waiting = new Map<string, () => void>();
  private timer = 0;
  private listeners = new Set<() => void>();

  constructor(
    private channel: TabChannel | null,
    // endLocal hangs up this tab's call and waits until its end is sent.
    private endLocal: () => Promise<void>,
    private now: () => number = Date.now,
  ) {
    if (!channel) return;
    channel.onmessage = (e) => this.receive(e.data);
    channel.postMessage({ kind: "ask", from: this.id });
    this.timer = window.setInterval(() => {
      this.post({ kind: "state", from: this.id, live: this.live });
      this.notify();
    }, HEARTBEAT_MS);
  }

  // setLive announces whether this tab's phone carries a call.
  setLive(live: boolean) {
    if (live === this.live) return;
    this.live = live;
    this.post({ kind: "state", from: this.id, live });
  }

  // liveElsewhere reports whether another tab has a live call.
  liveElsewhere(): boolean {
    const now = this.now();
    for (const [id, t] of this.others) {
      if (now - t.at > STALE_MS) this.others.delete(id);
      else if (t.live) return true;
    }
    return false;
  }

  // endElsewhere asks the tabs with a live call to hang up and send the
  // call's end, and waits for them (or until the time is up).
  async endElsewhere(withinMs = 5000): Promise<void> {
    if (!this.channel || !this.liveElsewhere()) return;
    const id = newId();
    const done = new Promise<void>((resolve) => this.waiting.set(id, resolve));
    this.post({ kind: "end", from: this.id, id });
    await Promise.race([done, new Promise((resolve) => window.setTimeout(resolve, withinMs))]);
    this.waiting.delete(id);
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  close() {
    window.clearInterval(this.timer);
    this.post({ kind: "gone", from: this.id });
    if (this.channel) {
      this.channel.onmessage = null;
      this.channel.close();
    }
    this.listeners.clear();
  }

  private post(m: TabMessage) {
    try {
      this.channel?.postMessage(m);
    } catch {
      // a closed channel; nothing to tell
    }
  }

  private notify() {
    this.listeners.forEach((fn) => fn());
  }

  private receive(m: TabMessage) {
    if (!m || m.from === this.id) return;
    switch (m.kind) {
      case "state":
        this.others.set(m.from, { live: m.live, at: this.now() });
        break;
      case "ask":
        this.post({ kind: "state", from: this.id, live: this.live });
        return;
      case "gone":
        this.others.delete(m.from);
        break;
      case "end":
        if (this.live) {
          void this.endLocal()
            .catch(() => undefined)
            .finally(() => this.post({ kind: "ended", from: this.id, id: m.id }));
        }
        return;
      case "ended":
        this.others.set(m.from, { live: false, at: this.now() });
        this.waiting.get(m.id)?.();
        break;
    }
    this.notify();
  }
}

// useTabPhone joins the tabs' conversation for this tab's phone. It returns
// whether another tab has a live call, and a way to end it there.
export function useTabPhone(status: PhoneStatus, endLocal: () => Promise<void>, channel?: () => TabChannel | null) {
  const endRef = useRef(endLocal);
  endRef.current = endLocal;
  const [tab, setTab] = useState<TabPhone | null>(null);
  const [liveElsewhere, setLiveElsewhere] = useState(false);

  useEffect(() => {
    const t = new TabPhone((channel ?? openChannel)(), () => endRef.current());
    const update = () => setLiveElsewhere(t.liveElsewhere());
    const off = t.subscribe(update);
    setTab(t);
    const onHide = () => t.close();
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      off();
      t.close();
    };
  }, [channel]);

  const live = LIVE_STATES.has(status);
  useEffect(() => {
    tab?.setLive(live);
  }, [tab, live]);

  const endElsewhere = useCallback(async () => {
    await tab?.endElsewhere();
    setLiveElsewhere(tab?.liveElsewhere() ?? false);
  }, [tab]);

  return { liveElsewhere, endElsewhere };
}
