// PeerHints: what the team knows about the number on the line, shown while a
// call rings or goes on. A customer calling for the second or third time
// today is marked, the last conversation says who had it, and colleagues
// who could not reach the number (or are calling it back right now) are
// named, so whoever picks up knows where things stand.

import { useEffect, useRef, useState } from "react";
import { History, PhoneMissed, PhoneOutgoing, Repeat } from "lucide-react";
import { clockTime, isToday, shortMonthDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { followApi, talkLabel, type PeerContext } from "./api";

const LIVE = new Set(["incoming", "calling", "ringing", "in-call", "held"]);

function when(iso: string): string {
  return isToday(iso) ? clockTime(iso) : `${shortMonthDate(iso)} ${clockTime(iso)}`;
}

// usePeerContext reads what the team knows about the current call's number,
// once per call.
function usePeerContext(): { ctx: PeerContext | null; incoming: boolean } {
  const phone = useSoftphoneContext();
  const live = LIVE.has(phone.status);
  const key = live ? `${phone.callId ?? ""}|${phone.peer ?? ""}` : "";
  const [ctx, setCtx] = useState<PeerContext | null>(null);
  // A call that rang in stays an incoming call once answered.
  const incoming = useRef(false);
  if (phone.status === "incoming") incoming.current = true;
  useEffect(() => {
    setCtx(null);
    if (!key || !phone.peer) {
      incoming.current = false;
      return;
    }
    let on = true;
    followApi.peer(phone.peer).then((c) => on && setCtx(c)).catch(() => undefined);
    return () => {
      on = false;
    };
    // the call, not its every change, decides when to read
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  return { ctx, incoming: incoming.current };
}

export default function PeerHints({ className, center }: { className?: string; center?: boolean }) {
  const { ctx, incoming } = usePeerContext();
  if (!ctx) return null;
  const lines: { icon: typeof History; tone: string; text: string }[] = [];
  if (incoming && ctx.inboundBefore > 0) {
    lines.push({ icon: Repeat, tone: "bg-warning/14 text-warning", text: `Bugün ${ctx.inboundBefore + 1}. kez arıyor` });
  }
  for (const u of ctx.unreached) {
    if (u.claim && !u.claim.mine) {
      lines.push({ icon: PhoneOutgoing, tone: "bg-primary/10 text-primary", text: `${u.claim.by.name} şu an geri arıyor` });
    }
    lines.push({
      icon: PhoneMissed,
      tone: "bg-destructive/10 text-destructive",
      text: `${u.mine ? "Sen" : u.user.name} ${u.attempts} kez ulaşamadı · son ${when(u.lastAt)}`,
    });
  }
  if (ctx.lastTalk) {
    const t = ctx.lastTalk;
    lines.push({ icon: History, tone: "bg-muted text-muted-foreground", text: `Son görüşme: ${t.mine ? "sen" : t.by.name} · ${when(t.at)} · ${talkLabel(t.seconds)}` });
  }
  if (lines.length === 0) return null;
  return (
    <div className={cn("flex flex-col gap-1", center && "items-center", className)}>
      {lines.map((l, i) => (
        <span key={i} className={cn("flex max-w-full items-center gap-1.5 rounded-lg px-2 py-1 text-[0.72rem] font-medium", l.tone)}>
          <l.icon className="size-3.5 shrink-0" />
          <span className="truncate">{l.text}</span>
        </span>
      ))}
    </div>
  );
}
