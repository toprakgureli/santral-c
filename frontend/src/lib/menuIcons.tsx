import { forwardRef } from "react";
import {
  BarChart3,
  BookOpen,
  Bot,
  Briefcase,
  Building2,
  Calendar,
  Cloud,
  CreditCard,
  Database,
  FileText,
  FolderKanban,
  Globe,
  Headset,
  LayoutGrid,
  Link2,
  Mail,
  MessageCircle,
  Phone,
  Send,
  Shield,
  ShoppingBag,
  Sparkles,
  Terminal,
  Truck,
  Wrench,
  type LucideIcon,
  type LucideProps,
} from "lucide-react";

export type CustomMenuIconComponent = LucideIcon;

export const TelegramGlyph: LucideIcon = forwardRef<SVGSVGElement, LucideProps>(function TelegramGlyph(
  { className },
  ref,
) {
  return (
    <svg ref={ref} viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true" focusable="false">
      <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
    </svg>
  );
});

export interface CustomMenuIconChoice {
  key: string;
  label: string;
  icon: LucideIcon;
}

export const CUSTOM_MENU_ICONS: CustomMenuIconChoice[] = [
  { key: "globe", label: "Web / Genel", icon: Globe },
  { key: "telegram", label: "Telegram", icon: TelegramGlyph },
  { key: "send", label: "Mesaj / Gönder", icon: Send },
  { key: "message-circle", label: "Sohbet", icon: MessageCircle },
  { key: "briefcase", label: "CRM / İş", icon: Briefcase },
  { key: "folder-kanban", label: "Proje Yönetimi", icon: FolderKanban },
  { key: "database", label: "Veritabanı / ERP", icon: Database },
  { key: "cloud", label: "Bulut Servisi", icon: Cloud },
  { key: "mail", label: "E-Posta", icon: Mail },
  { key: "calendar", label: "Takvim", icon: Calendar },
  { key: "bar-chart-3", label: "Rapor / Analitik", icon: BarChart3 },
  { key: "shopping-bag", label: "E-Ticaret / Sipariş", icon: ShoppingBag },
  { key: "file-text", label: "Doküman / Wiki", icon: FileText },
  { key: "bot", label: "Yapay Zeka / Bot", icon: Bot },
  { key: "wrench", label: "Teknik Araç", icon: Wrench },
  { key: "link", label: "Kısayol / Bağlantı", icon: Link2 },
  { key: "sparkles", label: "Özel Uygulama", icon: Sparkles },
  { key: "building-2", label: "Kurumsal", icon: Building2 },
  { key: "headset", label: "Destek / Çağrı", icon: Headset },
  { key: "phone", label: "Telefon", icon: Phone },
  { key: "shield", label: "Güvenlik", icon: Shield },
  { key: "terminal", label: "Konsol / Terminal", icon: Terminal },
  { key: "book-open", label: "Bilgi Bankası", icon: BookOpen },
  { key: "credit-card", label: "Finans / Ödeme", icon: CreditCard },
  { key: "truck", label: "Kargo / Lojistik", icon: Truck },
  { key: "layout-grid", label: "Uygulama Paneli", icon: LayoutGrid },
];

const BY_KEY: Record<string, LucideIcon> = Object.fromEntries(
  CUSTOM_MENU_ICONS.map((entry) => [entry.key, entry.icon]),
);

export function customMenuIcon(key?: string): LucideIcon {
  if (!key) return Globe;
  return BY_KEY[key] ?? Globe;
}
