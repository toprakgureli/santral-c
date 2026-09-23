// MentionToasts: the bottom-right cards that stay until dismissed when
// someone tags you with @. They live in the shell, so they show on every
// page, and survive a reload until you act on them. The whole card is
// the link to the message; the small cross dismisses it.

import { ArrowRight, AtSign, MessageSquare, X } from "lucide-react";
import { useNavigate } from "react-router-dom";
import UserAvatar from "@/components/ui/UserAvatar";
import { useTeams } from "@/teams/TeamsContext";

export default function MentionToasts() {
  const { mentions, dismissMention, toast, dismissToast } = useTeams();
  const navigate = useNavigate();
  if (mentions.length === 0 && !toast) return null;
  const visible = mentions.slice(-4);
  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-[70] flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-2">
      {toast && (
        <div
          key={toast.id}
          role="status"
          onClick={() => {
            dismissToast();
            navigate(`/teams/${toast.groupId}`);
          }}
          className="group pointer-events-auto animate-in slide-in-from-right-4 fade-in relative cursor-pointer overflow-hidden rounded-2xl border border-border bg-card shadow-2xl transition-[border-color,transform] duration-300 hover:-translate-y-0.5 hover:border-primary/60"
        >
          <span className="absolute inset-y-0 left-0 w-1 bg-primary" />
          <div className="flex items-start gap-3 py-3 pr-3 pl-4">
            <span className="relative inline-flex shrink-0">
              <UserAvatar userId={toast.sender.id} name={toast.sender.name} hasAvatar={toast.sender.hasAvatar} version={toast.sender.avatarVersion} className="size-10" fallbackClassName="bg-primary/10 text-sm text-primary" />
              <span className="absolute -right-1 -bottom-1 flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground ring-2 ring-card">
                <MessageSquare className="size-3" />
              </span>
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold leading-tight">
                {toast.sender.name} <span className="font-normal text-muted-foreground">· {toast.groupName}</span>
              </p>
              <p className="mt-1 line-clamp-2 text-sm">{toast.body}</p>
            </div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                dismissToast();
              }}
              aria-label="Kapat"
              className="-mt-1 -mr-1 rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <X className="size-4" />
            </button>
          </div>
          <span className="absolute inset-x-0 bottom-0 h-0.5 origin-left animate-[toast-drain_8s_linear_forwards] bg-primary/50" />
        </div>
      )}
      {mentions.length > visible.length && (
        <p className="pointer-events-auto self-end rounded-full bg-card/90 px-2.5 py-0.5 text-[0.7rem] text-muted-foreground shadow-sm backdrop-blur">+{mentions.length - visible.length} etiket daha</p>
      )}
      {visible.map((t) => (
        <div
          key={t.id}
          role="status"
          onClick={() => {
            dismissMention(t.id);
            navigate(t.gameId ? `/teams/${t.groupId}?game=${t.gameId}` : `/teams/${t.groupId}`);
          }}
          className="group pointer-events-auto animate-in slide-in-from-right-4 fade-in relative cursor-pointer overflow-hidden rounded-2xl border border-border bg-card shadow-2xl transition-[border-color,transform] duration-300 hover:-translate-y-0.5 hover:border-violet-500/60"
        >
          <span className="absolute inset-y-0 left-0 w-1 bg-violet-500" />
          <div className="flex items-start gap-3 py-3 pr-3 pl-4">
            <span className="relative inline-flex shrink-0">
              <UserAvatar userId={t.sender.id} name={t.sender.name} hasAvatar={t.sender.hasAvatar} version={t.sender.avatarVersion} className="size-10" fallbackClassName="bg-violet-500/15 text-sm text-violet-500" />
              <span className="absolute -right-1 -bottom-1 flex size-5 items-center justify-center rounded-full bg-violet-500 text-white ring-2 ring-card">
                <AtSign className="size-3" />
              </span>
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold leading-tight">
                {t.sender.name} <span className="font-normal text-muted-foreground">{t.gameId ? "seni oyuna çağırdı" : "seni etiketledi"}</span>
              </p>
              <p className="truncate text-xs text-muted-foreground">{t.groupName}</p>
              <p className="mt-1 line-clamp-2 text-sm">{t.body}</p>
              <p className="mt-1.5 inline-flex items-center gap-1 text-xs font-medium text-violet-500 opacity-80 transition-opacity group-hover:opacity-100">
                {t.gameId ? "Lobiye git" : "Mesaja git"} <ArrowRight className="size-3 transition-transform group-hover:translate-x-0.5" />
              </p>
            </div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                dismissMention(t.id);
              }}
              aria-label="Kapat"
              className="-mt-1 -mr-1 rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <X className="size-4" />
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
