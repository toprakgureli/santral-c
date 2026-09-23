// Composer: the message box. Enter sends, Shift+Enter breaks the line.
// Typing @ opens a member list; picking one writes "@Ad Soyad" and tags
// them, "@herkes" tags the whole room. Selected text can be styled from
// the right-click menu or with shortcuts, and a live preview shows the
// result. The same box edits a line when `editing` is set.

import { useEffect, useMemo, useRef, useState } from "react";
import { AtSign, CornerUpLeft, Paperclip, Pencil, SendHorizontal, Type, Users, X } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { TeamsGroupDetail, TeamsMessage } from "@/api/types";
import { ContextMenu, type MenuItem } from "@/components/ContextMenu";
import ImageEditor from "@/components/teams/ImageEditor";
import MediaTile from "@/components/teams/MediaTile";
import VideoTrimmer from "@/components/teams/VideoTrimmer";
import { VIDEO_MAX } from "@/lib/attachments";
import type { UploadsApi } from "@/teams/useUploads";
import UserAvatar from "@/components/ui/UserAvatar";
import { applyStyle, MARKUP_HINT, renderMarkup, STYLES, type Style } from "@/lib/markup";
import { cn } from "@/lib/utils";

export const EVERYONE = "herkes";

export interface Outgoing {
  body: string;
  mentionIds: number[];
  mentionsAll: boolean;
  attachmentIds: number[];
}

type Suggestion = { id: number; name: string; hasAvatar: boolean; avatarVersion?: number };

const TYPING_EVERY_MS = 2500;

