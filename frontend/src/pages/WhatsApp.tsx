// WhatsApp: the inbox. Left, the conversations; middle, the open chat on its
// wall; right, when asked for, the contact card. On a phone one column
// shows at a time.

import { useEffect, useMemo, useRef, useState } from "react";
import { PAGE_PERMISSIONS } from "@/lib/menu";
import { Link, useNavigate, useParams } from "react-router-dom";
import { BarChart3, BellOff, EllipsisVertical, MessageCirclePlus, PhoneCall, Settings2, SlidersHorizontal, Smartphone, Star } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import WhatsAppIcon from "@/components/icons/WhatsAppIcon";
import ChatPane from "@/components/whatsapp/ChatPane";
import ConversationList from "@/components/whatsapp/ConversationList";
import TicketPanel from "@/components/whatsapp/TicketPanel";
import { can, canAny } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { type Bucket } from "@/whatsapp/util";

// The contact card opens beside the chat when a conversation opens, as long
// as the chat keeps at least this much room next to the list and the card.
const LIST_W = 384;
const CARD_W = 352;
const CHAT_MIN = 480;
// The list's width is the person's to choose, by dragging its edge.
const LIST_KEY = "santral.wa-list-width";
const LIST_MIN = 280;
const LIST_MAX = 640;
const BUCKET_KEY = "santral.wa-bucket";

