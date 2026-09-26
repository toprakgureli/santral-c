// Composer is where an agent writes: a message to the customer or an
// internal note, a file, a ready answer with "/", an emoji, or a template
// when the 24-hour window is closed.

import { useEffect, useMemo, useRef, useState } from "react";
import { CornerUpLeft, FileText, Lock, MessageSquareText, Paperclip, SendHorizontal, SmilePlus, Sparkles, Undo2, X, Zap } from "lucide-react";
import EmojiPicker from "@/components/teams/EmojiPicker";
import { cn } from "@/lib/utils";
import type { WAMessage, WAQuickReply } from "@/whatsapp/types";

export interface ComposerSend {
  mode: "message" | "note";
  text: string;
  file?: File;
}

export default function Composer({
  canReply,
  canNote,
  canTemplate,
  windowOpen,
  quickReplies,
  vars,
  replyTo,
  onCancelReply,
  onSend,
  onTemplate,
  onTyping,
  onSuggest,
  disabledReason,
}: {
  canReply: boolean;
  canNote: boolean;
  canTemplate: boolean;
  windowOpen: boolean;
  quickReplies: WAQuickReply[];
  vars: Record<string, string>;
  replyTo: WAMessage | null;
  onCancelReply: () => void;
  onSend: (s: ComposerSend) => Promise<void>;
  onTemplate: () => void;
  onTyping: () => void;
  // the reply assistant: returns a draft for the box, or tidies the given one
  onSuggest?: (draft: string) => Promise<string>;
  disabledReason?: string;
}) {
  const [mode, setMode] = useState<"message" | "note">(canReply ? "message" : "note");
  const [text, setText] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [emoji, setEmoji] = useState(false);
  const [busy, setBusy] = useState(false);
  const [cursor, setCursor] = useState(0);
  const area = useRef<HTMLTextAreaElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  const lastTyping = useRef(0);
  const [thinking, setThinking] = useState(false);
  const [suggested, setSuggested] = useState<{ before: string } | null>(null);
  const [suggestError, setSuggestError] = useState<string | null>(null);

  const suggest = async () => {
    if (!onSuggest || thinking) return;
    setThinking(true);
    setSuggestError(null);
    const before = text;
    try {
      const out = await onSuggest(text);
      setText(out);
      setSuggested({ before });
      window.setTimeout(() => area.current?.focus(), 0);
    } catch (e) {
      setSuggestError(e instanceof Error ? e.message : "Öneri alınamadı.");
    } finally {
      setThinking(false);
    }
  };

  useEffect(() => {
    if (!canReply && mode === "message") setMode("note");
  }, [canReply, mode]);

  // Ready answers: "/" at the start of the box opens the list.
  const slash = mode === "message" && text.startsWith("/") && !text.includes("\n") ? text.slice(1).toLowerCase() : null;
  const matches = useMemo(() => (slash === null ? [] : quickReplies.filter((r) => r.shortcut.includes(slash) || r.title.toLocaleLowerCase("tr").includes(slash)).slice(0, 8)), [slash, quickReplies]);
  useEffect(() => setCursor(0), [slash]);

  const fillVars = (body: string) => body.replace(/\{(\w+)\}/g, (all, k) => vars[k] ?? all);
  const useReply = (r: WAQuickReply) => {
    setText(fillVars(r.body));
    window.setTimeout(() => area.current?.focus(), 0);
  };

  // The box grows with the text, up to a limit.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`;
  }, [text]);

  const messageBlocked = mode === "message" && (!canReply || !windowOpen);
  const send = async () => {
    const body = text.trim();
    if (busy || (!body && !file) || messageBlocked) return;
    if (suggested && mode === "message" && /\[[^\]]+\]/.test(body)) {
      setSuggestError("Önerideki köşeli parantezli yerleri doldurmadan gönderemezsiniz.");
      return;
    }
    setBusy(true);
    try {
      await onSend({ mode, text: body, file: file ?? undefined });
      setText("");
      setFile(null);
      setSuggested(null);
      setSuggestError(null);
    } finally {
      setBusy(false);
      window.setTimeout(() => area.current?.focus(), 0);
    }
  };

  const onKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (matches.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setCursor((c) => (c + 1) % matches.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setCursor((c) => (c - 1 + matches.length) % matches.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        useReply(matches[cursor]);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void send();
    }
  };

  const onChange = (v: string) => {
    setText(v);
    if (mode === "message" && Date.now() - lastTyping.current > 3000) {
      lastTyping.current = Date.now();
      onTyping();
    }
  };

  const onPaste = (e: React.ClipboardEvent) => {
    const f = e.clipboardData.files?.[0];
    if (f && mode === "message") {
      e.preventDefault();
      setFile(f);
    }
  };

  if (disabledReason) {
    return <div className="border-t border-border/50 bg-card/60 px-5 py-4 text-center text-xs text-muted-foreground backdrop-blur-md">{disabledReason}</div>;
  }

  return (
    <div className="relative border-t border-border/50 bg-card/60 px-4 py-3 backdrop-blur-md">
      {replyTo && (
        <div className="mb-2 flex items-center gap-2.5 rounded-xl border-l-2 border-primary bg-muted/40 py-1.5 pr-2 pl-3 text-xs">
          <CornerUpLeft className="size-3.5 shrink-0 text-primary" />
          <span className="min-w-0 flex-1 truncate"><span className="font-medium">{replyTo.direction === "in" ? "Müşteriye" : replyTo.sender.name ?? "Mesaja"}</span> <span className="text-muted-foreground">yanıt: {replyTo.body || replyTo.media?.name || "dosya"}</span></span>
          <button type="button" onClick={onCancelReply} aria-label="Yanıtı iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      )}
      {file && (
        <div className="mb-2 flex items-center gap-2.5 rounded-xl bg-muted/40 px-3 py-2 text-xs">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><FileText className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">{file.name}</span>
            <span className="block text-muted-foreground">{(file.size / 1024 / 1024).toFixed(1)} MB · yazdığınız metin açıklama olarak gider</span>
          </span>
          <button type="button" onClick={() => setFile(null)} aria-label="Dosyayı çıkar" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      )}
      {matches.length > 0 && (
        <div className="absolute bottom-full left-4 z-20 mb-1 w-96 overflow-hidden rounded-2xl border border-border bg-popover p-1 shadow-lg">
          <p className="px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Hazır yanıtlar</p>
          {matches.map((r, i) => (
            <button key={r.id} type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => useReply(r)} className={cn("flex w-full items-start gap-2.5 rounded-xl px-2 py-1.5 text-left", i === cursor ? "bg-accent" : "")}>
              <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Zap className="size-3.5" /></span>
              <span className="min-w-0">
                <span className="block text-sm font-medium">/{r.shortcut} <span className="font-normal text-muted-foreground">{r.title !== r.shortcut ? r.title : ""}</span></span>
                <span className="block truncate text-xs text-muted-foreground">{fillVars(r.body)}</span>
              </span>
            </button>
          ))}
        </div>
      )}
      {emoji && <EmojiPicker className="absolute right-4 bottom-full z-20 mb-1" onPick={(e) => { setText((t) => t + e); setEmoji(false); }} onClose={() => setEmoji(false)} />}

      <div className="mb-2 flex items-center gap-1">
        {canReply && (
          <button type="button" onClick={() => setMode("message")} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors", mode === "message" ? "bg-emerald-500/12 text-emerald-700 ring-1 ring-emerald-600/25 dark:text-emerald-400" : "text-muted-foreground hover:bg-accent")}>
            <MessageSquareText className="size-3.5" /> Müşteriye
          </button>
        )}
        {canNote && (
          <button type="button" onClick={() => setMode("note")} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors", mode === "note" ? "bg-amber-400/15 text-amber-700 ring-1 ring-amber-500/30 dark:text-amber-400" : "text-muted-foreground hover:bg-accent")}>
            <Lock className="size-3.5" /> İç not
          </button>
        )}
        {canTemplate && (
          <button type="button" onClick={onTemplate} className="ml-auto flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
            <FileText className="size-3.5" /> Şablonla yaz
          </button>
        )}
      </div>

      {mode === "message" && !windowOpen ? (
        <div className="flex items-center gap-3 rounded-[1.375rem] border border-dashed border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
          <span className="min-w-0 flex-1">Müşterinin son mesajının üzerinden 24 saat geçti. WhatsApp kuralı gereği artık yalnızca onaylı şablonla yazılabilir.</span>
          {canTemplate && <button type="button" onClick={onTemplate} className="shrink-0 rounded-full bg-primary px-3 py-1.5 font-semibold text-primary-foreground shadow-sm">Şablon seç</button>}
        </div>
      ) : (
        <div className={cn("flex items-end gap-1.5 rounded-[1.375rem] border bg-card px-2 py-1.5 shadow-sm transition-[box-shadow,border-color] focus-within:ring-4", mode === "note" ? "border-amber-500/40 focus-within:ring-amber-500/15" : "border-border/60 focus-within:border-ring/50 focus-within:ring-ring/15")}>
          {mode === "message" && (
            <>
              <button type="button" onClick={() => picker.current?.click()} data-tip="Dosya ekle (görsel 5 MB, video 16 MB, belge 100 MB)" className="mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><Paperclip className="size-4" /></button>
              <input ref={picker} type="file" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) setFile(f); e.target.value = ""; }} />
            </>
          )}
          <textarea
            ref={area}
            rows={1}
            value={text}
            onChange={(e) => onChange(e.target.value)}
            onKeyDown={onKey}
            onPaste={onPaste}
            placeholder={mode === "note" ? "Ekibe not yazın, müşteri görmez" : "Mesaj yazın · hazır yanıt için / yazın"}
            className="max-h-44 min-h-9 flex-1 resize-none bg-transparent px-1 py-2 text-sm outline-none placeholder:text-muted-foreground/60"
          />
          {mode === "message" && onSuggest && (
            <button type="button" onClick={() => void suggest()} disabled={thinking} data-tip={text.trim() ? "Yazdığımı düzelt" : "Cevap önerisi al"} className={cn("mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full text-violet-500 hover:bg-violet-500/10", thinking && "animate-pulse bg-violet-500/10")}><Sparkles className="size-4" /></button>
          )}
          <button type="button" onClick={() => setEmoji((v) => !v)} data-tip="Emoji" className={cn("mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground", emoji && "bg-accent text-foreground")}><SmilePlus className="size-4" /></button>
          <button
            type="button"
            onClick={() => void send()}
            disabled={busy || (!text.trim() && !file)}
            aria-label="Gönder"
            className={cn("mb-0.5 flex size-9 shrink-0 items-center justify-center rounded-full shadow-md transition-[transform,opacity] hover:scale-105 disabled:scale-100 disabled:opacity-40 disabled:shadow-none", mode === "note" ? "bg-amber-500 text-white shadow-amber-500/30" : "bg-primary text-primary-foreground shadow-primary/30")}
          >
            <SendHorizontal className="size-4" />
          </button>
        </div>
      )}
      {suggestError ? (
        <p className="mt-1 px-1 text-[0.7rem] text-destructive">{suggestError}</p>
      ) : suggested && mode === "message" ? (
        <p className="mt-1 flex items-center gap-2 px-1 text-[0.7rem] text-violet-600 dark:text-violet-400">
          <Sparkles className="size-3" /> Yapay zekâ önerisi. Göndermeden önce okuyun, köşeli parantezleri doldurun.
          <button type="button" onClick={() => { setText(suggested.before); setSuggested(null); }} className="ml-auto flex items-center gap-1 font-medium hover:underline"><Undo2 className="size-3" /> Geri al</button>
        </p>
      ) : (
        <p className="mt-1 px-1 text-[0.65rem] text-muted-foreground/70">Enter gönderir, Shift+Enter alt satıra geçer. *kalın* _italik_ ~çizili~</p>
      )}
    </div>
  );
}
