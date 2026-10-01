// The softphone against a stand-in SIP library: registering, calling out,
// taking a call, mute, hold, transfer, hang-up, a dropped connection, and a
// long run of calls back to back. The stand-in records what the phone asks
// of it and lets each test decide how the other side answers.
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useSoftphone, type Phone } from "./useSoftphone";

// ---------------------------------------------------------------- stand-ins

const sipMock = vi.hoisted(() => {
  const SessionState = { Initial: "Initial", Establishing: "Establishing", Established: "Established", Terminating: "Terminating", Terminated: "Terminated" } as const;
  const RegistererState = { Initial: "Initial", Registered: "Registered", Unregistered: "Unregistered", Terminated: "Terminated" } as const;

  class Emitter<T> {
    listeners: ((v: T) => void)[] = [];
    addListener(fn: (v: T) => void) {
      this.listeners.push(fn);
    }
    emit(v: T) {
      this.listeners.forEach((fn) => fn(v));
    }
  }

  // A microphone track and the connection that carries it.
  function connection() {
    const track = { kind: "audio", enabled: true };
    return { track, pc: { getSenders: () => [{ track }], getReceivers: () => [] } };
  }

  class Session {
    state: string = SessionState.Initial;
    stateChange = new Emitter<string>();
    media = connection();
    sessionDescriptionHandler = { peerConnection: this.media.pc, sendDtmf: vi.fn(() => true) };
    reinvites: unknown[] = [];
    referred: { user?: string }[] = [];
    referReject = false;
    ended = vi.fn();
    to(state: string) {
      this.state = state;
      this.stateChange.emit(state);
    }
    async invite(opts?: unknown) {
      this.reinvites.push(opts);
    }
    async bye() {
      this.ended("bye");
      this.to(SessionState.Terminated);
    }
    async refer(uri: { user?: string }, opts?: { requestDelegate?: { onReject?: () => void } }) {
      this.referred.push(uri);
      if (this.referReject) opts?.requestDelegate?.onReject?.();
    }
  }

  class Inviter extends Session {
    target: { user?: string };
    delegate?: { onProgress?: (r: unknown) => void; onReject?: (r: unknown) => void };
    constructor(_ua: unknown, uri: { user?: string }) {
      super();
      this.target = uri;
      state.inviters.push(this);
    }
    async invite(opts?: { requestDelegate?: Inviter["delegate"] }) {
      if (this.state === SessionState.Initial) {
        this.delegate = opts?.requestDelegate;
        this.to(SessionState.Establishing);
        return;
      }
      return super.invite(opts);
    }
    async cancel() {
      this.ended("cancel");
      this.to(SessionState.Terminated);
    }
    // What the other side does.
    ring() {
      this.delegate?.onProgress?.({ message: { statusCode: 180 } });
    }
    pickUp() {
      this.to(SessionState.Established);
    }
    refuse(code: number) {
      this.delegate?.onReject?.({ message: { statusCode: code } });
      this.to(SessionState.Terminated);
    }
  }

  class Invitation extends Session {
    remoteIdentity: { uri: { user: string } };
    constructor(from: string) {
      super();
      this.remoteIdentity = { uri: { user: from } };
    }
    async accept() {
      this.to(SessionState.Established);
    }
    async reject() {
      this.ended("reject");
      this.to(SessionState.Terminated);
    }
  }

  class UserAgent {
    static makeURI(s: string) {
      const m = /^sip:([^@]+)@/.exec(s);
      return m ? { user: m[1] } : undefined;
    }
    delegate: { onInvite?: (i: Invitation) => void; onDisconnect?: (e?: Error) => void };
    connected = true;
    reconnects = 0;
    constructor(opts: { delegate: UserAgent["delegate"] }) {
      this.delegate = opts.delegate;
      state.ua = this;
    }
    async start() {}
    async stop() {}
    isConnected() {
      return this.connected;
    }
    async reconnect() {
      this.reconnects++;
      this.connected = true;
    }
  }

  class Registerer {
    state: string = RegistererState.Initial;
    stateChange = new Emitter<string>();
    registrations = 0;
    constructor() {
      state.registerer = this;
      this.answers = [...state.nextAnswers];
    }
    // answers lists how the phone system answers the next registrations
    // (a status code; 200 is a yes). Empty means yes.
    answers: number[] = [];
    // Like the real library: the state event fires only on a change, and
    // the request's own delegate hears the phone system's answer.
    async register(opts?: { requestDelegate?: { onAccept?: () => void; onReject?: (r: { message: { statusCode: number } }) => void } }) {
      this.registrations++;
      const code = this.answers.shift() ?? 200;
      if (code !== 200) {
        if (this.state === RegistererState.Registered) {
          this.state = RegistererState.Unregistered;
          this.stateChange.emit(this.state);
        }
        opts?.requestDelegate?.onReject?.({ message: { statusCode: code } });
        return;
      }
      if (this.state !== RegistererState.Registered) {
        this.state = RegistererState.Registered;
        this.stateChange.emit(this.state);
      }
      opts?.requestDelegate?.onAccept?.();
    }
    async unregister() {
      this.state = RegistererState.Unregistered;
    }
  }

  const state = {
    ua: null as UserAgent | null,
    registerer: null as Registerer | null,
    inviters: [] as Inviter[],
    // nextAnswers is how the phone system answers the first registrations
    // of the next phone that starts.
    nextAnswers: [] as number[],
  };
  return { SessionState, RegistererState, Session, Inviter, Invitation, UserAgent, Registerer, state };
});

