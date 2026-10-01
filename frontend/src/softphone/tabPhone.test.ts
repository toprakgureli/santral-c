// Two and three tabs of the panel talking about the phone over a stand-in
// channel: who has a live call, ending it there before this tab cuts it,
// and forgetting a tab that went away.
import { afterEach, describe, expect, it, vi } from "vitest";
import { TabPhone, type TabChannel, type TabMessage } from "./tabPhone";

// hub is a stand-in for BroadcastChannel: a message reaches every other
// open channel, asynchronously like the real one.
function hub() {
  const open = new Set<TabChannel>();
  return () => {
    const ch: TabChannel = {
      onmessage: null,
      postMessage(m: TabMessage) {
        for (const other of open) {
          if (other !== ch) queueMicrotask(() => other.onmessage?.({ data: structuredClone(m) }));
        }
      },
      close() {
        open.delete(ch);
      },
    };
    open.add(ch);
    return ch;
  };
}

const flush = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve();
};

const tabs: TabPhone[] = [];
afterEach(() => {
  tabs.splice(0).forEach((t) => t.close());
  vi.useRealTimers();
});

function tab(channel: () => TabChannel, endLocal = vi.fn(async () => undefined), now?: () => number) {
  const t = new TabPhone(channel(), endLocal, now);
  tabs.push(t);
  return { t, endLocal };
}

describe("tabs and the phone", () => {
  it("tells other tabs about a live call and when it ends", async () => {
    const channel = hub();
    const holder = tab(channel);
    const other = tab(channel);
    await flush();
    expect(other.t.liveElsewhere()).toBe(false);
    holder.t.setLive(true);
    await flush();
    expect(other.t.liveElsewhere()).toBe(true);
    expect(holder.t.liveElsewhere()).toBe(false);
    holder.t.setLive(false);
    await flush();
    expect(other.t.liveElsewhere()).toBe(false);
  });

  it("a tab opened during a call learns about it at once", async () => {
    const channel = hub();
    const holder = tab(channel);
    holder.t.setLive(true);
    const late = tab(channel);
    await flush();
    expect(late.t.liveElsewhere()).toBe(true);
  });

  it("asks the holding tab to hang up, and waits until it has", async () => {
    const channel = hub();
    let finish: () => void = () => undefined;
    const endLocal = vi.fn(() => new Promise<void>((resolve) => (finish = resolve)));
    const holder = tab(channel, endLocal);
    const other = tab(channel);
    holder.t.setLive(true);
    await flush();

    let done = false;
    const ending = other.t.endElsewhere().then(() => (done = true));
    await flush();
    expect(endLocal).toHaveBeenCalledTimes(1);
    // The holder is still sending the call's end: the other tab waits.
    expect(done).toBe(false);
    holder.t.setLive(false);
    finish();
    await ending;
    expect(done).toBe(true);
    expect(other.t.liveElsewhere()).toBe(false);
  });

  it("does not wait for ever when the holding tab does not answer", async () => {
    vi.useFakeTimers();
    const channel = hub();
    const holder = tab(channel, vi.fn(() => new Promise<void>(() => undefined)));
    const other = tab(channel);
    holder.t.setLive(true);
    await flush();
    let done = false;
    const ending = other.t.endElsewhere(5000).then(() => (done = true));
    await vi.advanceTimersByTimeAsync(4900);
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(200);
    await ending;
    expect(done).toBe(true);
  });

  it("only the tab with the call hangs up", async () => {
    const channel = hub();
    const holder = tab(channel);
    const idle = tab(channel);
    const asking = tab(channel);
    holder.t.setLive(true);
    await flush();
    await asking.t.endElsewhere();
    expect(holder.endLocal).toHaveBeenCalledTimes(1);
    expect(idle.endLocal).not.toHaveBeenCalled();
    expect(asking.endLocal).not.toHaveBeenCalled();
  });

  it("forgets a tab that closed or went silent", async () => {
    const channel = hub();
    let clock = 1_000;
    const holder = tab(channel);
    const other = tab(channel, undefined, () => clock);
    holder.t.setLive(true);
    await flush();
    expect(other.t.liveElsewhere()).toBe(true);
    // Frozen: no heartbeat for longer than the limit.
    clock += 11_000;
    expect(other.t.liveElsewhere()).toBe(false);
    holder.t.close();
    tabs.splice(tabs.indexOf(holder.t), 1);

    const second = tab(channel);
    second.t.setLive(true);
    await flush();
    expect(other.t.liveElsewhere()).toBe(true);
    second.t.close();
    tabs.splice(tabs.indexOf(second.t), 1);
    await flush();
    expect(other.t.liveElsewhere()).toBe(false);
  });

  it("works alone when the browser has no channel", async () => {
    const lone = new TabPhone(null, vi.fn(async () => undefined));
    lone.setLive(true);
    expect(lone.liveElsewhere()).toBe(false);
    await lone.endElsewhere();
    lone.close();
  });
});
