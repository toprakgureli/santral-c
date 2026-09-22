import { useCallback, useEffect, useRef, useState } from "react";
import { Inviter, Invitation, Registerer, SessionState, UserAgent, type Session } from "sip.js";
import { api } from "../api/client";
import type { SipCredentials } from "../api/types";
import { normalizeDial } from "./dial";
import { tones } from "./tones";

// mediaError turns a getUserMedia failure into a message the agent can act on.
// Anything else keeps its own text.
function mediaError(e: unknown): string {
  const name = e instanceof Error ? e.name : "";
  switch (name) {
    case "NotAllowedError":
    case "SecurityError":
      return "Mikrofon izni verilmedi. Adres çubuğundaki kilit simgesinden mikrofona izin verip sayfayı yenileyin.";
    case "NotFoundError":
    case "OverconstrainedError":
      return "Mikrofon bulunamadı. Bir mikrofon bağlayıp sayfayı yenileyin.";
    case "NotReadableError":
    case "AbortError":
      return "Mikrofon başka bir uygulama tarafından kullanılıyor.";
  }
  return e instanceof Error && e.message ? e.message : "Çağrı için ses aygıtı açılamadı.";
}

// checkMicrophone opens and immediately releases the microphone, so the
// browser's permission prompt is settled early. It returns a message when the
// microphone cannot be used, or null when it can.
async function checkMicrophone(): Promise<string | null> {
  if (!navigator.mediaDevices?.getUserMedia) {
    return "Bu tarayıcı mikrofona erişemiyor. Sayfa HTTPS üzerinden açılmalı.";
  }
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
    stream.getTracks().forEach((t) => t.stop());
    return null;
  } catch (e) {
    return mediaError(e);
  }
}

export type PhoneStatus =
  | "connecting"
  | "registered"
  | "calling"
  | "ringing"
  | "incoming"
  | "in-call"
  | "held"
  | "error"
  | "disabled";

// EndedCall describes an answered call that has just ended, for the
// after-call wrap-up. id is the panel's call log id for that call.
export interface EndedCall {
  id: string;
  peer: string;
  direction: "inbound" | "outbound";
  answeredAt: number;
  endedAt: number;
}

// UnreachedCall is an outbound call that ended without a conversation.
export interface UnreachedCall {
  id: string;
  peer: string;
  endedAt: number;
  reason: "no_answer" | "busy" | "canceled" | "short";
}

// A connected outbound call that ends within this many seconds was the PBX
// playing a rejection or busy announcement, not the customer. Anything longer
// counts as reached, whoever hung up.
const ANNOUNCEMENT_SECONDS = 8;

export interface Phone {
  status: PhoneStatus;
  // The last answered call that ended (null until one does). Unanswered,
  // missed and cancelled calls never set it.
  lastEnded: EndedCall | null;
  // The panel's call log id of the current call (null when idle or for
  // feature codes), so an escalation entered mid-call can be tied to it.
  callId: string | null;
  // The other party of the most recent call (in or out, answered or not),
  // kept after hangup so a follow-up (WhatsApp) can target it.
  lastPeer: string | null;
  // The last outbound call that did not reach the customer.
  lastUnreached: UnreachedCall | null;
  extension: string | null;
  error: string | null;
  muted: boolean;
  held: boolean;
  peer: string | null;
  endReason: string | null;
  // Epoch ms, kept in the global softphone so timers survive page navigation.
  callStartedAt: number | null; // when the current call attempt/incoming began
  answeredAt: number | null; // when the current call was answered
  audioRef: React.RefObject<HTMLAudioElement>;
  call: (target: string) => Promise<void>;
  answer: () => Promise<void>;
  hangup: () => Promise<void>;
  toggleMute: () => void;
  toggleHold: () => Promise<void>;
  transfer: (target: string) => Promise<void>;
  sendDtmf: (key: string) => void;
}

function iceServers(creds: SipCredentials): RTCIceServer[] {
  const servers: RTCIceServer[] = [];
  if (creds.stunUrl) servers.push({ urls: creds.stunUrl });
  if (creds.turnUrl) servers.push({ urls: creds.turnUrl, username: creds.turnUser, credential: creds.turnPass });
  return servers;
}

function peerConnection(session: Session): RTCPeerConnection | undefined {
  const sdh = session.sessionDescriptionHandler as unknown as { peerConnection?: RTCPeerConnection } | undefined;
  return sdh?.peerConnection;
}

