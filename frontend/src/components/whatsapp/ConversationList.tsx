// ConversationList is the inbox's left column: a search box, filter chips
// for "mine", "waiting for an answer", the pool, everything and resolved,
// and the conversations, two lines each. Pinned ones stay on top, muted
// ones count in grey. A right click (or a long press) opens a small menu
// to mark as read or unread, pin, mute or copy the number.

import { useEffect, useMemo, useRef, useState } from "react";
import { BellOff, Bot, Check, ChevronRight, Copy, EyeOff, Hand, Hourglass, Inbox, MailOpen, Mail, Pin, PinOff, Search, Bell, X } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import UserAvatar from "@/components/ui/UserAvatar";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import Ticks from "@/components/whatsapp/Ticks";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation, WAMute } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { inBucket, isWaiting, listTime, since, sortTime, type Bucket } from "@/whatsapp/util";

const CHIPS: { key: Bucket; label: string; tip: string }[] = [
  { key: "mine", label: "Benim", tip: "Sorumlu olduğun ya da yardım ettiğin sohbetler" },
  { key: "waiting", label: "Bekleyen", tip: "Müşteri belirlenen süreden uzun süredir cevap bekliyor" },
  { key: "pool", label: "Havuz", tip: "Henüz kimsenin üstlenmediği sohbetler" },
  { key: "team", label: "Tümü", tip: "Görebildiğin bütün açık sohbetler" },
  { key: "resolved", label: "Çözülen", tip: "Çözülmüş sohbetler" },
];

const EMPTY: Record<Bucket, string> = {
  mine: "Şu an sende açık sohbet yok",
  waiting: "Cevap bekleyen müşteri yok",
  pool: "Havuz boş",
  team: "Açık sohbet yok",
  resolved: "Çözülmüş sohbet yok",
};

export const MUTES: { key: WAMute; label: string }[] = [
  { key: "1h", label: "1 saat" },
  { key: "8h", label: "8 saat" },
  { key: "1d", label: "1 gün" },
  { key: "1w", label: "1 hafta" },
  { key: "always", label: "Ben açana kadar" },
];

