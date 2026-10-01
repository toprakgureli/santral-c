import { useEffect, useState } from "react";
import type { TeamsPerson } from "@/api/types";
import { Modal } from "@/components/ui";
import { api, ApiError } from "@/api/client";
import type { TeamsGroupDetail, TeamsMessage, TeamsReceipt } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { Ticks } from "@/components/teams/Presence";
import { renderMarkup } from "@/lib/markup";
import { previewLabel } from "@/lib/attachments";
import { cn } from "@/lib/utils";
import { clockTime, isToday, numericDateTime, shortDateTime } from "@/lib/time";
import { useTopmost } from "@/components/ui/windowStack";

// MessageInfo: when the line was sent, received and read. A direct message
// reads as a short timeline; a group lists every seat with both times.
export function MessageInfo({ message, group, onClose }: { message: TeamsMessage; group: TeamsGroupDetail; onClose: () => void }) {
  const [rows, setRows] = useState<TeamsReceipt[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    api
      .teamsReceipts(group.id, message.id)
      .then(setRows)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Bilgi alınamadı."));
  }, [group.id, message.id]);

  const stamp = (iso?: string) => {
    if (!iso) return null;
    return isToday(iso) ? clockTime(iso) : shortDateTime(iso);
  };
  const sent = numericDateTime(message.createdAt);
  const peer = group.kind === "dm" ? rows?.[0] : undefined;
  const sorted = (rows ?? []).slice().sort((a, b) => (b.readAt ? 2 : b.deliveredAt ? 1 : 0) - (a.readAt ? 2 : a.deliveredAt ? 1 : 0) || a.name.localeCompare(b.name, "tr"));

  const Step = ({ status, title, when, hint }: { status: "sent" | "delivered" | "read"; title: string; when?: string | null; hint: string }) => (
    <li className="flex items-center gap-3">
      <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-full", when ? "bg-success/15" : "bg-muted")}>
        <Ticks status={status} size="size-4" className={cn(!when && "opacity-50")} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{title}</span>
        <span className={cn("block text-xs", when ? "text-muted-foreground" : "text-muted-foreground/60 italic")}>{when ?? hint}</span>
      </span>
    </li>
  );

  return (
    <Modal open onClose={onClose} title="Mesaj bilgisi" description={`${message.sender?.name ?? ""} · ${sent}${message.editedAt ? " · düzenlendi" : ""}`} size="md">
      <div className="space-y-4">
        <div className="rounded-xl bg-muted/40 px-3 py-2 text-sm whitespace-pre-wrap break-words">{message.body ? renderMarkup(message.body) : <span className="text-muted-foreground">{previewLabel("", message.attachments)}</span>}</div>
        {error && <p className="text-xs text-destructive">{error}</p>}
        {rows === null && !error && <p className="text-xs text-muted-foreground">Yükleniyor...</p>}
        {rows && group.kind === "dm" && (
          <ul className="space-y-3">
            <Step status="sent" title="Gönderildi" when={stamp(message.createdAt)} hint="" />
            <Step status="delivered" title="Teslim edildi" when={stamp(peer?.deliveredAt)} hint="Henüz teslim edilmedi" />
            <Step status="read" title="Okundu" when={stamp(peer?.readAt)} hint="Henüz okunmadı" />
          </ul>
        )}
        {rows && group.kind !== "dm" && (
          <div className="overflow-hidden rounded-xl border border-border/60">
            <div className="grid grid-cols-[1fr_auto_auto] gap-x-4 border-b border-border/60 bg-muted/30 px-3 py-1.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
              <span>Kişi</span>
              <span className="w-16 text-right">Teslim</span>
              <span className="w-16 text-right">Okundu</span>
            </div>
            {sorted.length === 0 && <p className="px-3 py-3 text-sm text-muted-foreground">Odada başka kimse yok.</p>}
            {sorted.map((r) => (
              <div key={r.id} className="grid grid-cols-[1fr_auto_auto] items-center gap-x-4 border-b border-border/40 px-3 py-1.5 text-sm last:border-b-0">
                <span className="flex min-w-0 items-center gap-2">
                  <UserAvatar userId={r.id} name={r.name} hasAvatar={r.hasAvatar} version={r.avatarVersion} className="size-6" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
                  <span className="truncate">{r.name}</span>
                  <Ticks status={r.readAt ? "read" : r.deliveredAt ? "delivered" : "sent"} size="size-3.5" />
                </span>
                <span className={cn("w-16 text-right text-xs tabular-nums", r.deliveredAt ? "text-muted-foreground" : "text-muted-foreground/40")}>{stamp(r.deliveredAt) ?? "—"}</span>
                <span className={cn("w-16 text-right text-xs tabular-nums", r.readAt ? "text-success" : "text-muted-foreground/40")}>{stamp(r.readAt) ?? "—"}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}

// ReactionPeople: who gave one emoji, opened by right-clicking the chip.
export function ReactionPeople({ x, y, emoji, people, selfId, onPerson, onClose }: { x: number; y: number; emoji: string; people: TeamsPerson[]; selfId: number; onPerson: (p: TeamsPerson, x: number, y: number) => void; onClose: () => void }) {
  const isTop = useTopmost(true);
  useEffect(() => {
    const close = () => onClose();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && isTop() && onClose();
    const t = window.setTimeout(() => {
      window.addEventListener("mousedown", close);
      window.addEventListener("keydown", onKey);
    }, 0);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose, isTop]);
  const left = Math.min(x, window.innerWidth - 240);
  const top = Math.min(y, window.innerHeight - 40 - people.length * 40);
  return (
    <div className="animate-in fade-in zoom-in-95 fixed z-[60] w-56 overflow-hidden rounded-xl border border-border bg-popover p-1 shadow-lg duration-100" style={{ left, top }} onMouseDown={(e) => e.stopPropagation()}>
      <p className="flex items-center gap-1.5 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
        <span className="text-base leading-none">{emoji}</span> {people.length} kişi
      </p>
      {people.map((p) => (
        <button
          key={p.id}
          type="button"
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            onPerson(p, r.right, r.top);
          }}
          className="flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-accent"
        >
          <UserAvatar userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.6rem] text-primary" />
          <span className="min-w-0 flex-1 truncate">{p.name}{p.id === selfId && <span className="text-muted-foreground"> (sen)</span>}</span>
        </button>
      ))}
      {people.length === 0 && <p className="px-2 py-2 text-xs text-muted-foreground">Kimse yok</p>}
    </div>
  );
}
