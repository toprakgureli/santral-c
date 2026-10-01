// Presence bits shared by the chat: the online dot, the "son görülme"
// label and the delivery ticks under a person's own line.
//
// "Sohbette" is room-scoped: a person shows as in the chat only inside
// the room they are looking at.
// Everywhere else they are simply online.

import { Check, CheckCheck } from "lucide-react";
import type { TeamsMessage } from "@/api/types";
import { cn } from "@/lib/utils";
import { clockTime, isToday, isYesterday, numericDate } from "@/lib/time";

export interface Presence {
  online: boolean;
  lastSeen?: string;
  // Looking at the room the card is shown in.
  here?: boolean;
}

export function OnlineDot({ presence, className }: { presence: Presence; className?: string }) {
  const chatting = presence.online && !!presence.here;
  return (
    <span
      className={cn(
        "absolute rounded-full border-2 border-card",
        chatting ? "bg-violet-500" : presence.online ? "bg-success" : "bg-muted-foreground/40",
        className ?? "-right-0.5 -bottom-0.5 size-3",
      )}
      data-tip={seenLabel(presence)}
    />
  );
}

export function seenLabel(p: Presence): string {
  if (p.online && p.here) return "Sohbette";
  if (p.online) return "Çevrimiçi";
  if (!p.lastSeen) return "Çevrimdışı";
  const d = new Date(p.lastSeen);
  if (Number.isNaN(d.getTime())) return "Çevrimdışı";
  const time = clockTime(d);
  if (isToday(d)) return `Son görülme bugün ${time}`;
  if (isYesterday(d)) return `Son görülme dün ${time}`;
  return `Son görülme ${numericDate(d)} ${time}`;
}

// presenceTone is the text colour that goes with the label.
export function presenceTone(p: Presence): string {
  if (p.online && p.here) return "text-violet-500";
  if (p.online) return "text-success";
  return "text-muted-foreground";
}

export type Status = NonNullable<TeamsMessage["status"]>;

export function Ticks({ status, readBy, className, size = "size-[1.05rem]" }: { status?: Status; readBy?: string[]; className?: string; size?: string }) {
  if (!status) return null;
  const title = status === "read" ? (readBy && readBy.length ? `Okudu: ${readBy.join(", ")}` : "Okundu") : status === "delivered" ? "Teslim edildi" : "Gönderildi";
  const Icon = status === "sent" ? Check : CheckCheck;
  return (
    <span data-tip={title} className={cn("inline-flex shrink-0 items-center align-middle", status === "read" ? "text-success" : "text-muted-foreground/70", className)}>
      <Icon className={cn(size, "stroke-[2.25]")} />
    </span>
  );
}

// statusOf grades one of the reader's own lines from the other seats'
// pointers, mirroring the server so live receipts update it in place.
export function statusOf(messageId: number, selfId: number, seats: Record<number, { deliveredId: number; readId: number; name: string; historyFrom?: number }>): { status: Status; readBy: string[] } {
  let others = 0;
  let delivered = 0;
  let read = 0;
  const readBy: string[] = [];
  for (const [id, s] of Object.entries(seats)) {
    // Someone who joined later and may not read the line is not waited on.
    if (Number(id) === selfId || (s.historyFrom ?? 0) > messageId) continue;
    others++;
    if (s.readId >= messageId) {
      read++;
      delivered++;
      readBy.push(s.name);
    } else if (s.deliveredId >= messageId) {
      delivered++;
    }
  }
  if (others === 0) return { status: "sent", readBy: [] };
  if (read === others) return { status: "read", readBy };
  if (delivered === others) return { status: "delivered", readBy };
  return { status: "sent", readBy };
}