vi.mock("sip.js", () => ({
  SessionState: sipMock.SessionState,
  RegistererState: sipMock.RegistererState,
  Inviter: sipMock.Inviter,
  Invitation: sipMock.Invitation,
  UserAgent: sipMock.UserAgent,
  Registerer: sipMock.Registerer,
}));

const apiMock = vi.hoisted(() => ({
  sipCredentials: vi.fn(),
  authorizeTransfer: vi.fn(),
}));
vi.mock("../api/client", () => {
  class ApiError extends Error {}
  return { api: apiMock, ApiError };
});

const logMock = vi.hoisted(() => ({ sendCallLog: vi.fn(), flushPendingCallLogs: vi.fn(), beaconCallEnd: vi.fn() }));
vi.mock("./callLogQueue", () => logMock);

vi.mock("./tones", () => ({
  tones: { stop: vi.fn(), unlock: vi.fn(), incoming: vi.fn(), ringback: vi.fn(), busy: vi.fn(), congestion: vi.fn(), endBeep: vi.fn() },
}));

vi.mock("./audioGraph", () => ({
  CallAudio: class {
    attach() {}
    detach() {}
    setGain() {}
    wave() {
      return null;
    }
    spectrum() {
      return null;
    }
  },
  GAIN_MAX: 3,
  GAIN_MIN: 0,
  loadGain: () => 1,
  saveGain: () => undefined,
}));

// ---------------------------------------------------------------- harness

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function mountPhone() {
  const ref: { current: Phone } = { current: undefined as unknown as Phone };
  function Probe() {
    ref.current = useSoftphone(true);
    return null;
  }
  const root = createRoot(document.createElement("div"));
  act(() => root.render(<Probe />));
  return { phone: ref, unmount: () => act(() => root.unmount()) };
}

// settle lets pending promises and state updates run.
async function settle() {
  await act(async () => {
    for (let i = 0; i < 5; i++) await Promise.resolve();
  });
}

const phases = () => logMock.sendCallLog.mock.calls.map((c) => c[0].phase);

beforeEach(() => {
  vi.clearAllMocks();
  sipMock.state.ua = null;
  sipMock.state.registerer = null;
  sipMock.state.inviters = [];
  sipMock.state.nextAnswers = [];
  apiMock.sipCredentials.mockResolvedValue({ extension: "2001", password: "x", domain: "pbx.local", webSocketUrl: "wss://pbx.local/ws" });
  apiMock.authorizeTransfer.mockResolvedValue(undefined);
  Object.defineProperty(navigator, "mediaDevices", {
    configurable: true,
    value: { getUserMedia: vi.fn().mockResolvedValue({ getTracks: () => [{ stop: () => undefined }] }) },
  });
});

let mounted: ReturnType<typeof mountPhone> | null = null;
afterEach(() => {
  mounted?.unmount();
  mounted = null;
});

async function readyPhone() {
  mounted = mountPhone();
  await settle();
  expect(mounted.phone.current.status).toBe("registered");
  return mounted.phone;
}

// ---------------------------------------------------------------- tests

