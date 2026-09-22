// GroupAvatar: a room's photo, or its initial letters; a direct message
// shows the other person's photo instead.

import { useEffect, useState } from "react";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn, initials } from "@/lib/utils";
import type { TeamsGroup } from "@/api/types";

export default function GroupAvatar({ group, className, src }: { group: Pick<TeamsGroup, "id" | "kind" | "name" | "hasAvatar" | "avatarVersion" | "peer">; className?: string; src?: string | null }) {
  if (group.kind === "dm" && group.peer) {
    return <UserAvatar userId={group.peer.id} name={group.peer.name} hasAvatar={group.peer.hasAvatar} version={group.peer.avatarVersion} className={className} fallbackClassName="bg-primary/10 text-primary" />;
  }
  return <Photo id={group.id} name={group.name} hasAvatar={group.hasAvatar} version={group.avatarVersion} className={className} src={src} />;
}

function Photo({ id, name, hasAvatar, version, className, src }: { id: number; name: string; hasAvatar: boolean; version?: number; className?: string; src?: string | null }) {
  const url = src || (hasAvatar ? `/api/v1/teams/groups/${id}/avatar${version ? `?v=${version}` : ""}` : undefined);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  return (
    <span className={cn("relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-xl", className)}>
      {url && !failed ? (
        <img src={url} alt={name} onError={() => setFailed(true)} className="size-full object-cover" draggable={false} />
      ) : (
        <span className="flex size-full items-center justify-center bg-violet-500/15 font-semibold text-violet-500">{initials(name)}</span>
      )}
    </span>
  );
}
