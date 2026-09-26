// WhatsApp: the inbox. Left, the conversations by list (mine, waiting for
// an answer, the pool, everything, resolved); middle, the open chat; right,
// the customer's and the conversation's card.

import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { BarChart3, MessageCirclePlus, PhoneCall, Settings2, Smartphone } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import ChatPane from "@/components/whatsapp/ChatPane";
import ConversationList from "@/components/whatsapp/ConversationList";
import NewChatDialog from "@/components/whatsapp/NewChatDialog";
import TicketPanel from "@/components/whatsapp/TicketPanel";
import { can, canAny } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel, WAConversation } from "@/whatsapp/types";
import { useWhatsApp } from "@/whatsapp/WhatsAppContext";
import { type Bucket } from "@/whatsapp/util";

const PANEL_KEY = "santral.wa-panel";
const BUCKET_KEY = "santral.wa-bucket";

export function WhatsApp() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const wa = useWhatsApp();
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [newChat, setNewChat] = useState(false);
  const [panel, setPanel] = useState(() => {
    try {
      if (window.innerWidth < 1024) return false;
      return localStorage.getItem(PANEL_KEY) !== "0";
    } catch {
      return true;
    }
  });
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

  useEffect(() => {
    wa.setOpenId(openId);
    return () => wa.setOpenId(null);
  }, [openId]); // eslint-disable-line react-hooks/exhaustive-deps

  // A chat opened by link that is not in the list yet (an old resolved one).
  const inList = openId ? wa.byId(openId) : undefined;
  useEffect(() => {
    if (!openId || inList) {
      setFetched(null);
      return;
    }
    waApi.conversation(openId).then(setFetched).catch(() => setFetched(null));
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
  const togglePanel = () => {
    setPanel((v) => {
      try {
        localStorage.setItem(PANEL_KEY, v ? "0" : "1");
      } catch {
        // storage unavailable
      }
      return !v;
    });
  };

  const canSettings = canAny(user, ["whatsapp.channel_manage", "whatsapp.template_manage", "whatsapp.quick_reply_manage", "whatsapp.automation_manage", "whatsapp.bot_manage", "whatsapp.bot_publish", "whatsapp.team_manage", "whatsapp.setting_general", "whatsapp.setting_greeting", "whatsapp.setting_distribution", "whatsapp.setting_read_receipts"]);

  if (wa.loaded && channels.length === 0 && wa.conversations.length === 0) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 text-center">
        <span className="flex size-16 items-center justify-center rounded-3xl bg-emerald-500/12 text-emerald-600 dark:text-emerald-400"><Smartphone className="size-7" /></span>
        <div className="space-y-1">
          <p className="text-base font-semibold">Henüz bağlı bir WhatsApp numarası yok</p>
          <p className="mx-auto max-w-md text-sm text-muted-foreground">{can(user, "whatsapp.channel_manage") ? "Ayarlardan bir numara ekleyin; Meta'dan gelen bilgileri girdikten sonra mesajlar burada görünür." : "Yöneticiniz bir numara bağladığında ve sizi o numaraya eklediğinde sohbetler burada görünür."}</p>
        </div>
        {canSettings && <Link to="/whatsapp/settings" className="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground shadow-md shadow-primary/30">Ayarlara git</Link>}
      </div>
    );
  }

  return (
    <div className="relative -mx-4 -my-6 flex h-[calc(100svh-4rem)] overflow-hidden md:-mx-6 lg:-mx-8">
      <div className={cn("flex min-h-0 flex-col max-md:w-full", openId && "max-md:hidden")}>
        <div className="flex items-center gap-1 border-r border-b border-border/50 bg-card/70 px-3 py-2">
          <span className="flex size-8 items-center justify-center rounded-xl bg-emerald-500/12 text-emerald-600 dark:text-emerald-400"><Smartphone className="size-4" /></span>
          <span className="ml-1 text-sm font-semibold">WhatsApp</span>
          <span className="ml-auto flex items-center gap-0.5">
            {can(user, "whatsapp.template_send") && <HeadBtn tip="Yeni sohbet başlat" onClick={() => setNewChat(true)}><MessageCirclePlus className="size-4" /></HeadBtn>}
            {can(user, "whatsapp.callbacks") && <HeadLink tip="Geri arama talepleri" to="/whatsapp/callbacks"><PhoneCall className="size-4" /></HeadLink>}
            {can(user, "whatsapp.reports") && <HeadLink tip="Raporlar" to="/whatsapp/reports"><BarChart3 className="size-4" /></HeadLink>}
            {canSettings && <HeadLink tip="WhatsApp ayarları" to="/whatsapp/settings"><Settings2 className="size-4" /></HeadLink>}
          </span>
        </div>
        <div className="flex min-h-0 flex-1">
          <ConversationList channels={channels} activeId={openId} onOpen={(cid) => navigate(`/whatsapp/${cid}`)} bucket={bucket} onBucket={chooseBucket} />
        </div>
      </div>
      {conv ? (
        <>
          <ChatPane key={conv.id} conv={conv} channel={channel} panel={panel} onPanel={togglePanel} onBack={() => navigate("/whatsapp")} />
          {panel && (
            <div className="flex max-lg:absolute max-lg:inset-y-0 max-lg:right-0 max-lg:z-30 max-lg:max-w-[calc(100%-2rem)] max-lg:shadow-2xl max-lg:[&>aside]:bg-card">
              <TicketPanel conv={conv} canEditContact={can(user, "whatsapp.contact_manage")} canEditTicket={can(user, "whatsapp.reply")} onOpen={(cid) => navigate(`/whatsapp/${cid}`)} onClose={togglePanel} />
            </div>
          )}
        </>
      ) : (
        <section className={cn("flex min-w-0 flex-1 flex-col items-center justify-center gap-3 bg-background text-center", !openId && "max-md:hidden")}>
          <span className="flex size-20 items-center justify-center rounded-[1.75rem] bg-gradient-to-br from-emerald-500/20 to-primary/15 text-emerald-600 shadow-inner dark:text-emerald-400"><Smartphone className="size-8" /></span>
          <p className="text-sm font-medium">{openId ? "Bu sohbeti göremiyorsunuz ya da artık yok" : "Soldan bir sohbet seçin"}</p>
          {openId && <Link to="/whatsapp" className="text-xs font-semibold text-primary md:hidden">Sohbet listesine dön</Link>}
          <p className="max-w-xs text-xs text-muted-foreground">
            {wa.counts.waiting > 0 ? `${wa.counts.waiting} müşteri uzun süredir cevap bekliyor. "Bekleyen" sekmesine göz atın.` : "Yeni mesajlar geldikçe liste kendiliğinden güncellenir."}
          </p>
        </section>
      )}
      <NewChatDialog open={newChat} channels={channels} onClose={() => setNewChat(false)} onStarted={(c) => { wa.upsert(c); navigate(`/whatsapp/${c.id}`); }} />
    </div>
  );
}

function HeadBtn({ tip, onClick, children }: { tip: string; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className="flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">{children}</button>;
}

function HeadLink({ tip, to, children }: { tip: string; to: string; children: React.ReactNode }) {
  return <Link to={to} data-tip={tip} aria-label={tip} className={cn("flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground")}>{children}</Link>;
}
