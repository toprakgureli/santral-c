// ProfilePopover: the Discord-style card that opens when a name or avatar
// is clicked in the chat. Anchored next to the click, it shows the photo
// with the presence dot, name, headline, roles, biography and when the
// person joined, with a way to message them or open the full profile.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { MessageSquare, User } from "lucide-react";
import { api, ApiError } from "@/api/client";
import type { Profile } from "@/api/types";
import { OnlineDot, presenceTone, seenLabel } from "@/components/teams/Presence";
import { Badge, Button } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";
import { useTeams } from "@/teams/TeamsContext";

export interface PopoverAnchor {
  userId: number;
  x: number;
  y: number;
  // The room the click happened in, for "Sohbette".
  room?: number;
}

const WIDTH = 300;

export default function ProfilePopover({ anchor, selfId, onClose }: { anchor: PopoverAnchor; selfId: number; onClose: () => void }) {
  const navigate = useNavigate();
  const { presenceOf } = useTeams();
  const box = useRef<HTMLDivElement>(null);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pos, setPos] = useState({ left: anchor.x, top: anchor.y });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setProfile(null);
    setError(null);
    api
      .profileOf(anchor.userId)
      .then(setProfile)
      .catch((e) => setError(e instanceof ApiError ? e.message : "Profil açılamadı."));
  }, [anchor.userId]);

  // Keep the card inside the window, flipping above the click when needed.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const h = el.offsetHeight || 320;
    const left = Math.min(Math.max(8, anchor.x + 8), window.innerWidth - WIDTH - 8);
    let top = anchor.y + 8;
    if (top + h > window.innerHeight - 8) top = Math.max(8, anchor.y - h - 8);
    setPos({ left, top });
  }, [anchor.x, anchor.y, profile]);

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const t = window.setTimeout(() => {
      window.addEventListener("mousedown", onDown);
      window.addEventListener("keydown", onKey);
    }, 0);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("mousedown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  const p = presenceOf({ id: anchor.userId }, anchor.room);
  const self = anchor.userId === selfId;

  async function message() {
    setBusy(true);
    try {
      const g = await api.teamsOpenDM(anchor.userId);
      onClose();
      navigate(`/teams/${g.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Sohbet açılamadı.");
      setBusy(false);
    }
  }

  const joined = profile?.joinedAt ? new Date(profile.joinedAt).toLocaleDateString("tr-TR", { day: "2-digit", month: "long", year: "numeric" }) : null;

  return (
    <div
      ref={box}
      role="dialog"
      style={{ left: pos.left, top: pos.top, width: WIDTH }}
      className="animate-in fade-in zoom-in-95 fixed z-[75] overflow-hidden rounded-2xl border border-border bg-popover text-popover-foreground shadow-2xl duration-150"
    >
      <div className="h-16 bg-gradient-to-r from-primary/70 via-violet-500/60 to-primary/40" />
      <div className="px-4 pb-4">
        <div className="-mt-9 mb-2 flex items-end justify-between">
          <span className="relative inline-flex rounded-full ring-4 ring-popover">
            <UserAvatar userId={anchor.userId} name={profile?.name} hasAvatar={profile?.hasAvatar ?? false} version={profile?.avatarVersion} className="size-[4.5rem] text-xl" fallbackClassName="bg-primary/15 text-primary" />
            <OnlineDot presence={p} className="right-0.5 bottom-0.5 size-4 border-[3px] border-popover" />
          </span>
          {profile && !profile.active && <Badge tone="red">Pasif</Badge>}
        </div>

        {error && <p className="py-2 text-sm text-destructive">{error}</p>}
        {!profile && !error && (
          <div className="space-y-2 py-1">
            <span className="block h-4 w-32 animate-pulse rounded bg-muted" />
            <span className="block h-3 w-48 animate-pulse rounded bg-muted" />
          </div>
        )}
        {profile && (
          <>
            <p className="text-base font-semibold leading-tight">{profile.name}{self && <span className="ml-1.5 text-xs font-normal text-muted-foreground">(sen)</span>}</p>
            <p className={cn("text-xs", presenceTone(p))}>{seenLabel(p)}</p>
            {profile.headline && <p className="mt-1.5 text-sm text-foreground/90">{profile.headline}</p>}

            <div className="mt-3 space-y-2.5 rounded-xl bg-muted/40 p-3">
              {profile.bio && (
                <div>
                  <p className="mb-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Hakkında</p>
                  <p className="line-clamp-4 text-xs leading-relaxed whitespace-pre-wrap">{profile.bio}</p>
                </div>
              )}
              {(profile.roles.length > 0 || profile.extension) && (
                <div>
                  <p className="mb-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Roller</p>
                  <div className="flex flex-wrap gap-1">
                    {profile.roles.map((r) => <Badge key={r} tone="slate">{r}</Badge>)}
                    {profile.extension && <Badge tone="blue">Dahili {profile.extension}</Badge>}
                  </div>
                </div>
              )}
              {joined && (
                <div>
                  <p className="mb-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Katılım</p>
                  <p className="text-xs">{joined}</p>
                </div>
              )}
            </div>

            <div className="mt-3 flex gap-2">
              {!self && (
                <Button onClick={() => void message()} disabled={busy} className="h-9 flex-1 text-sm">
                  <MessageSquare /> Mesaj gönder
                </Button>
              )}
              <Button
                variant="secondary"
                onClick={() => {
                  onClose();
                  navigate(self ? "/profile" : `/profile/${anchor.userId}`);
                }}
                className={cn("h-9 text-sm", self && "flex-1")}
              >
                <User /> {self ? "Profilim" : "Profili gör"}
              </Button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
