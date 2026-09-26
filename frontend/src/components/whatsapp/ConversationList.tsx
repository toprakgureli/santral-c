// ConversationList is the inbox's left column: tabs for "mine", "waiting
// for an answer", the pool, everything and resolved, a search box, and the
// conversations with their state at a glance.

import { useEffect, useMemo, useState } from "react";
import { Bot, Clock, Hourglass, Inbox, Search, UserRound, Users, X } from "lucide-react";
import UserAvatar from "@/components/ui/UserAvatar";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import Ticks from "@/components/whatsapp/Ticks";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { inBucket, isMine, isWaiting, listTime, since, sortTime, type Bucket } from "@/whatsapp/util";

const TABS: { key: Bucket; label: string; icon: typeof Inbox; tip: string }[] = [
  { key: "mine", label: "Benim", icon: UserRound, tip: "Sorumlu olduğun ya da yardım ettiğin sohbetler" },
  { key: "waiting", label: "Bekleyen", icon: Hourglass, tip: "Cevap Bekleyenler: müşteri belirlenen süreden uzun süredir cevap bekliyor" },
  { key: "pool", label: "Havuz", icon: Inbox, tip: "Henüz kimsenin üstlenmediği sohbetler" },
  { key: "team", label: "Tümü", icon: Users, tip: "Görebildiğin bütün açık sohbetler" },
  { key: "resolved", label: "Çözülen", icon: Clock, tip: "Çözülmüş sohbetler" },
];

