// The list language shared by the pages: an icon chip in a soft square, a
// row with a title, a small note and the value on the right, and a toolbar
// strip for filters. The same shapes the main menu and the team cards use,
// so every page reads the same way.

import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export type ChipTone = "muted" | "primary" | "success" | "warning" | "destructive" | "violet" | "solid";

const CHIP: Record<ChipTone, string> = {
  muted: "bg-muted/70 text-muted-foreground",
  primary: "bg-primary/10 text-primary",
  success: "bg-success/10 text-success",
  warning: "bg-warning/12 text-warning",
  destructive: "bg-destructive/10 text-destructive",
  violet: "bg-violet-500/12 text-violet-500",
  solid: "bg-primary text-primary-foreground shadow-sm shadow-primary/30",
};

export function IconChip({ icon: Icon, tone = "muted", size = "md", className }: { icon: LucideIcon; tone?: ChipTone; size?: "sm" | "md" | "lg"; className?: string }) {
  return (
    <span className={cn("flex shrink-0 items-center justify-center rounded-xl", size === "sm" ? "size-7 [&>svg]:size-3.5" : size === "lg" ? "size-10 rounded-2xl [&>svg]:size-5" : "size-8 [&>svg]:size-4", CHIP[tone], className)}>
      <Icon />
    </span>
  );
}

// ListRow is one entry of a list. Either an icon chip or a custom leading
// element on the left, the title and note in the middle, whatever goes on
// the right as trailing. Children render underneath, across the row.
export function ListRow({ icon, tone, leading, title, sub, trailing, onClick, active, className, children }: { icon?: LucideIcon; tone?: ChipTone; leading?: ReactNode; title: ReactNode; sub?: ReactNode; trailing?: ReactNode; onClick?: () => void; active?: boolean; className?: string; children?: ReactNode }) {
  const Tag = onClick ? "button" : "div";
  return (
    <Tag
      type={onClick ? "button" : undefined}
      onClick={onClick}
      className={cn(
        "flex w-full flex-col gap-2 rounded-2xl px-2.5 py-2 text-left transition-[background-color,box-shadow] duration-200",
        onClick && "cursor-pointer hover:bg-accent/60",
        active && "bg-card shadow-sm ring-1 ring-border/60",
        className,
      )}
    >
      <span className="flex w-full items-center gap-3">
        {leading ?? (icon ? <IconChip icon={icon} tone={tone} /> : null)}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium leading-tight">{title}</span>
          {sub && <span className="mt-0.5 block truncate text-xs text-muted-foreground">{sub}</span>}
        </span>
        {trailing && <span className="flex shrink-0 items-center gap-2">{trailing}</span>}
      </span>
      {children}
    </Tag>
  );
}

// Toolbar holds a page's filters in one soft strip.
export function Toolbar({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex flex-wrap items-center gap-2 rounded-2xl bg-muted/40 p-2", className)}>{children}</div>;
}

// Mono is a right-aligned figure in the row's trailing slot.
export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn("font-mono text-xs tabular-nums text-muted-foreground", className)}>{children}</span>;
}
