// UserAvatar shows a person's photo, or their initials until a photo exists.
// A missing photo answers 204, which the <img> reports as an error, and
// the fallback takes over; nothing ever renders as a broken image.

import { useEffect, useState } from "react";
import { avatarUrl } from "@/lib/avatar";
import { cn, initials } from "@/lib/utils";

export default function UserAvatar({
  userId,
  name,
  hasAvatar = true,
  version,
  src,
  className,
  fallbackClassName,
  children,
}: {
  userId?: number | null;
  name?: string | null;
  hasAvatar?: boolean;
  version?: number | null;
  // A data URI preview overrides the served photo (the profile dialog).
  src?: string | null;
  className?: string;
  fallbackClassName?: string;
  children?: React.ReactNode;
}) {
  const url = src || (hasAvatar ? avatarUrl(userId, version) : undefined);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  const showImage = !!url && !failed;

  return (
    <span className={cn("relative inline-flex shrink-0 items-center justify-center overflow-visible rounded-full", className)}>
      {showImage ? (
        <img src={url} alt={name ?? ""} onError={() => setFailed(true)} className="size-full rounded-full object-cover" draggable={false} />
      ) : (
        <span className={cn("flex size-full items-center justify-center rounded-full font-semibold", fallbackClassName)}>{initials(name)}</span>
      )}
      {children}
    </span>
  );
}