export function WhatsApp() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const wa = useWhatsApp();
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const frame = useRef<HTMLDivElement>(null);
  const [listW, setListW] = useState(() => {
    try {
      const v = Number(localStorage.getItem(LIST_KEY));
      return v >= LIST_MIN && v <= LIST_MAX ? v : LIST_W;
    } catch {
      return LIST_W;
    }
  });
  const listRef = useRef(listW);
  listRef.current = listW;
  const [dragging, setDragging] = useState(false);
  const [wide, setWide] = useState(true);
  const [panel, setPanel] = useState(false);
  const [bucket, setBucket] = useState<Bucket>(() => {
    try {
      return (localStorage.getItem(BUCKET_KEY) as Bucket) || "mine";
    } catch {
      return "mine";
    }
  });
  const [fetched, setFetched] = useState<WAConversation | null>(null);
  const openId = id ? Number(id) : null;

  useEffect(() => {
    waApi.channels().then(setChannels).catch(() => setChannels([]));
  }, []);

  const { setOpenId } = wa;
  useEffect(() => {
    setOpenId(openId);
    return () => setOpenId(null);
  }, [openId, setOpenId]);

  // Is there room for the card beside the chat? Measured, not guessed.
  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    const check = () => setWide(el.clientWidth >= listRef.current + CARD_W + CHAT_MIN);
    check();
    const ro = new ResizeObserver(check);
    ro.observe(el);
    return () => ro.disconnect();
  }, [listW]);

  const startResize = (e: React.PointerEvent) => {
    e.preventDefault();
    const x0 = e.clientX;
    const w0 = listRef.current;
    const room = (frame.current?.clientWidth ?? 1400) - CHAT_MIN;
    setDragging(true);
    const move = (ev: PointerEvent) => setListW(Math.round(Math.max(LIST_MIN, Math.min(LIST_MAX, room, w0 + ev.clientX - x0))));
    const up = () => {
      setDragging(false);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      try {
        localStorage.setItem(LIST_KEY, String(listRef.current));
      } catch {
        // storage unavailable
      }
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };
  const resetWidth = () => {
    setListW(LIST_W);
    try {
      localStorage.removeItem(LIST_KEY);
    } catch {
      // storage unavailable
    }
  };

  // Every conversation opens with its card, where it fits.
  useEffect(() => {
    if (!openId) return;
    const el = frame.current;
    setPanel(!!el && el.clientWidth >= listRef.current + CARD_W + CHAT_MIN);
  }, [openId]);

  // A chat opened by link that is not in the list yet (an old resolved one).
  const inList = openId ? wa.byId(openId) : undefined;
  useEffect(() => {
    if (!openId || inList) {
      setFetched(null);
      return;
    }
    // The page stays while the agent moves between chats: an answer for a
    // chat no longer open is dropped.
    let live = true;
    waApi.conversation(openId).then((c) => live && setFetched(c)).catch(() => live && setFetched(null));
    return () => {
      live = false;
    };
  }, [openId, inList]);
  const conv = inList ?? (fetched?.id === openId ? fetched : undefined);
  const channel = useMemo(() => channels.find((c) => c.id === conv?.channelId), [channels, conv?.channelId]);

  const chooseBucket = (b: Bucket) => {
    setBucket(b);
    try {
      localStorage.setItem(BUCKET_KEY, b);
    } catch {
      // storage unavailable
    }
  };
  const togglePanel = () => setPanel((v) => !v);

  // The same list the settings page is guarded with, so the link and the
  // page never disagree.
  const canSettings = canAny(user, PAGE_PERMISSIONS.whatsappSettings);

  if (wa.loaded && channels.length === 0 && wa.conversations.length === 0) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 text-center">
        <span className="flex size-16 items-center justify-center rounded-full bg-wa-accent/15 text-wa-accent"><Smartphone className="size-7" /></span>
        <div className="space-y-1">
          <p className="text-base font-semibold">Henüz bağlı bir WhatsApp numarası yok</p>
          <p className="mx-auto max-w-md text-sm text-muted-foreground">{can(user, "whatsapp.channel_manage") ? "Ayarlardan bir numara ekle; Meta'dan gelen bilgileri girdikten sonra mesajlar burada görünür." : "Yöneticin bir numara bağladığında ve seni o numaraya eklediğinde sohbetler burada görünür."}</p>
        </div>
        {canSettings && <Link to="/whatsapp/settings" className="rounded-full bg-wa-accent px-4 py-2 text-sm font-semibold text-wa-on-accent shadow-sm">Ayarlara git</Link>}
      </div>
    );
  }

  const header = (
    <div className="flex h-16 shrink-0 items-center gap-1 px-4">
      <h1 className="flex-1 text-xl font-bold tracking-tight">Sohbetler</h1>
      {wa.mutedAll && <Link to="/whatsapp/preferences" aria-label="WhatsApp bildirimleri sessizde. WhatsApp ayarlarım'dan açabilirsin." data-tip="WhatsApp bildirimleri sessizde. WhatsApp ayarlarım'dan açabilirsin." className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-accent"><BellOff className="size-[1.15rem]" /></Link>}
      {can(user, "whatsapp.template_send") && <HeadBtn tip="Yeni sohbet başlat" onClick={() => wa.startChat()}><MessageCirclePlus className="size-5" /></HeadBtn>}
      <HeadMenu>
        {can(user, "whatsapp.callbacks") && <MenuLink to="/whatsapp/callbacks" icon={PhoneCall} label="Geri arama talepleri" />}
        {can(user, "whatsapp.reports") && <MenuLink to="/whatsapp/reports" icon={BarChart3} label="Raporlar" />}
        {can(user, "whatsapp.ratings") && <MenuLink to="/whatsapp/ratings" icon={Star} label="Puanlamalar" />}
        {canSettings && <MenuLink to="/whatsapp/settings" icon={Settings2} label="WhatsApp ayarları" />}
        <MenuLink to="/whatsapp/preferences" icon={SlidersHorizontal} label="WhatsApp ayarlarım" />
      </HeadMenu>
    </div>
  );

  return (
    <div ref={frame} className={cn("relative -mx-4 -my-6 flex h-[calc(100svh-4rem)] overflow-hidden bg-card md:-mx-6 lg:-mx-8", dragging && "cursor-col-resize select-none")}>
      <div style={{ "--list-w": `${listW}px` } as React.CSSProperties} className={cn("relative flex min-h-0 shrink-0 max-md:w-full md:w-[var(--list-w)]", openId && "max-md:hidden")}>
        <ConversationList channels={channels} activeId={openId} onOpen={(cid) => navigate(`/whatsapp/${cid}`)} bucket={bucket} onBucket={chooseBucket} header={header} />
        <div onPointerDown={startResize} onDoubleClick={resetWidth} data-tip="Sürükleyerek genişlet, çift tıklayınca eski haline döner" className="group absolute inset-y-0 -right-1.5 z-20 hidden w-3 cursor-col-resize md:block">
          <span className={cn("mx-auto block h-full w-0.5 rounded-full transition-colors", dragging ? "bg-wa-accent" : "bg-transparent group-hover:bg-wa-accent/60")} />
        </div>
      </div>
      {conv ? (
        <>
          <ChatPane key={conv.id} conv={conv} channel={channel} panel={panel} onPanel={togglePanel} onBack={() => navigate("/whatsapp")} />
          {panel && (
            <div className={cn("flex", !wide && "absolute inset-y-0 right-0 z-30 w-[min(22rem,100%)] shadow-2xl")}>
              <TicketPanel key={conv.id} conv={conv} canEditContact={can(user, "whatsapp.contact_manage")} canEditTicket={can(user, "whatsapp.reply")} onOpen={(cid) => navigate(`/whatsapp/${cid}`)} onClose={togglePanel} />
            </div>
          )}
        </>
      ) : (
        <section className={cn("flex min-w-0 flex-1 flex-col items-center justify-center gap-4 border-b-4 border-wa-accent bg-muted/30 px-6 text-center", !openId && "max-md:hidden")}>
          <span className="flex size-24 items-center justify-center rounded-full bg-wa-accent/12 text-wa-accent"><WhatsAppIcon className="size-11" /></span>
          <div className="space-y-1.5">
            <p className="text-2xl font-light tracking-tight">{openId ? "Bu sohbet açılamadı" : "WhatsApp gelen kutusu"}</p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              {openId ? "Bu sohbeti göremiyorsun ya da artık yok." : wa.counts.waiting > 0 ? `${wa.counts.waiting} müşteri uzun süredir cevap bekliyor. "Bekleyen" filtresine göz at.` : "Soldan bir sohbet seç. Yeni mesajlar geldikçe liste kendiliğinden güncellenir."}
            </p>
          </div>
          {openId && <Link to="/whatsapp" className="text-sm font-semibold text-wa-accent md:hidden">Sohbet listesine dön</Link>}
        </section>
      )}
    </div>
  );
}

function HeadBtn({ tip, onClick, children }: { tip: string; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className="flex size-9 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">{children}</button>;
}

function HeadMenu({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    window.addEventListener("mousedown", close);
    return () => window.removeEventListener("mousedown", close);
  }, [open]);
  return (
    <span ref={box} className="relative">
      <HeadBtn tip="Menü" onClick={() => setOpen((v) => !v)}><EllipsisVertical className="size-5" /></HeadBtn>
      {open && (
        <div onClick={() => setOpen(false)} className="animate-in fade-in zoom-in-95 absolute right-0 top-full z-30 mt-1 w-56 origin-top-right rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-xl duration-100">
          {children}
        </div>
      )}
    </span>
  );
}

function MenuLink({ to, icon: Icon, label }: { to: string; icon: typeof PhoneCall; label: string }) {
  return (
    <Link to={to} className="flex items-center gap-3 rounded-xl px-3 py-2 text-sm transition-colors hover:bg-accent">
      <Icon className="size-4 text-muted-foreground" /> {label}
    </Link>
  );
}
