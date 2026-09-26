// ChatPane is the open conversation: its header with the actions that fit
// (Karşıla, Devral, Aktar, Çöz), the messages, and the composer. Messages
// arrive on the live stream; what the agent sends shows at once and gets
// its ticks as WhatsApp reports them.

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowLeft, ArrowRightLeft, Bot, Download, CheckCircle2, ChevronDown, Hand, Hourglass, PanelRightClose, PanelRightOpen, Phone, RotateCcw, Search, UserCheck, X } from "lucide-react";
import { ApiError } from "@/api/client";
import { useAuth } from "@/auth/AuthContext";
import { ConfirmDialog } from "@/components/ui";
import UserAvatar from "@/components/ui/UserAvatar";
import AssignDialog from "@/components/whatsapp/AssignDialog";
import Composer, { type ComposerSend } from "@/components/whatsapp/Composer";
import ContactAvatar from "@/components/whatsapp/ContactAvatar";
import MessageBubble from "@/components/whatsapp/MessageBubble";
import TemplatePicker, { type TemplateChoice } from "@/components/whatsapp/TemplatePicker";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation, WAMessage, WAQuickReply, WASearchHit } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { dayLabel, hm, isMine, newClientId, prettyPhone, since, STATUS_WORD, windowLeft } from "@/whatsapp/util";

