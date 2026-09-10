import { Headset, PhoneCall, Contact, TriangleAlert, Users, type LucideIcon } from "lucide-react";

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
        icon: PhoneCall,
        permission: ["cdr.view_all", "cdr.view_own", "call.view_all", "call.view_own"],
      },
      { label: "Kişiler", path: "/contacts", icon: Contact, permission: "contact.view" },
    ],
  },
  {
    title: "Yönetim",
    items: [
      { label: "Eskalasyon", path: "/escalations", icon: TriangleAlert, permission: "escalation.manage" },
      { label: "Kullanıcılar", path: "/users", icon: Users, permission: "user.view" },
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
  for (const group of MENU) {
    for (const item of group.items) {
      if (item.path === pathname) return item.label;
    }
  }
  return "Çağrı Yöneticisi";
}
