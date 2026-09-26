// MessageBubble draws one line of a conversation: the customer's bubbles on
// the left in white, ours on the right in soft green with the name of
// whoever wrote them inside (a person, the chatbot or a rule), internal
// notes in pale amber, and history lines ("Toprak sohbeti üstlendi") as
// small labels in the middle. The time and the ticks sit in the corner.

import { useState } from "react";
import { Bot, Copy, CornerUpLeft, Download, FileText, Lock, MapPin, MousePointerClick, RotateCcw, SmilePlus, UserSquare2, Zap } from "lucide-react";
import AdSource from "@/components/whatsapp/AdSource";
import Ticks from "@/components/whatsapp/Ticks";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import type { WAMessage } from "@/whatsapp/types";
import { clock, menuOptions, menuText } from "@/whatsapp/util";

const QUICK = ["👍", "❤️", "😂", "😮", "🙏", "✅"];

// Each person's name keeps one colour, readable on the green bubble in
// both themes, so a glance tells who wrote what.
const NAME_COLORS = [
  "text-sky-700 dark:text-sky-300",
  "text-orange-700 dark:text-orange-300",
  "text-pink-700 dark:text-pink-300",
  "text-violet-700 dark:text-violet-300",
  "text-blue-700 dark:text-blue-300",
  "text-rose-700 dark:text-rose-300",
  "text-fuchsia-700 dark:text-fuchsia-300",
  "text-amber-800 dark:text-amber-200",
  "text-indigo-700 dark:text-indigo-300",
  "text-cyan-800 dark:text-cyan-300",
];
export function nameColor(id?: number): string {
  return NAME_COLORS[Math.abs(id ?? 0) % NAME_COLORS.length];
}

