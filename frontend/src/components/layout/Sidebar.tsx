import { NavLink } from "react-router-dom";
import { ChevronsLeft, X } from "lucide-react";
import Logo from "@/components/ui/Logo";
import { cn } from "@/lib/utils";
import { visibleMenu } from "@/lib/menu";
import { APP_NAME } from "@/lib/brand";
import { useAuth } from "@/auth/AuthContext";

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
        <Logo className={cn("size-9 text-sm shrink-0", collapsed && "lg:mx-auto")} />
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

      <nav className="flex-1 space-y-5 overflow-x-hidden overflow-y-auto px-3 pt-2 pb-4">
        {groups.map((group) => (
          <div key={group.title} className="space-y-1">
            <p
              className={cn(
                "px-3 pb-1 text-[0.6875rem] font-semibold tracking-wider text-muted-foreground/70 uppercase",
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
                  end
                  onClick={onNavigate}
                  title={collapsed ? item.label : undefined}
                  className={({ isActive }) =>
                    cn(
                      "group flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium",
                      "transition-[color,background-color] duration-200 ease-out",
                      collapsed && "lg:mx-auto lg:size-10 lg:justify-center lg:gap-0 lg:px-0 lg:py-0",
                      isActive
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                    )
                  }
                >
                  {({ isActive }) => (
                    <>
                      <item.icon className={cn("size-4 shrink-0", isActive ? "text-foreground" : "text-muted-foreground/70")} />
                      <span className={cn("truncate", collapsed && "lg:hidden")}>{item.label}</span>
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
          aria-label={collapsed ? "Menüyü genişlet" : "Menüyü daralt"}
          className={cn(
            "group flex items-center rounded-xl text-muted-foreground outline-none",
            "transition-[color,background-color] duration-200 ease-out hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
            collapsed ? "mx-auto size-10 justify-center" : "w-full gap-2.5 px-3 py-2",
          )}
        >
          <ChevronsLeft className={cn("size-4 shrink-0 transition-transform duration-300 ease-out", collapsed && "rotate-180")} />
          <span className={cn("truncate text-xs font-medium", collapsed && "hidden")}>Menüyü daralt</span>
        </button>
      </div>
    </aside>
  );
}
