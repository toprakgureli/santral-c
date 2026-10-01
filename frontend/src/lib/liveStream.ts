import { ensureSession } from "../api/client";

export interface LiveStreamHandlers {
  // onOpen runs every time the stream (re)connects; a good moment to reload
  // what may have been missed while it was down.
  onOpen?: () => void;
  onMessage: (data: string) => void;
}

// openLiveStream keeps an event stream open. The server closes a stream when
// the access token expires, the permission is gone or it restarts; then the
// session is renewed if it can be and the stream reconnects, quickly the first
// times and then with a growing pause. A session that is over stops it (the
// sign-in screen takes over). It returns a function that closes it for good.
export function openLiveStream(url: string, handlers: LiveStreamHandlers): () => void {
  let es: EventSource | null = null;
  let closed = false;
  let retry = 0;
  let timer = 0;

  const connect = () => {
    if (closed) return;
    try {
      es = new EventSource(url, { withCredentials: true });
    } catch {
      timer = window.setTimeout(connect, 5000);
      return;
    }
    es.onopen = () => {
      retry = 0;
      handlers.onOpen?.();
    };
    es.onmessage = (ev) => handlers.onMessage(ev.data);
    es.onerror = () => {
      es?.close();
      es = null;
      if (closed) return;
      retry = Math.min(retry + 1, 6);
      const pause = 1000 * 2 ** retry;
      void ensureSession().then((renewal) => {
        if (closed || renewal === "ended") return;
        const wait = renewal === "renewed" && retry <= 2 ? 500 : pause;
        timer = window.setTimeout(connect, wait);
      });
    };
  };

  connect();
  return () => {
    closed = true;
    window.clearTimeout(timer);
    es?.close();
  };
}
