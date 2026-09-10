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

export function ErrorText({ children }: { children: ReactNode }) {
  if (!children) return null;
  return <p className="text-sm text-destructive">{children}</p>;
}
