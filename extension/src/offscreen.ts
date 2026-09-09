// The single SIP endpoint for the whole browser. Registers to Bulutsantralim
// over WSS with the credentials saved in the popup, drives calls, and reports
// state to the service worker which fans it out to the per-tab widgets.

import { Inviter, Invitation, Registerer, SessionState, UserAgent, type Session } from "sip.js";

type Config = { ext: string; password: string; wss: string; domain: string; stun?: string };

type State = {
  status: "idle" | "registered" | "calling" | "ringing" | "incoming" | "in-call" | "held" | "error" | "unconfigured";
  peer?: string;
  muted?: boolean;
  held?: boolean;
  extension?: string;
  endReason?: string;
  error?: string;
};

let ua: UserAgent | null = null;
let registerer: Registerer | null = null;
let session: Session | null = null;
let localEnd = false;
let domain = "";
const remote = document.getElementById("remote") as HTMLAudioElement;

let state: State = { status: "unconfigured" };

function post(patch: Partial<State>) {
  state = { ...state, ...patch };
  chrome.runtime.sendMessage({ to: "sw", type: "state", state }).catch(() => undefined);
}

function pc(s: Session): RTCPeerConnection | undefined {
  const sdh = s.sessionDescriptionHandler as unknown as { peerConnection?: RTCPeerConnection } | undefined;
  return sdh?.peerConnection;
}

function attachMedia(s: Session) {
  const conn = pc(s);
  if (!conn) return;
  const stream = new MediaStream();
  conn.getReceivers().forEach((r) => r.track && stream.addTrack(r.track));
  remote.srcObject = stream;
  void remote.play().catch(() => undefined);
}

function rejectReason(code?: number): string {
  switch (code) {
    case 486:
    case 600:
      return "Meşgul";
    case 404:
    case 484:
      return "Numara bulunamadı";
    case 480:
    case 408:
    case 603:
      return "Ulaşılamıyor";
    default:
      return code ? `Başarısız (${code})` : "Başarısız";
  }
}

function watch(s: Session) {
  let established = false;
  s.stateChange.addListener((st) => {
    if (st === SessionState.Established) {
      established = true;
      post({ status: "in-call", muted: false, held: false });
      attachMedia(s);
    } else if (st === SessionState.Terminated) {
      const reason = established ? (localEnd ? "Kapattınız" : "Karşı taraf kapattı") : state.endReason;
      session = null;
      post({ status: "registered", peer: undefined, muted: false, held: false, endReason: reason });
    }
  });
}

async function register(cfg: Config) {
  if (ua) {
    try {
      await ua.stop();
    } catch {
      // ignore
    }
    ua = null;
    registerer = null;
  }
  domain = cfg.domain;
  const uri = UserAgent.makeURI(`sip:${cfg.ext}@${cfg.domain}`);
  if (!uri) {
    post({ status: "error", error: "SIP adresi geçersiz." });
    return;
  }
  const iceServers: RTCIceServer[] = cfg.stun ? [{ urls: cfg.stun }] : [];
  ua = new UserAgent({
    uri,
    transportOptions: { server: cfg.wss },
    authorizationUsername: cfg.ext,
    authorizationPassword: cfg.password,
    displayName: cfg.ext,
    sessionDescriptionHandlerFactoryOptions: {
      iceGatheringTimeout: 5000,
      peerConnectionConfiguration: { iceServers },
    },
    delegate: {
      onInvite: (invitation: Invitation) => {
        if (session) {
          void invitation.reject();
          return;
        }
        session = invitation;
        localEnd = false;
        post({ status: "incoming", peer: invitation.remoteIdentity.uri.user ?? "", endReason: undefined });
        watch(invitation);
      },
    },
  });
  try {
    await ua.start();
    registerer = new Registerer(ua);
    await registerer.register();
    post({ status: "registered", extension: cfg.ext, error: undefined });
  } catch (e) {
    post({ status: "error", error: e instanceof Error ? e.message : "Kayıt başarısız." });
  }
}

async function call(target: string) {
  if (!ua) return;
  const uri = UserAgent.makeURI(`sip:${target}@${domain}`);
  if (!uri) return;
  const inviter = new Inviter(ua, uri, {
    earlyMedia: true,
    sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } },
  });
  session = inviter;
  localEnd = false;
  post({ status: "calling", peer: target, endReason: undefined });
  watch(inviter);
  try {
    await inviter.invite({
      requestDelegate: {
        onProgress: (r) => {
          post({ status: "ringing" });
          if (r.message.statusCode === 183) attachMedia(inviter);
        },
        onReject: (r) => {
          session = null;
          post({ status: "registered", peer: undefined, endReason: rejectReason(r.message.statusCode) });
        },
      },
    });
  } catch {
    session = null;
    post({ status: "registered", peer: undefined });
  }
}

async function answer() {
  if (session instanceof Invitation) {
    await session.accept({ sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } } }).catch(() => undefined);
  }
}

async function hangup() {
  const s = session;
  if (!s) return;
  localEnd = true;
  try {
    if (s instanceof Inviter && (s.state === SessionState.Initial || s.state === SessionState.Establishing)) await s.cancel();
    else if (s instanceof Invitation && s.state === SessionState.Initial) await s.reject();
    else if (s.state === SessionState.Established) await s.bye();
  } catch {
    // ignore
  }
}

function mute() {
  const s = session;
  if (!s) return;
  const conn = pc(s);
  if (!conn) return;
  const next = !state.muted;
  conn.getSenders().forEach((snd) => snd.track && snd.track.kind === "audio" && (snd.track.enabled = !next));
  post({ muted: next });
}

async function hold() {
  const s = session;
  if (!s || s.state !== SessionState.Established) return;
  const next = !state.held;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const opts: any = { sessionDescriptionHandlerOptions: { hold: next } };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (s as any).sessionDescriptionHandlerOptionsReInvite = { hold: next };
  try {
    await s.invite(opts);
    post({ status: next ? "held" : "in-call", held: next });
  } catch {
    // ignore
  }
}

async function transfer(target: string) {
  const s = session;
  if (!s || s.state !== SessionState.Established) return;
  const uri = UserAgent.makeURI(`sip:${target}@${domain}`);
  if (uri) await s.refer(uri).catch(() => undefined);
}

function dtmf(key: string) {
  const s = session;
  if (!s) return;
  const sdh = s.sessionDescriptionHandler as unknown as { sendDtmf?: (t: string) => boolean } | undefined;
  sdh?.sendDtmf?.(key);
}

async function boot() {
  const { sipConfig } = await chrome.storage.local.get("sipConfig");
  if (sipConfig?.ext && sipConfig?.password && sipConfig?.wss && sipConfig?.domain) {
    await register(sipConfig as Config);
  } else {
    post({ status: "unconfigured" });
  }
}

chrome.runtime.onMessage.addListener((msg) => {
  if (!msg || msg.to !== "offscreen" || !msg._fwd) return;
  switch (msg.cmd) {
    case "call":
      void call(String(msg.arg ?? ""));
      break;
    case "answer":
      void answer();
      break;
    case "hangup":
      void hangup();
      break;
    case "mute":
      mute();
      break;
    case "hold":
      void hold();
      break;
    case "transfer":
      void transfer(String(msg.arg ?? ""));
      break;
    case "dtmf":
      dtmf(String(msg.arg ?? ""));
      break;
    case "reconfigure":
      void boot();
      break;
  }
});

void boot();
