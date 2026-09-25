// Sidebar: the main menu. Each entry is an icon chip and a label; the open
// page carries a tinted pill and a short bar on its left edge, so the eye
// finds it without reading. Collapsed, only the chips remain.

import { useState } from "react";
import { NavLink } from "react-router-dom";
import { ChevronsLeft, X } from "lucide-react";
import Logo from "@/components/ui/Logo";
import { cn } from "@/lib/utils";
import { visibleMenu } from "@/lib/menu";
import { APP_NAME } from "@/lib/brand";
import { useAuth } from "@/auth/AuthContext";
import VersionInfo from "@/components/layout/VersionInfo";
import { useTeams } from "@/teams/TeamsContext";

type SidebarProps = {
  open: boolean;
  collapsed: boolean;
  onNavigate: () => void;
  onClose: () => void;
  onToggleCollapse: () => void;
};

export default function Sidebar({ open, collapsed, onNavigate, onClose, onToggleCollapse }: SidebarProps) {
  const { can } = useAuth();
  const groups = visibleMenu(can);
  const teams = useTeams();
  // The tip shown beside a chip while the menu is collapsed. It is drawn
  // outside the scrolling list so nothing clips it.
  const [tip, setTip] = useState<{ label: string; top: number } | null>(null);
  const showTip = (e: React.SyntheticEvent<HTMLElement>, label: string) => {
    if (!collapsed) return;
    const r = e.currentTarget.getBoundingClientRect();
    setTip({ label, top: r.top + r.height / 2 });
  };

  return (
    <aside
      data-collapsed={collapsed}
      className={cn(
        "fixed inset-y-0 left-0 z-50 flex flex-col border-r border-sidebar-border bg-sidebar",
        "transition-[width,transform] duration-300 ease-out lg:translate-x-0",
        collapsed ? "w-[16.5rem] lg:w-[4.75rem]" : "w-[16.5rem]",
        open ? "translate-x-0" : "-translate-x-full",
      )}
    >
      <div className={cn("flex h-16 shrink-0 items-center gap-2.5 px-4", collapsed && "lg:px-0")}>
        <Logo className={cn("size-9", collapsed && "lg:mx-auto")} />
        <span className={cn("min-w-0", collapsed && "lg:hidden")}>
          <span className="block truncate text-sm leading-tight font-semibold tracking-tight">{APP_NAME}</span>
          <span className="block truncate text-[0.6875rem] leading-tight text-muted-foreground">Çağrı Yönetimi</span>
        </span>
        <button
          type="button"
          onClick={onClose}
          aria-label="Menüyü kapat"
          className="ml-auto flex size-9 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent lg:hidden"
        >
          <X className="size-4" />
        </button>
      </div>

      <nav className="flex-1 space-y-6 overflow-x-hidden overflow-y-auto px-3 pt-2 pb-4">
        {groups.map((group) => (
          <div key={group.title} className="space-y-1">
            <p
              className={cn(
                "px-3 pb-1.5 text-[0.65rem] font-semibold tracking-[0.14em] text-muted-foreground/60 uppercase",
                collapsed && "lg:hidden",
              )}
            >
              {group.title}
            </p>
            <div className="space-y-1">
              {group.items.map((item) => (
                <NavLink
                  key={item.path}
                  to={item.path}
                  end={item.path !== "/teams"}
                  onClick={onNavigate}
                  onMouseEnter={(e) => showTip(e, item.label)}
                  onMouseLeave={() => setTip(null)}
                  onFocus={(e) => showTip(e, item.label)}
                  onBlur={() => setTip(null)}
                  className={({ isActive }) =>
                    cn(
                      "group relative flex items-center gap-3 rounded-2xl py-1.5 pr-3 pl-1.5 text-sm font-medium",
                      "transition-[color,background-color,box-shadow] duration-200 ease-out",
                      collapsed && "lg:mx-auto lg:size-11 lg:justify-center lg:gap-0 lg:p-0",
                      isActive
                        ? "bg-primary/10 text-foreground shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--primary)_18%,transparent)]"
                        : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
                    )
                  }
                >
                  {({ isActive }) => (
                    <>
                      {/* the bar on the left edge marks the open page */}
                      <span
                        aria-hidden
                        className={cn(
                          "absolute top-1/2 -left-3 h-5 w-1 -translate-y-1/2 rounded-r-full bg-primary transition-[opacity,transform] duration-200",
                          isActive ? "opacity-100" : "scale-y-0 opacity-0",
                          collapsed && "lg:-left-[calc((4.75rem-2.75rem)/2)]",
                        )}
                      />
                      <span
                        className={cn(
                          "flex size-8 shrink-0 items-center justify-center rounded-xl transition-colors duration-200",
                          isActive ? "bg-primary text-primary-foreground shadow-sm shadow-primary/30" : "bg-muted/70 text-muted-foreground group-hover:bg-card group-hover:text-foreground",
                        )}
                      >
                        <item.icon className="size-4" strokeWidth={isActive ? 2.25 : 2} />
                      </span>
                      <span className={cn("truncate", collapsed && "lg:hidden")}>{item.label}</span>
                      {item.path === "/teams" && teams.unread > 0 && (
                        <span className={cn("ml-auto rounded-full bg-primary px-1.5 py-0.5 text-[0.6rem] font-semibold tabular-nums leading-none text-primary-foreground shadow-sm", collapsed && "lg:absolute lg:-top-1 lg:-right-1 lg:ml-0")}>
                          {teams.unread > 99 ? "99+" : teams.unread}
                        </span>
                      )}
                    </>
                  )}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      <div className="hidden shrink-0 border-t border-sidebar-border p-3 lg:block">
        <button
          type="button"
          onClick={onToggleCollapse}
          onMouseEnter={(e) => showTip(e, "Menüyü genişlet")}
          onMouseLeave={() => setTip(null)}
          aria-label={collapsed ? "Menüyü genişlet" : "Menüyü daralt"}
          className={cn(
            "group flex items-center rounded-2xl text-muted-foreground outline-none",
            "transition-[color,background-color] duration-200 ease-out hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
            collapsed ? "mx-auto size-10 justify-center" : "w-full gap-2.5 px-3 py-2",
          )}
        >
          <ChevronsLeft className={cn("size-4 shrink-0 transition-transform duration-300 ease-out", collapsed && "rotate-180")} />
          <span className={cn("truncate text-xs font-medium", collapsed && "hidden")}>Menüyü daralt</span>
        </button>
        <VersionInfo collapsed={collapsed} />
      </div>

      {tip && collapsed && (
        <div
          role="tooltip"
          className="animate-in fade-in slide-in-from-left-1 pointer-events-none fixed z-[60] hidden -translate-y-1/2 items-center lg:flex duration-100"
          style={{ left: "calc(4.75rem + 6px)", top: tip.top }}
        >
          <span className="size-2 rotate-45 rounded-[2px] border-b border-l border-border bg-popover" style={{ marginRight: -5 }} />
          <span className="rounded-xl border border-border bg-popover px-3 py-1.5 text-xs font-medium text-popover-foreground shadow-lg whitespace-nowrap">{tip.label}</span>
        </div>
      )}
    </aside>
  );
}