describe("softphone", () => {
  it("registers with the phone system", async () => {
    const phone = await readyPhone();
    expect(phone.current.extension).toBe("2001");
    expect(sipMock.state.registerer?.registrations).toBe(1);
  });

  it("calls out, mutes, holds, hands over and hangs up", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("0555 123 45 67"));
    const out = sipMock.state.inviters[0];
    expect(out.target.user).toBe("05551234567");
    expect(phone.current.status).toBe("calling");

    act(() => out.ring());
    expect(phone.current.status).toBe("ringing");
    act(() => out.pickUp());
    expect(phone.current.status).toBe("in-call");
    const callId = phone.current.callId;
    expect(callId).toBeTruthy();

    // Mute switches the microphone track off and on again.
    act(() => phone.current.toggleMute());
    expect(phone.current.muted).toBe(true);
    expect(out.media.track.enabled).toBe(false);
    act(() => phone.current.toggleMute());
    expect(phone.current.muted).toBe(false);
    expect(out.media.track.enabled).toBe(true);

    // Hold asks for a re-invite with hold on, then off.
    await act(() => phone.current.toggleHold());
    expect(phone.current.status).toBe("held");
    expect(phone.current.held).toBe(true);
    expect(out.reinvites.at(-1)).toMatchObject({ sessionDescriptionHandlerOptions: { hold: true } });
    await act(() => phone.current.toggleHold());
    expect(phone.current.status).toBe("in-call");
    expect(out.reinvites.at(-1)).toMatchObject({ sessionDescriptionHandlerOptions: { hold: false } });

    // The panel approves the hand-over first, then the phone system carries it.
    await act(() => phone.current.transfer("2002"));
    expect(apiMock.authorizeTransfer).toHaveBeenCalledWith(callId, "2002");
    expect(out.referred.map((u) => u.user)).toEqual(["2002"]);

    await act(() => phone.current.hangup());
    expect(out.ended).toHaveBeenCalledWith("bye");
    expect(phone.current.status).toBe("registered");
    expect(phone.current.lastEnded?.id).toBe(callId);
    expect(phone.current.lastEnded?.peer).toBe("05551234567");
    expect(phases()).toEqual(["start", "answer", "end"]);
    expect(logMock.sendCallLog.mock.calls.at(-1)?.[0]).toMatchObject({ callId, disposition: "answered", direction: "outbound" });
  });

  it("does not hand over a call the panel refuses", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("05551234567"));
    const out = sipMock.state.inviters[0];
    act(() => out.pickUp());
    apiMock.authorizeTransfer.mockRejectedValueOnce(new Error("no"));
    await act(() => phone.current.transfer("05329999999"));
    expect(out.referred).toEqual([]);
    expect(phone.current.error).toBeTruthy();
    expect(phone.current.status).toBe("in-call");
  });

  it("tells the agent when the phone system refuses a hand-over", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("05551234567"));
    const out = sipMock.state.inviters[0];
    act(() => out.pickUp());
    out.referReject = true;
    await act(() => phone.current.transfer("2002"));
    expect(phone.current.error).toBeTruthy();
  });

  it("takes an incoming call and refuses a second one meanwhile", async () => {
    const phone = await readyPhone();
    const inbound = new sipMock.Invitation("05321112233");
    act(() => sipMock.state.ua?.delegate.onInvite?.(inbound));
    expect(phone.current.status).toBe("incoming");
    expect(phone.current.peer).toBe("05321112233");

    const second = new sipMock.Invitation("05320000000");
    act(() => sipMock.state.ua?.delegate.onInvite?.(second));
    await settle();
    expect(second.ended).toHaveBeenCalledWith("reject");
    expect(phone.current.peer).toBe("05321112233");

    await act(() => phone.current.answer());
    expect(phone.current.status).toBe("in-call");
    // The customer hangs up.
    act(() => inbound.to(sipMock.SessionState.Terminated));
    expect(phone.current.status).toBe("registered");
    expect(phone.current.lastEnded?.direction).toBe("inbound");
    expect(logMock.sendCallLog.mock.calls.at(-1)?.[0]).toMatchObject({ phase: "end", disposition: "answered", direction: "inbound" });
  });

  it("logs a missed call when the caller gives up", async () => {
    const phone = await readyPhone();
    const inbound = new sipMock.Invitation("05321112233");
    act(() => sipMock.state.ua?.delegate.onInvite?.(inbound));
    act(() => inbound.to(sipMock.SessionState.Terminated));
    expect(phone.current.status).toBe("registered");
    expect(logMock.sendCallLog.mock.calls.at(-1)?.[0]).toMatchObject({ phase: "end", disposition: "missed" });
  });

  it("reports a busy line and frees the phone", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("05551234567"));
    const out = sipMock.state.inviters[0];
    act(() => out.refuse(486));
    expect(phone.current.status).toBe("registered");
    expect(phone.current.lastUnreached?.reason).toBe("busy");
    expect(logMock.sendCallLog.mock.calls.at(-1)?.[0]).toMatchObject({ phase: "end", disposition: "busy" });
  });

  it("cancels a call that is still ringing", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("05551234567"));
    const out = sipMock.state.inviters[0];
    act(() => out.ring());
    await act(() => phone.current.hangup());
    expect(out.ended).toHaveBeenCalledWith("cancel");
    expect(phone.current.status).toBe("registered");
    expect(logMock.sendCallLog.mock.calls.at(-1)?.[0]).toMatchObject({ phase: "end", disposition: "canceled" });
  });

  it("keeps feature codes out of the call log", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("*52001"));
    expect(phone.current.callId).toBeNull();
    expect(logMock.sendCallLog).not.toHaveBeenCalled();
  });

  it("reconnects after the connection drops", async () => {
    const phone = await readyPhone();
    const ua = sipMock.state.ua!;
    ua.connected = false;
    await act(async () => ua.delegate.onDisconnect?.(new Error("network")));
    await settle();
    expect(ua.reconnects).toBe(1);
    // A fresh registration on the new connection, then ready again.
    expect(sipMock.state.registerer?.registrations).toBe(2);
    expect(phone.current.status).toBe("registered");
  });

  it("is not ready until the phone system accepts, and keeps trying", async () => {
    vi.useFakeTimers();
    try {
      // The phone system is down for the first two tries.
      sipMock.state.nextAnswers = [503, 503];
      mounted = mountPhone();
      await settle();
      expect(mounted.phone.current.status).toBe("connecting");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2_100);
      });
      expect(mounted.phone.current.status).toBe("connecting");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4_100);
      });
      expect(sipMock.state.registerer?.registrations).toBe(3);
      expect(mounted.phone.current.status).toBe("registered");
    } finally {
      vi.useRealTimers();
    }
  });

  it("says so when the line's password is refused, and recovers once it is fixed", async () => {
    vi.useFakeTimers();
    try {
      sipMock.state.nextAnswers = [403];
      mounted = mountPhone();
      await settle();
      expect(mounted.phone.current.status).toBe("error");
      expect(mounted.phone.current.error).toMatch(/şifresini kabul etmedi/);
      // The administrator fixes the password; the next try is accepted.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2_100);
      });
      expect(mounted.phone.current.status).toBe("registered");
      expect(mounted.phone.current.error).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("takes the line back when renewing it is refused twice in a row", async () => {
    vi.useFakeTimers();
    try {
      mounted = mountPhone();
      await settle();
      expect(mounted.phone.current.status).toBe("registered");
      const reg = sipMock.state.registerer!;
      // The phone system goes into maintenance: the library's own renewal is
      // refused (the line drops), and so are the next two tries.
      reg.answers = [503, 503];
      act(() => {
        reg.state = sipMock.RegistererState.Unregistered;
        reg.stateChange.emit(reg.state);
      });
      await settle();
      expect(mounted.phone.current.status).toBe("connecting");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2_100);
      });
      expect(mounted.phone.current.status).toBe("connecting");
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4_100);
      });
      expect(reg.registrations).toBe(4);
      expect(mounted.phone.current.status).toBe("registered");
    } finally {
      vi.useRealTimers();
    }
  });

  it("stays ready when the network blinks without dropping the line", async () => {
    const phone = await readyPhone();
    await act(async () => window.dispatchEvent(new Event("online")));
    await settle();
    expect(sipMock.state.ua?.reconnects).toBe(0);
    expect(sipMock.state.registerer?.registrations).toBe(1);
    expect(phone.current.status).toBe("registered");
  });

  it("sends keypad tones during a call", async () => {
    const phone = await readyPhone();
    await act(() => phone.current.call("4440000"));
    const out = sipMock.state.inviters[0];
    act(() => out.pickUp());
    act(() => phone.current.sendDtmf("5"));
    expect(out.sessionDescriptionHandler.sendDtmf).toHaveBeenCalledWith("5");
  });

  it("stays consistent through 60 calls in a row", async () => {
    const phone = await readyPhone();
    const ids = new Set<string>();
    for (let i = 0; i < 60; i++) {
      await act(() => phone.current.call(`0555000${String(i).padStart(4, "0")}`));
      const out = sipMock.state.inviters[i];
      act(() => out.ring());
      if (i % 3 === 2) {
        // Every third customer does not pick up.
        act(() => out.refuse(480));
      } else {
        act(() => out.pickUp());
        if (i % 2 === 0) act(() => phone.current.toggleMute());
        await act(() => phone.current.toggleHold());
        await act(() => phone.current.hangup());
      }
      expect(phone.current.status).toBe("registered");
      expect(phone.current.muted).toBe(false);
      expect(phone.current.held).toBe(false);
      expect(phone.current.callId).toBeNull();
      ids.add(String(logMock.sendCallLog.mock.calls.at(-1)?.[0].callId));
    }
    expect(ids.size).toBe(60);
    const ends = logMock.sendCallLog.mock.calls.filter((c) => c[0].phase === "end").map((c) => c[0].disposition);
    expect(ends.filter((d) => d === "answered")).toHaveLength(40);
    expect(ends.filter((d) => d === "no_answer")).toHaveLength(20);
  });
});
