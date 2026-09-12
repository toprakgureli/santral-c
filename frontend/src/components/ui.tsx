import { forwardRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

const buttonBase =
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-xl text-sm font-medium whitespace-nowrap h-10 px-4 " +
  "transition-[color,background-color,border-color,box-shadow,transform] duration-200 ease-out " +
  "outline-none focus-visible:ring-4 focus-visible:ring-ring/25 active:scale-[0.985] " +
  "disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0";

const buttonVariants: Record<string, string> = {
  primary: "bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:shadow-md",
  secondary: "border border-border/70 bg-card text-foreground shadow-sm hover:border-border hover:bg-accent",
  danger: "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90 focus-visible:ring-destructive/25",
  ghost: "text-muted-foreground hover:bg-accent/70 hover:text-foreground",
};

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "secondary" | "danger" | "ghost" }) {
  return <button className={cn(buttonBase, buttonVariants[variant], className)} {...props} />;
}

const fieldBase =
  "flex h-10 w-full min-w-0 rounded-xl border border-border/70 bg-muted/40 px-3.5 text-sm text-foreground shadow-sm outline-none " +
  "placeholder:text-muted-foreground/70 transition-[color,background-color,border-color,box-shadow] duration-200 ease-out " +
  "hover:border-border hover:bg-muted/60 focus-visible:border-ring/60 focus-visible:bg-card focus-visible:ring-4 focus-visible:ring-ring/20 " +
  "disabled:pointer-events-none disabled:opacity-50";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className = "", ...props },
  ref,
) {
  return <input ref={ref} className={cn(fieldBase, className)} {...props} />;
});

export function Select({ className = "", children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(fieldBase, "bg-card", className)} {...props}>
      {children}
    </select>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

export function Card({ title, actions, children }: { title?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col rounded-2xl bg-card text-card-foreground shadow-sm ring-1 ring-border/60">
      {(title || actions) && (
        <header className="flex items-center justify-between gap-4 border-b border-border/60 px-5 py-4">
          {title && <h2 className="text-sm font-semibold tracking-tight">{title}</h2>}
          {actions}
        </header>
      )}
      <div className="p-5">{children}</div>
    </section>
  );
}

const badgeTones: Record<string, string> = {
  slate: "bg-muted text-muted-foreground",
  green: "bg-success/15 text-success",
  red: "bg-destructive/15 text-destructive",
  amber: "bg-warning/15 text-warning",
  blue: "bg-accent text-accent-foreground",
};

export function Badge({ tone = "slate", children }: { tone?: "slate" | "green" | "red" | "amber" | "blue"; children: ReactNode }) {
  return <span className={cn("inline-block rounded-full px-2 py-0.5 text-xs font-medium", badgeTones[tone])}>{children}</span>;
}

export function Modal({
  open,
  onClose,
  title,
  description,
  size = "md",
  footer,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  size?: "md" | "lg";
  footer?: ReactNode;
  children: ReactNode;
}) {
  if (!open) return null;
  const width = size === "lg" ? "max-w-3xl" : "max-w-md";
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div
        className={cn("flex max-h-[88vh] w-full flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-xl", width)}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="border-b border-border/60 px-5 py-4">
          <h2 className="text-base font-semibold">{title}</h2>
          {description && <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>}
        </header>
        <div className="flex-1 overflow-y-auto p-5">{children}</div>
        {footer && <footer className="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-3">{footer}</footer>}
      </div>
    </div>
  );
}

export function Spinner() {
  return <div className="size-5 animate-spin rounded-full border-2 border-muted border-t-primary" />;
}

// Skeleton is a pulsing placeholder block shown while data loads.
export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded-md bg-muted/70 ${className}`} />;
}

// TableSkeleton fills a table body with placeholder rows while the first load is
// in flight, so the layout does not jump when data arrives.
export function TableSkeleton({ rows = 6, cols }: { rows?: number; cols: number }) {
  return (
    <>
      {Array.from({ length: rows }).map((_, r) => (
        <tr key={r} className="border-t border-border/60">
          {Array.from({ length: cols }).map((_, c) => (
            <td key={c} className="py-2.5">
              <Skeleton className={`h-4 ${c === 0 ? "w-16" : "w-24"}`} />
            </td>
          ))}
        </tr>
      ))}
    </>
  );
}

export function ErrorText({ children }: { children: ReactNode }) {
  if (!children) return null;
  return <p className="text-sm text-destructive">{children}</p>;
}

// FieldGroup labels a control that is not a single input (pills, buttons), so
// the caption is not a <label> that would steal focus on click.
export function FieldGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <span className="block text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}

// FieldHint is the small explanatory line under a field.
export function FieldHint({ children }: { children: ReactNode }) {
  return <p className="text-xs leading-relaxed text-muted-foreground">{children}</p>;
}

// CharCount shows used / max characters and turns amber near the limit and
// red at it. It counts code points so Turkish letters are not double counted.
export function CharCount({ value, max, className }: { value: string; max: number; className?: string }) {
  const used = [...value].length;
  const left = max - used;
  return (
    <span
      aria-live="polite"
      className={cn(
        "block text-[0.6875rem] tabular-nums transition-colors duration-200",
        left <= 0 ? "font-medium text-destructive" : left <= max * 0.1 ? "text-warning" : "text-muted-foreground/60",
        className,
      )}
    >
      {used} / {max}
    </span>
  );
}

export function Separator({ className }: { className?: string }) {
  return <hr className={cn("border-0 border-t border-border/60", className)} />;
}

// Notice is a quiet inline info box (system role note, copy hint).
export function Notice({ icon, children }: { icon?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-xl border border-border/60 bg-muted/50 px-3 py-2.5 text-xs text-muted-foreground">
      {icon && <span className="mt-0.5 shrink-0 [&_svg]:size-4">{icon}</span>}
      <span>{children}</span>
    </div>
  );
}

// EmptyState fills a list that has nothing to show.
export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 px-6 py-12 text-center">
      {icon && <span className="flex size-11 items-center justify-center rounded-2xl bg-muted text-muted-foreground [&_svg]:size-5">{icon}</span>}
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {description && <p className="mx-auto max-w-sm text-xs leading-relaxed text-muted-foreground">{description}</p>}
      </div>
      {action}
    </div>
  );
}

// Pagination is the list footer; it renders nothing when everything fits on
// one page.
export function Pagination({ page, perPage, total, onChange }: { page: number; perPage: number; total: number; onChange: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / perPage));
  if (total <= perPage) return null;
  const from = (page - 1) * perPage + 1;
  const to = Math.min(page * perPage, total);
  return (
    <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-border/60 pt-3">
      <span className="text-xs tabular-nums text-muted-foreground">{from}-{to} / {total} kayıt</span>
      <div className="flex items-center gap-1">
        <Button variant="secondary" className="h-8 px-3 text-xs" disabled={page <= 1} onClick={() => onChange(page - 1)}>Önceki</Button>
        <span className="px-2 text-xs tabular-nums text-muted-foreground">{page} / {pages}</span>
        <Button variant="secondary" className="h-8 px-3 text-xs" disabled={page >= pages} onClick={() => onChange(page + 1)}>Sonraki</Button>
      </div>
    </div>
  );
}
