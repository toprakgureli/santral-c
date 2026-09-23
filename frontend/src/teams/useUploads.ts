// useUploads: the composer's file tray for one room. Files come from the
// paperclip, a drop or a paste; each starts uploading right away and
// carries its own state, so one failing does not stop the others. Videos
// pass through the trimmer first; a picture can be marked or masked, in
// which case the old upload is dropped and the edited file goes up.

import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "@/api/client";
import { CANCEL_MESSAGE, checkFile, kindOf, newKey, uploadFile, type PendingAttachment } from "@/lib/attachments";
import { canEditVideo } from "@/lib/video";

export interface UploadsApi {
  items: PendingAttachment[];
  addFiles: (files: File[]) => void;
  remove: (key: string) => void;
  retry: (key: string) => void;
  clear: () => void;
  busy: boolean;
  readyIds: () => number[];
  error: string | null;
  clearError: () => void;
  trimming: File | null;
  onTrimCancel: () => void;
  onTrimReady: (file: File) => void;
  editing: PendingAttachment | null;
  setEditing: (item: PendingAttachment | null) => void;
  replace: (key: string, file: File) => void;
}

export function useUploads(groupId: number): UploadsApi {
  const [items, setItems] = useState<PendingAttachment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [trimming, setTrimming] = useState<File | null>(null);
  const [editing, setEditing] = useState<PendingAttachment | null>(null);
  const queue = useRef<File[]>([]);
  const aborters = useRef<Record<string, AbortController>>({});
  const live = useRef<PendingAttachment[]>([]);
  live.current = items;

  const patch = useCallback((key: string, p: Partial<PendingAttachment>) => {
    setItems((cur) => cur.map((i) => (i.key === key ? { ...i, ...p } : i)));
  }, []);

  const drop = useCallback((key: string) => {
    setItems((cur) => cur.filter((i) => i.key !== key));
  }, []);

  const start = useCallback(
    (item: PendingAttachment) => {
      const ctl = new AbortController();
      aborters.current[item.key] = ctl;
      patch(item.key, { status: "yükleniyor", progress: 0, error: undefined });
      uploadFile(
        groupId,
        item.file,
        item.kind,
        (r) => patch(item.key, { progress: Math.round(r * 100) }),
        (id) => patch(item.key, { attachmentId: id }),
        ctl.signal,
      )
        .then((view) => patch(item.key, { status: "hazır", progress: 100, attachmentId: view.id, view }))
        .catch((e: unknown) => {
          const msg = e instanceof Error ? e.message : "Dosya yüklenemedi.";
          if (msg === CANCEL_MESSAGE) {
            drop(item.key);
          } else {
            patch(item.key, { status: "hata", error: msg });
            setError(msg);
          }
        })
        .finally(() => {
          delete aborters.current[item.key];
        });
    },
    [groupId, patch, drop],
  );

  const add = useCallback(
    (file: File) => {
      const why = checkFile(file, live.current.length);
      if (why) {
        setError(why);
        return;
      }
      const item: PendingAttachment = {
        key: newKey(),
        file,
        name: file.name,
        mime: file.type || "application/octet-stream",
        size: file.size,
        kind: kindOf(file.type),
        previewUrl: URL.createObjectURL(file),
        status: "hazırlanıyor",
        progress: 0,
      };
      live.current = [...live.current, item];
      setItems(live.current);
      start(item);
    },
    [start],
  );

  const next = useCallback(() => {
    setTrimming(queue.current.shift() ?? null);
  }, []);

  const addFiles = useCallback(
    (files: File[]) => {
      const toTrim: File[] = [];
      for (const f of files) {
        if (f.type.startsWith("video/") && canEditVideo()) toTrim.push(f);
        else add(f);
      }
      if (toTrim.length) {
        queue.current.push(...toTrim);
        if (!trimming) next();
      }
    },
    [add, next, trimming],
  );

  const remove = useCallback(
    (key: string) => {
      const item = live.current.find((i) => i.key === key);
      aborters.current[key]?.abort();
      if (item?.attachmentId) void api.teamsCancelUpload(item.attachmentId).catch(() => undefined);
      if (item) URL.revokeObjectURL(item.previewUrl);
      drop(key);
    },
    [drop],
  );

  const retry = useCallback(
    (key: string) => {
      const item = live.current.find((i) => i.key === key);
      if (item) start(item);
    },
    [start],
  );

  const replace = useCallback(
    (key: string, file: File) => {
      remove(key);
      add(file);
    },
    [remove, add],
  );

  const clear = useCallback(() => {
    for (const i of live.current) URL.revokeObjectURL(i.previewUrl);
    live.current = [];
    setItems([]);
  }, []);

  // Leaving the room forgets the tray; half-done uploads are cancelled.
  useEffect(() => {
    return () => {
      for (const ctl of Object.values(aborters.current)) ctl.abort();
      for (const i of live.current) {
        URL.revokeObjectURL(i.previewUrl);
        if (i.attachmentId && i.status !== "hazır") void api.teamsCancelUpload(i.attachmentId).catch(() => undefined);
      }
    };
  }, [groupId]);

  return {
    items,
    addFiles,
    remove,
    retry,
    clear,
    busy: items.some((i) => i.status === "yükleniyor" || i.status === "hazırlanıyor"),
    readyIds: () => live.current.filter((i) => i.status === "hazır" && i.attachmentId).map((i) => i.attachmentId as number),
    error,
    clearError: () => setError(null),
    trimming,
    onTrimCancel: next,
    onTrimReady: (file: File) => {
      setTrimming(null);
      add(file);
      next();
    },
    editing,
    setEditing,
    replace,
  };
}