function size(n?: number): string {
  if (!n) return "";
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export default function MessageBubble({ m, head, onReply, onReact, onRetry, onImage, onMenu, highlight }: { m: WAMessage; head: boolean; onReply?: (m: WAMessage) => void; onReact?: (m: WAMessage, emoji: string) => void; onRetry?: (m: WAMessage) => void; onImage?: (m: WAMessage) => void; onMenu?: (m: WAMessage, x: number, y: number) => void; highlight?: boolean }) {
  const [emoji, setEmoji] = useState(false);

  if (m.direction === "event") {
    return (
      <div className="my-2 flex justify-center px-6">
        <span className="max-w-[80%] rounded-lg bg-card/95 px-3 py-1.5 text-center text-[0.72rem] leading-snug text-muted-foreground shadow-sm">
          {m.body} <span className="ml-1 tabular-nums text-muted-foreground/70">{clock(m.createdAt)}</span>
        </span>
      </div>
    );
  }

  const out = m.direction === "out" || m.direction === "note";
  const note = m.direction === "note";
  const bot = m.sender.kind === "bot";
  const auto = m.sender.kind === "automation";
  const who = note ? m.sender.name ?? m.sender.label : m.sender.kind === "agent" ? m.sender.name : bot ? `Chatbot · ${m.sender.label}` : auto ? `Otomatik · ${m.sender.label}` : "";
  const fill = note ? "bg-wa-note" : out ? "bg-wa-out" : "bg-wa-in";
  const tail = note ? "text-wa-note" : out ? "text-wa-out" : "text-wa-in";
  const options = m.kind === "interactive" && out ? menuOptions(m) : [];
  const body = m.kind === "interactive" && out ? menuText(m) : m.body;
  const media = !!m.media && ["image", "video", "sticker"].includes(m.kind);

  return (
    <div id={`wa-m-${m.id}`} className={cn("group flex px-[4%] md:px-[7%]", out ? "justify-end" : "justify-start", head ? "mt-2.5" : "mt-0.5")}>
      <div className={cn("relative flex max-w-[min(34rem,85%)] flex-col md:max-w-[65%]", out ? "items-end" : "items-start")}>
        <div onContextMenu={onMenu ? (e) => { e.preventDefault(); onMenu(m, e.clientX, e.clientY); } : undefined} className={cn("relative rounded-lg text-[0.9rem] leading-snug text-foreground shadow-[0_1px_0.5px_rgba(11,20,26,0.13)] transition-shadow", fill, media ? "p-1" : "px-2 pt-1.5 pb-1", head && (out ? "rounded-tr-none" : "rounded-tl-none"), highlight && "ring-2 ring-wa-accent")}>
          {head && (
            <svg viewBox="0 0 8 13" className={cn("absolute top-0 h-[13px] w-2", tail, out ? "-right-2" : "-left-2 -scale-x-100")} aria-hidden>
              <path d="M0 0h8L1.5 9.2C.9 10.1 0 9.6 0 8.6V0Z" fill="currentColor" />
            </svg>
          )}
          {head && out && who && (
            <span className={cn("mb-0.5 flex items-center gap-1 px-0.5 text-[0.8rem] font-medium", note ? "text-amber-800 dark:text-amber-300" : bot ? "text-violet-700 dark:text-violet-300" : auto ? "text-sky-700 dark:text-sky-300" : nameColor(m.sender.userId), media && "px-1.5 pt-0.5")}>
              {note ? <Lock className="size-3" /> : bot ? <Bot className="size-3" /> : auto ? <Zap className="size-3" /> : null}
              {note ? `İç not · ${who}` : who}
              {m.kind === "template" && <span className="rounded bg-foreground/8 px-1 py-px text-[0.6rem] font-medium text-wa-meta">şablon</span>}
            </span>
          )}
          {m.replyTo && (
            <button type="button" onClick={() => document.getElementById(`wa-m-${m.replyTo!.id}`)?.scrollIntoView({ behavior: "smooth", block: "center" })} className="mb-1 block w-full rounded-md border-l-4 border-wa-accent bg-foreground/5 px-2 py-1 text-left">
              <span className="block text-[0.72rem] font-semibold text-wa-accent">{m.replyTo.sender || "Mesaj"}</span>
              <span className="block truncate text-[0.78rem] text-muted-foreground">{m.replyTo.body}</span>
            </button>
          )}
          {m.referral && <AdSource r={m.referral} compact />}
          <Media m={m} onImage={onImage} />
          {m.kind === "interactive" && !out && <span className="mb-0.5 flex items-center gap-1 px-0.5 text-[0.68rem] font-medium text-muted-foreground"><MousePointerClick className="size-3" /> Seçti</span>}
          {m.kind === "location" && (
            <a href={locationLink(m)} target="_blank" rel="noopener noreferrer" className="mb-1 flex items-center gap-2 rounded-md bg-foreground/5 px-2.5 py-2 text-xs font-medium hover:bg-foreground/10"><MapPin className="size-4 text-destructive" /> Haritada aç</a>
          )}
          {m.kind === "contacts" && <span className="mb-1 flex items-center gap-2 rounded-md bg-foreground/5 px-2.5 py-2 text-xs font-medium"><UserSquare2 className="size-4" /> {contactsText(m)}</span>}
          <div className={cn(media && "px-1 pb-0.5")}>
            {m.kind === "unsupported" ? (
              <span className="text-xs italic text-muted-foreground">{m.body}</span>
            ) : body ? (
              <span className="whitespace-pre-wrap break-words">{waText(body)}</span>
            ) : null}
            <span className={cn("relative top-1.5 float-right ml-3 flex items-center gap-1 text-[0.68rem] tabular-nums text-wa-meta", !body && media && "absolute right-2 bottom-2 top-auto rounded-full bg-black/35 px-1.5 text-white")}>
              {clock(m.createdAt)}
              {m.direction === "out" && <Ticks status={m.status} className={cn(!body && media && "text-white")} />}
            </span>
          </div>
          {options.length > 0 && (
            <div className="mt-2 -mx-2 -mb-1 flex flex-col border-t border-foreground/10">
              {options.map((o, i) => <span key={i} className="border-b border-foreground/10 py-1.5 text-center text-[0.82rem] font-medium text-sky-600 last:border-0 dark:text-sky-400">{o.title}</span>)}
            </div>
          )}
          {m.reactions && m.reactions.length > 0 && (
            <span className={cn("absolute -bottom-3.5 flex gap-0.5 rounded-full bg-card px-1.5 py-0.5 text-xs shadow ring-1 ring-border/60", out ? "right-2" : "left-2")}>
              {m.reactions.map((r, i) => <span key={i} data-tip={r.ours ? `Bizden${r.by ? ": " + r.by : ""}` : "Müşteriden"}>{r.emoji}</span>)}
            </span>
          )}
        </div>
        {m.reactions && m.reactions.length > 0 && <span className="h-3" />}

        {(onReply || onReact) && m.direction !== "note" && (
          <div className={cn("absolute top-1 hidden items-center gap-0.5 rounded-full bg-card/95 px-1 py-0.5 shadow-md ring-1 ring-border/60 backdrop-blur group-hover:flex", out ? "-left-2 -translate-x-full" : "-right-2 translate-x-full", emoji && "flex")}>
            {onReply && <button type="button" onClick={() => onReply(m)} data-tip="Yanıtla" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><CornerUpLeft className="size-3.5" /></button>}
            {onReact && m.status !== "queued" && (
              <span className="relative">
                <button type="button" onClick={() => setEmoji((v) => !v)} data-tip="Tepki ver" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><SmilePlus className="size-3.5" /></button>
                {emoji && (
                  <span className="absolute bottom-full left-1/2 z-10 mb-1 flex -translate-x-1/2 gap-0.5 rounded-full bg-card px-1.5 py-1 shadow-lg ring-1 ring-border/60">
                    {QUICK.map((e) => <button key={e} type="button" onClick={() => { onReact(m, e); setEmoji(false); }} className="rounded-full px-1 text-lg leading-none transition-transform hover:scale-125">{e}</button>)}
                  </span>
                )}
              </span>
            )}
            {m.body && <button type="button" onClick={() => void navigator.clipboard?.writeText(m.body)} data-tip="Metni kopyala" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><Copy className="size-3.5" /></button>}
          </div>
        )}
        {m.status === "failed" && (
          <span className="mt-1 flex max-w-full items-center gap-2 rounded-lg bg-card/95 px-2 py-1 text-[0.72rem] text-destructive shadow-sm">
            <span className="min-w-0">{m.errorText || "Gönderilemedi."}</span>
            {onRetry && <button type="button" onClick={() => onRetry(m)} className="flex shrink-0 items-center gap-1 rounded-full bg-destructive/10 px-2 py-0.5 font-semibold hover:bg-destructive/15"><RotateCcw className="size-3" /> Tekrar gönder</button>}
          </span>
        )}
      </div>
    </div>
  );
}

function locationLink(m: WAMessage): string {
  const p = m.payload as { latitude?: number; longitude?: number } | undefined;
  if (p?.latitude != null && p?.longitude != null) return `https://www.google.com/maps?q=${p.latitude},${p.longitude}`;
  return `https://www.google.com/maps?q=${encodeURIComponent(m.body)}`;
}

function contactsText(m: WAMessage): string {
  const list = m.payload as { name?: { formatted_name?: string }; phones?: { phone?: string }[] }[] | undefined;
  if (!Array.isArray(list) || list.length === 0) return "Kişi kartı";
  return list.map((c) => [c.name?.formatted_name, c.phones?.[0]?.phone].filter(Boolean).join(" · ")).join(", ");
}

function Media({ m, onImage }: { m: WAMessage; onImage?: (m: WAMessage) => void }) {
  const md = m.media;
  if (!md) return null;
  if (md.failed) return <p className="mb-1 rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">{md.failed}</p>;
  switch (m.kind) {
    case "image":
    case "sticker":
      return (
        <button type="button" onClick={() => onImage?.(m)} className={cn("block overflow-hidden rounded-md", m.kind === "sticker" ? "size-32 bg-transparent" : "max-h-80 bg-foreground/5")}>
          <img src={md.url} alt="" loading="lazy" className={cn(m.kind === "sticker" ? "size-full object-contain" : "max-h-80 w-full min-w-48 object-cover")} />
        </button>
      );
    case "video":
      return <video src={md.url} controls preload="metadata" className="max-h-80 w-full min-w-56 rounded-md bg-black" />;
    case "audio":
      return (
        <div className="mb-1 min-w-64">
          {md.voice && <span className="mb-1 block px-0.5 text-[0.68rem] font-medium text-muted-foreground">Sesli mesaj</span>}
          <audio src={md.url} controls preload="metadata" className="h-10 w-full" />
        </div>
      );
    default:
      return (
        <a href={`${md.url}?download=1`} className="mb-1 flex min-w-60 items-center gap-2.5 rounded-md bg-foreground/5 px-2.5 py-2.5 hover:bg-foreground/10">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-wa-accent/15 text-wa-accent"><FileText className="size-5" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[0.82rem] font-medium">{md.name ?? "Belge"}</span>
            <span className="block text-[0.68rem] text-muted-foreground">{[md.mime?.split("/")[1]?.toUpperCase(), size(md.size)].filter(Boolean).join(" · ")}</span>
          </span>
          <Download className="size-4 shrink-0 text-muted-foreground" />
        </a>
      );
  }
}
