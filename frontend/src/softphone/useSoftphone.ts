import { useCallback, useEffect, useRef, useState } from "react";
import { Web } from "sip.js";
import { api } from "../api/client";
import type { SipCredentials } from "../api/types";

export type PhoneStatus = "idle" | "connecting" | "registered" | "ringing" | "in-call" | "error" | "disabled";

interface Phone {
  status: PhoneStatus;
  extension: string | null;
  error: string | null;
  incoming: boolean;
  audioRef: React.RefObject<HTMLAudioElement>;
  call: (target: string) => Promise<void>;
  answer: () => Promise<void>;
  hangup: () => Promise<void>;
}

function iceServers(creds: SipCredentials): RTCIceServer[] {
  const servers: RTCIceServer[] = [];
  if (creds.stunUrl) servers.push({ urls: creds.stunUrl });
  if (creds.turnUrl) servers.push({ urls: creds.turnUrl, username: creds.turnUser, credential: creds.turnPass });
  return servers;
}

export function useSoftphone(enabled: boolean): Phone {
  const [status, setStatus] = useState<PhoneStatus>(enabled ? "connecting" : "disabled");
  const [extension, setExtension] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [incoming, setIncoming] = useState(false);
  const audioRef = useRef<HTMLAudioElement>(null);
  const userRef = useRef<Web.SimpleUser | null>(null);

  useEffect(() => {
    if (!enabled) {
      setStatus("disabled");
      return;
    }
    let cancelled = false;
    let simpleUser: Web.SimpleUser | null = null;

    (async () => {
      try {
        const creds = await api.sipCredentials();
        if (cancelled) return;
        setExtension(creds.extension);

        simpleUser = new Web.SimpleUser(creds.webSocketUrl, {
          aor: `sip:${creds.extension}@${creds.domain}`,
          media: {
            constraints: { audio: true, video: false },
            remote: { audio: audioRef.current ?? undefined },
          },
          userAgentOptions: {
            authorizationUsername: creds.extension,
            authorizationPassword: creds.password,
            displayName: creds.extension,
            sessionDescriptionHandlerFactoryOptions: {
              peerConnectionConfiguration: { iceServers: iceServers(creds) },
            },
          },
        });
        simpleUser.delegate = {
          onCallReceived: async () => {
            setIncoming(true);
            setStatus("ringing");
          },
          onCallAnswered: () => {
            setIncoming(false);
            setStatus("in-call");
          },
          onCallHangup: () => {
            setIncoming(false);
            setStatus("registered");
          },
        };

        userRef.current = simpleUser;
        await simpleUser.connect();
        await simpleUser.register();
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
      const u = userRef.current;
      userRef.current = null;
      if (u) {
        u.unregister().catch(() => undefined);
        u.disconnect().catch(() => undefined);
      }
    };
  }, [enabled]);

  const call = useCallback(async (target: string) => {
    const creds = await api.sipCredentials();
    const u = userRef.current;
    if (!u) throw new Error("Softphone hazır değil.");
    setStatus("ringing");
    await u.call(`sip:${target}@${creds.domain}`);
  }, []);

  const answer = useCallback(async () => {
    await userRef.current?.answer().catch(() => undefined);
  }, []);

  const hangup = useCallback(async () => {
    await userRef.current?.hangup().catch(() => undefined);
    setIncoming(false);
    setStatus("registered");
  }, []);

  return { status, extension, error, incoming, audioRef, call, answer, hangup };
}
