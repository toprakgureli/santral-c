import { useEffect, useRef } from "react";
import { Hand } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTopmost } from "@/components/ui/windowStack";

// The small buttons and the overflow menu of the conversation header.
export function IconBtn({ tip, on, onClick, children }: { tip: string; on?: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} data-tip={tip} aria-label={tip} className={cn("flex size-10 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground", on && "bg-accent text-foreground")}>
      {children}
    </button>
  );
}

export function TextBtn({ icon: Icon, label, tip, onClick, busy, tone }: { icon: typeof Hand; label: string; tip: string; onClick: () => void; busy?: boolean; tone?: "success" }) {
  return (
    <button type="button" onClick={onClick} disabled={busy} data-tip={tip} className={cn("mr-1 flex h-9 items-center gap-1.5 rounded-full px-3 text-xs font-semibold transition-colors disabled:opacity-60", tone === "success" ? "bg-wa-accent/15 text-wa-accent hover:bg-wa-accent/25" : "bg-muted/70 text-foreground/80 hover:bg-accent")}>
      <Icon className="size-4" /> <span className="max-lg:hidden">{label}</span>
    </button>
  );
}

export function MoreMenu({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  const box = useRef<HTMLDivElement>(null);
  const isTop = useTopmost(true);
  useEffect(() => {
    const close = (e: MouseEvent) => { if (!box.current?.parentElement?.contains(e.target as Node)) onClose(); };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && isTop() && onClose();
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [onClose]);
  return <div ref={box} className="animate-in fade-in zoom-in-95 absolute right-0 top-full z-30 mt-1 w-60 origin-top-right rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-xl duration-100">{children}</div>;
}

export function MenuItem({ icon: Icon, label, onClick, trailing, small }: { icon?: typeof Hand; label: string; onClick: () => void; trailing?: React.ReactNode; small?: boolean }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-3 rounded-xl px-3 text-left transition-colors hover:bg-accent", small ? "py-1.5 text-[0.8rem]" : "py-2 text-sm")}>
      {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" />}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {trailing}
    </button>
  );
}
