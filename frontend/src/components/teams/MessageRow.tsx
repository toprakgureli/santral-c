import { CornerUpLeft, EyeOff, Pencil, SmilePlus, Trash2 } from "lucide-react";
import AttachmentGrid, { mediaOf } from "@/components/teams/AttachmentGrid";
import type { TeamsAttachment, TeamsMessage, TeamsPerson } from "@/api/types";
import { EVERYONE } from "@/components/teams/Composer";
import UserAvatar from "@/components/ui/UserAvatar";
import { statusOf, Ticks } from "@/components/teams/Presence";
import { renderMarkup, stripMarkup } from "@/lib/markup";
import GameCard from "@/games/GameCard";
import { cn } from "@/lib/utils";
import { clockTime, numericDateTime } from "@/lib/time";

// MessageRow is one line of a room: the day separator and the "Yeni
// mesajlar" mark when they fall before it, the sender, the quoted reply,
// the text or attachments, the reactions and the hover actions.

// historyFrom: the first line the seat may read; earlier ones do not wait
// for that seat's receipt.
export type Seats = Record<number, { deliveredId: number; readId: number; name: string; historyFrom?: number }>;

const QUICK = ["👍", "❤️", "😂", "😮", "🔥", "✅"];
const ALL = ["👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "✅", "👏", "🎉", "👀", "💯"];

type Props = {
  m: TeamsMessage;
  head: boolean;
  day: string | null;
  unread: boolean;
  selfId: number;
  seats: Seats;
  flash: boolean;
  editing: boolean;
  picking: boolean;
  canPost: boolean;
  onMenu: (e: React.MouseEvent, m: TeamsMessage) => void;
  onProfile: (e: React.MouseEvent, userId: number) => void;
  onJump: (id: number) => void;
  onReact: (m: TeamsMessage, emoji: string) => void;
  onPicker: () => void;
  onReply: (m: TeamsMessage) => void;
  onEdit: (m: TeamsMessage) => void;
  onDelete: (m: TeamsMessage) => void;
  onWho: (w: { x: number; y: number; emoji: string; people: TeamsPerson[] }) => void;
  onGallery: (items: TeamsAttachment[], index: number) => void;
  onOpenGame: (id: number) => void;
};

export default function MessageRow({ m, head, day, unread, selfId, seats, flash, editing, picking, canPost, onMenu, onProfile, onJump, onReact, onPicker, onReply, onEdit, onDelete, onWho, onGallery, onOpenGame }: Props) {
  const mentionsMe = (m.mentions?.includes(selfId) || m.mentionsAll) && !m.mine;
  const labels = [...(m.mentions ?? []).map((id) => seats[id]?.name).filter((n): n is string => !!n).map((n) => `@${n}`), ...(m.mentionsAll ? [`@${EVERYONE}`] : [])];
  return (
    <div data-mid={m.id}>
      {day && (
        <div className="my-4 flex items-center justify-center">
          <span className="rounded-full border border-border/60 bg-card/90 px-3 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground shadow-sm backdrop-blur">{day}</span>
        </div>
      )}
      {unread && (
        <div className="my-3 flex items-center gap-3">
          <span className="h-px flex-1 bg-destructive/50" />
          <span className="rounded-full border border-destructive/40 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-destructive">Yeni mesajlar</span>
          <span className="h-px flex-1 bg-destructive/50" />
        </div>
      )}
      {m.kind === "system" ? (
        <p className="my-2 text-center"><span className="rounded-full bg-muted/70 px-3 py-1 text-xs text-muted-foreground">{m.body}</span></p>
      ) : (
        <div
          onContextMenu={(e) => onMenu(e, m)}
          className={cn(
            "group relative flex gap-3 rounded-2xl px-2.5 py-1 transition-colors duration-700",
            head ? "mt-2.5" : "mt-0",
            flash ? "bg-primary/15" : "hover:bg-card/80",
            editing && "bg-warning/10",
            mentionsMe && "bg-violet-500/[0.07] before:absolute before:top-1 before:bottom-1 before:left-0 before:w-[3px] before:rounded-full before:bg-violet-500 hover:bg-violet-500/10",
          )}
        >
          <div className={cn("w-9 shrink-0", m.replyTo && "mt-6")}>
            {head && m.sender && (
              <button type="button" onClick={(e) => onProfile(e, m.sender!.id)} className="rounded-full transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none" aria-label="Profili aç" data-tip="Profili aç">
                <UserAvatar userId={m.sender.id} name={m.sender.name} hasAvatar={m.sender.hasAvatar} version={m.sender.avatarVersion} className="size-9 shadow-sm ring-2 ring-card" fallbackClassName="bg-primary/10 text-xs text-primary" />
              </button>
            )}
            {!head && <span className="hidden pt-1 text-[0.65rem] tabular-nums text-muted-foreground group-hover:block">{clockTime(m.createdAt)}</span>}
          </div>
          <div className="min-w-0 flex-1">
            {m.replyTo?.hidden ? (
              <span
                data-tip="Bu mesaj sen gruba katılmadan önce yazılmış, göremezsin."
                className="relative mb-1 flex h-5 w-fit max-w-[75%] items-center gap-1.5 text-xs text-muted-foreground"
              >
                <span className="pointer-events-none absolute top-1/2 -left-[30px] h-[14px] w-[24px] rounded-tl-lg border-t-2 border-l-2 border-border" />
                <EyeOff className="size-3.5 shrink-0" aria-hidden />
                <span className="truncate italic">Katılmadan önceki bir mesaja yanıt</span>
              </span>
            ) : m.replyTo && (
              <button
                type="button"
                onClick={() => onJump(m.replyTo!.id)}
                data-tip="Yanıtlanan mesaja git"
                className="group/reply relative mb-1 flex h-5 w-fit max-w-[75%] items-center gap-1.5 text-left text-xs text-muted-foreground transition-colors hover:text-foreground"
              >
                <span className="pointer-events-none absolute top-1/2 -left-[30px] h-[14px] w-[24px] rounded-tl-lg border-t-2 border-l-2 border-border" />
                <UserAvatar name={m.replyTo.sender} hasAvatar={false} className="size-4" fallbackClassName="bg-primary/10 text-[0.5rem] text-primary" />
                <span className="shrink-0 font-medium text-foreground/80">{m.replyTo.sender}</span>
                <span className={cn("truncate", m.replyTo.deleted && "italic")}>{m.replyTo.deleted ? "Bu mesaj silindi." : stripMarkup(m.replyTo.body) || "Ek"}</span>
              </button>
            )}
            {head && m.sender && (
              <div className="flex items-center gap-2">
                <button type="button" onClick={(e) => onProfile(e, m.sender!.id)} className={cn("text-sm font-semibold leading-tight hover:underline", m.mine && "text-primary")}>{m.sender.name}</button>
                <span className="text-[0.7rem] text-muted-foreground">{clockTime(m.createdAt)}</span>
                {m.mine && !m.deleted && (() => {
                  const st = statusOf(m.id, selfId, seats);
                  return <Ticks status={st.status} readBy={st.readBy} size="size-3.5" className="-ml-0.5" />;
                })()}
              </div>
            )}
            {m.deleted ? (
              <p className="text-sm italic text-muted-foreground">Bu mesaj silindi.</p>
            ) : m.kind === "game" && m.gameId ? (
              <GameCard gameId={m.gameId} onOpen={() => onOpenGame(m.gameId!)} />
            ) : (
              <div className="whitespace-pre-wrap break-words text-sm leading-relaxed">
                {m.attachments?.length > 0 && (
                  <AttachmentGrid attachments={m.attachments} className={m.body ? "mt-1 mb-1.5" : "mt-1"} onOpen={(i) => onGallery(mediaOf(m.attachments), i)} />
                )}
                {m.body && renderMarkup(m.body, { mentions: labels })}
                {m.editedAt && <span className="ml-1.5 text-[0.65rem] text-muted-foreground" data-tip={`Düzenlendi: ${numericDateTime(m.editedAt)}`}>(düzenlendi)</span>}
              </div>
            )}
            {m.reactions.length > 0 && (
              <div className="mt-1 flex flex-wrap gap-1">
                {m.reactions.map((r) => (
                  <button
                    key={r.emoji}
                    type="button"
                    onClick={() => onReact(m, r.emoji)}
                    onContextMenu={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      onWho({ x: e.clientX, y: e.clientY, emoji: r.emoji, people: r.people ?? [] });
                    }}
                    data-tip={`${r.names.join(", ")}
Sağ tık: kimler verdi`}
                    className={cn("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors", r.mine ? "border-primary/50 bg-primary/10" : "border-border/70 bg-muted/40 hover:bg-accent")}
                  >
                    <span>{r.emoji}</span>
                    <span className="tabular-nums">{r.count}</span>
                  </button>
                ))}
                <button type="button" onClick={() => onPicker()} aria-label="Başka tepki" data-tip="Başka tepki" className="inline-flex items-center rounded-full border border-dashed border-border/70 px-1.5 text-muted-foreground hover:bg-accent"><SmilePlus className="size-3.5" /></button>
              </div>
            )}
          </div>
          {!m.deleted && (
            <div className={cn("absolute -top-4 right-3 items-center gap-0.5 rounded-full border border-border/70 bg-card px-1 py-0.5 shadow-md group-hover:flex", picking ? "flex" : "hidden")}>
              {QUICK.map((e) => (
                <button key={e} type="button" onClick={() => onReact(m, e)} data-tip="Tepki ver" className={cn("rounded-md px-1 py-0.5 text-base leading-none hover:bg-accent", m.reactions.some((r) => r.emoji === e && r.mine) && "bg-primary/15")}>{e}</button>
              ))}
              <button type="button" onClick={() => onPicker()} aria-label="Daha fazla tepki" data-tip="Daha fazla tepki" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><SmilePlus className="size-4" /></button>
              <span className="mx-0.5 h-4 w-px bg-border" />
              {canPost && <button type="button" onClick={() => onReply(m)} aria-label="Yanıtla" data-tip="Yanıtla" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><CornerUpLeft className="size-4" /></button>}
              {m.mine && canPost && <button type="button" onClick={() => onEdit(m)} aria-label="Düzenle" data-tip="Düzenle" className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"><Pencil className="size-4" /></button>}
              {m.canDelete && <button type="button" onClick={() => onDelete(m)} aria-label="Sil" data-tip="Sil" className="rounded-md p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"><Trash2 className="size-4" /></button>}
            </div>
          )}
          {picking && (
            <div className="absolute top-5 right-3 z-10 grid grid-cols-6 gap-0.5 rounded-xl border border-border bg-popover p-1 shadow-lg">
              {ALL.map((e) => (
                <button key={e} type="button" onClick={() => onReact(m, e)} className="rounded-lg px-1.5 py-1 text-lg hover:bg-accent">{e}</button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
