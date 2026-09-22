import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { LogOut, Menu, UserRound } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuth } from "@/auth/AuthContext";
import ShiftButton from "./ShiftButton";
import ThemeMenu from "./ThemeMenu";
import UserAvatar from "@/components/ui/UserAvatar";

type TopbarProps = {
  title: string;
  onMenuClick: () => void;
};

export default function Topbar({ title, onMenuClick }: TopbarProps) {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  return (
    <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center gap-3 border-b border-border/70 bg-background/80 px-4 backdrop-blur-md md:px-6">
      <button
        type="button"
        onClick={onMenuClick}
        aria-label="Menüyü aç"
        className="flex size-10 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent lg:hidden"
      >
        <Menu className="size-4" />
      </button>

      <h1 className="truncate text-[0.9375rem] font-semibold tracking-tight">{title}</h1>

      <div className="ml-auto flex items-center gap-1.5">
        <ShiftButton />
        <span className="mx-1 hidden h-6 w-px bg-border sm:block" />
        <ThemeMenu />

        <div className="relative" ref={menuRef}>
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            className="flex items-center gap-2.5 rounded-xl py-1 pr-2.5 pl-1 transition-colors hover:bg-accent"
          >
            <UserAvatar userId={user?.id} name={user?.name} hasAvatar={user?.hasAvatar} version={user?.avatarVersion} className="size-9" fallbackClassName="bg-primary text-xs text-primary-foreground" />
            <span className="hidden text-left leading-tight sm:block">
              <span className="block text-sm font-medium">{user?.name}</span>
              <span className="block text-xs text-muted-foreground">{user?.roles?.join(", ")}</span>
            </span>
          </button>

          {open && (
            <div className="animate-in fade-in slide-in-from-top-1 absolute right-0 mt-2 w-56 overflow-hidden rounded-xl border border-border bg-popover p-1 text-popover-foreground shadow-lg duration-150">
              <div className="px-3 py-2">
                <span className="block text-sm font-medium">{user?.name}</span>
                <span className="block truncate text-xs text-muted-foreground">{user?.email}</span>
              </div>
              <div className="my-1 h-px bg-border" />
              <button
                type="button"
                onClick={() => {
                  setOpen(false);
                  navigate("/profile");
                }}
                className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm hover:bg-accent"
              >
                <UserRound className="size-4" />
                Profilim
              </button>
              <button
                type="button"
                onClick={async () => {
                  setOpen(false);
                  await logout();
                  navigate("/login");
                }}
                className={cn(
                  "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-destructive",
                  "hover:bg-destructive/10",
                )}
              >
                <LogOut className="size-4" />
                Çıkış Yap
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