export default function ConversationList({ channels, activeId, onOpen, bucket, onBucket }: { channels: WAChannel[]; activeId: number | null; onOpen: (id: number) => void; bucket: Bucket; onBucket: (b: Bucket) => void }) {
  const wa = useWhatsApp();
  const [q, setQ] = useState("");
  const [channel, setChannel] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const [older, setOlder] = useState<WAConversation[]>([]);
  const [olderDone, setOlderDone] = useState(false);

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(t);
  }, []);

  // Older resolved conversations come from the server on demand.
  useEffect(() => {
    if (bucket !== "resolved") return;
    let live = true;
    const t = window.setTimeout(() => {
      waApi.resolved(undefined, q).then((list) => {
        if (!live) return;
        setOlder(list);
        setOlderDone(list.length < 60);
      }).catch(() => undefined);
    }, q ? 300 : 0);
    return () => {
      live = false;
      window.clearTimeout(t);
    };
  }, [bucket, q]);

  const counts = useMemo(() => {
    const out: Record<Bucket, number> = { mine: 0, waiting: 0, pool: 0, team: 0, resolved: 0 };
    for (const c of wa.conversations) {
      if (channel && c.channelId !== channel) continue;
      for (const t of TABS) if (inBucket(c, t.key, wa.me)) out[t.key]++;
    }
    return out;
  }, [wa.conversations, wa.me, channel]);

  const list = useMemo(() => {
    const needle = q.trim().toLocaleLowerCase("tr");
    const digits = q.replace(/\D/g, "");
    const pool = new Map<number, WAConversation>();
    for (const c of wa.conversations) pool.set(c.id, c);
    if (bucket === "resolved") for (const c of older) if (!pool.has(c.id)) pool.set(c.id, c);
    return [...pool.values()]
      .filter((c) => (!channel || c.channelId === channel) && inBucket(c, bucket, wa.me))
      .filter((c) => !needle || c.contact.display.toLocaleLowerCase("tr").includes(needle) || (digits.length >= 3 && c.contact.waId.includes(digits)) || c.contact.tags.some((t) => t.toLocaleLowerCase("tr").includes(needle)) || String(c.ticket?.number ?? "") === needle.replace("#", ""))
      .sort((a, b) => (bucket === "waiting" ? Date.parse(a.ticket?.awaitingSince ?? "") - Date.parse(b.ticket?.awaitingSince ?? "") : sortTime(b) - sortTime(a)));
  }, [wa.conversations, wa.me, bucket, channel, q, older]);

  const loadOlder = async () => {
    const last = list[list.length - 1];
    if (!last?.last) return;
    const more = await waApi.resolved(last.last.at, q).catch(() => []);
    setOlder((cur) => [...cur, ...more]);
    if (more.length < 60) setOlderDone(true);
  };

  return (
    <aside className="flex w-full shrink-0 flex-col border-r md:w-[22rem] border-border/50 bg-gradient-to-b from-card/70 to-card/30">
      <div className="space-y-2.5 px-3 pt-3 pb-2">
        <div className="flex items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="İsim, numara, etiket ya da #numara" className="h-9 w-full rounded-full border border-transparent bg-muted/60 pl-9 pr-8 text-sm outline-none transition-[background-color,box-shadow] placeholder:text-muted-foreground/60 focus:border-ring/40 focus:bg-card focus:ring-4 focus:ring-ring/15" />
            {q && (
              <button type="button" onClick={() => setQ("")} aria-label="Temizle" className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-md p-0.5 text-muted-foreground hover:text-foreground"><X className="size-3.5" /></button>
            )}
          </div>
          {channels.length > 1 && (
            <select value={channel} onChange={(e) => setChannel(Number(e.target.value))} data-tip="Cihaza göre süz" className="h-9 max-w-[7.5rem] shrink-0 truncate rounded-full border border-transparent bg-muted/60 px-3 text-xs outline-none focus:border-ring/40">
              <option value={0}>Tüm cihazlar</option>
              {channels.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          )}
        </div>
        <div className="grid grid-cols-5 gap-1 rounded-2xl bg-muted/50 p-1">
          {TABS.map((t) => {
            const n = counts[t.key];
            const on = bucket === t.key;
            const urgent = t.key === "waiting" && n > 0;
            return (
              <button
                key={t.key}
                type="button"
                onClick={() => onBucket(t.key)}
                data-tip={t.tip}
                className={cn(
                  "relative flex flex-col items-center gap-0.5 rounded-xl px-1 py-1.5 text-[0.65rem] font-medium transition-colors",
                  on ? "bg-card text-foreground shadow-sm ring-1 ring-border/60" : "text-muted-foreground hover:text-foreground",
                )}
              >
                <t.icon className={cn("size-4", urgent && "text-destructive")} />
                <span className="truncate">{t.label}</span>
                {n > 0 && t.key !== "resolved" && (
                  <span className={cn("absolute -top-1 right-0.5 min-w-4 rounded-full px-1 text-[0.6rem] font-bold leading-4 tabular-nums", urgent ? "bg-destructive text-white animate-pulse" : "bg-primary/15 text-primary")}>{n > 99 ? "99+" : n}</span>
                )}
              </button>
            );
          })}
        </div>
      </div>

      <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-3">
        {!wa.loaded && Array.from({ length: 6 }).map((_, i) => <div key={i} className="mx-1 my-1 h-16 animate-pulse rounded-2xl bg-muted/40" />)}
        {wa.loaded && list.length === 0 && (
          <div className="flex flex-col items-center gap-2 px-6 py-12 text-center">
            <span className="flex size-12 items-center justify-center rounded-2xl bg-muted/70 text-muted-foreground"><Inbox className="size-5" /></span>
            <p className="text-sm font-medium">{q ? "Eşleşen sohbet yok" : EMPTY[bucket]}</p>
          </div>
        )}
        {list.map((c) => (
          <Row key={c.id} c={c} me={wa.me} now={now} active={c.id === activeId} typing={wa.typing(c.id)} showChannel={channels.length > 1} onOpen={() => onOpen(c.id)} />
        ))}
        {bucket === "resolved" && list.length >= 20 && !olderDone && (
          <button type="button" onClick={() => void loadOlder()} className="mx-auto mt-2 block rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground">Daha eski sohbetler</button>
        )}
      </div>
    </aside>
  );
}

const EMPTY: Record<Bucket, string> = {
  mine: "Şu an sende açık sohbet yok",
  waiting: "Cevap bekleyen müşteri yok",
  pool: "Havuz boş",
  team: "Açık sohbet yok",
  resolved: "Çözülmüş sohbet yok",
};

