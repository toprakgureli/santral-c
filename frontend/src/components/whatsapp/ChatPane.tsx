// ChatPane is the open conversation: its header with the actions that fit
// (Karşıla, Devral, Aktar, Çöz), the messages, and the composer. Messages
// arrive on the live stream; what the agent sends shows at once and gets
// its ticks as WhatsApp reports them.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, ArrowRightLeft, Bell, BellOff, Bot, ChevronDown, ChevronRight, CircleCheck, Download, EllipsisVertical, Hand, Hourglass, Info, Mail, Paperclip, Phone, Pin, PinOff, RotateCcw, Search, UserCheck, X } from "lucide-react";
import { MUTES } from "@/components/whatsapp/ConversationList";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { ConfirmDialog } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import AssignDialog from "@/components/whatsapp/AssignDialog";
import Composer, { MAX_FILES, type ComposerSend } from "@/components/whatsapp/Composer";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import MessageBubble from "@/components/whatsapp/MessageBubble";
import { MessageInfo, MessageMenu } from "@/components/whatsapp/MessageInfo";
import TemplatePicker, { type TemplateChoice } from "@/components/whatsapp/TemplatePicker";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation, WAMessage, WAQuickReply, WASearchHit } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { clock, dayLabel, hm, isMine, mergeMessage, newClientId, since, windowLeft } from "@/whatsapp/util";

const upsert = mergeMessage;

