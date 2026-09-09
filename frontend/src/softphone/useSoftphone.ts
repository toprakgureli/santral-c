import { useCallback, useEffect, useRef, useState } from "react";
import { Inviter, Invitation, Registerer, SessionState, UserAgent, type Session } from "sip.js";
import { api } from "../api/client";
import type { SipCredentials } from "../api/types";
import { tones } from "./tones";

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

export interface Phone {
  status: PhoneStatus;
  extension: string | null;
  error: string | null;
  muted: boolean;
  held: boolean;
  peer: string | null;
  endReason: string | null;
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

  const audioRef = useRef<HTMLAudioElement>(null);
  const uaRef = useRef<UserAgent | null>(null);
  const sessionRef = useRef<Session | null>(null);
  const localEndRef = useRef(false);
  const domainRef = useRef("");

  const attachRemoteMedia = useCallback((session: Session) => {
    const pc = peerConnection(session);
    if (!pc || !audioRef.current) return;
    const stream = new MediaStream();
    pc.getReceivers().forEach((r) => r.track && stream.addTrack(r.track));
    audioRef.current.srcObject = stream;
    void audioRef.current.play().catch(() => undefined);
  }, []);

  const watchSession = useCallback(
    (session: Session) => {
      let wasEstablished = false;
      session.stateChange.addListener((state) => {
        if (state === SessionState.Established) {
          wasEstablished = true;
          tones.stop();
          setMuted(false);
          setHeld(false);
          setStatus("in-call");
          attachRemoteMedia(session);
        } else if (state === SessionState.Terminated) {
          tones.stop();
          // Only beep when an actual conversation ended, so a rejected or
          // failed call does not collide with the PBX's own announcement.
          if (wasEstablished) {
            tones.endBeep();
            setEndReason(localEndRef.current ? "Kapattınız" : "Karşı taraf kapattı");
          }
          setPeer(null);
          setMuted(false);
          setHeld(false);
          setStatus("registered");
          sessionRef.current = null;
        }
      });
    },
    [attachRemoteMedia],
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
          sessionDescriptionHandlerFactoryOptions: {
            iceGatheringTimeout: 5000,
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
              setPeer(invitation.remoteIdentity.uri.user ?? "");
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
  }, [enabled, watchSession]);

  const call = useCallback(
    async (target: string) => {
      const ua = uaRef.current;
      if (!ua) throw new Error("Softphone hazır değil.");
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
              setPeer(null);
              setStatus("registered");
              sessionRef.current = null;
            },
          },
        });
      } catch (e) {
        tones.stop();
        setStatus("registered");
        sessionRef.current = null;
        throw e;
      }
    },
    [attachRemoteMedia, watchSession],
  );

  const answer = useCallback(async () => {
    const s = sessionRef.current;
    if (!(s instanceof Invitation)) return;
    tones.stop();
    await s.accept({ sessionDescriptionHandlerOptions: { constraints: { audio: true, video: false } } }).catch(() => undefined);
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

  const transfer = useCallback(async (target: string) => {
    const s = sessionRef.current;
    if (!s || s.state !== SessionState.Established) return;
    const uri = UserAgent.makeURI(`sip:${target}@${domainRef.current}`);
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
    extension,
    error,
    muted,
    held,
    peer,
    endReason,
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