export default function Composer({
  group,
  selfId,
  reply,
  editing,
  onCancelReply,
  onCancelEdit,
  onSend,
  onEdit,
  onEditLast,
  uploads,
}: {
  group: TeamsGroupDetail;
  selfId: number;
  reply: TeamsMessage | null;
  editing: TeamsMessage | null;
  onCancelReply: () => void;
  onCancelEdit: () => void;
  onSend: (m: Outgoing) => Promise<void>;
  onEdit: (id: number, m: Outgoing) => Promise<void>;
  onEditLast: () => void;
  uploads: UploadsApi;
}) {
  const picker = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Names inserted through the picker, so a plain "@" in prose never tags.
  const [tagged, setTagged] = useState<Record<number, string>>({});
  const [query, setQuery] = useState<{ start: number; text: string } | null>(null);
  const [cursor, setCursor] = useState(0);
  const [menu, setMenu] = useState<{ x: number; y: number; items: MenuItem[] } | null>(null);
  const [help, setHelp] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const lastTyping = useRef(0);

  useEffect(() => {
    setText("");
    setTagged({});
    setQuery(null);
    setError(null);
  }, [group.id]);

  useEffect(() => {
    if (reply) area.current?.focus();
  }, [reply]);

  // Entering edit mode loads the line; leaving it clears the box.
  useEffect(() => {
    if (!editing) return;
    setText(editing.body);
    const names: Record<number, string> = {};
    for (const id of editing.mentions ?? []) {
      const m = group.members.find((x) => x.id === id);
      if (m) names[id] = m.name;
    }
    setTagged(names);
    requestAnimationFrame(() => {
      const el = area.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(el.value.length, el.value.length);
      grow(el);
    });
  }, [editing, group.members]);

  function grow(el: HTMLTextAreaElement) {
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }

  const suggestions = useMemo<Suggestion[]>(() => {
    if (!query) return [];
    const q = query.text.toLocaleLowerCase("tr");
    const people: Suggestion[] = group.members
      .filter((m) => m.id !== selfId && (!q || m.name.toLocaleLowerCase("tr").includes(q)))
      .slice(0, 6)
      .map((m) => ({ id: m.id, name: m.name, hasAvatar: m.hasAvatar, avatarVersion: m.avatarVersion }));
    if (group.kind === "group" && (!q || EVERYONE.startsWith(q))) people.push({ id: 0, name: EVERYONE, hasAvatar: false });
    return people;
  }, [query, group.members, group.kind, selfId]);

  useEffect(() => setCursor(0), [suggestions.length, query?.text]);

  // Find an "@word" right before the caret.
  function detect(value: string, caret: number) {
    const head = value.slice(0, caret);
    const at = head.lastIndexOf("@");
    if (at === -1) return null;
    if (at > 0 && !/\s/.test(head[at - 1])) return null;
    const word = head.slice(at + 1);
    if (/\s/.test(word) || word.length > 40) return null;
    return { start: at, text: word };
  }

  function onChange(value: string) {
    setText(value);
    const caret = area.current?.selectionStart ?? value.length;
    setQuery(detect(value, caret));
    // "yazıyor..." for the others, at most once every few seconds.
    if (value.trim() && !editing && Date.now() - lastTyping.current > TYPING_EVERY_MS) {
      lastTyping.current = Date.now();
      void api.teamsTyping(group.id).catch(() => undefined);
    }
  }

  function pick(s: Suggestion) {
    if (!query) return;
    const label = `@${s.name}`;
    const caret = area.current?.selectionStart ?? text.length;
    const next = text.slice(0, query.start) + label + " " + text.slice(caret);
    setText(next);
    setQuery(null);
    if (s.id !== 0) setTagged((cur) => ({ ...cur, [s.id]: s.name }));
    requestAnimationFrame(() => {
      const el = area.current;
      if (!el) return;
      el.focus();
      const pos = query.start + label.length + 1;
      el.setSelectionRange(pos, pos);
      grow(el);
    });
  }

  function style(kind: Style) {
    const el = area.current;
    if (!el) return;
    const r = applyStyle(text, el.selectionStart, el.selectionEnd, kind);
    setText(r.text);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(r.start, r.end);
      grow(el);
    });
  }

  function outgoing(body: string): Outgoing {
    const mentionIds = Object.entries(tagged)
      .filter(([, name]) => body.includes(`@${name}`))
      .map(([id]) => Number(id));
    const mentionsAll = group.kind === "group" && new RegExp(`(^|\\s)@${EVERYONE}(?=$|[\\s.,!?:;])`, "u").test(body);
    return { body, mentionIds, mentionsAll, attachmentIds: editing ? [] : uploads.readyIds() };
  }

  function reset() {
    setText("");
    setTagged({});
    setQuery(null);
    requestAnimationFrame(() => {
      const el = area.current;
      if (!el) return;
      el.style.height = "auto";
      el.focus();
    });
  }

  async function submit() {
    const body = text.trim();
    const files = editing ? 0 : uploads.readyIds().length;
    if ((!body && files === 0) || busy) return;
    if (!editing && uploads.busy) {
      setError("Dosyalar yüklenmeyi bitirsin.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      if (editing) {
        if (body === editing.body.trim()) {
          onCancelEdit();
        } else {
          await onEdit(editing.id, outgoing(body));
        }
      } else {
        await onSend(outgoing(body));
      }
      reset();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
    } finally {
      setBusy(false);
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (query && suggestions.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setCursor((c) => (c + 1) % suggestions.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setCursor((c) => (c - 1 + suggestions.length) % suggestions.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        pick(suggestions[cursor]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setQuery(null);
        return;
      }
    }
    if ((e.ctrlKey || e.metaKey) && !e.altKey) {
      const k = e.key.toLowerCase();
      const map: Record<string, Style | undefined> = {
        b: "bold",
        i: "italic",
        u: "underline",
        e: e.shiftKey ? "block" : "code",
        x: e.shiftKey ? "strike" : undefined,
      };
      const s = map[k];
      if (s) {
        e.preventDefault();
        style(s);
        return;
      }
    }
    if (e.key === "Escape") {
      if (editing) {
        e.preventDefault();
        onCancelEdit();
        reset();
        return;
      }
      if (reply) {
        e.preventDefault();
        onCancelReply();
        return;
      }
    }
    if (e.key === "ArrowUp" && !text && !editing) {
      e.preventDefault();
      onEditLast();
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void submit();
    }
  }

  // Right-click on a selection: the style menu. Without one, the browser's.
  function onContextMenu(e: React.MouseEvent<HTMLTextAreaElement>) {
    const el = e.currentTarget;
    if (el.selectionStart === el.selectionEnd) return;
    e.preventDefault();
    setMenu({
      x: e.clientX,
      y: e.clientY,
      items: STYLES.map((s) => ({ label: `${s.label}  ${s.shortcut ? `(${s.shortcut})` : ""}`.trim(), onClick: () => style(s.key) })),
    });
  }

  const showPreview = text.length > 0 && MARKUP_HINT.test(text);
  const mentionLabels = [...Object.values(tagged).map((n) => `@${n}`), `@${EVERYONE}`];

  if (!group.canPost) {
    return (
      <div className="border-t border-border/60 px-5 py-3 text-center text-xs text-muted-foreground">
        {group.postPolicy === "admins" ? "Bu bir duyuru grubu, yalnızca yöneticiler yazabilir." : "Bu grupta yazma yetkin kapatılmış."}
      </div>
    );
  }

  return (
    <div className="relative border-t border-border/60 px-4 py-3">
      {editing ? (
        <div className="mb-2 flex items-center gap-2 rounded-lg bg-warning/10 px-3 py-1.5 text-xs">
          <Pencil className="size-3.5 shrink-0 text-warning" />
          <span className="min-w-0 flex-1 truncate">Mesaj düzenleniyor <span className="text-muted-foreground">· Esc ile vazgeç, Enter ile kaydet</span></span>
          <button type="button" onClick={() => { onCancelEdit(); reset(); }} aria-label="Düzenlemeyi iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      ) : reply ? (
        <div className="mb-2 flex items-center gap-2.5 rounded-lg border-l-2 border-primary bg-muted/40 py-1.5 pr-2 pl-3 text-xs">
          <CornerUpLeft className="size-3.5 shrink-0 text-primary" />
          {reply.sender && <UserAvatar userId={reply.sender.id} name={reply.sender.name} hasAvatar={reply.sender.hasAvatar} version={reply.sender.avatarVersion} className="size-5" fallbackClassName="bg-primary/10 text-[0.55rem] text-primary" />}
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium">{reply.sender?.name}</span>
            <span className="text-muted-foreground"> kişisine yanıt: {reply.body.slice(0, 100)}</span>
          </span>
          <button type="button" onClick={onCancelReply} aria-label="Yanıtı iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      ) : null}

      {uploads.items.length > 0 && (
        <div className="mb-2 flex flex-wrap items-start gap-2 rounded-xl border border-border/60 bg-muted/20 p-2">
          {uploads.items.map((item) => (
            <MediaTile
              key={item.key}
              item={item}
              onRemove={() => uploads.remove(item.key)}
              onRetry={item.status === "hata" ? () => uploads.retry(item.key) : undefined}
              onEdit={item.kind === "image" ? () => uploads.setEditing(item) : undefined}
            />
          ))}
        </div>
      )}

      {showPreview && (
        <div className="mb-2 rounded-lg border border-dashed border-border/70 bg-muted/20 px-3 py-2 text-sm">
          <p className="mb-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Önizleme</p>
          <div className="whitespace-pre-wrap break-words leading-relaxed">{renderMarkup(text, { mentions: mentionLabels })}</div>
        </div>
      )}

      {query && suggestions.length > 0 && (
        <div className="absolute bottom-full left-4 z-20 mb-1 w-72 overflow-hidden rounded-xl border border-border bg-popover p-1 shadow-lg">
          <p className="px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Etiketle</p>
          {suggestions.map((s, i) => (
            <button
              key={s.id}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(s)}
              onMouseEnter={() => setCursor(i)}
              className={cn("flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left text-sm", i === cursor ? "bg-accent" : "")}
            >
              {s.id === 0 ? (
                <span className="flex size-7 items-center justify-center rounded-full bg-violet-500/15 text-violet-500"><Users className="size-3.5" /></span>
              ) : (
                <UserAvatar userId={s.id} name={s.name} hasAvatar={s.hasAvatar} version={s.avatarVersion} className="size-7" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
              )}
              <span className="min-w-0 flex-1 truncate">
                {s.id === 0 ? <span className="font-medium">@herkes</span> : s.name}
                {s.id === 0 && <span className="ml-1.5 text-xs text-muted-foreground">gruptaki herkesi etiketler</span>}
              </span>
            </button>
          ))}
        </div>
      )}

      {help && (
        <div className="absolute right-4 bottom-full z-20 mb-1 w-72 rounded-xl border border-border bg-popover p-3 shadow-lg">
          <p className="mb-2 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Biçimlendirme</p>
          <ul className="space-y-1 text-xs">
            {STYLES.map((s) => (
              <li key={s.key} className="flex items-center justify-between gap-3">
                <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => { style(s.key); setHelp(false); }} className="font-medium hover:underline">{s.label}</button>
                <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.7rem] text-muted-foreground">{s.sample.replace(/\n/g, "⏎")}</code>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-[0.65rem] text-muted-foreground">Metni seçip sağ tıklayarak da stil seçebilirsin.</p>
        </div>
      )}

      <div className={cn("flex items-end gap-2 rounded-2xl border bg-muted/30 px-2 py-1.5 focus-within:ring-4", editing ? "border-warning/60 focus-within:ring-warning/15" : "border-border/70 focus-within:border-ring/60 focus-within:ring-ring/15")}>
        <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => picker.current?.click()} disabled={!!editing} title="Dosya ekle (görsel 200 MB, video 1 GB, dosya 3 GB)" className="mb-1 rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-40"><Paperclip className="size-4" /></button>
        <input
          ref={picker}
          type="file"
          multiple
          className="hidden"
          aria-label="Dosya ekle"
          onChange={(e) => {
            uploads.addFiles([...(e.target.files ?? [])]);
            e.target.value = "";
          }}
        />
        <textarea
          ref={area}
          value={text}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          onContextMenu={onContextMenu}
          onPaste={(e) => {
            const files = [...e.clipboardData.files];
            if (files.length && !editing) {
              e.preventDefault();
              uploads.addFiles(files);
            }
          }}
          onClick={(e) => setQuery(detect(text, e.currentTarget.selectionStart))}
          rows={1}
          maxLength={4000}
          placeholder={group.kind === "dm" ? `${group.name} kişisine yaz...` : `#${group.name} grubuna yaz... (@ ile etiketle)`}
          className="max-h-40 min-h-9 flex-1 resize-none bg-transparent px-1 py-1.5 text-sm outline-none placeholder:text-muted-foreground/60"
          style={{ height: "auto" }}
          onInput={(e) => grow(e.currentTarget)}
        />
        <button
          type="button"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setHelp((v) => !v)}
          title="Biçimlendirme"
          className={cn("mb-1 rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground", help && "bg-accent text-foreground")}
        >
          <Type className="size-4" />
        </button>
        <button
          type="button"
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            const el = area.current;
            const caret = el?.selectionStart ?? text.length;
            const needsSpace = caret > 0 && !/\s/.test(text[caret - 1]);
            const next = text.slice(0, caret) + (needsSpace ? " @" : "@") + text.slice(caret);
            setText(next);
            const pos = caret + (needsSpace ? 2 : 1);
            setQuery({ start: pos - 1, text: "" });
            requestAnimationFrame(() => {
              el?.focus();
              el?.setSelectionRange(pos, pos);
            });
          }}
          title="Etiketle"
          className="mb-1 rounded-lg p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <AtSign className="size-4" />
        </button>
        <button type="button" onClick={() => void submit()} disabled={busy || (!text.trim() && (!!editing || uploads.items.filter((i) => i.status === "hazır").length === 0)) || (!editing && uploads.busy)} aria-label={editing ? "Kaydet" : "Gönder"} className={cn("mb-0.5 flex size-9 items-center justify-center rounded-xl transition-opacity disabled:opacity-40", editing ? "bg-warning text-black" : "bg-primary text-primary-foreground")}>
          {editing ? <Pencil className="size-4" /> : <SendHorizontal className="size-4" />}
        </button>
      </div>
      <div className="mt-1 flex items-center justify-between px-1 text-[0.65rem] text-muted-foreground/70">
        <span>Enter gönderir, Shift+Enter yeni satır{!editing && ", ↑ son mesajını düzenler"}. Dosyayı sürükle ya da yapıştır.</span>
        {error || uploads.error ? (
          <button type="button" onClick={() => { setError(null); uploads.clearError(); }} className="text-destructive">{error ?? uploads.error}</button>
        ) : (
          <span>{text.length} / 4000</span>
        )}
      </div>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menu.items} onClose={() => setMenu(null)} />}
      {uploads.trimming && <VideoTrimmer file={uploads.trimming} maxBytes={VIDEO_MAX} onCancel={uploads.onTrimCancel} onReady={uploads.onTrimReady} />}
      {uploads.editing && (
        <ImageEditor
          file={uploads.editing.file}
          onCancel={() => uploads.setEditing(null)}
          onReady={(f) => {
            const key = uploads.editing?.key;
            uploads.setEditing(null);
            if (key) uploads.replace(key, f);
          }}
        />
      )}
    </div>
  );
}
