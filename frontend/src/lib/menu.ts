import { ChartColumn, ClipboardList, FileClock, Headset, History, KeyRound, MessageCircleMore, SlidersHorizontal, Tags, UsersRound, type LucideIcon } from "lucide-react";

export type MenuItem = {
  label: string;
  path: string;
  icon: LucideIcon;
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
      { label: "Teams", path: "/teams", icon: MessageCircleMore, permission: "teams.view" },
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
      { label: "Sistem Ayarları", path: "/settings", icon: SlidersHorizontal, permission: ["system.settings", "system.logs", "agent.break_limit"] },
      { label: "Denetim Kayıtları", path: "/audit", icon: FileClock, permission: "system.audit_view" },
    ],
  },
];

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
  if (pathname.startsWith("/teams")) return "Teams";
  if (pathname.startsWith("/games")) return "Mini Oyunlar";
  if (pathname.startsWith("/profile/")) return "Profil";
  for (const group of MENU) {
    for (const item of group.items) {
      if (item.path === pathname) return item.label;
    }
  }
  return "Çağrı Yöneticisi";
}
