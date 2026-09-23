// Composer: the message box. Enter sends, Shift+Enter breaks the line.
// Typing @ opens a member list; picking one writes "@Ad Soyad" and tags
// them, "@herkes" tags the whole room.

import { useEffect, useMemo, useRef, useState } from "react";
import { AtSign, CornerUpLeft, Paperclip, SendHorizontal, Users, X } from "lucide-react";
import { ApiError } from "@/api/client";
import type { TeamsGroupDetail, TeamsMessage } from "@/api/types";
import UserAvatar from "@/components/ui/UserAvatar";
import { cn } from "@/lib/utils";

export const EVERYONE = "herkes";

export interface Outgoing {
  body: string;
  mentionIds: number[];
  mentionsAll: boolean;
}

type Suggestion = { id: number; name: string; hasAvatar: boolean; avatarVersion?: number };

export default function Composer({ group, selfId, reply, onCancelReply, onSend }: { group: TeamsGroupDetail; selfId: number; reply: TeamsMessage | null; onCancelReply: () => void; onSend: (m: Outgoing) => Promise<void> }) {
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Names inserted through the picker, so a plain "@" in prose never tags.
  const [tagged, setTagged] = useState<Record<number, string>>({});
  const [query, setQuery] = useState<{ start: number; text: string } | null>(null);
  const [cursor, setCursor] = useState(0);
  const area = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    setText("");
    setTagged({});
    setQuery(null);
    setError(null);
  }, [group.id]);

  useEffect(() => {
    if (reply) area.current?.focus();
  }, [reply]);

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
      el.style.height = "auto";
      el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
    });
  }

  function outgoing(body: string): Outgoing {
    const mentionIds = Object.entries(tagged)
      .filter(([, name]) => body.includes(`@${name}`))
      .map(([id]) => Number(id));
    const mentionsAll = group.kind === "group" && new RegExp(`(^|\\s)@${EVERYONE}(?=$|[\\s.,!?:;])`, "u").test(body);
    return { body, mentionIds, mentionsAll };
  }

  async function submit() {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    setError(null);
    try {
      await onSend(outgoing(body));
      setText("");
      setTagged({});
      setQuery(null);
      requestAnimationFrame(() => {
        const el = area.current;
        if (!el) return;
        el.style.height = "auto";
        el.focus();
      });
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
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void submit();
    }
  }

  if (!group.canPost) {
    return (
      <div className="border-t border-border/60 px-5 py-3 text-center text-xs text-muted-foreground">
        {group.postPolicy === "admins" ? "Bu bir duyuru grubu, yalnızca yöneticiler yazabilir." : "Bu grupta yazma yetkin kapatılmış."}
      </div>
    );
  }

  return (
    <div className="relative border-t border-border/60 px-4 py-3">
      {reply && (
        <div className="mb-2 flex items-center gap-2 rounded-lg bg-muted/40 px-3 py-1.5 text-xs">
          <CornerUpLeft className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium">{reply.sender?.name}</span>
            <span className="text-muted-foreground">: {reply.body.slice(0, 100)}</span>
          </span>
          <button type="button" onClick={onCancelReply} aria-label="Yanıtı iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
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

      <div className="flex items-end gap-2 rounded-2xl border border-border/70 bg-muted/30 px-2 py-1.5 focus-within:border-ring/60 focus-within:ring-4 focus-within:ring-ring/15">
        <button type="button" disabled title="Dosya ekleme yakında" className="mb-1 rounded-lg p-1.5 text-muted-foreground/50"><Paperclip className="size-4" /></button>
        <textarea
          ref={area}
          value={text}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={onKeyDown}
          onClick={(e) => setQuery(detect(text, e.currentTarget.selectionStart))}
          rows={1}
          maxLength={4000}
          placeholder={group.kind === "dm" ? `${group.name} kişisine yaz...` : `#${group.name} grubuna yaz... (@ ile etiketle)`}
          className="max-h-40 min-h-9 flex-1 resize-none bg-transparent px-1 py-1.5 text-sm outline-none placeholder:text-muted-foreground/60"
          style={{ height: "auto" }}
          onInput={(e) => {
            const el = e.currentTarget;
            el.style.height = "auto";
            el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
          }}
        />
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
        <button type="button" onClick={() => void submit()} disabled={busy || !text.trim()} aria-label="Gönder" className="mb-0.5 flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground transition-opacity disabled:opacity-40">
          <SendHorizontal className="size-4" />
        </button>
      </div>
      <div className="mt-1 flex items-center justify-between px-1 text-[0.65rem] text-muted-foreground/70">
        <span>Enter gönderir, Shift+Enter yeni satır.</span>
        {error ? <span className="text-destructive">{error}</span> : <span>{text.length} / 4000</span>}
      </div>
    </div>
  );
}