function upsert(list: WAMessage[], m: WAMessage): WAMessage[] {
  const i = list.findIndex((x) => x.id === m.id || (!!m.clientId && x.clientId === m.clientId));
  if (i >= 0) {
    const next = list.slice();
    next[i] = m;
    return next;
  }
  const next = [...list, m];
  next.sort((a, b) => (a.pending ? 1e15 : a.id) - (b.pending ? 1e15 : b.id));
  return next;
}

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

  const send = useCallback(async (s: ComposerSend) => {
    setError(null);
    stick.current = true;
    const clientId = newClientId();
    const base: WAMessage = {
      id: -Date.now(), conversationId: conv.id, clientId, direction: s.mode === "note" ? "note" : "out", kind: s.file ? "document" : "text",
      body: s.text, status: "queued", pending: true, createdAt: new Date().toISOString(),
      sender: { kind: "agent", userId: me, name: user?.name, hasAvatar: user?.hasAvatar, avatarVersion: user?.avatarVersion },
    };
    if (s.file) base.media = { url: "", mime: s.file.type, name: s.file.name, size: s.file.size };
    setMessages((cur) => [...cur, base]);
    const reply = replyTo?.id;
    setReplyTo(null);
    try {
      const saved = s.mode === "note"
        ? await waApi.note(conv.id, s.text)
        : s.file
          ? await waApi.sendMedia(conv.id, s.file, s.text, clientId, reply)
          : await waApi.send(conv.id, { clientId, kind: "text", body: s.text, replyTo: reply });
      setMessages((cur) => upsert(cur.filter((m) => m.clientId !== clientId || m.id === saved.id), { ...saved, clientId }));
    } catch (e) {
      setMessages((cur) => cur.filter((m) => m.clientId !== clientId));
      setError(e instanceof ApiError ? e.message : "Gönderilemedi.");
      throw e;
    }
  }, [conv.id, me, user, replyTo]);

  const sendTemplate = async (c: TemplateChoice) => {
    const saved = await waApi.send(conv.id, { clientId: newClientId(), kind: "template", templateId: c.templateId, params: c.params });
    setMessages((cur) => upsert(cur, saved));
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

  const claimLabel = greeting ? "Karşıla" : t?.owner ? "Yardıma katıl" : "Üstlen";
  const claimHint = t?.owner
    ? `${t.owner.name} ilgileniyor. Katılırsan sen de sohbete eklenirsin, kimse çıkarılmaz.${greeting ? " Karşılama mesajın gönderilir." : ""}`
    : `Bu sohbeti kimse üstlenmedi.${greeting ? " Karşıla dersen sohbet sana geçer ve karşılama mesajın gönderilir." : " Üstlenirsen sohbet sana geçer."}`;

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-background">
      <header className="flex h-16 shrink-0 items-center gap-3 border-b border-border/50 bg-card/60 px-4 backdrop-blur-md max-md:gap-2 max-md:px-2">
        {onBack && <button type="button" onClick={onBack} aria-label="Sohbet listesine dön" className="flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-accent md:hidden"><ArrowLeft className="size-4" /></button>}
        <ContactAvatar name={conv.contact.display} seed={conv.contact.waId} className="size-10 max-sm:hidden" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-[0.9375rem] font-semibold tracking-tight">{conv.contact.display}</h2>
            {t && <span className={cn("shrink-0 rounded-full px-2 py-0.5 text-[0.6rem] font-semibold max-sm:hidden", t.status === "resolved" ? "bg-muted text-muted-foreground" : t.status === "bot" ? "bg-violet-500/12 text-violet-600 dark:text-violet-400" : t.status === "pending" ? "bg-sky-500/12 text-sky-600 dark:text-sky-400" : "bg-success/12 text-success")}>{STATUS_WORD[t.status]}</span>}
            {t?.waitingListedAt && !resolved && <span className="flex shrink-0 items-center gap-1 rounded-full bg-destructive/10 px-2 py-0.5 text-[0.6rem] font-semibold text-destructive"><Hourglass className="size-3" /> {since(t.awaitingSince, now)} cevap bekliyor</span>}
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {typing ? <span className="italic text-primary">{typing}</span> : (
              <>
                <span className="font-mono tabular-nums">{prettyPhone(conv.contact.waId)}</span> · {conv.channelName}
                {t && ` · #${t.number}`}
                {left > 0 ? <span data-tip="Müşterinin son mesajından sonraki 24 saat içinde serbestçe yazılabilir"> · pencere {hm(left)} daha açık</span> : <span className="text-warning"> · 24 saat doldu</span>}
              </>
            )}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <IconBtn tip="Sohbette ara" on={searching} onClick={() => setSearching((v) => !v)}><Search className="size-4" /></IconBtn>
          {can(user, "whatsapp.export") && <span className="max-md:hidden"><IconBtn tip="Yazışmayı dosya olarak indir" onClick={() => void waApi.exportChat(conv.id).catch((e) => setError(e instanceof ApiError ? e.message : "İndirilemedi."))}><Download className="size-4" /></IconBtn></span>}
          {canCall && <IconBtn tip="Müşteriyi ara" onClick={() => void phone.call("0" + conv.contact.waId.replace(/^90/, "")).catch(() => undefined)}><Phone className="size-4" /></IconBtn>}
          {canTake && t && t.owner && t.owner.id !== me && !resolved && <TextBtn icon={Hand} label="Devral" tip="Sorumlu sen olursun, şimdiki sorumlu yardımcı olarak kalır" busy={busy === "take"} onClick={() => void act("take", () => waApi.take(conv.id))} />}
          {canAssign && t && !resolved && <TextBtn icon={ArrowRightLeft} label="Aktar" tip="Başka bir kişiye ya da ekibe aktar" onClick={() => setAssign(true)} />}
          {canResolve && t && !resolved && <TextBtn icon={CheckCircle2} label="Çöz" tone="success" tip="Sohbeti çözüldü olarak kapat. Anket açıksa müşteriye gider." onClick={() => setConfirmResolve(true)} />}
          {canResolve && resolved && <TextBtn icon={RotateCcw} label="Yeniden aç" tip="Sohbeti tekrar açık yap" busy={busy === "reopen"} onClick={() => void act("reopen", () => waApi.reopen(conv.id))} />}
          <IconBtn tip={panel ? "Müşteri panelini gizle" : "Müşteri panelini göster"} onClick={onPanel}>{panel ? <PanelRightClose className="size-4" /> : <PanelRightOpen className="size-4" />}</IconBtn>
        </div>
      </header>

      {searching && (
        <div className="relative border-b border-border/50 bg-card/40 px-4 py-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input autoFocus value={sq} onChange={(e) => setSq(e.target.value)} placeholder="Bu sohbette ara" className="h-9 w-full rounded-full border border-border/60 bg-card pl-9 pr-9 text-sm outline-none focus:border-ring/50" />
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

      <div ref={list} onScroll={onScroll} className="relative min-h-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_1px_1px,color-mix(in_oklab,var(--foreground)_5%,transparent)_1px,transparent_0)] bg-[size:20px_20px] pb-4">
        {loading && messages.length === 0 && (
          <div className="space-y-3 p-6">{[0, 1, 2, 3].map((i) => <div key={i} className={cn("h-12 w-1/2 animate-pulse rounded-2xl bg-muted/50", i % 2 && "ml-auto")} />)}</div>
        )}
        {older && messages.length > 0 && <p className="py-3 text-center text-[0.7rem] text-muted-foreground">{loading ? "Yükleniyor..." : "Yukarı kaydırınca eski mesajlar gelir"}</p>}
        {messages.map((m, i) => {
          const prev = messages[i - 1];
          const newDay = !prev || new Date(prev.createdAt).toDateString() !== new Date(m.createdAt).toDateString();
          const sameSide = prev && prev.direction === m.direction && prev.direction !== "event" && (prev.sender.userId ?? prev.sender.kind) === (m.sender.userId ?? m.sender.kind) && Date.parse(m.createdAt) - Date.parse(prev.createdAt) < 5 * 60000;
          return (
            <div key={m.clientId ?? m.id}>
              {newDay && (
                <div className="my-4 flex items-center justify-center">
                  <span className="rounded-full border border-border/60 bg-card/90 px-3 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.12em] text-muted-foreground shadow-sm backdrop-blur">{dayLabel(m.createdAt)}</span>
                </div>
              )}
              <MessageBubble m={m} head={newDay || !sameSide} highlight={flash === m.id}
                onReply={canReply && !m.pending && m.direction !== "event" ? setReplyTo : undefined}
                onReact={canReply && open && m.direction !== "event" && !m.pending ? react : undefined}
                onRetry={canReply ? retry : undefined}
                onImage={setViewer}
              />
            </div>
          );
        })}
        {!atBottom && (
          <button type="button" onClick={() => { const el = list.current; if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" }); }} className="sticky bottom-3 left-full mr-4 ml-auto flex size-10 items-center justify-center rounded-full bg-card shadow-lg ring-1 ring-border/60" aria-label="En alta in">
            <ChevronDown className="size-5" />
          </button>
        )}
      </div>

      {error && <div className="flex items-center justify-between gap-2 border-t border-destructive/20 bg-destructive/10 px-4 py-2 text-xs text-destructive"><span>{error}</span><button type="button" onClick={() => setError(null)} aria-label="Kapat"><X className="size-3.5" /></button></div>}

      {canReply && t && !participant && !resolved && (
        <div className="flex items-center gap-3 border-t border-primary/20 bg-primary/5 px-4 py-2.5">
          {t.owner ? <UserAvatar userId={t.owner.id} name={t.owner.name} hasAvatar={t.owner.hasAvatar} version={t.owner.avatarVersion} className="size-8" fallbackClassName="bg-primary/10 text-xs text-primary" /> : <span className="flex size-8 items-center justify-center rounded-full bg-primary/10 text-primary">{t.status === "bot" ? <Bot className="size-4" /> : <UserCheck className="size-4" />}</span>}
          <p className="min-w-0 flex-1 text-xs text-foreground/80">{claimHint}</p>
          <button type="button" disabled={busy === "greet"} onClick={() => void act("greet", () => waApi.greet(conv.id))} className="flex shrink-0 items-center gap-1.5 rounded-full bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground shadow-md shadow-primary/30 transition-transform hover:scale-105 disabled:opacity-60">
            <Hand className="size-3.5" /> {busy === "greet" ? "Bekleyin..." : claimLabel}
          </button>
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
      <ConfirmDialog open={confirmResolve} title="Sohbet çözüldü mü?" description={channel?.settings.survey.mode && channel.settings.survey.mode !== "off" ? "Sohbet kapanır ve müşteriye değerlendirme anketi gider. Müşteri tekrar yazarsa sohbet yeniden açılır." : "Sohbet kapanır. Müşteri tekrar yazarsa yeniden açılır."} confirmLabel="Çözüldü olarak kapat" tone="warning" busy={busy === "resolve"}
        onConfirm={() => void act("resolve", () => waApi.resolve(conv.id)).then(() => setConfirmResolve(false))} onCancel={() => setConfirmResolve(false)} />
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
    <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className={cn("flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", on && "bg-primary/10 text-primary")}>
      {children}
    </button>
  );
}

function TextBtn({ icon: Icon, label, tip, onClick, busy, tone }: { icon: typeof Hand; label: string; tip: string; onClick: () => void; busy?: boolean; tone?: "success" }) {
  return (
    <button type="button" onClick={onClick} disabled={busy} data-tip={tip} className={cn("flex h-9 items-center gap-1.5 rounded-full px-3 text-xs font-semibold transition-colors disabled:opacity-60", tone === "success" ? "bg-success/12 text-success hover:bg-success/20" : "text-muted-foreground ring-1 ring-border/60 hover:bg-accent hover:text-foreground")}>
      <Icon className="size-3.5" /> <span className="max-lg:hidden">{label}</span>
    </button>
  );
}