function Row({ c, me, now, active, typing, showChannel, onOpen }: { c: WAConversation; me: number; now: number; active: boolean; typing: string | null; showChannel: boolean; onOpen: () => void }) {
  const t = c.ticket;
  const waiting = isWaiting(c);
  const mine = isMine(c, me);
  const last = c.last;
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "group relative flex w-full items-center gap-3 rounded-2xl px-2.5 py-2 text-left transition-[background-color,box-shadow] duration-200",
        active ? "bg-card shadow-sm ring-1 ring-border/60" : "hover:bg-card/60",
      )}
    >
      {waiting && <span aria-hidden className="absolute inset-y-2 left-0 w-1 rounded-r-full bg-destructive" />}
      <ContactAvatar name={c.contact.display} seed={c.contact.waId} className="size-11">
        {t?.status === "bot" && (
          <span className="absolute -right-0.5 -bottom-0.5 flex size-4.5 items-center justify-center rounded-full bg-violet-500 text-white ring-2 ring-card" data-tip="Chatbot ile konuşuyor"><Bot className="size-2.5" /></span>
        )}
      </ContactAvatar>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className={cn("min-w-0 flex-1 truncate text-sm", c.unread > 0 ? "font-semibold text-foreground" : "font-medium")}>{c.contact.display}</span>
          <span className={cn("shrink-0 text-[0.65rem] tabular-nums", c.unread > 0 ? "font-semibold text-primary" : "text-muted-foreground")}>{listTime(last?.at)}</span>
        </span>
        <span className="flex items-center gap-1.5">
          {typing ? (
            <span className="min-w-0 flex-1 truncate text-xs italic text-primary">{typing}</span>
          ) : (
            <>
              {last && last.direction === "out" && <Ticks status={last.status} />}
              <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {last ? (
                  <>
                    {last.direction === "out" && last.senderName ? <span className="text-foreground/70">{last.senderName}: </span> : null}
                    {last.direction === "note" ? <span className="text-amber-600 dark:text-amber-400">Not: </span> : null}
                    {last.preview || " "}
                  </>
                ) : "Henüz mesaj yok"}
              </span>
            </>
          )}
          {c.unread > 0 && <span className="shrink-0 rounded-full bg-primary px-1.5 py-0.5 text-[0.6rem] font-bold leading-none tabular-nums text-primary-foreground shadow-sm shadow-primary/30">{c.unread > 99 ? "99+" : c.unread}</span>}
        </span>
        <span className="mt-1 flex items-center gap-1.5 text-[0.65rem] text-muted-foreground">
          {t?.owner ? (
            <span className="flex min-w-0 items-center gap-1" data-tip={`Sorumlu: ${t.owner.name}`}>
              <UserAvatar userId={t.owner.id} name={t.owner.name} hasAvatar={t.owner.hasAvatar} version={t.owner.avatarVersion} className="size-4" fallbackClassName="bg-primary/10 text-[0.45rem] text-primary" />
              <span className={cn("truncate", mine && "font-medium text-foreground/80")}>{t.owner.id === me ? "Sen" : t.owner.name.split(" ")[0]}</span>
              {t.participants.length > 1 && <span className="text-muted-foreground/70">+{t.participants.length - 1}</span>}
            </span>
          ) : t && t.status !== "resolved" && t.status !== "bot" ? (
            <span className="rounded-full bg-muted px-1.5 py-px font-medium">Havuzda</span>
          ) : null}
          {waiting && t?.awaitingSince && (
            <span className="flex items-center gap-0.5 rounded-full bg-destructive/10 px-1.5 py-px font-semibold text-destructive" data-tip="Müşteri bu kadar süredir cevap bekliyor"><Hourglass className="size-2.5" />{since(t.awaitingSince, now)}</span>
          )}
          {t && (t.priority === "high" || t.priority === "urgent") && (
            <span className={cn("rounded-full px-1.5 py-px font-semibold", t.priority === "urgent" ? "bg-destructive/10 text-destructive" : "bg-warning/12 text-warning")}>{t.priority === "urgent" ? "Acil" : "Yüksek"}</span>
          )}
          {t?.status === "pending" && <span className="rounded-full bg-sky-500/10 px-1.5 py-px font-medium text-sky-600 dark:text-sky-400">Müşteri bekleniyor</span>}
          {showChannel && <span className="ml-auto truncate text-muted-foreground/70">{c.channelName}</span>}
        </span>
      </span>
    </button>
  );
}
