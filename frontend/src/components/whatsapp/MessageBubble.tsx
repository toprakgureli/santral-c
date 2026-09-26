// MessageBubble draws one line of a conversation: the customer's bubbles on
// the left, ours on the right with the name of whoever wrote them above
// (a person, the chatbot or a rule), internal notes in amber, and history
// lines ("Toprak sohbeti üstlendi") in the middle.

import { useState } from "react";
import { Bot, Copy, CornerUpLeft, Download, FileText, Lock, MapPin, MousePointerClick, RotateCcw, SmilePlus, UserSquare2, Zap } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import Ticks from "@/components/whatsapp/Ticks";
import { waText } from "@/components/whatsapp/waText";
import { cn } from "@/lib/utils";
import type { WAMessage } from "@/whatsapp/types";
import { clock, menuOptions, menuText } from "@/whatsapp/util";

const QUICK = ["👍", "❤️", "😂", "😮", "🙏", "✅"];

function size(n?: number): string {
  if (!n) return "";
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export default function MessageBubble({ m, head, onReply, onReact, onRetry, onImage, highlight }: { m: WAMessage; head: boolean; onReply?: (m: WAMessage) => void; onReact?: (m: WAMessage, emoji: string) => void; onRetry?: (m: WAMessage) => void; onImage?: (m: WAMessage) => void; highlight?: boolean }) {
  const [emoji, setEmoji] = useState(false);

  if (m.direction === "event") {
    return (
      <div className="my-2 flex justify-center px-6">
        <span className="max-w-[80%] rounded-full bg-muted/70 px-3 py-1 text-center text-[0.7rem] leading-snug text-muted-foreground">
          {m.body} <span className="ml-1 tabular-nums text-muted-foreground/60">{clock(m.createdAt)}</span>
        </span>
      </div>
    );
  }

  const out = m.direction === "out" || m.direction === "note";
  const note = m.direction === "note";
  const bot = m.sender.kind === "bot";
  const auto = m.sender.kind === "automation";
  const who = note ? m.sender.name ?? m.sender.label : m.sender.kind === "agent" ? m.sender.name : bot ? `Chatbot · ${m.sender.label}` : auto ? `Otomatik · ${m.sender.label}` : "";

  const tone = note
    ? "bg-amber-400/15 ring-1 ring-amber-500/25"
    : !out
      ? "bg-card ring-1 ring-border/60"
      : bot
        ? "bg-violet-500/10 ring-1 ring-violet-500/20"
        : auto
          ? "bg-sky-500/10 ring-1 ring-sky-500/20"
          : "bg-emerald-500/12 ring-1 ring-emerald-600/20";

  const options = m.kind === "interactive" && out ? menuOptions(m) : [];
  const body = m.kind === "interactive" && out ? menuText(m) : m.body;

  return (
    <div id={`wa-m-${m.id}`} className={cn("group flex gap-2 px-4", out ? "flex-row-reverse" : "flex-row", head ? "mt-3" : "mt-0.5")}>
      <div className="w-8 shrink-0">
        {head && out && (
          note || m.sender.kind === "agent" ? (
            <UserAvatar userId={m.sender.userId} name={m.sender.name} hasAvatar={m.sender.hasAvatar} version={m.sender.avatarVersion} className="size-8 shadow-sm ring-2 ring-card" fallbackClassName="bg-primary/10 text-[0.65rem] text-primary" />
          ) : (
            <span className={cn("flex size-8 items-center justify-center rounded-full ring-2 ring-card", bot ? "bg-violet-500/15 text-violet-600 dark:text-violet-400" : "bg-sky-500/15 text-sky-600 dark:text-sky-400")}>
              {bot ? <Bot className="size-4" /> : <Zap className="size-4" />}
            </span>
          )
        )}
      </div>
      <div className={cn("flex max-w-[min(34rem,78%)] flex-col", out ? "items-end" : "items-start")}>
        {head && out && who && (
          <span className={cn("mb-1 flex items-center gap-1 px-1 text-[0.7rem] font-semibold", note ? "text-amber-700 dark:text-amber-400" : "text-foreground/70")}>
            {note && <Lock className="size-3" />}
            {note ? `İç not · ${who}` : who}
            {m.kind === "template" && <span className="rounded-full bg-muted px-1.5 py-px text-[0.6rem] font-medium text-muted-foreground">Şablon · {m.sender.label}</span>}
          </span>
        )}
        <div className="relative flex items-end gap-1">
          <div className={cn("relative min-w-[4.5rem] rounded-2xl px-3 py-2 text-sm shadow-sm transition-shadow", tone, out ? "rounded-tr-md" : "rounded-tl-md", highlight && "ring-2 ring-primary")}>
            {m.replyTo && (
              <button type="button" onClick={() => document.getElementById(`wa-m-${m.replyTo!.id}`)?.scrollIntoView({ behavior: "smooth", block: "center" })} className="mb-1.5 block w-full rounded-lg border-l-2 border-primary/60 bg-foreground/5 px-2 py-1 text-left">
                <span className="block text-[0.65rem] font-semibold text-primary">{m.replyTo.sender || "Mesaj"}</span>
                <span className="block truncate text-xs text-muted-foreground">{m.replyTo.body}</span>
              </button>
            )}
            <Media m={m} onImage={onImage} />
            {m.kind === "interactive" && !out && (
              <span className="mb-0.5 flex items-center gap-1 text-[0.65rem] font-medium text-muted-foreground"><MousePointerClick className="size-3" /> Seçti</span>
            )}
            {m.kind === "location" && (
              <a href={locationLink(m)} target="_blank" rel="noopener noreferrer" className="mb-1 flex items-center gap-2 rounded-xl bg-foreground/5 px-2.5 py-2 text-xs font-medium hover:bg-foreground/10"><MapPin className="size-4 text-destructive" /> Haritada aç</a>
            )}
            {m.kind === "contacts" && (
              <span className="mb-1 flex items-center gap-2 rounded-xl bg-foreground/5 px-2.5 py-2 text-xs font-medium"><UserSquare2 className="size-4" /> {contactsText(m)}</span>
            )}
            {m.kind === "unsupported" ? (
              <p className="text-xs italic text-muted-foreground">{m.body}</p>
            ) : body ? (
              <p className="whitespace-pre-wrap break-words leading-relaxed">{waText(body)}</p>
            ) : null}
            {options.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {options.map((o, i) => (
                  <span key={i} className="rounded-full bg-card/80 px-2.5 py-1 text-xs font-medium text-sky-600 ring-1 ring-border/60 dark:text-sky-400">{o.title}</span>
                ))}
              </div>
            )}
            <span className={cn("mt-1 flex items-center justify-end gap-1 text-[0.65rem] tabular-nums text-muted-foreground")}>
              {clock(m.createdAt)}
              {m.direction === "out" && <Ticks status={m.status} />}
            </span>
            {m.reactions && m.reactions.length > 0 && (
              <span className={cn("absolute -bottom-3 flex gap-0.5 rounded-full bg-card px-1.5 py-0.5 text-xs shadow-sm ring-1 ring-border/60", out ? "right-3" : "left-3")}>
                {m.reactions.map((r, i) => (
                  <span key={i} data-tip={r.ours ? `Bizden${r.by ? ": " + r.by : ""}` : "Müşteriden"}>{r.emoji}</span>
                ))}
              </span>
            )}
          </div>
          {/* Hover actions */}
          {(onReply || onReact) && m.direction !== "note" && (
            <div className={cn("absolute top-1 hidden items-center gap-0.5 rounded-full bg-card px-1 py-0.5 shadow-md ring-1 ring-border/60 group-hover:flex", out ? "-left-2 -translate-x-full" : "-right-2 translate-x-full", emoji && "flex")}>
              {onReply && <button type="button" onClick={() => onReply(m)} data-tip="Yanıtla" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><CornerUpLeft className="size-3.5" /></button>}
              {onReact && m.status !== "queued" && (
                <span className="relative">
                  <button type="button" onClick={() => setEmoji((v) => !v)} data-tip="Tepki ver" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><SmilePlus className="size-3.5" /></button>
                  {emoji && (
                    <span className="absolute bottom-full left-1/2 z-10 mb-1 flex -translate-x-1/2 gap-0.5 rounded-full bg-card px-1.5 py-1 shadow-lg ring-1 ring-border/60">
                      {QUICK.map((e) => (
                        <button key={e} type="button" onClick={() => { onReact(m, e); setEmoji(false); }} className="rounded-full px-1 text-lg leading-none transition-transform hover:scale-125">{e}</button>
                      ))}
                    </span>
                  )}
                </span>
              )}
              {m.body && <button type="button" onClick={() => void navigator.clipboard?.writeText(m.body)} data-tip="Metni kopyala" className="rounded-full p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground"><Copy className="size-3.5" /></button>}
            </div>
          )}
        </div>
        {m.status === "failed" && (
          <span className="mt-1 flex max-w-full items-center gap-2 px-1 text-[0.7rem] text-destructive">
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
  if (md.failed) return <p className="mb-1 rounded-lg bg-destructive/10 px-2 py-1.5 text-xs text-destructive">{md.failed}</p>;
  switch (m.kind) {
    case "image":
    case "sticker":
      return (
        <button type="button" onClick={() => onImage?.(m)} className={cn("mb-1 block overflow-hidden rounded-xl bg-foreground/5", m.kind === "sticker" ? "size-32" : "max-h-80")}>
          <img src={md.url} alt="" loading="lazy" className={cn(m.kind === "sticker" ? "size-full object-contain" : "max-h-80 w-full min-w-40 object-cover")} />
        </button>
      );
    case "video":
      return <video src={md.url} controls preload="metadata" className="mb-1 max-h-80 w-full min-w-56 rounded-xl bg-black" />;
    case "audio":
      return (
        <div className="mb-1 min-w-60">
          {md.voice && <span className="mb-1 block text-[0.65rem] font-medium text-muted-foreground">Sesli mesaj</span>}
          <audio src={md.url} controls preload="metadata" className="h-10 w-full" />
        </div>
      );
    default:
      return (
        <a href={`${md.url}?download=1`} className="mb-1 flex min-w-56 items-center gap-2.5 rounded-xl bg-foreground/5 px-2.5 py-2 hover:bg-foreground/10">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><FileText className="size-4" /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-semibold">{md.name ?? "Belge"}</span>
            <span className="block text-[0.65rem] text-muted-foreground">{[md.mime?.split("/")[1]?.toUpperCase(), size(md.size)].filter(Boolean).join(" · ")}</span>
          </span>
          <Download className="size-4 shrink-0 text-muted-foreground" />
        </a>
      );
  }
}
