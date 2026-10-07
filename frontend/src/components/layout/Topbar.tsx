import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { KeyRound, LogOut, Menu, UserRound } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuth } from "@/auth/AuthContext";
import NumberSearch from "./NumberSearch";
import NoticeBell from "./NoticeBell";
import ShiftButton from "./ShiftButton";
import ThemeMenu from "./ThemeMenu";
import UserAvatar from "@/components/ui/UserAvatar";
import { ConfirmDialog } from "@/components/ui";
import { useSoftphoneContext } from "@/softphone/SoftphoneContext";

type TopbarProps = {
  title: string;
  onMenuClick: () => void;
};

export default function Topbar({ title, onMenuClick }: TopbarProps) {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [leaveError, setLeaveError] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const phone = useSoftphoneContext();

  // Signing out cuts a call live in any of the panel's tabs. The call is
  // hung up properly and its end reaches the server before the session goes,
  // so the call log never loses it.
  const live = phone.liveHere || phone.liveElsewhere;
  const leave = async () => {
    setLeaving(true);
    setLeaveError(null);
    try {
      await phone.endCalls();
    } catch {
      // the stale sweeper closes a call whose end could not be sent
    }
    try {
      await logout();
    } catch (e) {
      // The session is still open on the server; say so instead of showing
      // the sign-in page over it.
      setLeaveError(e instanceof Error ? e.message : "Çıkış yapılamadı. Biraz sonra tekrar dene.");
      setConfirmLeave(true);
      setLeaving(false);
      return;
    }
    setConfirmLeave(false);
    setLeaving(false);
    navigate("/login");
  };

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
        className="flex size-10 items-center justify-center rounded-xl text-muted-foreground hover:bg-accent lg:hidden" data-tip="Menüyü aç">
        <Menu className="size-4" />
      </button>

      <h1 className="truncate text-[0.9375rem] font-semibold tracking-tight">{title}</h1>

      <div className="ml-auto flex items-center gap-1.5">
        <NumberSearch />
        <span className="mx-1 hidden h-6 w-px bg-border md:block" />
        <ShiftButton />
        <span className="mx-1 hidden h-6 w-px bg-border sm:block" />
        <NoticeBell />
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
                onClick={() => {
                  setOpen(false);
                  navigate("/account");
                }}
                className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm hover:bg-accent"
              >
                <KeyRound className="size-4" />
                Hesap ve güvenlik
              </button>
              <button
                type="button"
                onClick={() => {
                  setOpen(false);
                  if (live) setConfirmLeave(true);
                  else void leave();
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
      <ConfirmDialog
        open={confirmLeave}
        title={leaveError ? "Çıkış yapılamadı" : "Görüşme sürüyor"}
        description={
          leaveError ??
          (phone.liveHere
            ? "Çıkış yaparsan devam eden görüşme kapanır. Yine de çıkmak istiyor musun?"
            : "Diğer sekmede bir görüşme sürüyor. Devam edersen görüşme kapanacak.")
        }
        confirmLabel={leaveError ? "Tekrar dene" : "Çıkış yap"}
        tone="warning"
        busy={leaving}
        onConfirm={() => void leave()}
        onCancel={() => {
          setConfirmLeave(false);
          setLeaveError(null);
        }}
      />
    </header>
  );
}
