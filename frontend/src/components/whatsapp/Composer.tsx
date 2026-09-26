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
  files?: File[];
}

// How many files go in one go. Each one reaches the customer as its own
// message, in order; the typed text rides along with the first.
export const MAX_FILES = 30;

// fileProblem says why WhatsApp would refuse a file, before it is sent.
export function fileProblem(f: File): string | null {
  const mb = f.size / (1 << 20);
  if (f.type === "image/jpeg" || f.type === "image/png") return mb > 5 ? `${f.name}: görsel en fazla 5 MB olabilir.` : null;
  if (f.type.startsWith("video/") || f.type.startsWith("audio/")) return mb > 16 ? `${f.name}: video ve ses en fazla 16 MB olabilir.` : null;
  return mb > 100 ? `${f.name}: dosya en fazla 100 MB olabilir.` : null;
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
  dropped,
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
  // files dropped on the chat; n changes with every drop
  dropped?: { files: File[]; n: number };
  disabledReason?: string;
}) {
  const [mode, setMode] = useState<"message" | "note">(canReply ? "message" : "note");
  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [fileError, setFileError] = useState<string | null>(null);
  const addFiles = (list: File[]) => {
    if (list.length === 0) return;
    const bad = list.map(fileProblem).find(Boolean) ?? null;
    const good = list.filter((f) => !fileProblem(f));
    setFiles((cur) => {
      const next = [...cur, ...good];
      if (next.length > MAX_FILES) setFileError(`Tek seferde en fazla ${MAX_FILES} dosya gönderilebilir; fazlası eklenmedi.`);
      else setFileError(bad);
      return next.slice(0, MAX_FILES);
    });
  };
  useEffect(() => {
    if (dropped && dropped.files.length) addFiles(dropped.files);
  }, [dropped?.n]); // eslint-disable-line react-hooks/exhaustive-deps
  const previews = useMemo(() => files.map((f) => (f.type.startsWith("image/") ? URL.createObjectURL(f) : "")), [files]);
  useEffect(() => () => previews.forEach((u) => u && URL.revokeObjectURL(u)), [previews]);
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
    if (busy || (!body && files.length === 0) || messageBlocked) return;
    if (suggested && mode === "message" && /\[[^\]]+\]/.test(body)) {
      setSuggestError("Önerideki köşeli parantezli yerleri doldurmadan gönderemezsiniz.");
      return;
    }
    setBusy(true);
    try {
      const sending = mode === "message" ? files : [];
      setFiles([]);
      setFileError(null);
      setText("");
      try {
        await onSend({ mode, text: body, files: sending });
      } catch (e) {
        // nothing is lost: what was not sent comes back to the box
        const left = e as { remaining?: File[]; textSent?: boolean };
        if (!left.textSent) setText((cur) => cur || body);
        setFiles((cur) => (cur.length ? cur : left.remaining ?? sending));
        return;
      }
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
    const list = Array.from(e.clipboardData.files ?? []);
    if (list.length && mode === "message") {
      e.preventDefault();
      addFiles(list);
    }
  };

  if (disabledReason) {
    return <div className="bg-card px-5 py-4 text-center text-xs text-muted-foreground">{disabledReason}</div>;
  }

  const isNote = mode === "note";
  return (
    <div className="relative bg-card px-3 pt-2 pb-2.5 md:px-4">
      {(canReply && canNote) || canTemplate ? (
        <div className="mb-2 flex items-center gap-1">
          {canReply && canNote && (
            <div className="flex rounded-full bg-muted/70 p-0.5">
              <button type="button" onClick={() => setMode("message")} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors", !isNote ? "bg-card text-wa-accent shadow-sm" : "text-muted-foreground hover:text-foreground")}>
                <MessageSquareText className="size-3.5" /> Müşteriye
              </button>
              <button type="button" onClick={() => setMode("note")} className={cn("flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors", isNote ? "bg-card text-amber-600 shadow-sm dark:text-amber-400" : "text-muted-foreground hover:text-foreground")}>
                <Lock className="size-3.5" /> İç not
              </button>
            </div>
          )}
          {canTemplate && (
            <button type="button" onClick={onTemplate} className="ml-auto flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
              <FileText className="size-3.5" /> Şablonla yaz
            </button>
          )}
        </div>
      ) : null}
      {replyTo && (
        <div className="mb-2 flex items-center gap-2.5 rounded-lg border-l-4 border-wa-accent bg-muted/60 py-1.5 pr-2 pl-3 text-xs">
          <CornerUpLeft className="size-3.5 shrink-0 text-wa-accent" />
          <span className="min-w-0 flex-1 truncate"><span className="font-medium">{replyTo.direction === "in" ? "Müşteriye" : replyTo.sender.name ?? "Mesaja"}</span> <span className="text-muted-foreground">yanıt: {replyTo.body || replyTo.media?.name || "dosya"}</span></span>
          <button type="button" onClick={onCancelReply} aria-label="Yanıtı iptal et" className="rounded-md p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"><X className="size-3.5" /></button>
        </div>
      )}
      {files.length > 0 && mode === "message" && (
        <div className="mb-2 rounded-xl bg-muted/60 p-2">
          <div className="flex gap-2 overflow-x-auto pb-1">
            {files.map((f, i) => (
              <div key={i} className="group relative size-20 shrink-0 overflow-hidden rounded-lg bg-card ring-1 ring-border/60" data-tip={f.name}>
                {previews[i] ? <img src={previews[i]} alt="" className="size-full object-cover" /> : (
                  <span className="flex size-full flex-col items-center justify-center gap-1 p-1 text-center">
                    <FileText className="size-6 text-wa-accent" />
                    <span className="line-clamp-2 break-all text-[0.6rem] leading-tight text-muted-foreground">{f.name}</span>
                  </span>
                )}
                {f.type.startsWith("video/") && <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1 text-[0.55rem] font-semibold text-white">video</span>}
                <button type="button" onClick={() => setFiles((cur) => cur.filter((_, j) => j !== i))} aria-label="Çıkar" className="absolute right-1 top-1 flex size-5 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"><X className="size-3" /></button>
              </div>
            ))}
            {files.length < MAX_FILES && (
              <button type="button" onClick={() => picker.current?.click()} className="flex size-20 shrink-0 flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-border text-[0.65rem] text-muted-foreground hover:bg-card">
                <Paperclip className="size-4" /> Ekle
              </button>
            )}
          </div>
          <p className="flex items-center gap-2 px-0.5 pt-1 text-[0.7rem] text-muted-foreground">
            <span className="flex-1">{files.length} dosya · her biri ayrı mesaj olarak sırayla gider{text.trim() ? "; yazdığınız metin ilk dosyanın açıklaması olur" : ""}.</span>
            <button type="button" onClick={() => { setFiles([]); setFileError(null); }} className="font-medium hover:text-foreground">Hepsini kaldır</button>
          </p>
        </div>
      )}
      {fileError && <p className="mb-2 flex items-center gap-2 rounded-lg bg-destructive/10 px-3 py-1.5 text-[0.72rem] text-destructive"><span className="flex-1">{fileError}</span><button type="button" onClick={() => setFileError(null)} aria-label="Kapat"><X className="size-3.5" /></button></p>}
      {matches.length > 0 && (
        <div className="absolute bottom-full left-3 z-20 mb-1 w-[min(24rem,calc(100%-1.5rem))] overflow-hidden rounded-2xl border border-border bg-popover p-1 shadow-lg">
          <p className="px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">Hazır yanıtlar</p>
          {matches.map((r, i) => (
            <button key={r.id} type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => useReply(r)} className={cn("flex w-full items-start gap-2.5 rounded-xl px-2 py-1.5 text-left", i === cursor ? "bg-accent" : "")}>
              <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-wa-accent/15 text-wa-accent"><Zap className="size-3.5" /></span>
              <span className="min-w-0">
                <span className="block text-sm font-medium">/{r.shortcut} <span className="font-normal text-muted-foreground">{r.title !== r.shortcut ? r.title : ""}</span></span>
                <span className="block truncate text-xs text-muted-foreground">{fillVars(r.body)}</span>
              </span>
            </button>
          ))}
        </div>
      )}
      {emoji && <EmojiPicker className="absolute right-4 bottom-full z-20 mb-1" onPick={(e) => { setText((t) => t + e); setEmoji(false); }} onClose={() => setEmoji(false)} />}

      {mode === "message" && !windowOpen ? (
        <div className="flex items-center gap-3 rounded-xl bg-muted/60 px-4 py-3 text-xs text-muted-foreground">
          <span className="min-w-0 flex-1">Müşterinin son mesajının üzerinden 24 saat geçti. WhatsApp kuralı gereği artık yalnızca onaylı şablonla yazılabilir.</span>
          {canTemplate && <button type="button" onClick={onTemplate} className="shrink-0 rounded-full bg-wa-accent px-3.5 py-1.5 font-semibold text-wa-on-accent shadow-sm">Şablon seç</button>}
        </div>
      ) : (
        <div className="flex items-end gap-1.5">
          <button type="button" onClick={() => setEmoji((v) => !v)} data-tip="Emoji" className={cn("mb-0.5 flex size-10 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground", emoji && "bg-accent text-foreground")}><SmilePlus className="size-5" /></button>
          {mode === "message" && (
            <>
              <button type="button" onClick={() => picker.current?.click()} data-tip={`Dosya ekle ya da sohbete sürükleyip bırakın. En fazla ${MAX_FILES} dosya; görsel 5 MB, video 16 MB, belge 100 MB`} className="mb-0.5 flex size-10 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><Paperclip className="size-5" /></button>
              <input ref={picker} type="file" multiple className="hidden" onChange={(e) => { addFiles(Array.from(e.target.files ?? [])); e.target.value = ""; }} />
            </>
          )}
          <div className={cn("flex min-w-0 flex-1 items-end rounded-xl px-1 transition-shadow", isNote ? "bg-wa-note/70 ring-1 ring-amber-500/30" : "bg-muted/70 focus-within:bg-card focus-within:ring-2 focus-within:ring-wa-accent/30")}>
            <textarea
              ref={area}
              rows={1}
              value={text}
              onChange={(e) => onChange(e.target.value)}
              onKeyDown={onKey}
              onPaste={onPaste}
              placeholder={isNote ? "Ekibe not yazın, müşteri görmez" : "Mesaj yazın · hazır yanıt için / yazın"}
              className="max-h-44 min-h-10 flex-1 resize-none bg-transparent px-2.5 py-2.5 text-[0.92rem] outline-none placeholder:text-muted-foreground/70"
            />
            {mode === "message" && onSuggest && (
              <button type="button" onClick={() => void suggest()} disabled={thinking} data-tip={text.trim() ? "Yazdığımı düzelt" : "Cevap önerisi al"} className={cn("mb-1 flex size-8 shrink-0 items-center justify-center rounded-full text-violet-500 hover:bg-violet-500/10", thinking && "animate-pulse bg-violet-500/10")}><Sparkles className="size-4" /></button>
            )}
          </div>
          <button
            type="button"
            onClick={() => void send()}
            disabled={busy || (!text.trim() && files.length === 0)}
            aria-label="Gönder"
            className={cn("mb-0.5 flex size-10 shrink-0 items-center justify-center rounded-full text-wa-on-accent shadow-sm transition-[transform,opacity] hover:scale-105 disabled:scale-100 disabled:opacity-40", isNote ? "bg-amber-500" : "bg-wa-accent")}
          >
            <SendHorizontal className="size-5" />
          </button>
        </div>
      )}
      {suggestError ? (
        <p className="mt-1.5 px-1 text-[0.7rem] text-destructive">{suggestError}</p>
      ) : suggested && mode === "message" ? (
        <p className="mt-1.5 flex items-center gap-2 px-1 text-[0.7rem] text-violet-600 dark:text-violet-400">
          <Sparkles className="size-3" /> Yapay zekâ önerisi. Göndermeden önce okuyun, köşeli parantezleri doldurun.
          <button type="button" onClick={() => { setText(suggested.before); setSuggested(null); }} className="ml-auto flex items-center gap-1 font-medium hover:underline"><Undo2 className="size-3" /> Geri al</button>
        </p>
      ) : null}
    </div>
  );
}
