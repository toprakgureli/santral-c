import { beforeEach, describe, expect, it } from "vitest";
import { findCachedTranscript, liveTranscript } from "./liveTranscript";

describe("liveTranscript engine", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    window.localStorage.clear();
    liveTranscript.syncCall({
      inCall: false,
      callId: null,
      peer: null,
      answeredAt: null,
      muted: false,
      wave: () => null,
    });
    liveTranscript.clearLines();
  });

  it("records agent and customer utterances and caches them by callId and peer", () => {
    const started = Date.now() - 10_000;
    liveTranscript.syncCall({
      inCall: true,
      callId: "call-unit-1",
      peer: "05551234567",
      direction: "inbound",
      answeredAt: started,
      muted: false,
      wave: () => null,
    });

    liveTranscript.pushLine("customer", "Merhaba internetim kesildi");
    liveTranscript.pushLine("agent", "Hemen kontrol ediyorum");

    const snap = liveTranscript.getSnapshot();
    expect(snap.lines).toHaveLength(2);
    expect(snap.lines[0].speaker).toBe("customer");
    expect(snap.lines[0].text).toBe("Merhaba internetim kesildi");
    expect(snap.lines[1].speaker).toBe("agent");
    expect(snap.lines[1].text).toBe("Hemen kontrol ediyorum");

    const byId = findCachedTranscript({ callId: "call-unit-1" });
    expect(byId?.lines).toHaveLength(2);

    const byPeer = findCachedTranscript({ peer: "+90 555 123 45 67" });
    expect(byPeer?.callId).toBe("call-unit-1");
  });

  it("merges consecutive short fragments from the same speaker within 4 seconds", () => {
    liveTranscript.syncCall({
      inCall: true,
      callId: "call-unit-2",
      peer: "05559998877",
      direction: "inbound",
      answeredAt: Date.now() - 5000,
      muted: false,
      wave: () => null,
    });

    liveTranscript.pushLine("customer", "Abone numaram");
    liveTranscript.pushLine("customer", "12345678");

    const snap = liveTranscript.getSnapshot();
    expect(snap.lines).toHaveLength(1);
    expect(snap.lines[0].text).toBe("Abone numaram 12345678");
  });
});
