// SharedRatings: the ratings opened through a link, without signing in. It
// is the same board as the panel's ratings page; customers arrive cut short
// (first name and initial, the last four digits of the number), the search
// reads the comments only, and nothing can be changed or opened further. A
// cancelled or expired link shows why it does not open.

import { useCallback, useEffect, useRef, useState } from "react";
import { Link2Off, Star } from "lucide-react";
import { ApiError } from "@/api/client";
import ThemeMenu from "@/components/layout/ThemeMenu";
import { clockTime, shortMonthDate } from "@/lib/time";
import { sharedRatings } from "@/whatsapp/api";
import type { WARatingFilter } from "@/whatsapp/types";
import { RatingsBoard } from "./WhatsAppRatings";

// sharedToken reads the link's token out of the address
// /shared/ratings/<token>.
export function sharedToken(pathname: string): string {
  return decodeURIComponent(pathname.replace(/^\/shared\/ratings\//, "").replace(/\/+$/, ""));
}

// headMeta adds a tag to the page's head for as long as the page is open.
function useHeadMeta(name: string, content: string) {
  useEffect(() => {
    const m = document.createElement("meta");
    m.name = name;
    m.content = content;
    document.head.appendChild(m);
    return () => m.remove();
  }, [name, content]);
}

export default function SharedRatings({ token }: { token: string }) {
  const [info, setInfo] = useState<{ label: string; expiresAt: string; channels: { id: number; name: string }[] } | null>(null);
  const [gone, setGone] = useState<string | null>(null);
  const first = useRef(true);
  // The page is never indexed and its address is never passed on.
  useHeadMeta("robots", "noindex, nofollow");
  useHeadMeta("referrer", "no-referrer");
  useEffect(() => {
    document.title = "Puanlamalar";
  }, []);

  const load = useCallback(
    async (f: WARatingFilter) => {
      try {
        const r = await sharedRatings(token, f, first.current);
        first.current = false;
        setInfo({ label: r.label, expiresAt: r.expiresAt, channels: r.channels });
        return r;
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) setGone(e.message);
        throw e;
      }
    },
    [token],
  );

  if (gone) {
    return (
      <div className="flex min-h-svh items-center justify-center bg-background px-4">
        <div className="max-w-sm space-y-3 rounded-2xl bg-card p-6 text-center shadow-sm ring-1 ring-border/60">
          <span className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground"><Link2Off className="size-6" /></span>
          <p className="text-base font-semibold">Bu link açılmıyor</p>
          <p className="text-sm text-muted-foreground">{gone}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-svh bg-background px-4 py-6 md:px-6 lg:px-8">
      <RatingsBoard
        load={load}
        channels={info?.channels ?? []}
        shared
        head={() => (
          <div className="flex flex-wrap items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-2xl bg-warning/12 text-warning"><Star className="size-5" /></span>
            <div className="min-w-0 flex-1">
              <h1 className="text-lg font-semibold tracking-tight">Puanlamalar{info?.label ? ` · ${info.label}` : ""}</h1>
              <p className="text-xs text-muted-foreground">
                Müşterilerin WhatsApp sohbeti sonunda ve telefon görüşmesinden sonra verdiği puanlar. Sadece görüntüleme içindir; müşteri bilgileri kısaltılmıştır.
                {info && ` Link ${shortMonthDate(info.expiresAt)} ${clockTime(info.expiresAt)} tarihine kadar açık.`}
              </p>
            </div>
            <ThemeMenu />
          </div>
        )}
      />
    </div>
  );
}
