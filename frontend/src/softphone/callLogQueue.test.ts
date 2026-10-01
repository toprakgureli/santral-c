// A call's end belongs to the person whose call it was: it is filed under
// their key, sent only with their session, and a sign-out can wait for it.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { setStorageUser } from "../lib/userStorage";

const apiMock = vi.hoisted(() => ({ logCall: vi.fn() }));
vi.mock("@/api/client", () => {
  class ApiError extends Error {
    status: number;
    constructor(status: number, message = "") {
      super(message);
      this.status = status;
    }
  }
  return { api: apiMock, ApiError };
});

import { flushPendingCallLogs, sendCallLog, settleCallLogs } from "./callLogQueue";

const end = { callId: "c-1", phase: "end" as const, disposition: "answered", durationSeconds: 42 };
const stored = (id: number) => JSON.parse(window.localStorage.getItem(`santral.calllog.pending:u${id}`) ?? "[]");

beforeEach(() => {
  window.localStorage.clear();
  apiMock.logCall.mockReset();
});
afterEach(() => setStorageUser(0));

describe("call log delivery", () => {
  it("sends an end and forgets it once the server has it", async () => {
    setStorageUser(7);
    apiMock.logCall.mockResolvedValue(undefined);
    await sendCallLog(end, 7);
    expect(apiMock.logCall).toHaveBeenCalledWith(end);
    expect(stored(7)).toEqual([]);
  });

  it("keeps an end for its owner when the panel signed out, and sends it when they are back", async () => {
    setStorageUser(0);
    apiMock.logCall.mockResolvedValue(undefined);
    await sendCallLog(end, 7);
    // Nobody's session would carry it: not sent, kept under the owner.
    expect(apiMock.logCall).not.toHaveBeenCalled();
    expect(stored(7)).toEqual([end]);
    expect(stored(0)).toEqual([]);

    // Someone else signs in on the same computer: still not theirs.
    setStorageUser(9);
    flushPendingCallLogs();
    await settleCallLogs();
    expect(apiMock.logCall).not.toHaveBeenCalled();

    setStorageUser(7);
    flushPendingCallLogs();
    await settleCallLogs();
    expect(apiMock.logCall).toHaveBeenCalledWith(end);
    expect(stored(7)).toEqual([]);
  });

  it("lets a sign-out wait until the end has landed", async () => {
    setStorageUser(7);
    let land: () => void = () => undefined;
    apiMock.logCall.mockReturnValue(new Promise<void>((resolve) => (land = resolve)));
    void sendCallLog(end, 7);
    let settled = false;
    const waiting = settleCallLogs(5000).then(() => (settled = true));
    await Promise.resolve();
    expect(settled).toBe(false);
    land();
    await waiting;
    expect(settled).toBe(true);
    expect(stored(7)).toEqual([]);
  });

  it("keeps a failed end for the next try", async () => {
    vi.useFakeTimers();
    try {
      setStorageUser(7);
      apiMock.logCall.mockRejectedValueOnce(new TypeError("Failed to fetch")).mockResolvedValue(undefined);
      await sendCallLog(end, 7);
      expect(stored(7)).toEqual([end]);
      await vi.advanceTimersByTimeAsync(3100);
      expect(apiMock.logCall).toHaveBeenCalledTimes(2);
      expect(stored(7)).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });
});