export default function ConversationList({ channels, activeId, onOpen, bucket, onBucket, header }: { channels: WAChannel[]; activeId: number | null; onOpen: (id: number) => void; bucket: Bucket; onBucket: (b: Bucket) => void; header?: React.ReactNode }) {
  const wa = useWhatsApp();
  const { user } = useAuth();
  const [q, setQ] = useState("");
  const [channel, setChannel] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const [older, setOlder] = useState<WAConversation[]>([]);
  const [olderDone, setOlderDone] = useState(false);
  const [menu, setMenu] = useState<{ c: WAConversation; x: number; y: number } | null>(null);

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
      for (const t of CHIPS) if (inBucket(c, t.key, wa.me)) out[t.key]++;
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
      .sort((a, b) => {
        if (bucket === "waiting") return Date.parse(a.ticket?.awaitingSince ?? "") - Date.parse(b.ticket?.awaitingSince ?? "");
        const pa = wa.pinned(a.id) ? 1 : 0;
        const pb = wa.pinned(b.id) ? 1 : 0;
        return pb - pa || sortTime(b) - sortTime(a);
      });
  }, [wa, bucket, channel, q, older]);

  // Without "whatsapp.view_all" the inbox is only part of the picture; say
  // plainly which part.
  const limited = useMemo(() => {
    if (can(user, "whatsapp.view_all")) return "";
    const parts = ["sana ait"];
    if (can(user, "whatsapp.view_team")) parts.push("ekibinin");
    if (can(user, "whatsapp.pool")) parts.push("havuzdaki");
    if (can(user, "whatsapp.waiting")) parts.push("cevap bekleyen");
    return `${parts.length > 1 ? `${parts.slice(0, -1).join(", ")} ve ${parts[parts.length - 1]}` : parts[0]} sohbetleri`;
  }, [user]);

  const loadOlder = async () => {
    const last = list[list.length - 1];
    if (!last?.last) return;
    const more = await waApi.resolved(last.last.at, q).catch(() => []);
    setOlder((cur) => [...cur, ...more]);
    if (more.length < 60) setOlderDone(true);
  };

  return (
    <aside className="flex h-full w-full shrink-0 flex-col border-r border-border/60 bg-card">
      {header}
      <div className="space-y-2.5 px-3 pb-2">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Ara: isim, numara, etiket" className="h-10 w-full rounded-xl bg-muted/70 pl-10 pr-9 text-sm outline-none transition-[background-color,box-shadow] placeholder:text-muted-foreground/70 focus:bg-card focus:ring-2 focus:ring-wa-accent/40" />
          {q && <button type="button" onClick={() => setQ("")} aria-label="Temizle" className="absolute right-3 top-1/2 -translate-y-1/2 rounded-md p-0.5 text-muted-foreground hover:text-foreground"><X className="size-4" /></button>}
        </div>
        <div className="-mx-3 flex gap-1 overflow-x-auto px-3 pb-0.5 [scrollbar-width:none]">
          {CHIPS.map((t) => {
            const n = counts[t.key];
            const on = bucket === t.key;
            const urgent = t.key === "waiting" && n > 0;
            return (
              <button key={t.key} type="button" onClick={() => onBucket(t.key)} data-tip={t.tip}
                className={cn("flex h-8 shrink-0 items-center gap-1 rounded-full px-3 text-[0.8rem] font-medium transition-colors",
                  on ? "bg-wa-accent/15 text-wa-accent" : "bg-muted/70 text-muted-foreground hover:bg-accent hover:text-foreground")}>
                {t.label}
                {n > 0 && t.key !== "resolved" && <span className={cn("min-w-[1.1rem] rounded-full px-1 text-center text-[0.65rem] font-bold leading-[1.1rem] tabular-nums", urgent ? "bg-destructive text-white" : on ? "bg-wa-accent text-wa-on-accent" : "bg-foreground/10")}>{n > 99 ? "99+" : n}</span>}
              </button>
            );
          })}
          {channels.length > 1 && (
            <select value={channel} onChange={(e) => setChannel(Number(e.target.value))} data-tip="Numaraya göre süz" className={cn("h-8 shrink-0 rounded-full px-3 text-[0.8rem] font-medium outline-none", channel ? "bg-wa-accent/15 text-wa-accent" : "bg-muted/70 text-muted-foreground")}>
              <option value={0}>Tüm numaralar</option>
              {channels.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          )}
        </div>
        {limited && (
          <p className="flex items-start gap-2 rounded-xl bg-warning/10 px-3 py-2 text-[0.72rem] leading-snug text-warning">
            <EyeOff className="mt-px size-3.5 shrink-0" />
            <span><b className="font-semibold">Tüm talepleri görme yetkin yok.</b> Sadece {limited} görüyorsun.</span>
          </p>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto pb-3">
        {!wa.loaded && Array.from({ length: 7 }).map((_, i) => (
          <div key={i} className="flex items-center gap-3 px-4 py-3">
            <div className="size-12 animate-pulse rounded-full bg-muted/70" />
            <div className="flex-1 space-y-2"><div className="h-3 w-2/3 animate-pulse rounded bg-muted/70" /><div className="h-3 w-1/2 animate-pulse rounded bg-muted/50" /></div>
          </div>
        ))}
        {wa.loaded && list.length === 0 && (
          <div className="flex flex-col items-center gap-2 px-6 py-14 text-center">
            <span className="flex size-14 items-center justify-center rounded-full bg-muted/70 text-muted-foreground"><Inbox className="size-6" /></span>
            <p className="text-sm font-medium">{q ? "Eşleşen sohbet yok" : EMPTY[bucket]}</p>
          </div>
        )}
        {list.map((c) => (
          <Row key={c.id} c={c} me={wa.me} now={now} active={c.id === activeId} typing={wa.typing(c.id)} showChannel={channels.length > 1} muted={wa.muted(c.id)} pinned={wa.pinned(c.id)}
            onOpen={() => onOpen(c.id)} onMenu={(x, y) => setMenu({ c, x, y })} />
        ))}
        {bucket === "resolved" && list.length >= 20 && !olderDone && (
          <button type="button" onClick={() => void loadOlder()} className="mx-auto mt-2 block rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground">Daha eski sohbetler</button>
        )}
      </div>
      {menu && <ConversationMenu c={menu.c} x={menu.x} y={menu.y} onClose={() => setMenu(null)} />}
    </aside>
  );
}

function Row({ c, me, now, active, typing, showChannel, muted, pinned, onOpen, onMenu }: { c: WAConversation; me: number; now: number; active: boolean; typing: string | null; showChannel: boolean; muted: boolean; pinned: boolean; onOpen: () => void; onMenu: (x: number, y: number) => void }) {
  const t = c.ticket;
  const waiting = isWaiting(c);
  const last = c.last;
  const unread = c.unread > 0;
  const press = useRef<number | null>(null);
  return (
    <button
      type="button"
      onClick={onOpen}
      onContextMenu={(e) => { e.preventDefault(); onMenu(e.clientX, e.clientY); }}
      onTouchStart={(e) => { const p = e.touches[0]; press.current = window.setTimeout(() => onMenu(p.clientX, p.clientY), 550); }}
      onTouchEnd={() => { if (press.current) window.clearTimeout(press.current); }}
      onTouchMove={() => { if (press.current) window.clearTimeout(press.current); }}
      className={cn("group flex w-full items-center gap-3 pl-3 pr-0 text-left transition-colors", active ? "bg-accent" : "hover:bg-accent/50")}
    >
      <ContactAvatar name={c.contact.display} seed={c.contact.waId} className="my-2 size-12 shrink-0 text-sm">
        {t?.status === "bot" && <span className="absolute -right-0.5 -bottom-0.5 flex size-5 items-center justify-center rounded-full bg-violet-500 text-white ring-2 ring-card" data-tip="Chatbot ile konuşuyor"><Bot className="size-3" /></span>}
      </ContactAvatar>
      <span className={cn("min-w-0 flex-1 border-b py-3 pr-3", active ? "border-transparent" : "border-border/50")}>
        <span className="flex items-baseline gap-2">
          <span className={cn("min-w-0 flex-1 truncate text-[0.95rem]", unread ? "font-semibold" : "font-medium")}>{c.contact.display}</span>
          <span className={cn("shrink-0 text-[0.7rem] tabular-nums", unread && !muted ? "font-semibold text-wa-accent" : "text-muted-foreground")}>{listTime(last?.at)}</span>
        </span>
        <span className="mt-0.5 flex items-center gap-1.5">
          {typing ? (
            <span className="min-w-0 flex-1 truncate text-[0.8rem] font-medium text-wa-accent">{typing}</span>
          ) : (
            <>
              {last && last.direction === "out" && <Ticks status={last.status} className="size-4" />}
              <span className={cn("min-w-0 flex-1 truncate text-[0.8rem]", unread ? "text-foreground/85" : "text-muted-foreground")}>
                {last ? (
                  <>
                    {last.direction === "out" && last.senderName ? <span>{last.senderName.split(" ")[0]}: </span> : null}
                    {last.direction === "note" ? <span className="text-amber-600 dark:text-amber-400">Not: </span> : null}
                    {last.preview || " "}
                  </>
                ) : "Henüz mesaj yok"}
              </span>
            </>
          )}
          <span className="flex shrink-0 items-center gap-1">
            {waiting && t?.awaitingSince && <span className="flex items-center gap-0.5 rounded-full bg-destructive/10 px-1.5 py-px text-[0.62rem] font-semibold text-destructive" data-tip="Müşteri bu kadar süredir cevap bekliyor"><Hourglass className="size-2.5" />{since(t.awaitingSince, now)}</span>}
            {muted && <BellOff className="size-3.5 text-muted-foreground" aria-label="Sessizde" />}
            {pinned && <Pin className="size-3.5 rotate-45 text-muted-foreground" aria-label="Sabitlendi" />}
            {unread && <span className={cn("min-w-5 rounded-full px-1.5 text-center text-[0.68rem] font-bold leading-5 tabular-nums", muted ? "bg-muted-foreground/25 text-foreground/80" : "bg-wa-accent text-wa-on-accent")}>{c.unread > 99 ? "99+" : c.unread}</span>}
          </span>
        </span>
        <Handler c={c} me={me} showChannel={showChannel} />
      </span>
    </button>
  );
}

// Handler is the last line of a row: who is looking after the conversation
// (their photo and first name, "+2" when others help), or that it waits in
// the pool or talks to the chatbot. The number it came to is added when there
// are several.
function Handler({ c, me, showChannel }: { c: WAConversation; me: number; showChannel: boolean }) {
  const t = c.ticket;
  const owner = t?.owner;
  const helpers = (t?.participants ?? []).filter((p) => p.id !== owner?.id);
  const channel = showChannel ? <span className="truncate">{c.channelName}</span> : null;
  let who: React.ReactNode = null;
  if (owner) {
    const mine = owner.id === me;
    // everyone on it, the one responsible first; three faces at most
    const all = [owner, ...helpers];
    const shown = all.slice(0, 3);
    const extra = all.length - shown.length;
    who = (
      <span className="flex min-w-0 items-center gap-1.5" data-tip={`Sorumlu: ${owner.name}${helpers.length ? ` · yardım eden: ${helpers.map((h) => h.name).join(", ")}` : ""}`}>
        <span className="flex shrink-0 -space-x-1.5">
          {shown.map((p) => (
            <UserAvatar key={p.id} userId={p.id} name={p.name} hasAvatar={p.hasAvatar} version={p.avatarVersion} className="size-5 ring-2 ring-card" fallbackClassName="bg-primary/15 text-[0.45rem] text-primary" />
          ))}
          {extra > 0 && <span className="relative flex size-5 items-center justify-center rounded-full bg-muted text-[0.52rem] font-bold text-muted-foreground ring-2 ring-card">+{extra}</span>}
        </span>
        <span className={cn("truncate", mine && "font-semibold text-wa-accent")}>{mine ? "Sen" : owner.name.split(" ")[0]}</span>
      </span>
    );
  } else if (t?.status === "bot") {
    who = <span className="shrink-0 font-medium text-violet-600 dark:text-violet-400">Chatbot'ta</span>;
  } else if (t && t.status !== "resolved") {
    who = <span className="shrink-0 font-medium text-amber-600 dark:text-amber-400">Havuzda</span>;
  }
  if (!who && !channel) return null;
  return (
    <span className="mt-1 flex min-w-0 items-center gap-1.5 text-[0.7rem] text-muted-foreground">
      {who}
      {who && channel && <span className="shrink-0 opacity-50">·</span>}
      {channel}
    </span>
  );
}

// ConversationMenu is the right-click menu of a conversation.
function ConversationMenu({ c, x, y, onClose }: { c: WAConversation; x: number; y: number; onClose: () => void }) {
  const wa = useWhatsApp();
  const { user } = useAuth();
  const [muteOpen, setMuteOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });
  const muted = wa.muted(c.id);
  const pinned = wa.pinned(c.id);
  const t = c.ticket;
  const canClaim = can(user, "whatsapp.reply") && t && !t.owner && t.status !== "resolved" && t.status !== "bot";

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({ left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)), top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) });
  }, [x, y, muteOpen]);

  useEffect(() => {
    const close = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) onClose(); };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    const t = window.setTimeout(() => {
      window.addEventListener("mousedown", close);
      window.addEventListener("keydown", esc);
      window.addEventListener("resize", onClose);
    }, 0);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
      window.removeEventListener("resize", onClose);
    };
  }, [onClose]);

  const run = (fn: () => Promise<unknown> | void) => {
    onClose();
    void Promise.resolve(fn()).catch(() => undefined);
  };

  return (
    <div ref={box} className="animate-in fade-in zoom-in-95 fixed z-[70] w-60 origin-top-left overflow-hidden rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-xl duration-100" style={pos} onContextMenu={(e) => e.preventDefault()}>
      <p className="truncate px-3 pt-1 pb-1.5 text-xs font-semibold text-muted-foreground">{c.contact.display}</p>
      {c.unread > 0
        ? <Item icon={MailOpen} label="Okundu olarak işaretle" onClick={() => run(() => wa.markRead(c.id))} />
        : <Item icon={Mail} label="Okunmadı olarak işaretle" onClick={() => run(() => wa.markUnread(c.id))} />}
      <Item icon={pinned ? PinOff : Pin} label={pinned ? "Sabitlemeyi kaldır" : "Sabitle"} onClick={() => run(() => wa.setConvPref(c.id, { pin: !pinned }))} />
      {muted ? (
        <Item icon={Bell} label="Sesi aç" onClick={() => run(() => wa.setConvPref(c.id, { mute: "off" }))} />
      ) : (
        <>
          <Item icon={BellOff} label="Sessize al" trailing={<ChevronRight className={cn("size-4 transition-transform", muteOpen && "rotate-90")} />} onClick={() => setMuteOpen((v) => !v)} />
          {muteOpen && (
            <div className="mb-1 ml-8 space-y-0.5 border-l border-border/60 pl-1.5">
              {MUTES.map((m) => <Item key={m.key} label={m.label} onClick={() => run(() => wa.setConvPref(c.id, { mute: m.key }))} small />)}
            </div>
          )}
        </>
      )}
      {canClaim && <Item icon={Hand} label="Sohbeti üstlen" onClick={() => run(() => waApi.greet(c.id))} />}
      <div className="my-1 h-px bg-border/70" />
      <Item icon={Copy} label="Numarayı kopyala" onClick={() => run(() => navigator.clipboard?.writeText("0" + c.contact.waId.replace(/^90/, "")))} />
    </div>
  );
}

function Item({ icon: Icon, label, onClick, trailing, small }: { icon?: typeof Check; label: string; onClick: () => void; trailing?: React.ReactNode; small?: boolean }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-3 rounded-xl px-3 text-left transition-colors hover:bg-accent", small ? "py-1.5 text-[0.8rem]" : "py-2 text-sm")}>
      {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" />}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {trailing}
    </button>
  );
}
