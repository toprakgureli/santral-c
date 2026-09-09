import { useCallback, useEffect, useRef, useState } from "react";
import { Web } from "sip.js";
import { api } from "../api/client";

export type PhoneStatus = "idle" | "connecting" | "registered" | "ringing" | "in-call" | "error" | "disabled";

interface Phone {
  status: PhoneStatus;
  extension: string | null;
  error: string | null;
  incomingFrom: string | null;
  audioRef: React.RefObject<HTMLAudioElement>;
  call: (target: string) => Promise<void>;
  answer: () => Promise<void>;
  hangup: () => Promise<void>;
}

// domainFromWs extracts the SIP domain (host) from a ws(s):// URL.
function domainFromWs(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "localhost";
  }
}

export function useSoftphone(enabled: boolean): Phone {
  const [status, setStatus] = useState<PhoneStatus>(enabled ? "connecting" : "disabled");
  const [extension, setExtension] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [incomingFrom, setIncomingFrom] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement>(null);
  const userRef = useRef<Web.SimpleUser | null>(null);
  const domainRef = useRef<string>("localhost");

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
        const domain = domainFromWs(creds.webSocketUrl);
        domainRef.current = domain;
        setExtension(creds.extension);

        simpleUser = new Web.SimpleUser(creds.webSocketUrl, {
          aor: `sip:${creds.extension}@${domain}`,
          media: {
            constraints: { audio: true, video: false },
            remote: { audio: audioRef.current ?? undefined },
          },
          userAgentOptions: {
            authorizationUsername: creds.extension,
            authorizationPassword: creds.secret,
            displayName: creds.extension,
          },
        });
        simpleUser.delegate = {
          onCallReceived: async () => {
            setIncomingFrom("Gelen çağrı");
            setStatus("ringing");
          },
          onCallAnswered: () => setStatus("in-call"),
          onCallHangup: () => {
            setIncomingFrom(null);
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
    const u = userRef.current;
    if (!u) throw new Error("Softphone hazır değil.");
    setStatus("ringing");
    await u.call(`sip:${target}@${domainRef.current}`);
  }, []);

  const answer = useCallback(async () => {
    const u = userRef.current;
    if (!u) return;
    await u.answer();
  }, []);

  const hangup = useCallback(async () => {
    const u = userRef.current;
    if (!u) return;
    await u.hangup().catch(() => undefined);
    setStatus("registered");
    setIncomingFrom(null);
  }, []);

  return { status, extension, error, incomingFrom, audioRef, call, answer, hangup };
}
