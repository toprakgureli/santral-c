// WhatsAppSettings: every WhatsApp setting in one place, one tab per
// subject. A tab shows only when the person may use it.

import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ArrowLeft, Bot, FileText, PhoneCall, Plug, ShieldAlert, SlidersHorizontal, Smartphone, Sparkles, UsersRound, Wand2, Zap, type LucideIcon } from "lucide-react";
import { useAuth } from "@/auth/AuthContext";
import AITab from "@/components/whatsapp/settings/AITab";
import BotsTab from "@/components/whatsapp/settings/BotsTab";
import CallSurveyTab from "@/components/whatsapp/settings/CallSurveyTab";
import ChannelsTab from "@/components/whatsapp/settings/ChannelsTab";
import DeviceSettingsTab from "@/components/whatsapp/settings/DeviceSettingsTab";
import EventsTab from "@/components/whatsapp/settings/EventsTab";
import IntegrationsTab from "@/components/whatsapp/settings/IntegrationsTab";
import QuickRepliesTab from "@/components/whatsapp/settings/QuickRepliesTab";
import RulesTab from "@/components/whatsapp/settings/RulesTab";
import TeamsTab from "@/components/whatsapp/settings/TeamsTab";
import TemplatesTab from "@/components/whatsapp/settings/TemplatesTab";
import { can, canAny } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { waApi } from "@/whatsapp/api";
import type { WAChannel } from "@/whatsapp/types";

interface Tab {
  key: string;
  label: string;
  icon: LucideIcon;
  allowed: boolean;
}

export function WhatsAppSettings() {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const [channels, setChannels] = useState<WAChannel[]>([]);
  const [loaded, setLoaded] = useState(false);

  const reload = useCallback(() => {
    waApi.channels().then(setChannels).catch(() => setChannels([])).finally(() => setLoaded(true));
  }, []);
  useEffect(reload, [reload]);

  const tabs: Tab[] = useMemo(() => [
    { key: "devices", label: "Cihazlar", icon: Smartphone, allowed: canAny(user, ["whatsapp.channel_manage", "whatsapp.team_manage"]) },
    { key: "device-settings", label: "Cihaz ayarları", icon: SlidersHorizontal, allowed: canAny(user, ["whatsapp.setting_general", "whatsapp.setting_greeting", "whatsapp.setting_distribution", "whatsapp.setting_read_receipts"]) },
    { key: "teams", label: "Ekipler", icon: UsersRound, allowed: can(user, "whatsapp.team_manage") },
    { key: "templates", label: "Şablonlar", icon: FileText, allowed: canAny(user, ["whatsapp.template_manage", "whatsapp.template_send"]) },
    { key: "quick", label: "Hazır yanıtlar", icon: Zap, allowed: can(user, "whatsapp.quick_reply_manage") },
    { key: "rules", label: "Otomatik mesajlar", icon: Wand2, allowed: can(user, "whatsapp.automation_manage") },
    { key: "bots", label: "Chatbot'lar", icon: Bot, allowed: canAny(user, ["whatsapp.bot_manage", "whatsapp.bot_publish"]) },
    { key: "integrations", label: "Dış sistemler", icon: Plug, allowed: can(user, "whatsapp.bot_manage") },
    { key: "ai", label: "Yapay zekâ", icon: Sparkles, allowed: can(user, "whatsapp.ai_manage") },
    { key: "call-survey", label: "Çağrı sonrası anket", icon: PhoneCall, allowed: can(user, "whatsapp.call_survey_manage") },
    { key: "events", label: "İşlenemeyenler", icon: ShieldAlert, allowed: can(user, "whatsapp.channel_manage") },
  ].filter((t) => t.allowed), [user]);

  const active = tabs.find((t) => t.key === params.get("tab"))?.key ?? tabs[0]?.key;
  const choose = (k: string) => setParams((p) => { p.set("tab", k); return p; }, { replace: true });

  if (tabs.length === 0) {
    return <p className="rounded-2xl bg-card p-8 text-center text-sm text-muted-foreground ring-1 ring-border/60">WhatsApp ayarlarını görme yetkiniz yok.</p>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Link to="/whatsapp" data-tip="Gelen kutusuna dön" className="flex size-9 items-center justify-center rounded-xl bg-card text-muted-foreground shadow-sm ring-1 ring-border/60 hover:text-foreground"><ArrowLeft className="size-4" /></Link>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">WhatsApp ayarları</h1>
          <p className="text-xs text-muted-foreground">{channels.length ? `${channels.length} numara bağlı` : "Henüz numara bağlı değil"}</p>
        </div>
      </div>
      <nav className="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1 lg:flex-wrap lg:overflow-visible">
        {tabs.map((t) => (
          <button key={t.key} type="button" onClick={() => choose(t.key)} className={cn("flex shrink-0 items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition-colors", active === t.key ? "bg-card text-foreground shadow-sm ring-1 ring-border/60" : "text-muted-foreground hover:bg-accent/60 hover:text-foreground")}>
            <t.icon className={cn("size-4", active === t.key && "text-primary")} />
            {t.label}
          </button>
        ))}
      </nav>
      {!loaded ? (
        <div className="h-64 animate-pulse rounded-2xl bg-muted/40" />
      ) : (
        <>
          {active === "devices" && <ChannelsTab channels={channels} reload={reload} />}
          {active === "device-settings" && <DeviceSettingsTab channels={channels} reload={reload} />}
          {active === "teams" && <TeamsTab />}
          {active === "templates" && <TemplatesTab channels={channels} />}
          {active === "quick" && <QuickRepliesTab channels={channels} />}
          {active === "rules" && <RulesTab channels={channels} />}
          {active === "bots" && <BotsTab channels={channels} />}
          {active === "integrations" && <IntegrationsTab />}
          {active === "ai" && <AITab />}
          {active === "call-survey" && <CallSurveyTab channels={channels} />}
          {active === "events" && <EventsTab channels={channels} />}
        </>
      )}
    </div>
  );
}
