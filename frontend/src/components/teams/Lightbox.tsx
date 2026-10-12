// Lightbox: wraps the shared Lightbox for Teams attachments.

import type { TeamsAttachment } from "@/api/types";
import { attachmentUrl } from "@/lib/attachments";
import SharedLightbox, { type LightboxItem } from "@/components/ui/Lightbox";

export default function Lightbox({
  items,
  index,
  onIndex,
  onClose,
}: {
  items: TeamsAttachment[];
  index: number;
  onIndex: (i: number) => void;
  onClose: () => void;
}) {
  const lightboxItems: LightboxItem[] = items.map((a) => ({
    id: a.id,
    url: attachmentUrl(a.id),
    downloadUrl: attachmentUrl(a.id, true),
    name: a.name,
    size: a.size,
    width: a.width,
    height: a.height,
    kind: a.kind,
    hasThumb: a.hasThumb,
    thumbUrl: a.hasThumb ? `/api/v1/teams/attachments/${a.id}/thumb` : undefined,
  }));

  return (
    <SharedLightbox
      items={lightboxItems}
      index={index}
      onIndex={onIndex}
      onClose={onClose}
    />
  );
}
