import { ChartColumn, ClipboardList, FileClock, Headset, History, KeyRound, MessageCircleMore, PhoneMissed, SlidersHorizontal, Tags, UsersRound, type LucideIcon } from "lucide-react";
import WhatsAppIcon from "@/components/icons/WhatsAppIcon";

export type MenuItem = {
  label: string;
  path: string;
  icon: LucideIcon | typeof WhatsAppIcon;
  permission?: string | string[];
};

export type MenuGroup = {
  title: string;
  items: MenuItem[];
};

export const MENU: MenuGroup[] = [
  {
    title: "Genel",
    items: [
      { label: "Çağrı Yöneticisi", path: "/", icon: Headset },
      {
        label: "Çağrılar",
        path: "/calls",
        icon: History,
        permission: ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"],
      },
      { label: "Geri Dönüşler", path: "/followups", icon: PhoneMissed, permission: ["call.originate", "call.view_own", "cdr.view_own", "call.view_all", "cdr.view_all"] },
      { label: "Teams", path: "/teams", icon: MessageCircleMore, permission: "teams.view" },
      { label: "WhatsApp", path: "/whatsapp", icon: WhatsAppIcon, permission: "whatsapp.view" },
      { label: "Ekip Performansı", path: "/performance", icon: ChartColumn, permission: ["performance.view_role", "performance.view_all"] },
      { label: "Eskalasyonlar", path: "/escalation-search", icon: ClipboardList, permission: ["escalation.list_own", "escalation.list_all", "escalation.search"] },
    ],
  },
  {
    title: "Yönetim",
    items: [
      { label: "Eskalasyon Durumları", path: "/escalations", icon: Tags, permission: "escalation.manage" },
      { label: "Kullanıcılar", path: "/users", icon: UsersRound, permission: "user.view" },
      { label: "Roller", path: "/roles", icon: KeyRound, permission: "role.view" },
      { label: "Sistem Ayarları", path: "/settings", icon: SlidersHorizontal, permission: ["system.settings", "system.logs", "agent.break_limit", "games.manage", "system.backup"] },
      { label: "Denetim Kayıtları", path: "/audit", icon: FileClock, permission: "system.audit_view" },
    ],
  },
];

// WA_BASE is what every WhatsApp page needs before its own permission; the
// server asks the same.
export const WA_BASE = "whatsapp.view";

// Permissions of pages that are not in the side menu. Opening any of them
// needs one of the listed permissions, the same ones the links to them ask.
export const PAGE_PERMISSIONS = {
  whatsappSettings: [
    "whatsapp.channel_manage", "whatsapp.team_manage", "whatsapp.setting_general", "whatsapp.setting_greeting",
    "whatsapp.setting_distribution", "whatsapp.setting_read_receipts", "whatsapp.template_manage", "whatsapp.template_send",
    "whatsapp.quick_reply_manage", "whatsapp.automation_manage", "whatsapp.bot_manage", "whatsapp.bot_publish",
    "whatsapp.ai_manage", "whatsapp.call_survey_manage",
  ],
  whatsappBot: ["whatsapp.bot_manage", "whatsapp.bot_publish"],
  whatsappReports: "whatsapp.reports",
  whatsappRatings: "whatsapp.ratings",
  whatsappCallbacks: "whatsapp.callbacks",
  gamesAdmin: "games.manage",
} satisfies Record<string, string | string[]>;

// menuPermission is what a side-menu page needs, looked up by its path.
export function menuPermission(path: string): string | string[] | undefined {
  for (const group of MENU) {
    for (const item of group.items) {
      if (item.path === path) return item.permission;
    }
  }
  return undefined;
}

export function allows(can: (permission: string) => boolean, permission?: string | string[]) {
  if (!permission) return true;
  return Array.isArray(permission) ? permission.some(can) : can(permission);
}

export function visibleMenu(can: (permission: string) => boolean): MenuGroup[] {
  return MENU.map((group) => ({
    ...group,
    items: group.items.filter((item) => allows(can, item.permission)),
  })).filter((group) => group.items.length > 0);
}

export function titleFor(pathname: string): string {
  if (pathname === "/profile") return "Profilim";
  if (pathname === "/account") return "Hesap ve Güvenlik";
  if (pathname === "/whatsapp/preferences") return "WhatsApp Ayarlarım";
  if (pathname.startsWith("/teams")) return "Teams";
  if (pathname.startsWith("/whatsapp/settings")) return "WhatsApp Ayarları";
  if (pathname.startsWith("/whatsapp/bots")) return "Chatbot";
  if (pathname.startsWith("/whatsapp/reports")) return "WhatsApp Raporları";
  if (pathname.startsWith("/whatsapp/ratings")) return "Puanlamalar";
  if (pathname.startsWith("/whatsapp/callbacks")) return "Geri Arama Talepleri";
  if (pathname.startsWith("/whatsapp")) return "WhatsApp";
  if (pathname.startsWith("/games")) return "Mini Oyunlar";
  if (pathname.startsWith("/profile/")) return "Profil";
  for (const group of MENU) {
    for (const item of group.items) {
      if (item.path === pathname) return item.label;
    }
  }
  return "Çağrı Yöneticisi";
}