function newCallId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `c-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  }
}

function rejectReason(code: number | undefined): string {
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
    case 403:
      return "İzin yok";
    default:
      return code ? `Başarısız (${code})` : "Başarısız";
  }
}

export function useSoftphone(enabled: boolean): Phone {
  const [status, setStatus] = useState<PhoneStatus>(enabled ? "connecting" : "disabled");
  const [extension, setExtension] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [muted, setMuted] = useState(false);
  const [held, setHeld] = useState(false);
  const [peer, setPeer] = useState<string | null>(null);
  const [endReason, setEndReason] = useState<string | null>(null);
  const [callStartedAt, setCallStartedAt] = useState<number | null>(null);
  const [answeredAt, setAnsweredAt] = useState<number | null>(null);
  const [lastEnded, setLastEnded] = useState<EndedCall | null>(null);
  const [callId, setCallId] = useState<string | null>(null);
  const [lastPeer, setLastPeer] = useState<string | null>(null);
  const [lastUnreached, setLastUnreached] = useState<UnreachedCall | null>(null);

  const audioRef = useRef<HTMLAudioElement>(null);
  const uaRef = useRef<UserAgent | null>(null);
  const sessionRef = useRef<Session | null>(null);
  const localEndRef = useRef(false);
  const domainRef = useRef("");

  // Call-log correlation: one id per call, its direction/peer, and when it was
  // answered, so we can record the call to our own store as it progresses.
  const callIdRef = useRef<string>("");
  const callDirRef = useRef<"inbound" | "outbound">("outbound");
  const callPeerRef = useRef<string>("");
  const establishedAtRef = useRef<number>(0);

  const attachRemoteMedia = useCallback((session: Session) => {
    const pc = peerConnection(session);
    if (!pc || !audioRef.current) return;
    const stream = new MediaStream();
    pc.getReceivers().forEach((r) => r.track && stream.addTrack(r.track));
    audioRef.current.srcObject = stream;
    void audioRef.current.play().catch(() => undefined);
  }, []);

  const logCall = useCallback((phase: "start" | "answer" | "end", extra: { disposition?: string; durationSeconds?: number } = {}) => {
    if (!callIdRef.current) return;
    api
      .logCall({
        callId: callIdRef.current,
        phase,
        direction: callDirRef.current,
        peer: callPeerRef.current,
        ...extra,
      })
      .catch(() => undefined);
  }, []);

  const watchSession = useCallback(
    (session: Session) => {
      let wasEstablished = false;
      session.stateChange.addListener((state) => {
        if (state === SessionState.Established) {
          wasEstablished = true;
          establishedAtRef.current = Date.now();
          setAnsweredAt(Date.now());
          tones.stop();
          setMuted(false);
          setHeld(false);
          setStatus("in-call");
          attachRemoteMedia(session);
          logCall("answer");
        } else if (state === SessionState.Terminated) {
          tones.stop();
          // Only beep when an actual conversation ended, so a rejected or
          // failed call does not collide with the PBX's own announcement.
          if (wasEstablished) {
            tones.endBeep();
            setEndReason(localEndRef.current ? "Kapattınız" : "Karşı taraf kapattı");
          }
          const duration = wasEstablished ? Math.round((Date.now() - establishedAtRef.current) / 1000) : 0;
          if (wasEstablished && callIdRef.current) {
            setLastEnded({
              id: callIdRef.current,
              peer: callPeerRef.current,
              direction: callDirRef.current,
              answeredAt: establishedAtRef.current,
              endedAt: Date.now(),
            });
          }
          const disposition = wasEstablished
            ? "answered"
            : localEndRef.current
              ? "canceled"
              : callDirRef.current === "inbound"
                ? "missed"
                : "no_answer";
          if (callDirRef.current === "outbound" && callIdRef.current && (!wasEstablished || duration < ANNOUNCEMENT_SECONDS)) {
            setLastUnreached({
              id: callIdRef.current,
              peer: callPeerRef.current,
              endedAt: Date.now(),
              reason: wasEstablished ? "short" : localEndRef.current ? "canceled" : "no_answer",
            });
          }
          logCall("end", { disposition, durationSeconds: duration });
          callIdRef.current = "";
          setCallId(null);
          setPeer(null);
          setMuted(false);
          setHeld(false);
          setCallStartedAt(null);
          setAnsweredAt(null);
          setStatus("registered");
          sessionRef.current = null;
        }
      });
    },
    [attachRemoteMedia, logCall],
  );

  useEffect(() => {
    if (!enabled) {
      setStatus("disabled");
      return;
    }
    let cancelled = false;
    let ua: UserAgent | null = null;
    let registerer: Registerer | null = null;

    // Browsers block audio until a user gesture; unlock on the first one so the
    // incoming ring can play even when no call control was clicked yet.
    const unlock = () => tones.unlock();
    window.addEventListener("pointerdown", unlock, { once: true });
    window.addEventListener("keydown", unlock, { once: true });

    (async () => {
      try {
        const creds = await api.sipCredentials();
        if (cancelled) return;
        setExtension(creds.extension);
        domainRef.current = creds.domain;

        const uri = UserAgent.makeURI(`sip:${creds.extension}@${creds.domain}`);
        if (!uri) throw new Error("SIP adresi oluşturulamadı.");

        ua = new UserAgent({
          uri,
          transportOptions: { server: creds.webSocketUrl },
          authorizationUsername: creds.extension,
          authorizationPassword: creds.password,
          displayName: creds.extension,
          // Shown as the device in the PBX's call details instead of the library name.
          userAgentString: "santral-c/toprakgureli",
          sessionDescriptionHandlerFactoryOptions: {
            // SIP.js holds the INVITE until ICE gathering completes or this
            // timeout elapses, so a large value delays ringing by that long when
            // a STUN/TURN candidate is slow. Host candidates are ready almost
            // immediately and are enough for the hosted PBX, so cap the wait low
            // to make calls ring right away.
            iceGatheringTimeout: 500,
            peerConnectionConfiguration: { iceServers: iceServers(creds) },
          },
          delegate: {
            onInvite: (invitation: Invitation) => {
              if (sessionRef.current) {
                void invitation.reject();
                return;
              }
              sessionRef.current = invitation;
              localEndRef.current = false;
              setEndReason(null);
              const from = invitation.remoteIdentity.uri.user ?? "";
              setPeer(from);
              callIdRef.current = newCallId();
              setCallId(callIdRef.current);
              callDirRef.current = "inbound";
              callPeerRef.current = from;
              setLastPeer(from);
              setCallStartedAt(Date.now());
              logCall("start");
              setStatus("incoming");
              tones.incoming();
              watchSession(invitation);
            },
          },
        });
        uaRef.current = ua;
        await ua.start();
        registerer = new Registerer(ua);
        await registerer.register();
        if (!cancelled) setStatus("registered");
        // Ask for the microphone now, so the permission prompt is answered
        // before the first call rings instead of during it. A refusal is shown
        // as a warning; the phone stays registered so calls still come in.
        const micProblem = await checkMicrophone();
        if (!cancelled && micProblem) setError(micProblem);
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "Softphone başlatılamadı.");
          setStatus("error");
        }
      }
    })();

    return () => {
      cancelled = true;
      tones.stop();
      window.removeEventListener("pointerdown", unlock);
      window.removeEventListener("keydown", unlock);
      uaRef.current = null;
      sessionRef.current = null;
      if (registerer) registerer.unregister().catch(() => undefined);
      if (ua) ua.stop().catch(() => undefined);
    };
  }, [enabled, watchSession, logCall]);

  const call = useCallback(
    async (raw: string) => {
      const ua = uaRef.current;
      if (!ua) throw new Error("Softphone hazır değil.");
      const target = normalizeDial(raw);
      const uri = UserAgent.makeURI(`sip:${target}@${domainRef.current}`);
      if (!uri) throw new Error("Geçersiz numara.");

      const inviter = new Inviter(ua, uri, {
        earlyMedia: true,
        sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } },
      });
      sessionRef.current = inviter;
      localEndRef.current = false;
      setEndReason(null);
      setPeer(target);
      // Feature codes (*5<ext> listen-in, *60 echo test and the like) are not
      // customer calls: an empty call id keeps them out of the call log so
      // they never count as an outbound call.
      const featureCode = /^[*#]/.test(target);
      callIdRef.current = featureCode ? "" : newCallId();
      setCallId(callIdRef.current || null);
      callDirRef.current = "outbound";
      callPeerRef.current = target;
      if (!featureCode) setLastPeer(target);
      setCallStartedAt(Date.now());
      setError(null);
      logCall("start");
      setStatus("calling");
      watchSession(inviter);

      try {
        await inviter.invite({
          requestDelegate: {
            onProgress: (response) => {
              const code = response.message.statusCode;
              setStatus("ringing");
              if (code === 183) {
                tones.stop(); // early media / announcement from the PBX
                attachRemoteMedia(inviter);
              } else {
                tones.ringback();
              }
            },
            onReject: (response) => {
              tones.stop();
              const reason = rejectReason(response.message.statusCode);
              setEndReason(reason);
              if (reason === "Meşgul") tones.busy();
              else tones.congestion();
              window.setTimeout(() => tones.stop(), 2500);
              logCall("end", { disposition: reason === "Meşgul" ? "busy" : "no_answer", durationSeconds: 0 });
              if (callIdRef.current) {
                setLastUnreached({ id: callIdRef.current, peer: callPeerRef.current, endedAt: Date.now(), reason: reason === "Meşgul" ? "busy" : "no_answer" });
              }
              callIdRef.current = "";
          setCallId(null);
              setPeer(null);
              setCallStartedAt(null);
              setAnsweredAt(null);
              setStatus("registered");
              sessionRef.current = null;
            },
          },
        });
      } catch (e) {
        tones.stop();
        setError(mediaError(e));
        setStatus("registered");
        sessionRef.current = null;
        throw e;
      }
    },
    [attachRemoteMedia, watchSession, logCall],
  );

  // answer picks up the ringing call. Accepting needs the microphone, and a
  // blocked or missing microphone used to fail silently (the call kept ringing
  // and the button seemed dead), so the reason is now shown and the call keeps
  // ringing for a retry after the permission is granted.
  const answer = useCallback(async () => {
    const s = sessionRef.current;
    if (!(s instanceof Invitation)) return;
    tones.stop();
    setError(null);
    try {
      await s.accept({ sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } } });
    } catch (e) {
      setError(mediaError(e));
      if (s.state === SessionState.Initial) tones.incoming();
    }
  }, []);

  const hangup = useCallback(async () => {
    const s = sessionRef.current;
    if (!s) return;
    localEndRef.current = true;
    tones.stop();
    try {
      if (s instanceof Inviter && (s.state === SessionState.Initial || s.state === SessionState.Establishing)) {
        await s.cancel();
      } else if (s instanceof Invitation && s.state === SessionState.Initial) {
        await s.reject();
      } else if (s.state === SessionState.Established) {
        await s.bye();
      }
    } catch {
      // ignore
    }
  }, []);

  const toggleMute = useCallback(() => {
    const s = sessionRef.current;
    if (!s) return;
    const pc = peerConnection(s);
    if (!pc) return;
    const next = !muted;
    pc.getSenders().forEach((sender) => {
      if (sender.track && sender.track.kind === "audio") sender.track.enabled = !next;
    });
    setMuted(next);
  }, [muted]);

  const toggleHold = useCallback(async () => {
    const s = sessionRef.current;
    if (!s || s.state !== SessionState.Established) return;
    const next = !held;
    // The SIP.js Web SDH implements hold via its `hold` option on a re-INVITE.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const inviteOptions: any = { sessionDescriptionHandlerOptions: { hold: next } };
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (s as any).sessionDescriptionHandlerOptionsReInvite = { hold: next };
    try {
      await s.invite(inviteOptions);
      setHeld(next);
      setStatus(next ? "held" : "in-call");
    } catch {
      // renegotiation failed; leave state unchanged
    }
  }, [held]);

  const transfer = useCallback(async (raw: string) => {
    const s = sessionRef.current;
    if (!s || s.state !== SessionState.Established) return;
    const uri = UserAgent.makeURI(`sip:${normalizeDial(raw)}@${domainRef.current}`);
    if (!uri) return;
    await s.refer(uri).catch(() => undefined);
  }, []);

  const sendDtmf = useCallback((key: string) => {
    const s = sessionRef.current;
    if (!s) return;
    const sdh = s.sessionDescriptionHandler as unknown as { sendDtmf?: (t: string) => boolean } | undefined;
    if (sdh?.sendDtmf) sdh.sendDtmf(key);
  }, []);

  return {
    status,
    lastEnded,
    callId,
    lastPeer,
    lastUnreached,
    extension,
    error,
    muted,
    held,
    peer,
    endReason,
    callStartedAt,
    answeredAt,
    audioRef,
    call,
    answer,
    hangup,
    toggleMute,
    toggleHold,
    transfer,
    sendDtmf,
  };
}
