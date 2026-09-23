// MentionToasts: the bottom-right cards that stay until dismissed when
// someone tags you with @. They live in the shell, so they show on every
// page, and survive a reload until you act on them.

import { AtSign, X } from "lucide-react";
import { useNavigate } from "react-router-dom";
import UserAvatar from "@/components/ui/UserAvatar";
import { useTeams } from "@/teams/TeamsContext";

export default function MentionToasts() {
  const { mentions, dismissMention } = useTeams();
  const navigate = useNavigate();
  if (mentions.length === 0) return null;
  const visible = mentions.slice(-4);
  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-[70] flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-2">
      {mentions.length > visible.length && (
        <p className="pointer-events-auto self-end rounded-full bg-card/90 px-2.5 py-0.5 text-[0.7rem] text-muted-foreground shadow-sm backdrop-blur">+{mentions.length - visible.length} etiket daha</p>
      )}
      {visible.map((t) => (
        <div
          key={t.id}
          role="status"
          className="pointer-events-auto animate-in slide-in-from-right-4 fade-in overflow-hidden rounded-2xl border border-violet-500/40 bg-card shadow-2xl duration-300"
        >
          <div className="flex items-start gap-3 p-3.5">
            <span className="relative shrink-0">
              <UserAvatar userId={t.sender.id} name={t.sender.name} hasAvatar={t.sender.hasAvatar} version={t.sender.avatarVersion} className="size-10" fallbackClassName="bg-violet-500/15 text-sm text-violet-500" />
              <span className="absolute -right-1 -bottom-1 flex size-5 items-center justify-center rounded-full bg-violet-500 text-white ring-2 ring-card">
                <AtSign className="size-3" />
              </span>
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold leading-tight">
                {t.sender.name} <span className="font-normal text-muted-foreground">seni etiketledi</span>
              </p>
              <p className="truncate text-xs text-muted-foreground">{t.groupName}</p>
              <p className="mt-1 line-clamp-2 text-sm">{t.body}</p>
            </div>
            <button type="button" onClick={() => dismissMention(t.id)} aria-label="Kapat" className="-mt-1 -mr-1 rounded-lg p-1 text-muted-foreground hover:bg-accent hover:text-foreground">
              <X className="size-4" />
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              dismissMention(t.id);
              navigate(`/teams/${t.groupId}`);
            }}
            className="block w-full border-t border-border/60 bg-violet-500/10 px-3.5 py-2 text-left text-xs font-medium text-violet-500 hover:bg-violet-500/15"
          >
            Mesaja git
          </button>
        </div>
      ))}
    </div>
  );
}
