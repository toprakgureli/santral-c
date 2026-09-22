// Presence bits shared by the chat: the online dot, the "son görülme"
// label and the delivery ticks under a person's own line.

import { Check, CheckCheck } from "lucide-react";
import type { TeamsMessage } from "@/api/types";
import { cn } from "@/lib/utils";

export function OnlineDot({ online, className }: { online: boolean; className?: string }) {
  return (
    <span
      className={cn("absolute rounded-full border-2 border-card", online ? "bg-success" : "bg-muted-foreground/40", className ?? "-right-0.5 -bottom-0.5 size-3")}
      title={online ? "Çevrimiçi" : "Çevrimdışı"}
    />
  );
}

export function seenLabel(online: boolean, lastSeen?: string): string {
  if (online) return "Çevrimiçi";
  if (!lastSeen) return "Çevrimdışı";
  const d = new Date(lastSeen);
  if (Number.isNaN(d.getTime())) return "Çevrimdışı";
  const now = new Date();
  const y = new Date(now);
  y.setDate(now.getDate() - 1);
  const time = d.toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" });
  if (d.toDateString() === now.toDateString()) return `Son görülme bugün ${time}`;
  if (d.toDateString() === y.toDateString()) return `Son görülme dün ${time}`;
  return `Son görülme ${d.toLocaleDateString("tr-TR", { day: "2-digit", month: "2-digit", year: "numeric" })} ${time}`;
}

export type Status = NonNullable<TeamsMessage["status"]>;

export function Ticks({ status, readBy, className }: { status?: Status; readBy?: string[]; className?: string }) {
  if (!status) return null;
  const title = status === "read" ? (readBy && readBy.length ? `Okudu: ${readBy.join(", ")}` : "Okundu") : status === "delivered" ? "Teslim edildi" : "Gönderildi";
  const Icon = status === "sent" ? Check : CheckCheck;
  return (
    <span title={title} className={cn("inline-flex shrink-0 items-center", status === "read" ? "text-primary" : "text-muted-foreground/70", className)}>
      <Icon className="size-3.5" />
    </span>
  );
}

// statusOf grades one of the reader's own lines from the other seats'
// pointers, mirroring the server so live receipts update it in place.
export function statusOf(messageId: number, selfId: number, seats: Record<number, { deliveredId: number; readId: number; name: string }>): { status: Status; readBy: string[] } {
  let others = 0;
  let delivered = 0;
  let read = 0;
  const readBy: string[] = [];
  for (const [id, s] of Object.entries(seats)) {
    if (Number(id) === selfId) continue;
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
