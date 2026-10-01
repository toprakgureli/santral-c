import { afterEach, describe, expect, it, vi } from "vitest";
import { beaconCallEnd } from "./callLogQueue";
import { setStorageUser } from "../lib/userStorage";

describe("call end from a closing tab", () => {
  afterEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  // The server takes a change only with a JSON body (a plain form or text
  // from another site is refused), so the beacon must say it is JSON.
  it("is sent as a JSON body", async () => {
    const sent: { url: string; data: BodyInit | null | undefined }[] = [];
    Object.defineProperty(navigator, "sendBeacon", {
      configurable: true,
      value: (url: string, data?: BodyInit | null) => {
        sent.push({ url, data });
        return true;
      },
    });
    // The end belongs to the signed-in person and goes with their session.
    setStorageUser(7);
    const body = { callId: "c-1", phase: "end" as const, direction: "outbound", peer: "05550001122", disposition: "answered", durationSeconds: 42 };
    beaconCallEnd(body);
    expect(sent).toHaveLength(1);
    expect(sent[0].url).toBe("/api/v1/calls/log/");
    const blob = sent[0].data as Blob;
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe("application/json");
    const text = await new Promise<string>((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.readAsText(blob);
    });
    expect(JSON.parse(text)).toEqual(body);
  });
});