export default function ChatPane({ conv, channel, panel, onPanel, onBack }: { conv: WAConversation; channel?: WAChannel; panel: boolean; onPanel: () => void; onBack?: () => void }) {
  const { user } = useAuth();
  const wa = useWhatsApp();
  const phone = useSoftphoneContext();
  const me = user?.id ?? 0;
  const [messages, setMessages] = useState<WAMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [older, setOlder] = useState(true);
  const [replyTo, setReplyTo] = useState<WAMessage | null>(null);
  const [templates, setTemplates] = useState(false);
  const [assign, setAssign] = useState(false);
  const [confirmResolve, setConfirmResolve] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [exporting, setExporting] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [viewer, setViewer] = useState<WAMessage | null>(null);
  const [quick, setQuick] = useState<WAQuickReply[]>([]);
  const [now, setNow] = useState(() => Date.now());
  const [searching, setSearching] = useState(false);
  const [sq, setSq] = useState("");
  const [hits, setHits] = useState<WASearchHit[]>([]);
  const [flash, setFlash] = useState<number | null>(null);
  const [atBottom, setAtBottom] = useState(true);
  const list = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const keepFrom = useRef<number | null>(null);

  const t = conv.ticket;
  const canReply = can(user, "whatsapp.reply");
  const canNote = can(user, "whatsapp.note");
  const canTemplate = can(user, "whatsapp.template_send");
  const canTake = can(user, "whatsapp.take");
  const canAssign = can(user, "whatsapp.assign");
  const canResolve = can(user, "whatsapp.resolve");
  const canCall = can(user, "call.originate") && phone.status === "registered";
  const participant = isMine(conv, me);
  const resolved = t?.status === "resolved";
  const open = windowLeft(conv, now) > 0;
  const greeting = channel?.settings.greeting.enabled ?? false;

  useEffect(() => {
    const i = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(i);
  }, []);

  // Load the latest page when the chat changes.
  useEffect(() => {
    let live = true;
    setLoading(true);
    setMessages([]);
    setReplyTo(null);
    setError(null);
    setOlder(true);
    setSearching(false);
    stick.current = true;
    waApi.messages(conv.id).then((m) => {
      if (!live) return;
      setMessages(m);
      setOlder(m.length >= 60);
    }).catch((e) => live && setError(e instanceof ApiError ? e.message : "Mesajlar alınamadı.")).finally(() => live && setLoading(false));
    waApi.quickReplies(conv.channelId).then((r) => live && setQuick(r)).catch(() => undefined);
    return () => {
      live = false;
    };
  }, [conv.id, conv.channelId]);

  // New and changed messages from the stream.
  useEffect(() => wa.onMessage((m) => {
    if (m.conversationId !== conv.id) return;
    setMessages((cur) => upsert(cur, m));
  }), [wa, conv.id]);

  // Keep the view at the bottom while the agent is there.
  useLayoutEffect(() => {
    const el = list.current;
    if (!el) return;
    if (keepFrom.current !== null) {
      el.scrollTop = el.scrollHeight - keepFrom.current;
      keepFrom.current = null;
    } else if (stick.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [messages]);

  const onScroll = () => {
    const el = list.current;
    if (!el) return;
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    stick.current = bottom;
    setAtBottom(bottom);
    if (el.scrollTop < 60 && older && !loading && messages.length > 0) void loadOlder();
  };

  const loadOlder = async () => {
    const first = messages.find((m) => !m.pending);
    if (!first) return;
    setLoading(true);
    try {
      const page = await waApi.messages(conv.id, { before: first.id });
      keepFrom.current = (list.current?.scrollHeight ?? 0) - (list.current?.scrollTop ?? 0);
      setMessages((cur) => [...page, ...cur]);
      setOlder(page.length >= 60);
    } finally {
      setLoading(false);
    }
  };

  // Read: when the agent is looking, the team's badge drops.
  const lastIn = useMemo(() => [...messages].reverse().find((m) => m.direction === "in" && !m.pending)?.id ?? 0, [messages]);
  useEffect(() => {
    if (!lastIn || lastIn <= conv.teamReadId || document.visibilityState !== "visible") return;
    const tm = window.setTimeout(() => void waApi.read(conv.id, lastIn).catch(() => undefined), 500);
    return () => window.clearTimeout(tm);
  }, [lastIn, conv.id, conv.teamReadId]);
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible" && lastIn > conv.teamReadId) void waApi.read(conv.id, lastIn).catch(() => undefined);
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [lastIn, conv.id, conv.teamReadId]);

  const act = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "İşlem yapılamadı.");
    } finally {
      setBusy(null);
    }
  };

  // One message out: shown at once, then replaced by what the server keeps.
  const sendOne = useCallback(async (mode: "message" | "note", text: string, file: File | undefined, reply: number | undefined) => {
    const clientId = newClientId();
    const kind = !file ? "text" : file.type.startsWith("image/") ? "image" : file.type.startsWith("video/") ? "video" : file.type.startsWith("audio/") ? "audio" : "document";
    const base: WAMessage = {
      id: -Date.now() - Math.random(), conversationId: conv.id, clientId, direction: mode === "note" ? "note" : "out", kind,
      body: text, status: "queued", pending: true, createdAt: new Date().toISOString(),
      sender: { kind: "agent", userId: me, name: user?.name, hasAvatar: user?.hasAvatar, avatarVersion: user?.avatarVersion },
    };
    if (file) base.media = { url: file.type.startsWith("image/") ? URL.createObjectURL(file) : "", mime: file.type, name: file.name, size: file.size };
    setMessages((cur) => [...cur, base]);
    try {
      const saved = mode === "note"
        ? await waApi.note(conv.id, text)
        : file
          ? await waApi.sendMedia(conv.id, file, text, clientId, reply)
          : await waApi.send(conv.id, { clientId, kind: "text", body: text, replyTo: reply });
      setMessages((cur) => upsert(cur.filter((m) => m.clientId !== clientId || m.id === saved.id), { ...saved, clientId }));
    } catch (e) {
      setMessages((cur) => cur.filter((m) => m.clientId !== clientId));
      throw e;
    }
  }, [conv.id, me, user]);

  const send = useCallback(async (s: ComposerSend) => {
    setError(null);
    stick.current = true;
    const reply = replyTo?.id;
    setReplyTo(null);
    const files = s.mode === "message" ? s.files ?? [] : [];
    try {
      if (files.length === 0) {
        await sendOne(s.mode, s.text, undefined, reply);
        return;
      }
      // Files go one after another so they arrive in the order chosen; the
      // text rides with the first as its caption.
      for (let i = 0; i < files.length; i++) {
        try {
          await sendOne("message", i === 0 ? s.text : "", files[i], i === 0 ? reply : undefined);
        } catch (e) {
          // hand back only what did not go
          throw Object.assign(e instanceof Error ? e : new Error("Gönderilemedi."), { remaining: files.slice(i), textSent: i > 0 });
        }
      }
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
      throw e;
    }
  }, [sendOne, replyTo]);

  // Right click on a message, and its info panel.
  const [msgMenu, setMsgMenu] = useState<{ m: WAMessage; x: number; y: number } | null>(null);
  const [info, setInfo] = useState<WAMessage | null>(null);

  // Files dragged onto the chat go to the composer.
  const [drag, setDrag] = useState(false);
  const [dropped, setDropped] = useState<{ files: File[]; n: number }>({ files: [], n: 0 });
  const dragDepth = useRef(0);
  const hasFiles = (e: React.DragEvent) => Array.from(e.dataTransfer.types).includes("Files");
  const onDragEnter = (e: React.DragEvent) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current++;
    setDrag(true);
  };
  const onDragLeave = (e: React.DragEvent) => {
    if (!hasFiles(e)) return;
    dragDepth.current = Math.max(0, dragDepth.current - 1);
    if (dragDepth.current === 0) setDrag(false);
  };
  const onDrop = (e: React.DragEvent) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current = 0;
    setDrag(false);
    if (!canReply || !open || conv.contact.blocked) return;
    const list = Array.from(e.dataTransfer.files ?? []);
    if (list.length) setDropped((d) => ({ files: list, n: d.n + 1 }));
  };

  const sendTemplate = async (c: TemplateChoice) => {
    const saved = await waApi.send(conv.id, { clientId: newClientId(), kind: "template", templateId: c.templateId, params: c.params });
    setMessages((cur) => upsert(cur, saved));
  };

  // A resolved conversation closes; the list stays where it was.
  const resolveNow = async () => {
    setBusy("resolve");
    setError(null);
    try {
      await waApi.resolve(conv.id);
      setConfirmResolve(false);
      onBack?.();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Çözülemedi.");
      setConfirmResolve(false);
    } finally {
      setBusy(null);
    }
  };

  const react = (m: WAMessage, emoji: string) => void act("react", () => waApi.send(conv.id, { clientId: newClientId(), kind: "reaction", targetId: m.id, emoji }));
  const retry = (m: WAMessage) => void act("retry", () => waApi.retry(m.id));

  // Search inside this chat and jump to a hit.
  useEffect(() => {
    if (!searching || sq.trim().length < 2) {
      setHits([]);
      return;
    }
    const tm = window.setTimeout(() => waApi.search(sq, conv.id).then(setHits).catch(() => setHits([])), 300);
    return () => window.clearTimeout(tm);
  }, [sq, searching, conv.id]);
  const jump = async (id: number) => {
    if (!messages.some((m) => m.id === id)) {
      const page = await waApi.messages(conv.id, { around: id });
      stick.current = false;
      setMessages(page);
      setOlder(true);
    }
    window.setTimeout(() => {
      document.getElementById(`wa-m-${id}`)?.scrollIntoView({ behavior: "smooth", block: "center" });
      setFlash(id);
      window.setTimeout(() => setFlash(null), 1800);
    }, 60);
  };

  const vars = useMemo(() => ({ musteri: conv.contact.display.split(" ")[0], ad: (user?.name ?? "").split(" ")[0], adsoyad: user?.name ?? "" }), [conv.contact.display, user?.name]);
  const typing = wa.typing(conv.id);
  const left = windowLeft(conv, now);
  const [more, setMore] = useState(false);
  const [muteOpen, setMuteOpen] = useState(false);
  useEffect(() => { if (!more) setMuteOpen(false); }, [more]);
  // Where it stands and how long the customer can still be written to. Who
  // handles it shows in the list and on the contact card.
  const subtitle = [
    t?.status === "bot" ? "Chatbot ile konuşuyor" : t?.status === "resolved" ? "Çözüldü" : t && !t.owner ? "Havuzda, kimse üstlenmedi" : null,
    left > 0 ? `${hm(left)} daha yazılabilir` : "24 saat doldu, şablonla yazılır",
  ].filter(Boolean).join(" · ");

  const claimLabel = greeting ? "Karşıla" : t?.owner ? "Yardıma katıl" : "Üstlen";
  const claimHint = t?.owner
    ? `${t.owner.name} ilgileniyor. Katılırsan sen de sohbete eklenirsin, kimse çıkarılmaz.${greeting ? " Karşılama mesajın gönderilir." : ""}`
    : `Bu sohbeti kimse üstlenmedi.${greeting ? " Karşıla dersen sohbet sana geçer ve karşılama mesajın gönderilir." : " Üstlenirsen sohbet sana geçer."}`;

  return (
    <section className="relative flex min-w-0 flex-1 flex-col bg-card" onDragEnter={onDragEnter} onDragLeave={onDragLeave} onDragOver={(e) => hasFiles(e) && e.preventDefault()} onDrop={onDrop}>
      {drag && (
        <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center bg-card/80 p-6 backdrop-blur-sm">
          <div className="flex max-w-md flex-col items-center gap-3 rounded-3xl border-2 border-dashed border-wa-accent/60 bg-card px-10 py-12 text-center shadow-xl">
            <span className="flex size-16 items-center justify-center rounded-full bg-wa-accent/15 text-wa-accent"><Paperclip className="size-7" /></span>
            {canReply && open && !conv.contact.blocked ? (
              <>
                <p className="text-lg font-semibold">Dosyaları buraya bırakın</p>
                <p className="text-sm text-muted-foreground">Görsel, video, ses ya da belge. Tek seferde en fazla {MAX_FILES} dosya; her biri ayrı mesaj olarak gider.</p>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">{!open ? "Müşterinin son mesajının üzerinden 24 saat geçti; dosya gönderilemez, önce şablonla yazın." : "Bu sohbete dosya gönderemezsiniz."}</p>
            )}
          </div>
        </div>
      )}
      <header className="flex h-16 shrink-0 items-center gap-2 border-b border-border/60 bg-card px-3 md:px-4">
        {onBack && <button type="button" onClick={onBack} aria-label="Sohbet listesine dön" className="flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent md:hidden"><ArrowLeft className="size-5" /></button>}
        <button type="button" onClick={onPanel} data-tip="Kişi bilgisi" className="flex min-w-0 flex-1 items-center gap-3 rounded-xl py-1 pr-2 text-left">
          <ContactAvatar name={conv.contact.display} seed={conv.contact.waId} className="size-10 shrink-0" />
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-2">
              <span className="truncate text-[0.95rem] font-semibold">{conv.contact.display}</span>
              {wa.muted(conv.id) && <BellOff className="size-3.5 shrink-0 text-muted-foreground" />}
              {t?.waitingListedAt && !resolved && <span data-tip={`Cevap beklemeye başladığı saat: ${clock(t.awaitingSince!)}. Süre, chatbot sohbeti aktardığında ya da müşterinin ilk cevapsız mesajında başlar; bir temsilci cevap yazınca sıfırlanır.`} className="flex shrink-0 items-center gap-1 rounded-full bg-destructive/10 px-2 py-0.5 text-[0.62rem] font-semibold text-destructive"><Hourglass className="size-3" /> Cevap bekliyor · {since(t.awaitingSince, now)}</span>}
            </span>
            <span className="block truncate text-[0.78rem] text-muted-foreground">
              {typing ? <span className="font-medium text-wa-accent">{typing}</span> : subtitle}
            </span>
          </span>
        </button>
        <div className="flex shrink-0 items-center gap-0.5">
          {canTake && t && t.owner && t.owner.id !== me && !resolved && <TextBtn icon={Hand} label="Devral" tip="Sorumlu sen olursun, şimdiki sorumlu yardımcı olarak kalır" busy={busy === "take"} onClick={() => void act("take", () => waApi.take(conv.id))} />}
          {canAssign && t && !resolved && <TextBtn icon={ArrowRightLeft} label="Aktar" tip="Başka bir kişiye ya da ekibe aktar" onClick={() => setAssign(true)} />}
          {canResolve && t && !resolved && <TextBtn icon={CircleCheck} label="Çöz" tone="success" tip="Sohbeti çözüldü olarak kapat. Anket açıksa müşteriye gider." onClick={() => setConfirmResolve(true)} />}
          {canResolve && resolved && <TextBtn icon={RotateCcw} label="Yeniden aç" tip="Sohbeti tekrar açık yap" busy={busy === "reopen"} onClick={() => void act("reopen", () => waApi.reopen(conv.id))} />}
          <IconBtn tip="Sohbette ara" on={searching} onClick={() => setSearching((v) => !v)}><Search className="size-[1.15rem]" /></IconBtn>
          {canCall && <IconBtn tip="Müşteriyi ara" onClick={() => void phone.call("0" + conv.contact.waId.replace(/^90/, "")).catch(() => undefined)}><Phone className="size-[1.15rem]" /></IconBtn>}
          <span className="relative">
            <IconBtn tip="Diğer" on={more} onClick={() => setMore((v) => !v)}><EllipsisVertical className="size-[1.15rem]" /></IconBtn>
            {more && (
              <MoreMenu onClose={() => setMore(false)}>
                <MenuItem icon={Info} label={panel ? "Kişi bilgisini kapat" : "Kişi bilgisi"} onClick={() => { setMore(false); onPanel(); }} />
                {wa.muted(conv.id) ? (
                  <MenuItem icon={Bell} label="Sesi aç" onClick={() => { setMore(false); void wa.setConvPref(conv.id, { mute: "off" }); }} />
                ) : (
                  <>
                    <MenuItem icon={BellOff} label="Sessize al" trailing={<ChevronRight className={cn("size-4 transition-transform", muteOpen && "rotate-90")} />} onClick={() => setMuteOpen((v) => !v)} />
                    {muteOpen && <div className="mb-1 ml-8 space-y-0.5 border-l border-border/60 pl-1.5">{MUTES.map((mu) => <MenuItem key={mu.key} label={mu.label} small onClick={() => { setMore(false); void wa.setConvPref(conv.id, { mute: mu.key }); }} />)}</div>}
                  </>
                )}
                <MenuItem icon={wa.pinned(conv.id) ? PinOff : Pin} label={wa.pinned(conv.id) ? "Sabitlemeyi kaldır" : "Sabitle"} onClick={() => { setMore(false); void wa.setConvPref(conv.id, { pin: !wa.pinned(conv.id) }); }} />
                <MenuItem icon={Mail} label="Okunmadı olarak işaretle" onClick={() => { setMore(false); void wa.markUnread(conv.id); }} />
                {can(user, "whatsapp.export") && <MenuItem icon={Download} label={exporting ? "Hazırlanıyor..." : "Yazışmayı indir"} onClick={() => {
                  setMore(false);
                  if (exporting) return;
                  setExporting(true);
                  void waApi.exportChat(conv.id).catch((e) => setError(e instanceof ApiError ? e.message : "İndirilemedi.")).finally(() => setExporting(false));
                }} />}
              </MoreMenu>
            )}
          </span>
        </div>
      </header>

      {searching && (
        <div className="relative border-b border-border/60 bg-card px-4 py-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input autoFocus value={sq} onChange={(e) => setSq(e.target.value)} placeholder="Bu sohbette ara" className="h-9 w-full rounded-xl bg-muted/70 pl-9 pr-9 text-sm outline-none focus:bg-card focus:ring-2 focus:ring-wa-accent/30" />
            <button type="button" onClick={() => { setSearching(false); setSq(""); }} aria-label="Aramayı kapat" className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"><X className="size-3.5" /></button>
          </div>
          {hits.length > 0 && (
            <div className="absolute inset-x-4 top-full z-20 mt-1 max-h-72 overflow-y-auto rounded-2xl border border-border bg-popover p-1 shadow-lg">
              {hits.map((h) => (
                <button key={h.messageId} type="button" onClick={() => void jump(h.messageId)} className="block w-full rounded-xl px-3 py-2 text-left hover:bg-accent">
                  <span className="block text-[0.65rem] text-muted-foreground">{new Date(h.at).toLocaleString("tr-TR")}</span>
                  <span className="block truncate text-sm">{h.snippet}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div ref={list} onScroll={onScroll} className="wa-wall relative min-h-0 flex-1 overflow-y-auto pb-4">
        {loading && messages.length === 0 && (
          <div className="space-y-3 p-6">{[0, 1, 2, 3].map((i) => <div key={i} className={cn("h-12 w-1/2 animate-pulse rounded-2xl bg-muted/50", i % 2 && "ml-auto")} />)}</div>
        )}
        {older && messages.length > 0 && <p className="py-3 text-center"><span className="rounded-lg bg-card/90 px-3 py-1 text-[0.7rem] text-muted-foreground shadow-sm">{loading ? "Yükleniyor..." : "Yukarı kaydırınca eski mesajlar gelir"}</span></p>}
        {messages.map((m, i) => {
          const prev = messages[i - 1];
          const newDay = !prev || new Date(prev.createdAt).toDateString() !== new Date(m.createdAt).toDateString();
          const sameSide = prev && prev.direction === m.direction && prev.direction !== "event" && (prev.sender.userId ?? prev.sender.kind) === (m.sender.userId ?? m.sender.kind) && Date.parse(m.createdAt) - Date.parse(prev.createdAt) < 5 * 60000;
          return (
            <div key={m.clientId ?? m.id}>
              {newDay && (
                <div className="my-4 flex items-center justify-center">
                  <span className="rounded-lg bg-card/95 px-3 py-1 text-[0.72rem] font-medium text-muted-foreground shadow-sm">{dayLabel(m.createdAt)}</span>
                </div>
              )}
              <MessageBubble m={m} head={newDay || !sameSide} highlight={flash === m.id}
                onReply={canReply && !m.pending && m.direction !== "event" ? setReplyTo : undefined}
                onReact={canReply && open && m.direction !== "event" && !m.pending ? react : undefined}
                onRetry={canReply ? retry : undefined}
                onImage={setViewer}
                onMenu={m.pending ? undefined : (msg, x, y) => setMsgMenu({ m: msg, x, y })}
              />
            </div>
          );
        })}
        {!atBottom && (
          <button type="button" onClick={() => { const el = list.current; if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" }); }} className="sticky bottom-3 left-full mr-4 ml-auto flex size-10 items-center justify-center rounded-full bg-card text-muted-foreground shadow-lg" aria-label="En alta in">
            <ChevronDown className="size-5" />
          </button>
        )}
      </div>

      {exporting && <div className="flex items-center gap-2 border-t border-border/60 bg-muted/60 px-4 py-2 text-xs text-muted-foreground"><Download className="size-3.5 animate-pulse" /> Yazışma, görseller ve videolarla birlikte hazırlanıyor. Bitince zip olarak iner; açıp içindeki sohbet.html dosyasına çift tıklayın.</div>}
      {error && <div className="flex items-center justify-between gap-2 border-t border-destructive/20 bg-destructive/10 px-4 py-2 text-xs text-destructive"><span>{error}</span><button type="button" onClick={() => setError(null)} aria-label="Kapat"><X className="size-3.5" /></button></div>}

      {canReply && t && !participant && !resolved && (
        <div className="wa-wall px-3 pb-2">
          <div className="mx-auto flex max-w-2xl items-center gap-3 rounded-2xl bg-card px-4 py-2.5 shadow-sm">
            {t.owner ? <UserAvatar userId={t.owner.id} name={t.owner.name} hasAvatar={t.owner.hasAvatar} version={t.owner.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" /> : <span className="flex size-8 items-center justify-center rounded-full bg-wa-accent/15 text-wa-accent">{t.status === "bot" ? <Bot className="size-4" /> : <UserCheck className="size-4" />}</span>}
            <p className="min-w-0 flex-1 text-xs text-foreground/80">{claimHint}</p>
            <button type="button" disabled={busy === "greet"} onClick={() => void act("greet", () => waApi.greet(conv.id))} className="flex shrink-0 items-center gap-1.5 rounded-full bg-wa-accent px-4 py-2 text-xs font-semibold text-wa-on-accent shadow-sm transition-transform hover:scale-105 disabled:opacity-60">
              <Hand className="size-3.5" /> {busy === "greet" ? "Bekleyin..." : claimLabel}
            </button>
          </div>
        </div>
      )}

      <Composer
        canReply={canReply && !conv.contact.blocked}
        canNote={canNote}
        canTemplate={canTemplate && !conv.contact.blocked}
        windowOpen={open}
        quickReplies={quick}
        vars={vars}
        replyTo={replyTo}
        onCancelReply={() => setReplyTo(null)}
        onSend={send}
        onTemplate={() => setTemplates(true)}
        onTyping={() => void waApi.typing(conv.id).catch(() => undefined)}
        dropped={dropped}
        onSuggest={wa.ai ? async (draft) => {
          try {
            return (await waApi.suggest(conv.id, draft)).text;
          } catch (e) {
            throw new Error(e instanceof ApiError ? e.message : "Öneri alınamadı.");
          }
        } : undefined}
        disabledReason={!canReply && !canNote ? "Bu sohbete yazma yetkiniz yok." : conv.contact.blocked && !canNote ? "Müşteri engellenmiş." : undefined}
      />

      <TemplatePicker channelId={conv.channelId} open={templates} onClose={() => setTemplates(false)} onSend={sendTemplate} defaults={vars} />
      <AssignDialog conv={conv} open={assign} onClose={() => setAssign(false)} />
      {msgMenu && (
        <MessageMenu m={msgMenu.m} x={msgMenu.x} y={msgMenu.y} onClose={() => setMsgMenu(null)} onInfo={() => setInfo(msgMenu.m)}
          onReply={canReply && msgMenu.m.direction !== "note" ? () => setReplyTo(msgMenu.m) : undefined}
          onReact={canReply && open && msgMenu.m.direction !== "note" && msgMenu.m.status !== "queued" ? (e) => react(msgMenu.m, e) : undefined} />
      )}
      {info && <MessageInfo m={info} onClose={() => setInfo(null)} />}
      <ConfirmDialog open={confirmResolve} title="Sohbet çözüldü mü?" description={channel?.settings.survey.mode && channel.settings.survey.mode !== "off" ? "Sohbet kapanır ve müşteriye değerlendirme anketi gider. Müşteri tekrar yazarsa sohbet yeniden açılır." : "Sohbet kapanır. Müşteri tekrar yazarsa yeniden açılır."} confirmLabel="Çözüldü olarak kapat" tone="warning" busy={busy === "resolve"}
        onConfirm={() => void resolveNow()} onCancel={() => setConfirmResolve(false)} />
      {viewer?.media && (
        <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/85 p-6" onClick={() => setViewer(null)}>
          <img src={viewer.media.url} alt="" className="max-h-full max-w-full rounded-xl object-contain shadow-2xl" />
          <a href={`${viewer.media.url}?download=1`} onClick={(e) => e.stopPropagation()} className="absolute right-16 top-5 rounded-full bg-white/10 px-3 py-1.5 text-xs font-semibold text-white hover:bg-white/20">İndir</a>
          <button type="button" onClick={() => setViewer(null)} aria-label="Kapat" className="absolute right-5 top-5 flex size-9 items-center justify-center rounded-full bg-white/10 text-white hover:bg-white/20"><X className="size-5" /></button>
        </div>
      )}
    </section>
  );
}

function IconBtn({ tip, on, onClick, children }: { tip: string; on?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className={cn("flex size-10 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", on && "bg-accent text-foreground")}>
      {children}
    </button>
  );
}

function TextBtn({ icon: Icon, label, tip, onClick, busy, tone }: { icon: typeof Hand; label: string; tip: string; onClick: () => void; busy?: boolean; tone?: "success" }) {
  return (
    <button type="button" onClick={onClick} disabled={busy} data-tip={tip} className={cn("mr-1 flex h-9 items-center gap-1.5 rounded-full px-3 text-xs font-semibold transition-colors disabled:opacity-60", tone === "success" ? "bg-wa-accent/15 text-wa-accent hover:bg-wa-accent/25" : "bg-muted/70 text-foreground/80 hover:bg-accent")}>
      <Icon className="size-4" /> <span className="max-lg:hidden">{label}</span>
    </button>
  );
}

function MoreMenu({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const close = (e: MouseEvent) => { if (!box.current?.parentElement?.contains(e.target as Node)) onClose(); };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [onClose]);
  return <div ref={box} className="animate-in fade-in zoom-in-95 absolute right-0 top-full z-30 mt-1 w-60 origin-top-right rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-xl duration-100">{children}</div>;
}

function MenuItem({ icon: Icon, label, onClick, trailing, small }: { icon?: typeof Hand; label: string; onClick: () => void; trailing?: React.ReactNode; small?: boolean }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-3 rounded-xl px-3 text-left transition-colors hover:bg-accent", small ? "py-1.5 text-[0.8rem]" : "py-2 text-sm")}>
      {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" />}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {trailing}
    </button>
  );
}
